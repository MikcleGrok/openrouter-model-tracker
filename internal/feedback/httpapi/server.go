package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

// Default tuning values for Config's optional fields (plan 10.1/10.3). None
// of these are exposed as cmd/feedback-server flags (section 7.2 lists only
// --listen/--db/--token-file/--consumer-token-file/--backup-dir/--log-level)
// — they are contract-level MVP constants, the same reasoning
// internal/feedback.limits.go already applies to MaxRequestBodyBytes.
const (
	// DefaultRateLimitBurst is the write token bucket's burst capacity.
	DefaultRateLimitBurst = 20
	// DefaultRateLimitInterval is how often one token is added back; a
	// bucket of DefaultRateLimitBurst refilling every DefaultRateLimitInterval
	// sustains DefaultRateLimitBurst requests immediately followed by
	// roughly one every 3s indefinitely — generous for one interactive
	// local user, tight enough to stop a runaway client loop.
	DefaultRateLimitInterval = 3 * time.Second
	// DefaultMaxInFlight bounds concurrently-processing requests.
	DefaultMaxInFlight = 64
	// DefaultRequestTimeout bounds how long any single request may run
	// before the server answers 503 and cancels its context.
	DefaultRequestTimeout = 10 * time.Second
	// DefaultDeleteRetryAttempts bounds how many times DELETE
	// /v1/me/feedback's handler will drain a blocking maintenance job and
	// retry its own delete within one HTTP request before answering 202
	// cleanup_pending (see delete.go).
	DefaultDeleteRetryAttempts = 3
)

// Config configures a Server. Only UserToken is required; every other field
// has a documented default applied by New.
type Config struct {
	// UserToken is the shared user secret (plan 6.1's token-file content,
	// already read and trimmed by the caller). Required, non-empty.
	UserToken []byte
	// ConsumerToken is the separate trusted-consumer secret (plan 6.3's
	// consumer-token-file content). Nil/empty means the consumer signal
	// endpoint can never authenticate any request (every token classifies
	// as tokenNone against an empty configured secret) — a safe "disabled"
	// state rather than a panic, though cmd/feedback-server always
	// supplies one for a normal run.
	ConsumerToken []byte
	// BackupDir is where DELETE /v1/me/feedback's post-commit cleanup
	// phase (privacy.go's RunCleanup) writes and prunes backups.
	BackupDir string
	// Logger receives structured, payload-free diagnostic logs (panics,
	// post-commit cleanup failures, DELETE internal errors). Defaults to
	// slog.Default().
	Logger *slog.Logger
	// Now is this package's own clock, used for computed_at/freshness
	// calculations and DB timestamps this layer passes explicitly (DELETE,
	// cleanup). Defaults to time.Now. Tests needing a fixed clock for
	// freshness/staleness boundary assertions (plan 11.2) set this
	// directly — internal/feedback.Service's own clock is a private
	// implementation detail this package cannot reach, so every
	// httpapi-owned computation uses this one instead.
	Now func() time.Time
	// RateLimitBurst/RateLimitInterval configure the per-token write rate
	// limiter (plan 10.3). Default DefaultRateLimitBurst/DefaultRateLimitInterval.
	RateLimitBurst    int
	RateLimitInterval time.Duration
	// MaxInFlight bounds concurrently-processing requests. Default DefaultMaxInFlight.
	MaxInFlight int
	// RequestTimeout bounds how long any single request may run. Default
	// DefaultRequestTimeout.
	RequestTimeout time.Duration
	// MaxBodyBytes bounds the PUT feedback request body. Default
	// feedback.MaxRequestBodyBytes (the contract-level limit from Task 1).
	MaxBodyBytes int64
	// DeleteRetryAttempts bounds DELETE /v1/me/feedback's own-delete retry
	// loop. Default DefaultDeleteRetryAttempts.
	DeleteRetryAttempts int
}

// Server wires an internal/feedback/sqlite.Store into an
// internal/feedback.Service and answers HTTP requests over both auth scopes
// (plan 3.1/6.1/6.3). It holds the concrete *sqlite.Store directly, not just
// the abstract feedback.Repository interface Service depends on, because
// DELETE /v1/me/feedback and the consumer signal endpoint both need
// Store-specific operations (DeleteIdentity/ActiveCleanupJob/RunCleanup,
// LastUpdatedAt) that are deliberately not part of Repository's narrow CRUD
// surface (Task 2's own design choice, repository.go).
type Server struct {
	store   *sqlite.Store
	service *feedback.Service

	userToken     []byte
	consumerToken []byte

	backupDir string
	logger    *slog.Logger
	nowFunc   func() time.Time

	putLimiter          *rateLimiter
	inFlight            chan struct{}
	requestTimeout      time.Duration
	maxBodyBytes        int64
	deleteRetryAttempts int
}

// New constructs a Server over store. It never mutates store's schema
// (migrations are cmd/feedback-server's job, run before New) and never
// starts listening — call Handler() and pass the result to an *http.Server.
func New(store *sqlite.Store, cfg Config) (*Server, error) {
	if len(cfg.UserToken) == 0 {
		return nil, fmt.Errorf("httpapi: user token must not be empty")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.RateLimitBurst <= 0 {
		cfg.RateLimitBurst = DefaultRateLimitBurst
	}
	if cfg.RateLimitInterval <= 0 {
		cfg.RateLimitInterval = DefaultRateLimitInterval
	}
	if cfg.MaxInFlight <= 0 {
		cfg.MaxInFlight = DefaultMaxInFlight
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = feedback.MaxRequestBodyBytes
	}
	if cfg.DeleteRetryAttempts <= 0 {
		cfg.DeleteRetryAttempts = DefaultDeleteRetryAttempts
	}

	s := &Server{
		store:               store,
		service:             feedback.NewService(store),
		userToken:           cfg.UserToken,
		consumerToken:       cfg.ConsumerToken,
		backupDir:           cfg.BackupDir,
		logger:              cfg.Logger,
		nowFunc:             cfg.Now,
		inFlight:            make(chan struct{}, cfg.MaxInFlight),
		requestTimeout:      cfg.RequestTimeout,
		maxBodyBytes:        cfg.MaxBodyBytes,
		deleteRetryAttempts: cfg.DeleteRetryAttempts,
	}
	s.putLimiter = newRateLimiter(cfg.RateLimitBurst, cfg.RateLimitInterval, s.nowFunc)
	return s, nil
}

func (s *Server) now() time.Time { return s.nowFunc() }

// Handler returns the complete http.Handler for this server: routing plus
// every middleware (plan 10.1/10.3's concurrency/timeout/panic-recovery
// requirements), correctly ordered.
//
// Ordering is load-bearing, not stylistic: recoverMiddleware must wrap
// routes() *directly*, and that pair together must sit *inside*
// http.TimeoutHandler — because TimeoutHandler runs the wrapped handler in
// its own new goroutine, and a recover() only catches a panic in the same
// goroutine it runs in. Putting recoverMiddleware outside TimeoutHandler
// would leave a handler panic free to crash the whole process exactly in
// the case this middleware exists to prevent. concurrencyMiddleware sits
// outside the timeout entirely: rejecting a request the server has no
// capacity for should be immediate, never itself subject to the request
// timeout clock.
func (s *Server) Handler() http.Handler {
	inner := s.recoverMiddleware(s.routes())
	withTimeout := http.TimeoutHandler(inner, s.requestTimeout, "request timeout")
	return s.concurrencyMiddleware(withTimeout)
}

// ResumePendingCleanup checks for a durable privacy_cleanup_jobs row left
// active by a previous process (crash or ordinary restart while a DELETE's
// post-commit cleanup had not yet finished) and, if found, resumes it (plan
// 4.7: "При crash/restart сервер видит durable cleanup_pending... и
// возобновляет cleanup"). cmd/feedback-server calls this once at startup,
// after migrations and before — or concurrently with — accepting
// connections; a failure here is not fatal to startup, since the DELETE
// handler's own drain-and-retry logic (delete.go) will keep trying to
// resume the same job on the next relevant request regardless.
func (s *Server) ResumePendingCleanup(ctx context.Context) error {
	_, found, err := s.store.ActiveCleanupJob(ctx)
	if err != nil {
		return fmt.Errorf("httpapi: check active cleanup job: %w", err)
	}
	if !found {
		return nil
	}
	if _, err := s.store.RunCleanup(ctx, s.backupDir, s.now()); err != nil {
		return fmt.Errorf("httpapi: resume cleanup: %w", err)
	}
	return nil
}

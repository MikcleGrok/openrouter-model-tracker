package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// DefaultRequestTimeout is applied when Config.RequestTimeout is zero or
// negative (matching internal/config.DefaultFeedbackRequestTimeout — kept as
// a separate constant here so this package has no dependency on
// internal/config; see doc.go).
const DefaultRequestTimeout = 3 * time.Second

// Config configures a Client. Endpoint, TokenFile and IdentityFile are
// required; RequestTimeout defaults to DefaultRequestTimeout when zero or
// negative. Relative TokenFile/IdentityFile paths are the caller's
// responsibility to resolve (e.g. against a config file's directory,
// cmd/openrouter's own convention) — this package treats whatever path it
// is given as final.
type Config struct {
	// Endpoint is the feedback-server's base URL, e.g.
	// "http://127.0.0.1:8787". Must be an absolute http/https URL with a
	// host.
	Endpoint string
	// TokenFile is the path to the shared user bearer token secret (plan
	// 6.1). Read fresh on every request, never cached, never logged.
	TokenFile string
	// IdentityFile is the path to this client's stable pseudonymous
	// identity (plan 6.1). Read fresh on every request, never cached.
	IdentityFile string
	// RequestTimeout bounds how long any single request may run, applied on
	// top of whatever context.Context the caller passes to a Client method
	// (whichever deadline is sooner wins). Defaults to
	// DefaultRequestTimeout.
	RequestTimeout time.Duration
	// HTTPClient overrides the *http.Client used to send requests. Defaults
	// to http.DefaultClient's zero-value equivalent (no Timeout of its
	// own — RequestTimeout, applied per-request via context, is what
	// actually bounds a request). Tests that need to intercept the
	// transport (a custom RoundTripper) set this directly.
	HTTPClient *http.Client
}

// Client is a minimal HTTP client for internal/feedback/httpapi's four
// user-scope routes: PUT a model's feedback, read the caller's own
// feedback, read a model's summary, and delete the caller's identity.
// Construct one with New. A Client is safe for concurrent use by multiple
// goroutines.
type Client struct {
	baseURL      *url.URL
	tokenFile    string
	identityFile string
	timeout      time.Duration
	httpClient   *http.Client
}

// New validates cfg and constructs a Client. It does not check that
// TokenFile/IdentityFile actually exist or are readable yet — that happens
// per-request (or can be checked ahead of time via CheckTokenFileReadable
// and Init).
func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("feedback client: endpoint must not be empty")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("feedback client: invalid endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("feedback client: endpoint must use the http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("feedback client: endpoint must include a host")
	}
	if cfg.TokenFile == "" {
		return nil, errors.New("feedback client: token file must not be empty")
	}
	if cfg.IdentityFile == "" {
		return nil, errors.New("feedback client: identity file must not be empty")
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	base := *u
	return &Client{
		baseURL:      &base,
		tokenFile:    cfg.TokenFile,
		identityFile: cfg.IdentityFile,
		timeout:      timeout,
		httpClient:   httpClient,
	}, nil
}

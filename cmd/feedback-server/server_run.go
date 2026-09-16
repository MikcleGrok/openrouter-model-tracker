package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/httpapi"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

// runConfig is the resolved set of flags the root command needs (plan 7.2).
type runConfig struct {
	listen            string
	dbPath            string
	tokenFile         string
	consumerTokenFile string
	backupDir         string
	logLevel          string
}

// shutdownTimeout bounds how long graceful shutdown waits for in-flight
// requests to finish once the process is asked to stop.
const shutdownTimeout = 10 * time.Second

// resumeCleanupTimeout bounds the best-effort startup attempt to resume any
// privacy-cleanup job left durable-but-unfinished by a previous crash/
// restart (plan 4.7). It runs in the background and never blocks startup;
// failure here is logged, not fatal — the DELETE handler's own
// drain-and-retry (httpapi/delete.go) will keep trying on the next relevant
// request regardless.
const resumeCleanupTimeout = 30 * time.Second

// serverHandles bundles everything runServer/serve need, split out from
// buildServerHandles so a test can construct real handles (a real
// listener bound to 127.0.0.1:0, a real temp-file store) and drive
// serve/shutdown directly without going through the cobra command layer.
type serverHandles struct {
	httpServer *http.Server
	listener   net.Listener
	store      *sqlite.Store
	api        *httpapi.Server
	logger     *slog.Logger
}

// buildServerHandles reads both token files, opens and migrates the
// database, constructs the httpapi.Server, and binds the listener — every
// startup step plan 3.1/7.2 requires to happen (in this order) before any
// connection is accepted. A migration error stops here, before the
// listener is ever created (plan 7.2: "Ошибка миграции должна завершить
// процесс до открытия HTTP listener").
func buildServerHandles(ctx context.Context, cfg runConfig, stdout, stderr io.Writer) (*serverHandles, error) {
	logger, err := newLogger(stderr, cfg.logLevel)
	if err != nil {
		return nil, err
	}

	userToken, err := readSecretFile(cfg.tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	consumerToken, err := readSecretFile(cfg.consumerTokenFile)
	if err != nil {
		return nil, fmt.Errorf("read consumer token file: %w", err)
	}

	if dir := filepath.Dir(cfg.dbPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	store, err := sqlite.Open(ctx, cfg.dbPath, sqlite.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	version, err := store.Migrate(ctx, stdout)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	if cfg.backupDir != "" {
		if err := os.MkdirAll(cfg.backupDir, 0o700); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("create backup directory: %w", err)
		}
	}

	api, err := httpapi.New(store, httpapi.Config{
		UserToken:     userToken,
		ConsumerToken: consumerToken,
		BackupDir:     cfg.backupDir,
		Logger:        logger,
	})
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	ln, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("listen on %s: %w", cfg.listen, err)
	}

	// Startup summary without secrets (plan 7.2: "без secret: listen
	// address, database path..., schema version"). The database path is
	// printed absolute: this is a personal-machine local MVP tool (plan
	// 6.1), not a shared multi-tenant service, so the path itself is local
	// diagnostic information, not a secret.
	absDB, absErr := filepath.Abs(cfg.dbPath)
	if absErr != nil {
		absDB = cfg.dbPath
	}
	fmt.Fprintf(stdout, "feedback-server: listening on %s, db=%s, schema version %d\n", ln.Addr().String(), absDB, version)

	return &serverHandles{
		httpServer: &http.Server{Handler: api.Handler()},
		listener:   ln,
		store:      store,
		api:        api,
		logger:     logger,
	}, nil
}

// runServer builds the server and runs it until ctx is cancelled (SIGINT/
// SIGTERM via main's signal.NotifyContext) or it fails to serve, always
// closing the SQLite store on the way out (plan 3.1: "Завершение сервера
// через context должно закрывать HTTP listener и SQLite").
func runServer(ctx context.Context, cfg runConfig, stdout, stderr io.Writer) error {
	h, err := buildServerHandles(ctx, cfg, stdout, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = h.store.Close() }()

	resumeCtx, cancelResume := context.WithTimeout(context.Background(), resumeCleanupTimeout)
	go func() {
		defer cancelResume()
		if err := h.api.ResumePendingCleanup(resumeCtx); err != nil {
			h.logger.Warn("resume pending cleanup at startup failed", "error", err.Error())
		}
	}()

	return serve(ctx, h)
}

// serve runs h.httpServer until ctx is cancelled, then shuts it down
// gracefully: stop accepting new connections, let in-flight requests
// finish (up to shutdownTimeout — plan 3.1: "незавершённые запросы
// получают корректную ошибку" means they finish normally if they can, not
// that they are abruptly severed), and close the listener. It never closes
// the store itself — that is the caller's job (runServer's defer), so a
// test can call serve directly and inspect the store afterward.
func serve(ctx context.Context, h *serverHandles) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- h.httpServer.Serve(h.listener) }()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := h.httpServer.Shutdown(shutdownCtx); err != nil {
			h.logger.Warn("graceful shutdown did not finish within the timeout", "error", err.Error())
		}
		<-serveErr // wait for Serve to actually return before we come back
		return nil
	}
}

// newLogger builds a payload-free structured logger writing to w at the
// given level (plan 7.2: "--log-level без payload logging" — nothing in
// this package ever logs a request/response body or a secret; level only
// controls verbosity of the structural fields it does log).
func newLogger(w io.Writer, level string) (*slog.Logger, error) {
	lvl, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})), nil
}

func parseLogLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("--log-level: unknown level %q (want debug, info, warn, or error)", level)
	}
}

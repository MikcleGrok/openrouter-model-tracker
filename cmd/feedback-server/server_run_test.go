package main

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	// Registers the "sqlite" database/sql driver (transitively, via
	// internal/feedback/sqlite's own blank import of modernc.org/sqlite),
	// so corruptMigrationChecksum below can open a plain database/sql
	// connection without depending on any sqlite-package internals beyond
	// the driver name it registers.
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

func writeTestToken(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("test-secret-"+name+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func baseTestConfig(t *testing.T) runConfig {
	t.Helper()
	dir := t.TempDir()
	return runConfig{
		listen:            "127.0.0.1:0",
		dbPath:            filepath.Join(dir, "feedback.db"),
		tokenFile:         writeTestToken(t, dir, "token"),
		consumerTokenFile: writeTestToken(t, dir, "consumer-token"),
		backupDir:         filepath.Join(dir, "backups"),
		logLevel:          "error",
	}
}

// TestBuildServerHandles_Success builds real handles against a temp DB and
// confirms migrations ran and a real listener is bound before returning.
func TestBuildServerHandles_Success(t *testing.T) {
	cfg := baseTestConfig(t)
	ctx := context.Background()

	h, err := buildServerHandles(ctx, cfg, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("buildServerHandles: %v", err)
	}
	defer h.listener.Close()
	defer h.store.Close()

	if h.listener.Addr().String() == "" {
		t.Error("listener has no bound address")
	}
	version, err := h.store.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version < 1 {
		t.Errorf("schema version = %d, want >= 1 (migrations must have run)", version)
	}
}

// TestBuildServerHandles_MigrationFailureNeverOpensAListener reproduces
// plan 7.2's "Ошибка миграции должна завершить процесс до открытия HTTP
// listener": a database whose recorded migration checksum no longer
// matches the embedded migration file must fail *before* any port is
// bound.
func TestBuildServerHandles_MigrationFailureNeverOpensAListener(t *testing.T) {
	cfg := baseTestConfig(t)
	ctx := context.Background()

	// Pre-create and migrate the database normally, then corrupt the
	// recorded checksum directly, simulating a schema_migrations row that
	// no longer matches this binary's embedded migration file.
	store, err := sqlite.Open(ctx, cfg.dbPath, sqlite.Config{})
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	if _, err := store.Migrate(ctx, io.Discard); err != nil {
		t.Fatalf("initial Migrate: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := corruptMigrationChecksum(cfg.dbPath); err != nil {
		t.Fatalf("corruptMigrationChecksum: %v", err)
	}

	h, err := buildServerHandles(ctx, cfg, io.Discard, io.Discard)
	if err == nil {
		h.listener.Close()
		h.store.Close()
		t.Fatal("buildServerHandles with a corrupted checksum: want an error, got nil")
	}
	if h != nil {
		t.Errorf("buildServerHandles returned non-nil handles alongside an error: %+v", h)
	}

	// Confirm the configured listen address really was never bound: a
	// fresh listener on the *original* requested address is not directly
	// checkable when the port is "0" (OS-assigned), so instead confirm no
	// leftover *sql.DB lock prevents reopening the same file cleanly —
	// which it would if buildServerHandles had left a Store open.
	store2, err := sqlite.Open(ctx, cfg.dbPath, sqlite.Config{})
	if err != nil {
		t.Fatalf("reopen after failed build: %v", err)
	}
	store2.Close()
}

// corruptMigrationChecksum rewrites schema_migrations' recorded checksum
// for version 1 to a value that cannot match the embedded migration file,
// using a plain database/sql connection (no dependency on sqlite package
// internals beyond the driver name it already registers).
func corruptMigrationChecksum(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`UPDATE schema_migrations SET checksum = 'not-the-real-checksum' WHERE version = 1`)
	return err
}

// TestServe_GracefulShutdown_ClosesListenerButNotStore drives a real
// listener + http.Server through serve() end to end: a request succeeds
// while running, cancelling the context makes serve() return promptly, the
// listener stops accepting new connections, and the store — which serve()
// deliberately does not close (runServer's job) — remains fully usable
// afterward, proving no locks were left dangling by the shutdown path.
func TestServe_GracefulShutdown_ClosesListenerButNotStore(t *testing.T) {
	cfg := baseTestConfig(t)
	ctx, cancel := context.WithCancel(context.Background())

	h, err := buildServerHandles(context.Background(), cfg, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("buildServerHandles: %v", err)
	}
	addr := h.listener.Addr().String()

	serveDone := make(chan error, 1)
	go func() { serveDone <- serve(ctx, h) }()

	// The server must actually be reachable before we test shutdown.
	waitForListening(t, addr)
	resp, err := http.Get("http://" + addr + "/v1/models/acme/x/feedback/me")
	if err != nil {
		t.Fatalf("GET while running: %v", err)
	}
	resp.Body.Close() // 401 (no auth) is fine — only reachability matters here.

	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Errorf("serve() after shutdown: %v, want nil", err)
		}
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("serve() did not return after context cancellation")
	}

	// The listener must be closed: a new connection attempt fails.
	if _, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		t.Error("listener still accepting connections after shutdown")
	}

	// The store must still be open and usable — serve() does not close it.
	if _, err := h.store.SchemaVersion(context.Background()); err != nil {
		t.Errorf("store unusable after serve() returned (should only close on runServer's own defer): %v", err)
	}
	if err := h.store.Close(); err != nil {
		t.Errorf("closing store after shutdown: %v", err)
	}
}

func waitForListening(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server never started listening on %s", addr)
}

// TestRunServer_ClosesStoreOnShutdown covers runServer's own contract (the
// one main() actually calls): once ctx is cancelled, it returns and the
// store it opened is closed — a fresh sqlite.Open against the same file
// must then succeed immediately, proving no dangling lock survives.
func TestRunServer_ClosesStoreOnShutdown(t *testing.T) {
	cfg := baseTestConfig(t)
	ctx, cancel := context.WithCancel(context.Background())

	var stdout, stderr bytes.Buffer
	runDone := make(chan error, 1)
	go func() { runDone <- runServer(ctx, cfg, &stdout, &stderr) }()

	// runServer prints its startup line synchronously before serving, but
	// we don't have the bound address until it's written — parse it isn't
	// worth it here; just wait until *something* is listening on the
	// configured host by polling the stdout buffer for the summary line
	// isn't reliable across goroutines without locking, so instead give it
	// a short, generous grace period to reach net.Listen.
	time.Sleep(200 * time.Millisecond)

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("runServer after cancel: %v, want nil", err)
		}
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("runServer did not return after context cancellation")
	}

	store, err := sqlite.Open(context.Background(), cfg.dbPath, sqlite.Config{})
	if err != nil {
		t.Fatalf("reopen db after runServer shutdown: %v", err)
	}
	store.Close()
}

func TestParseLogLevel(t *testing.T) {
	valid := []string{"debug", "info", "", "warn", "warning", "error"}
	for _, v := range valid {
		if _, err := parseLogLevel(v); err != nil {
			t.Errorf("parseLogLevel(%q): unexpected error: %v", v, err)
		}
	}
	if _, err := parseLogLevel("verbose"); err == nil {
		t.Error("parseLogLevel(\"verbose\"): want an error, got nil")
	}
}

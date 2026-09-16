package sqlite

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"
)

// pragmaSnapshot reads back the three connection-scoped PRAGMAs the brief
// requires on every connection.
type pragmaSnapshot struct {
	foreignKeys int
	journalMode string
	busyTimeout int
}

func readPragmas(t *testing.T, s *Store) pragmaSnapshot {
	t.Helper()
	ctx := context.Background()
	var snap pragmaSnapshot
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&snap.foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&snap.journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&snap.busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	return snap
}

func TestOpen_AppliesConnectionPragmas(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	store, err := Open(ctx, dbPath, Config{BusyTimeout: 0})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	snap := readPragmas(t, store)
	if snap.foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", snap.foreignKeys)
	}
	if snap.journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", snap.journalMode, "wal")
	}
	wantBusy := int(DefaultBusyTimeout.Milliseconds())
	if snap.busyTimeout != wantBusy {
		t.Errorf("busy_timeout = %d, want %d", snap.busyTimeout, wantBusy)
	}
}

func TestOpen_CustomBusyTimeout(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	const custom = 2500 * time.Millisecond
	store, err := Open(ctx, dbPath, Config{BusyTimeout: custom})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	snap := readPragmas(t, store)
	if snap.busyTimeout != int(custom.Milliseconds()) {
		t.Errorf("busy_timeout = %d, want %d", snap.busyTimeout, custom.Milliseconds())
	}
}

// TestOpen_ReapplyPragmasOnFreshConnection proves the brief's central
// concern about connection-scoped PRAGMAs: SetMaxOpenConns(1) alone does not
// guarantee a PRAGMA survives the pool opening a brand new physical
// connection later (e.g. after a full process restart against the same
// file) -- only DSN-level, per-connection application does. It closes a
// Store and opens a completely new one against the same database file, then
// re-reads the same three PRAGMAs.
func TestOpen_ReapplyPragmasOnFreshConnection(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	first, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open (first): %v", err)
	}
	firstSnap := readPragmas(t, first)
	if err := first.Close(); err != nil {
		t.Fatalf("Close (first): %v", err)
	}

	second, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open (second): %v", err)
	}
	defer second.Close()
	secondSnap := readPragmas(t, second)

	if secondSnap != firstSnap {
		t.Fatalf("pragmas not reapplied on fresh connection: first=%+v second=%+v", firstSnap, secondSnap)
	}
	if secondSnap.foreignKeys != 1 || secondSnap.journalMode != "wal" || secondSnap.busyTimeout != int(DefaultBusyTimeout.Milliseconds()) {
		t.Fatalf("unexpected pragma values on fresh connection: %+v", secondSnap)
	}
}

func TestOpen_SingleConnectionPool(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	store, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	stats := store.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
}

func TestOpen_EmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), "", Config{}); err == nil {
		t.Fatal("Open(\"\") succeeded, want error")
	}
}

func TestOpen_MigrateIsIdempotentAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	store1, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	v1, err := store1.Migrate(ctx, io.Discard)
	if err != nil {
		t.Fatalf("Migrate (first): %v", err)
	}
	if err := store1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store2, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}
	defer store2.Close()
	v2, err := store2.Migrate(ctx, io.Discard)
	if err != nil {
		t.Fatalf("Migrate (reopen/restart): %v", err)
	}
	if v1 != v2 {
		t.Fatalf("schema version changed across restart: %d != %d", v1, v2)
	}
}

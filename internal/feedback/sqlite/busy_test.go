package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// openRawConn opens a single-connection *sql.DB directly against path with
// a short busy_timeout, bypassing Store/Open entirely -- these tests need
// two independent connections to the same file to provoke a genuine
// SQLITE_BUSY, which a single Store (MaxOpenConns(1)) can never produce
// against itself.
func openRawConn(t *testing.T, path string, busyTimeoutMs int) *sql.DB {
	t.Helper()
	dsn := path + "?_foreign_keys=1&_journal_mode=WAL&_txlock=immediate"
	if busyTimeoutMs > 0 {
		dsn += "&_busy_timeout=" + strconv.Itoa(busyTimeoutMs)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRetryOnBusy_ReturnsNonBusyErrorImmediately(t *testing.T) {
	calls := 0
	err := retryOnBusy(context.Background(), 5, time.Millisecond, func() error {
		calls++
		return errors.New("not a busy error")
	})
	if err == nil || errors.Is(err, ErrBusyTimeout) {
		t.Fatalf("err = %v, want the original non-busy error unwrapped", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1 (no retry on a non-busy error)", calls)
	}
}

func TestRetryOnBusy_SucceedsWithoutRetryingWhenFnSucceeds(t *testing.T) {
	calls := 0
	err := retryOnBusy(context.Background(), 5, time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1", calls)
	}
}

// TestRetryOnBusy_BoundedAgainstRealContention provokes a genuine
// SQLITE_BUSY from two independent connections to the same file: one holds
// an open write transaction (BEGIN IMMEDIATE, never committed) while the
// other's write is wrapped in retryOnBusy with a short bound. It must
// return ErrBusyTimeout in bounded time rather than hanging until the test
// itself times out.
func TestRetryOnBusy_BoundedAgainstRealContention(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "contention.sqlite")

	setup, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open (setup): %v", err)
	}
	if _, err := setup.Migrate(ctx, nil); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("Close (setup): %v", err)
	}

	// A very short busy_timeout on both connections keeps this test fast:
	// the engine's own wait is short, so any remaining bound-testing is on
	// retryOnBusy's own attempt count and backoff, not on PRAGMA busy_timeout.
	holder := openRawConn(t, dbPath, 50)
	holderTx, err := holder.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("holder.BeginTx: %v", err)
	}
	if _, err := holderTx.ExecContext(ctx, `INSERT INTO identities (id, created_at, last_seen_at) VALUES (?, 'x', 'x')`, []byte("holder")); err != nil {
		t.Fatalf("holder insert: %v", err)
	}
	defer holderTx.Rollback()

	contender := openRawConn(t, dbPath, 50)

	start := time.Now()
	err = retryOnBusy(ctx, 4, 10*time.Millisecond, func() error {
		tx, beginErr := contender.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		defer tx.Rollback()
		_, execErr := tx.ExecContext(ctx, `INSERT INTO identities (id, created_at, last_seen_at) VALUES (?, 'y', 'y')`, []byte("contender"))
		return execErr
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrBusyTimeout) {
		t.Fatalf("err = %v, want ErrBusyTimeout (holder never released its write lock)", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("retryOnBusy took %v, want a bounded, fast failure", elapsed)
	}
}

func TestIsBusyErr_FalseForOtherErrors(t *testing.T) {
	if isBusyErr(errors.New("some other error")) {
		t.Fatal("isBusyErr(plain error) = true, want false")
	}
	if isBusyErr(nil) {
		t.Fatal("isBusyErr(nil) = true, want false")
	}
}

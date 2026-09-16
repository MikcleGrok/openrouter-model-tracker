package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	modernc "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// defaultBusyRetryAttempts and defaultBusyRetryBaseDelay bound the retry
// loop in retryOnBusy. Five attempts with a doubling 20ms base delay (20,
// 40, 80, 160, 320ms — under 1s total) is on top of, not instead of, the
// PRAGMA busy_timeout set on every connection (conn.go): busy_timeout is
// SQLite's own engine-level wait before a statement returns SQLITE_BUSY at
// all, this retry is a second, bounded layer for when a driver call still
// returns it after that wait.
const (
	defaultBusyRetryAttempts  = 5
	defaultBusyRetryBaseDelay = 20 * time.Millisecond
)

// sqliteBusyPrimaryCode is SQLITE_BUSY's primary result code. A driver may
// report an extended code (SQLITE_BUSY_RECOVERY, SQLITE_BUSY_SNAPSHOT, ...)
// whose low byte is still this value, hence the mask in isBusyErr rather
// than an exact match.
const sqliteBusyPrimaryCode = sqlite3.SQLITE_BUSY

// isBusyErr reports whether err is a SQLITE_BUSY (or SQLITE_BUSY_* extended)
// error from modernc.org/sqlite, the only case retryOnBusy retries. Every
// other error (a CHECK/FK violation, a syntax error, ctx cancellation) is
// returned immediately: retrying those would either never succeed or mask a
// real bug.
func isBusyErr(err error) bool {
	var sqliteErr *modernc.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.Code()&0xff == sqliteBusyPrimaryCode
}

// retryOnBusy runs fn, retrying with exponential backoff while it fails with
// isBusyErr, up to attempts times. It returns fn's error immediately if that
// error is not a busy error. Once attempts is exhausted, or ctx is done
// while waiting between attempts, it returns ErrBusyTimeout wrapping the
// last error — a controlled, bounded failure rather than blocking a caller
// (an HTTP handler, ultimately) indefinitely (plan 5.1).
func retryOnBusy(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		lastErr = fn()
		if lastErr == nil || !isBusyErr(lastErr) {
			return lastErr
		}
		if i == attempts-1 {
			break
		}
		delay := baseDelay * time.Duration(1<<uint(i))
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v (context: %v)", ErrBusyTimeout, lastErr, ctx.Err())
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("%w: %v", ErrBusyTimeout, lastErr)
}

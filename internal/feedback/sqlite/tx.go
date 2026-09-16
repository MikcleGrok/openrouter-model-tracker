package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// withTx runs fn inside a transaction on db, retrying only the BeginTx and
// Commit steps on a bounded SQLITE_BUSY (busy.go) — the two points where a
// lock held by some other connection or process can surface, since fn's own
// statements already run under the engine's PRAGMA busy_timeout wait and,
// within one Store, are fully serialized by SetMaxOpenConns(1). Retrying
// Commit specifically (rather than starting over) is safe: SQLite documents
// that a COMMIT which fails with SQLITE_BUSY leaves the transaction open and
// uncommitted, so calling Commit again on the same *sql.Tx is the correct
// retry, not a new attempt on a stale handle.
//
// On any error from fn, the transaction is rolled back and that error is
// returned (wrapped with the rollback error too, if that also failed) —
// never a partial commit. Because every Repository/DeleteIdentity write path
// in this package goes through withTx, "an error before COMMIT rolls back
// everything in the transaction" (plan 4.7) holds uniformly rather than
// being reimplemented per call site.
func withTx[T any](ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) (T, error)) (T, error) {
	var zero T

	var tx *sql.Tx
	beginErr := retryOnBusy(ctx, defaultBusyRetryAttempts, defaultBusyRetryBaseDelay, func() error {
		t, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		tx = t
		return nil
	})
	if beginErr != nil {
		return zero, fmt.Errorf("sqlite: begin transaction: %w", beginErr)
	}

	result, fnErr := fn(tx)
	if fnErr != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return zero, fmt.Errorf("%w (rollback also failed: %v)", fnErr, rbErr)
		}
		return zero, fnErr
	}

	commitErr := retryOnBusy(ctx, defaultBusyRetryAttempts, defaultBusyRetryBaseDelay, tx.Commit)
	if commitErr != nil {
		return zero, fmt.Errorf("sqlite: commit: %w", commitErr)
	}
	return result, nil
}

// withTxVoid is withTx for a transaction body with nothing to return.
func withTxVoid(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	_, err := withTx(ctx, db, func(tx *sql.Tx) (struct{}, error) {
		return struct{}{}, fn(tx)
	})
	return err
}

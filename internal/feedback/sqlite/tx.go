package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// withTx runs fn inside a transaction on db, retrying only the BeginTx step
// on a bounded SQLITE_BUSY (busy.go) — the point where a lock held by some
// other connection or process can surface before this transaction has done
// anything at all, so a retry there is simply a fresh, independent attempt.
// fn's own statements run under the engine's PRAGMA busy_timeout wait and,
// within one Store, are fully serialized by SetMaxOpenConns(1).
//
// Commit is deliberately never retried. database/sql marks a *sql.Tx done
// (via an atomic compare-and-swap) and releases its connection back to the
// pool as part of Tx.Commit, before the driver's own COMMIT even runs —
// regardless of whether that COMMIT succeeds. So a second call to the same
// tx.Commit() after a failure returns sql.ErrTxDone, never the real
// underlying error, which would make a "retry" both silently swallow the
// actual failure (ErrTxDone is not a busy error, so it would stop the retry
// loop immediately anyway) and, worse, leave the connection sitting in the
// pool mid-transaction if the driver-level COMMIT genuinely failed without
// rolling back on its own: with SetMaxOpenConns(1) that connection can be
// the only one available, wedging every subsequent BEGIN IMMEDIATE for the
// rest of the process. Calling Rollback on a commit failure — a no-op that
// harmlessly returns sql.ErrTxDone if the driver already closed the
// transaction, and actually releases the lock if it did not — is the safe
// response; retrying would have to re-run fn's whole body from scratch on a
// brand new transaction, not re-call Commit.
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

	if commitErr := tx.Commit(); commitErr != nil {
		// Best-effort: if the driver left the transaction open (the
		// documented SQLITE_BUSY-on-COMMIT case), release it rather than
		// leaving it held on a connection that may be the pool's only one.
		_ = tx.Rollback()
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

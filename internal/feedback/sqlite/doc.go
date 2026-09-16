// Package sqlite implements internal/feedback's Repository interface
// (repository.go there) against a real SQLite database, plus the pieces
// that interface alone does not cover: versioned embedded migrations with
// checksum tracking (migrate.go), the privacy_cleanup_jobs maintenance-lock
// mechanics behind DELETE /v1/me/feedback (privacy.go), and a backup/restore
// helper (backup.go). It has no dependency on HTTP: internal/feedback/httpapi
// (Task 4) wires this package's Store into internal/feedback.Service and
// exposes it over the network, neither of which this package knows about.
//
// Every exact schema value (tables, columns, constraints) and privacy-
// cleanup transaction rule this package's migrations and privacy.go encode
// is restated from the .task/model-feedback-plan/plan.md sections (5.1,
// 5.2, 9.1, 9.2, 10.2) quoted in .superpowers/sdd/plan/task-3-brief.md —
// not from .task/model-feedback-plan/contract.md, whose own section 9
// explicitly places SQLite schema/migrations/upsert implementation out of
// its scope and defers to plan.md for all of it. Where this package's own
// doc comments repeat a rule from there, keep both in sync rather than
// letting them drift.
//
// # Connection model
//
// Store wraps a single *sql.DB opened by Open, configured exactly per the
// brief's MVP serialization: SetMaxOpenConns(1) and SetMaxIdleConns(1), plus
// foreign_keys=ON, busy_timeout, and journal_mode=WAL applied through the
// driver's own per-connection DSN parameters (modernc.org/sqlite re-parses
// and re-applies those parameters every time it opens a new physical
// connection — see Open's doc comment) rather than a one-time PRAGMA Exec
// after sql.Open, since the latter would not survive the pool silently
// opening a replacement connection later. conn_test.go proves this by
// closing a Store and opening a fresh one against the same file.
//
// SetMaxOpenConns(1) alone serializes every operation issued through one
// Store (database/sql blocks a second caller's Begin/Exec/Query until the
// sole connection is free), which is this MVP's answer to "не блокировать
// HTTP бесконечно": callers should pass a context with a deadline, and a
// blocked caller unblocks with that context's error rather than hanging
// forever. Genuine SQLITE_BUSY — contention this single-connection
// serialization cannot see, e.g. a second OS-level connection or process
// touching the same file — is handled by a bounded retry with backoff at
// transaction begin only (busy.go); Commit is deliberately never retried
// (see withTx's own doc comment for why retrying it would be unsafe), which
// returns ErrBusyTimeout once the begin retry is exhausted instead of
// retrying indefinitely.
package sqlite

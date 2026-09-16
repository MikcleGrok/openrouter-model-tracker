package sqlite

import "errors"

// ErrChecksumMismatch is returned by Migrate when a migration already
// recorded in schema_migrations no longer matches the checksum of the
// embedded .sql file with that version — e.g. an already-applied migration
// file was edited after the fact. Per the brief (plan 9.1: "несовпадение
// останавливает сервер"), this must stop startup rather than proceed with a
// schema the running binary no longer agrees with.
var ErrChecksumMismatch = errors.New("sqlite: migration checksum mismatch")

// ErrMaintenanceLocked is returned by every feedback-write path (currently
// UpsertFeedback) and by DeleteIdentity when a privacy_cleanup_jobs row is
// already active (state cleanup_pending or failed — anything not done). Plan
// 5.2: "активная job блокирует feedback writes и privacy operations". It is
// never wrapped in a partial success: the caller's operation did not happen
// at all.
var ErrMaintenanceLocked = errors.New("sqlite: an active privacy cleanup job is in progress")

// ErrNoActiveCleanupJob is returned by RunCleanup when there is no
// privacy_cleanup_jobs row to resume (nothing in cleanup_pending or failed
// state) — calling it without a job already created by DeleteIdentity is a
// caller error, not a normal empty result.
var ErrNoActiveCleanupJob = errors.New("sqlite: no active privacy cleanup job to run")

// ErrBusyTimeout is returned once a bounded SQLITE_BUSY retry (busy.go) is
// exhausted. It always wraps the last underlying SQLite error so the cause
// is not lost, per plan 5.1: "после исчерпания попыток вернуть
// контролируемую ошибку, не блокировать HTTP бесконечно".
var ErrBusyTimeout = errors.New("sqlite: exhausted retries waiting for a locked database")

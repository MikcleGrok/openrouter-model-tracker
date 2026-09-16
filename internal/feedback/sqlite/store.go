package sqlite

import "database/sql"

// Store is this package's single connection handle: the *sql.DB Open
// configured with the brief's single-connection PRAGMA/WAL/busy-timeout
// semantics, plus every SQLite-specific operation built on top of it —
// internal/feedback.Repository (repository.go), the migration runner
// (migrate.go), the privacy_cleanup_jobs maintenance-lock mechanics
// (privacy.go), and the backup helper (backup.go).
//
// A Store's exported methods are safe to call concurrently: every read is a
// plain query and every write goes through withTx, and SetMaxOpenConns(1)
// (Open) serializes access to the single underlying connection either way.
type Store struct {
	db   *sql.DB
	path string
}

// Close closes the underlying *sql.DB. A Store must not be used after Close.
func (s *Store) Close() error {
	return s.db.Close()
}

// Path returns the filesystem path this Store was opened against.
func (s *Store) Path() string {
	return s.path
}

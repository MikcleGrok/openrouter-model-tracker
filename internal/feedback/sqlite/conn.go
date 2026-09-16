package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DefaultBusyTimeout is the PRAGMA busy_timeout applied to every connection
// when Config.BusyTimeout is zero. Plan 5.1 suggests 5 seconds as the
// example bound; kept as this package's default rather than a caller-tuned
// value since the brief treats it as an MVP constant, not an operational
// knob.
const DefaultBusyTimeout = 5 * time.Second

// Config controls how Open configures the SQLite connection. The zero value
// is valid and uses DefaultBusyTimeout.
type Config struct {
	// BusyTimeout is the PRAGMA busy_timeout (in whole milliseconds) applied
	// to every connection. Zero means DefaultBusyTimeout.
	BusyTimeout time.Duration
}

// Open opens (creating if necessary) the SQLite database at path and
// returns a Store ready for Migrate. It applies the brief's single-
// connection semantics:
//
//   - SetMaxOpenConns(1) and SetMaxIdleConns(1): at most one physical
//     connection at a time, the MVP's serialization story (plan 5.1). This
//     bounds concurrency but, on its own, does not guarantee that a
//     connection-scoped PRAGMA survives the pool silently replacing the one
//     connection — the next point covers that.
//   - foreign_keys=ON, busy_timeout, and journal_mode=WAL are passed as
//     modernc.org/sqlite DSN query parameters (_foreign_keys, _busy_timeout,
//     _journal_mode) rather than executed once after Open. modernc.org/sqlite
//     re-parses and re-applies its DSN query parameters every time its
//     Driver.Open is called to create a brand new physical connection — not
//     only on the first one — so these three settings are reapplied
//     automatically no matter when or why the pool opens a fresh connection
//     (including after this exact Store is closed and a new one opened
//     against the same file, which conn_test.go exercises directly).
//   - _txlock=immediate makes every non-read-only transaction begin with
//     `BEGIN IMMEDIATE` (https://www.sqlite.org/lang_transaction.html), the
//     locking mode the brief calls for explicitly for the DELETE-identity
//     flow (plan 4.7/10.2) and, for the same single-writer MVP reasoning,
//     applied uniformly to every write transaction this package issues via
//     withTx/withTxVoid.
//
// Open also verifies the connection with a PingContext before returning, so
// a bad path or malformed DSN parameter fails here rather than on first use.
func Open(ctx context.Context, path string, cfg Config) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite: path must not be empty")
	}
	if cfg.BusyTimeout <= 0 {
		cfg.BusyTimeout = DefaultBusyTimeout
	}

	dsn := buildDSN(path, cfg)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping %s: %w", path, err)
	}

	return &Store{db: db, path: path}, nil
}

// buildDSN renders path plus this package's fixed connection parameters as a
// modernc.org/sqlite data source name. path is passed through unescaped
// (modernc.org/sqlite splits the DSN on the first "?" and, absent a "file:"
// prefix, uses everything before it as the raw filesystem path — see
// modernc.org/sqlite's conn.go newConn — so path never needs URI escaping
// here).
func buildDSN(path string, cfg Config) string {
	q := url.Values{}
	q.Set("_foreign_keys", "1")
	q.Set("_journal_mode", "WAL")
	q.Set("_busy_timeout", strconv.FormatInt(cfg.BusyTimeout.Milliseconds(), 10))
	q.Set("_txlock", "immediate")
	return path + "?" + q.Encode()
}

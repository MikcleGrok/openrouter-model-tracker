package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationFilenamePattern matches this package's forward-only migration
// naming convention (plan 9.1: "номера 0001_init.sql, далее одна миграция на
// изменение схемы"): a monotonic, at-least-4-digit version, an underscore,
// then a name of ASCII letters/digits/underscores, and the .sql extension.
var migrationFilenamePattern = regexp.MustCompile(`^(\d{4,})_([A-Za-z0-9]+(?:_[A-Za-z0-9]+)*)\.sql$`)

// migrationFile is one embedded migration: its declared version and name
// (from the filename), its raw SQL, and the sha256 checksum Migrate records
// in schema_migrations and later re-verifies on every startup.
type migrationFile struct {
	version  int
	name     string
	sql      string
	checksum string
}

// loadMigrations reads every embedded migrations/*.sql file, computes its
// checksum, and returns them sorted by version ascending. It fails if a
// filename does not match migrationFilenamePattern or if two files declare
// the same version — both are packaging bugs in this binary, not a runtime
// condition a caller can recover from.
func loadMigrations() ([]migrationFile, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("sqlite: read embedded migrations: %w", err)
	}

	out := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationFilenamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("sqlite: embedded migration %q does not match NNNN_name.sql", entry.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("sqlite: embedded migration %q: invalid version: %w", entry.Name(), err)
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("sqlite: read embedded migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(content)
		out = append(out, migrationFile{
			version:  version,
			name:     match[2],
			sql:      string(content),
			checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("sqlite: duplicate embedded migration version %d", out[i].version)
		}
	}
	return out, nil
}

// Migrate ensures schema_migrations exists, then applies every embedded
// migration not yet recorded there, each in its own transaction (schema DDL
// plus the schema_migrations INSERT, so a failure partway through one
// migration's SQL leaves no partial record of it). For a migration already
// recorded, it re-verifies the recorded checksum against the embedded file's
// current checksum and returns ErrChecksumMismatch on a mismatch — per plan
// 9.1 ("несовпадение останавливает сервер") that error must stop startup,
// not be silently ignored.
//
// It returns the resulting schema version (the highest version applied or
// already present). If out is non-nil, it writes one line reporting that
// version and nothing else (plan 9.1: "в startup выводить текущую schema
// version без пользовательских данных") — pass nil (or io.Discard) to
// suppress it.
func (s *Store) Migrate(ctx context.Context, out io.Writer) (int, error) {
	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}

	if err := s.ensureSchemaMigrationsTable(ctx); err != nil {
		return 0, fmt.Errorf("sqlite: ensure schema_migrations: %w", err)
	}

	version := 0
	for _, m := range migrations {
		var recordedChecksum string
		err := s.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, m.version).Scan(&recordedChecksum)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if err := s.applyMigration(ctx, m); err != nil {
				return 0, err
			}
		case err != nil:
			return 0, fmt.Errorf("sqlite: read schema_migrations for version %d: %w", m.version, err)
		case recordedChecksum != m.checksum:
			return 0, fmt.Errorf("%w: version %d (%s): recorded checksum %s, embedded checksum %s",
				ErrChecksumMismatch, m.version, m.name, recordedChecksum, m.checksum)
		}
		version = m.version
	}

	if out != nil {
		fmt.Fprintf(out, "feedback: schema version %d\n", version)
	}
	return version, nil
}

func (s *Store) ensureSchemaMigrationsTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL,
			checksum TEXT NOT NULL
		)
	`)
	return err
}

func (s *Store) applyMigration(ctx context.Context, m migrationFile) error {
	appliedAt := formatTime(time.Now())
	return withTxVoid(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("sqlite: apply migration %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO schema_migrations (version, applied_at, checksum) VALUES (?, ?, ?)
		`, m.version, appliedAt, m.checksum); err != nil {
			return fmt.Errorf("sqlite: record migration %04d_%s: %w", m.version, m.name, err)
		}
		return nil
	})
}

// SchemaVersion returns the highest version currently recorded in
// schema_migrations (0 if none). It exists for the backup helper (backup.go)
// to tag a backup filename with the schema it was taken against, without
// re-running Migrate's own file-comparison logic.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var version sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("sqlite: read schema version: %w", err)
	}
	return int(version.Int64), nil
}

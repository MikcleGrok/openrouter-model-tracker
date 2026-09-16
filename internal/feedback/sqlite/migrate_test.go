package sqlite

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrate_CleanDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")
	store, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	var out bytes.Buffer
	version, err := store.Migrate(ctx, &out)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}
	if !strings.Contains(out.String(), "schema version 1") {
		t.Fatalf("startup output = %q, want it to mention schema version 1", out.String())
	}

	// Every table the contract fixes must now exist and be queryable.
	for _, table := range []string{"identities", "model_feedback", "skill_ratings", "privacy_cleanup_jobs", "schema_migrations"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Errorf("table %s not queryable: %v", table, err)
		}
	}

	var recordedChecksum string
	if err := store.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = 1`).Scan(&recordedChecksum); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if recordedChecksum == "" {
		t.Fatal("recorded checksum is empty")
	}
}

// TestMigrate_RestartIsNoOp proves that running Migrate again against an
// already-migrated database (as a server restart would) is a safe no-op:
// no error, and the migration is not re-applied a second time (plan 11.1:
// "повторный startup").
func TestMigrate_RestartIsNoOp(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")
	store, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if _, err := store.Migrate(ctx, io.Discard); err != nil {
		t.Fatalf("Migrate (first run): %v", err)
	}
	if _, err := store.Migrate(ctx, io.Discard); err != nil {
		t.Fatalf("Migrate (second run / restart): %v", err)
	}

	var migrationRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationRows); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if migrationRows != 1 {
		t.Fatalf("schema_migrations has %d rows after two Migrate calls, want 1 (no duplicate application)", migrationRows)
	}
}

func TestMigrate_ChecksumMismatchStopsStartup(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)

	// Tamper with the recorded checksum, simulating an already-applied
	// migration file that was edited after the fact.
	if _, err := store.db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'deadbeef' WHERE version = 1`); err != nil {
		t.Fatalf("tamper with schema_migrations: %v", err)
	}

	_, err := store.Migrate(ctx, io.Discard)
	if err == nil {
		t.Fatal("Migrate succeeded after checksum tampering, want ErrChecksumMismatch")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Migrate error = %v, want ErrChecksumMismatch", err)
	}
}

func TestMigrate_SchemaVersion(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)

	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", version)
	}
}

func TestMigrate_SchemaVersionBeforeAnyMigration(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")
	store, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if err := store.ensureSchemaMigrationsTable(ctx); err != nil {
		t.Fatalf("ensureSchemaMigrationsTable: %v", err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 0 {
		t.Fatalf("SchemaVersion before any migration = %d, want 0", version)
	}
}

func TestLoadMigrations_SortedAndChecksummed(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("loadMigrations returned no migrations")
	}
	for i, m := range migrations {
		if m.checksum == "" {
			t.Errorf("migration %d (%s) has empty checksum", m.version, m.name)
		}
		if i > 0 && migrations[i-1].version >= m.version {
			t.Errorf("migrations not strictly ascending by version: %d then %d", migrations[i-1].version, m.version)
		}
	}
	if migrations[0].version != 1 {
		t.Fatalf("first migration version = %d, want 1", migrations[0].version)
	}
}

package sqlite

import (
	"context"
	"io"
	"path/filepath"
	"testing"
)

// openMigratedStore opens a fresh Store at a temp-directory database file
// and runs Migrate against it, failing the test on any error. t.TempDir()
// gives every test its own directory, cleaned up automatically, so this
// never touches a shared or repo-tracked path.
func openMigratedStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	store, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if _, err := store.Migrate(ctx, io.Discard); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return store
}

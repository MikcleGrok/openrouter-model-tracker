package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

func TestBackup_ProducesReadableFileWithData(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, []feedback.SkillRating{{Key: "coding", Rating: 5}}, "great"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	path, err := store.Backup(ctx, backupDir, 0)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), "feedback-v1-") {
		t.Fatalf("backup filename = %q, want it tagged with schema version 1", filepath.Base(path))
	}

	restored, err := Open(ctx, path, Config{})
	if err != nil {
		t.Fatalf("Open backup file: %v", err)
	}
	defer restored.Close()

	own, found, err := restored.OwnFeedback(ctx, "user-a", feedback.ModelKey("anthropic/claude"))
	if err != nil || !found {
		t.Fatalf("OwnFeedback on backup: found=%v err=%v", found, err)
	}
	if own.Overall != 5 || own.Review != "great" {
		t.Fatalf("backup data = %+v, want the original feedback", own)
	}
}

func TestBackup_NoTemporaryFileLeftBehind(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	path, err := store.Backup(ctx, backupDir, 0)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Fatalf("temporary backup file %s.tmp still exists after Backup", path)
	}
}

func TestBackup_PostDeleteBackupExcludesDeletedIdentity(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, "secret"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, _, err := store.DeleteIdentity(ctx, "user-a", now); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	path, err := store.Backup(ctx, backupDir, 0)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	backupStore, err := Open(ctx, path, Config{})
	if err != nil {
		t.Fatalf("Open backup: %v", err)
	}
	defer backupStore.Close()

	var count int
	if err := backupStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback`).Scan(&count); err != nil {
		t.Fatalf("count model_feedback in backup: %v", err)
	}
	if count != 0 {
		t.Fatalf("backup taken after delete still has %d model_feedback rows, want 0", count)
	}
}

func TestBackup_PrunesOldBackupsBeyondRetain(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	var paths []string
	for i := 0; i < 5; i++ {
		path, err := store.Backup(ctx, backupDir, 2)
		if err != nil {
			t.Fatalf("Backup #%d: %v", i, err)
		}
		paths = append(paths, path)
		// Ensure the next backup's timestamp-based filename sorts strictly
		// after this one even at low clock resolution.
		time.Sleep(2 * time.Millisecond)
	}

	matches, err := filepath.Glob(filepath.Join(backupDir, backupFileGlob))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("backups remaining = %d, want 2 (retain=2)", len(matches))
	}

	// The two most recent backups (by creation order) must be the ones kept.
	for _, kept := range paths[len(paths)-2:] {
		found := false
		for _, m := range matches {
			if m == kept {
				found = true
			}
		}
		if !found {
			t.Errorf("expected the most recent backup %s to survive pruning; remaining=%v", kept, matches)
		}
	}
}

func TestBackup_DefaultRetainWhenNonPositive(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	for i := 0; i < DefaultBackupRetain+2; i++ {
		if _, err := store.Backup(ctx, backupDir, 0); err != nil {
			t.Fatalf("Backup #%d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	matches, err := filepath.Glob(filepath.Join(backupDir, backupFileGlob))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != DefaultBackupRetain {
		t.Fatalf("backups remaining = %d, want %d (default retain)", len(matches), DefaultBackupRetain)
	}
}

func TestRestore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 4, []feedback.SkillRating{{Key: "reasoning", Rating: 4}}, "round trip"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	backupPath, err := store.Backup(ctx, backupDir, 0)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Simulate the DB being replaced with an older/different state: a fresh,
	// empty destination database with the same schema but no data.
	destPath := filepath.Join(t.TempDir(), "restored.sqlite")
	destStore, err := Open(ctx, destPath, Config{})
	if err != nil {
		t.Fatalf("Open dest: %v", err)
	}
	if _, err := destStore.Migrate(ctx, nil); err != nil {
		t.Fatalf("Migrate dest: %v", err)
	}
	if err := destStore.Close(); err != nil {
		t.Fatalf("Close dest before restore: %v", err)
	}

	if err := Restore(ctx, backupPath, destPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	reopened, err := Open(ctx, destPath, Config{})
	if err != nil {
		t.Fatalf("Open restored: %v", err)
	}
	defer reopened.Close()

	own, found, err := reopened.OwnFeedback(ctx, "user-a", feedback.ModelKey("anthropic/claude"))
	if err != nil || !found {
		t.Fatalf("OwnFeedback on restored db: found=%v err=%v", found, err)
	}
	if own.Overall != 4 || own.Review != "round trip" {
		t.Fatalf("restored data = %+v, want the original feedback", own)
	}

	version, err := reopened.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 1 {
		t.Fatalf("restored schema version = %d, want 1", version)
	}
}

func TestRestore_RejectsUnreadableSource(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "dest.sqlite")
	if err := os.WriteFile(destPath, []byte("original content"), 0o600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	badSource := filepath.Join(t.TempDir(), "not-a-database.sqlite")
	if err := os.WriteFile(badSource, []byte("this is not a sqlite file"), 0o600); err != nil {
		t.Fatalf("write bad source: %v", err)
	}

	err := Restore(context.Background(), badSource, destPath)
	if err == nil {
		t.Fatal("Restore from an unreadable source succeeded, want an error")
	}

	content, readErr := os.ReadFile(destPath)
	if readErr != nil {
		t.Fatalf("read dest after failed restore: %v", readErr)
	}
	if string(content) != "original content" {
		t.Fatalf("dest content changed despite Restore failing: %q", content)
	}
}

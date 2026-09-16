package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

func TestDeleteIdentity_DeletesFeedbackAndSkillsAndIdentity(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-a")
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, identity, mustInput(t, "anthropic/claude", 5, []feedback.SkillRating{{Key: "coding", Rating: 5}}, "hello"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, identity, mustInput(t, "openai/gpt", 3, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	job, created, err := store.DeleteIdentity(ctx, identity, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	if !created {
		t.Fatal("DeleteIdentity created=false, want true (no active job existed)")
	}
	if job.State != CleanupJobPending {
		t.Fatalf("job.State = %s, want %s", job.State, CleanupJobPending)
	}
	if job.ID == "" {
		t.Fatal("job.ID is empty")
	}

	for _, table := range []string{"identities", "model_feedback", "skill_ratings"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE 1=1").Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("table %s has %d rows after DeleteIdentity, want 0", table, count)
		}
	}

	own, found, err := store.OwnFeedback(ctx, identity, feedback.ModelKey("anthropic/claude"))
	if err != nil {
		t.Fatalf("OwnFeedback after delete: %v", err)
	}
	if found {
		t.Fatalf("OwnFeedback found data after DeleteIdentity: %+v", own)
	}
}

func TestDeleteIdentity_IdempotentWhenIdentityAbsent(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	job, created, err := store.DeleteIdentity(ctx, "never-existed", now)
	if err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	if !created {
		t.Fatal("created = false for an identity with no rows, want true (still creates and commits a job)")
	}
	if job.State != CleanupJobPending {
		t.Fatalf("job.State = %s, want %s", job.State, CleanupJobPending)
	}
}

func TestDeleteIdentity_ActiveJobBlocksSecondDelete(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback (a): %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, "user-b", mustInput(t, "anthropic/claude", 4, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback (b): %v", err)
	}

	firstJob, created, err := store.DeleteIdentity(ctx, "user-a", now)
	if err != nil || !created {
		t.Fatalf("DeleteIdentity (a): created=%v err=%v", created, err)
	}

	// Job is still cleanup_pending (RunCleanup has not run yet); a second
	// DeleteIdentity for a different identity must not start a new delete.
	secondJob, created, err := store.DeleteIdentity(ctx, "user-b", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("DeleteIdentity (b): %v", err)
	}
	if created {
		t.Fatal("DeleteIdentity (b) created=true while a cleanup job was already active, want false")
	}
	if secondJob.ID != firstJob.ID {
		t.Fatalf("DeleteIdentity (b) returned a different job: got %s, want the still-active %s", secondJob.ID, firstJob.ID)
	}

	// user-b's data must be untouched.
	own, found, err := store.OwnFeedback(ctx, "user-b", feedback.ModelKey("anthropic/claude"))
	if err != nil || !found {
		t.Fatalf("OwnFeedback (b) after blocked delete: found=%v err=%v", found, err)
	}
	if own.Overall != 4 {
		t.Fatalf("user-b feedback changed: %+v", own)
	}

	// Complete the cleanup for user-a's job, then user-b's delete must
	// succeed and create a fresh job.
	backupDir := filepath.Join(t.TempDir(), "backups")
	if _, err := store.RunCleanup(ctx, backupDir, 0, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("RunCleanup: %v", err)
	}

	thirdJob, created, err := store.DeleteIdentity(ctx, "user-b", now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("DeleteIdentity (b, after cleanup done): %v", err)
	}
	if !created {
		t.Fatal("DeleteIdentity (b) after prior job completed: created=false, want true")
	}
	if thirdJob.ID == firstJob.ID {
		t.Fatal("DeleteIdentity (b) reused the completed job's ID instead of creating a new one")
	}
}

func TestUpsertFeedback_BlockedByActiveCleanupJob(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback (a): %v", err)
	}
	if _, _, err := store.DeleteIdentity(ctx, "user-a", now); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}

	_, err := store.UpsertFeedback(ctx, "user-b", mustInput(t, "anthropic/claude", 3, nil, ""), now.Add(time.Minute))
	if !errors.Is(err, ErrMaintenanceLocked) {
		t.Fatalf("UpsertFeedback while a cleanup job is active: err = %v, want ErrMaintenanceLocked", err)
	}

	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback`).Scan(&count); err != nil {
		t.Fatalf("count model_feedback: %v", err)
	}
	if count != 0 {
		t.Fatalf("model_feedback has %d rows, want 0 (blocked write must not have applied)", count)
	}
}

func TestRunCleanup_NoActiveJob(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	_, err := store.RunCleanup(ctx, filepath.Join(t.TempDir(), "backups"), 0, time.Now())
	if !errors.Is(err, ErrNoActiveCleanupJob) {
		t.Fatalf("RunCleanup with no active job: err = %v, want ErrNoActiveCleanupJob", err)
	}
}

func TestRunCleanup_MarksJobDoneAndReleasesLock(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, _, err := store.DeleteIdentity(ctx, "user-a", now); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	job, err := store.RunCleanup(ctx, backupDir, 0, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RunCleanup: %v", err)
	}
	if job.State != CleanupJobDone {
		t.Fatalf("job.State = %s, want %s", job.State, CleanupJobDone)
	}

	if _, found, err := store.ActiveCleanupJob(ctx); err != nil || found {
		t.Fatalf("ActiveCleanupJob after RunCleanup: found=%v err=%v, want no active job", found, err)
	}

	// The maintenance lock is released: a normal write now succeeds.
	if _, err := store.UpsertFeedback(ctx, "user-b", mustInput(t, "anthropic/claude", 3, nil, ""), now.Add(2*time.Minute)); err != nil {
		t.Fatalf("UpsertFeedback after cleanup done: %v", err)
	}
}

// TestRunCleanup_RetryAfterFailure simulates a failed post-commit cleanup
// (an unwritable backup directory) and proves: the job moves to failed
// without touching the already-committed delete, and a subsequent retry
// with a valid directory moves it through cleanup_pending back to done.
func TestRunCleanup_RetryAfterFailure(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, _, err := store.DeleteIdentity(ctx, "user-a", now); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}

	// A backup dir path that cannot be created (its parent is a regular
	// file, not a directory) forces Backup -> RunCleanup to fail.
	blockerFile := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blockerFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	badBackupDir := filepath.Join(blockerFile, "backups")

	job, err := store.RunCleanup(ctx, badBackupDir, 0, now.Add(time.Minute))
	if err == nil {
		t.Fatal("RunCleanup with an unwritable backup dir succeeded, want an error")
	}
	if job.State != CleanupJobFailed {
		t.Fatalf("job.State after failed cleanup = %s, want %s", job.State, CleanupJobFailed)
	}

	// The committed delete must be untouched by the failed cleanup attempt.
	if _, found, err := store.OwnFeedback(ctx, "user-a", feedback.ModelKey("anthropic/claude")); err != nil || found {
		t.Fatalf("OwnFeedback after failed cleanup: found=%v err=%v, want still deleted", found, err)
	}

	active, found, err := store.ActiveCleanupJob(ctx)
	if err != nil || !found {
		t.Fatalf("ActiveCleanupJob after failure: found=%v err=%v, want the failed job still active", found, err)
	}
	if active.State != CleanupJobFailed {
		t.Fatalf("active job state = %s, want %s", active.State, CleanupJobFailed)
	}

	// Retry with a valid directory.
	goodBackupDir := filepath.Join(t.TempDir(), "backups")
	retried, err := store.RunCleanup(ctx, goodBackupDir, 0, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("RunCleanup (retry): %v", err)
	}
	if retried.State != CleanupJobDone {
		t.Fatalf("retried job.State = %s, want %s", retried.State, CleanupJobDone)
	}
	if retried.ID != job.ID {
		t.Fatalf("retry created a new job (%s) instead of resuming the failed one (%s)", retried.ID, job.ID)
	}
}

// TestRunCleanup_ResumesAfterSimulatedCrash proves that RunCleanup does not
// care how a durable cleanup_pending job came to exist: a job inserted
// directly (standing in for "DeleteIdentity committed, then the process
// crashed before any cleanup ran") is picked up and completed exactly like
// one DeleteIdentity itself just created.
func TestRunCleanup_ResumesAfterSimulatedCrash(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	nowText := formatTime(now)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO privacy_cleanup_jobs (id, state, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, "crash-job", string(CleanupJobPending), nowText, nowText); err != nil {
		t.Fatalf("insert durable job: %v", err)
	}

	job, err := store.RunCleanup(ctx, filepath.Join(t.TempDir(), "backups"), 0, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RunCleanup: %v", err)
	}
	if job.ID != "crash-job" || job.State != CleanupJobDone {
		t.Fatalf("job = %+v, want id=crash-job state=done", job)
	}
}

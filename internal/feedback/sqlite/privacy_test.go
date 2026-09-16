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
	// DeleteIdentity for a different identity must be refused outright, not
	// silently treated as an equivalent success -- there is no way to tell
	// "my own retry" from "someone else's delete" once a job carries no
	// identity, so ErrMaintenanceLocked is the only honest answer.
	_, created, err = store.DeleteIdentity(ctx, "user-b", now.Add(time.Minute))
	if !errors.Is(err, ErrMaintenanceLocked) {
		t.Fatalf("DeleteIdentity (b) while a cleanup job was already active: err = %v, want ErrMaintenanceLocked", err)
	}
	if created {
		t.Fatal("DeleteIdentity (b) created=true despite returning ErrMaintenanceLocked")
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
	if _, err := store.RunCleanup(ctx, backupDir, now.Add(2*time.Minute)); err != nil {
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
	_, err := store.RunCleanup(ctx, filepath.Join(t.TempDir(), "backups"), time.Now())
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
	job, err := store.RunCleanup(ctx, backupDir, now.Add(time.Minute))
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

// TestRunCleanup_PostDeleteCleanupRemovesAllOtherBackups is the regression
// test for the critical review finding: RunCleanup's post-commit backup
// must not apply ordinary retention (which would let up to
// DefaultBackupRetain-1 pre-delete backups survive with the deleted
// identity's data still in them, while the job reports done). It seeds two
// pre-delete backups, deletes the identity, runs RunCleanup, and asserts
// both old backups are gone and the one survivor has zero model_feedback
// rows for the deleted identity.
func TestRunCleanup_PostDeleteCleanupRemovesAllOtherBackups(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	backupDir := filepath.Join(t.TempDir(), "backups")

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, "secret"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	// Two pre-delete backups, both still containing user-a's data. Passing
	// a generous retain here proves the fix is specific to RunCleanup's own
	// call, not a change to Backup's default behavior.
	for i := 0; i < 2; i++ {
		if _, err := store.Backup(ctx, backupDir, 10); err != nil {
			t.Fatalf("pre-delete Backup #%d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	preDeleteMatches, err := filepath.Glob(filepath.Join(backupDir, backupFileGlob))
	if err != nil {
		t.Fatalf("glob pre-delete backups: %v", err)
	}
	if len(preDeleteMatches) != 2 {
		t.Fatalf("pre-delete backups = %d, want 2 (test setup problem)", len(preDeleteMatches))
	}

	if _, _, err := store.DeleteIdentity(ctx, "user-a", now.Add(time.Hour)); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	if _, err := store.RunCleanup(ctx, backupDir, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("RunCleanup: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(backupDir, backupFileGlob))
	if err != nil {
		t.Fatalf("glob backups after cleanup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("backups after RunCleanup = %v, want exactly 1 (the post-delete backup, every pre-delete backup removed)", matches)
	}
	for _, stale := range preDeleteMatches {
		if _, err := os.Stat(stale); err == nil {
			t.Errorf("pre-delete backup %s still exists after RunCleanup", stale)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat %s: %v", stale, err)
		}
	}

	survivor, err := Open(ctx, matches[0], Config{})
	if err != nil {
		t.Fatalf("Open surviving backup: %v", err)
	}
	defer survivor.Close()
	var count int
	if err := survivor.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback`).Scan(&count); err != nil {
		t.Fatalf("count model_feedback in surviving backup: %v", err)
	}
	if count != 0 {
		t.Fatalf("surviving backup has %d model_feedback rows, want 0 (deleted identity's data must not survive)", count)
	}
}

// TestRunCleanup_KeepsExactlyTheFreshBackupRegardlessOfFilenameSort is the
// regression test for the round-2 review finding: RunCleanup's "keep only
// the fresh post-delete backup" guarantee must not depend on
// pruneOldBackups' plain lexicographic filename sort, which misorders once
// two backups' version numbers have a different digit count --
// "feedback-v10-..." sorts before "feedback-v9-..." as a string even though
// 10 > 9 numerically. It seeds a stale dummy file named as if from schema
// version 10 (this repo's real schema is v1, so any dummy with a version
// number that sorts after "v1" reproduces the same class of misordering)
// before running the real DeleteIdentity + RunCleanup sequence, and asserts
// RunCleanup ends up keeping the backup it just took -- not the stale dummy
// a sort-based scheme would have preferred.
func TestRunCleanup_KeepsExactlyTheFreshBackupRegardlessOfFilenameSort(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	backupDir := filepath.Join(t.TempDir(), "backups")

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "anthropic/claude", 5, nil, "secret"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("mkdir backupDir: %v", err)
	}
	// A dummy file matching backupFileGlob whose name sorts lexicographically
	// AFTER any "feedback-v1-..." backup this schema version can ever
	// produce, exactly reproducing what "feedback-v10-..." vs
	// "feedback-v9-..." would do at version 9/10: a plain sort-and-keep-the-
	// last-N scheme (pruneOldBackups with retain=1) would keep THIS stale
	// file over the fresh post-delete backup RunCleanup is about to take.
	// Its content is irrelevant -- pruning only matches filenames.
	stalePath := filepath.Join(backupDir, "feedback-v10-99999999T999999.000000000Z.sqlite")
	if err := os.WriteFile(stalePath, []byte("not a real backup, just needs to match the glob"), 0o600); err != nil {
		t.Fatalf("write stale dummy backup: %v", err)
	}

	if _, _, err := store.DeleteIdentity(ctx, "user-a", now.Add(time.Hour)); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	job, err := store.RunCleanup(ctx, backupDir, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("RunCleanup: %v", err)
	}
	if job.State != CleanupJobDone {
		t.Fatalf("job.State = %s, want %s", job.State, CleanupJobDone)
	}

	matches, err := filepath.Glob(filepath.Join(backupDir, backupFileGlob))
	if err != nil {
		t.Fatalf("glob backups after cleanup: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("backups after RunCleanup = %v, want exactly 1", matches)
	}
	if matches[0] == stalePath {
		t.Fatalf("RunCleanup kept the stale dummy backup (%s) instead of the fresh post-delete backup -- the exact leak this fix prevents", stalePath)
	}
	if _, err := os.Stat(stalePath); err == nil {
		t.Fatalf("stale dummy backup %s still exists after RunCleanup", stalePath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", stalePath, err)
	}

	survivor, err := Open(ctx, matches[0], Config{})
	if err != nil {
		t.Fatalf("Open surviving backup: %v", err)
	}
	defer survivor.Close()
	var count int
	if err := survivor.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback`).Scan(&count); err != nil {
		t.Fatalf("count model_feedback in surviving backup: %v", err)
	}
	if count != 0 {
		t.Fatalf("surviving backup has %d model_feedback rows, want 0", count)
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

	job, err := store.RunCleanup(ctx, badBackupDir, now.Add(time.Minute))
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
	retried, err := store.RunCleanup(ctx, goodBackupDir, now.Add(2*time.Minute))
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

	job, err := store.RunCleanup(ctx, filepath.Join(t.TempDir(), "backups"), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("RunCleanup: %v", err)
	}
	if job.ID != "crash-job" || job.State != CleanupJobDone {
		t.Fatalf("job = %+v, want id=crash-job state=done", job)
	}
}

// TestDeleteIdentity_RollsBackJobAndDeleteOnMidTransactionFailure injects a
// failure between the job INSERT and the deletes inside DeleteIdentity's own
// transaction (a BEFORE DELETE trigger on model_feedback, forcing exactly
// the DELETE FROM model_feedback statement to fail) and proves the whole
// transaction rolled back: no durable job, no partial delete, no lock left
// behind. This is the "crash/error before COMMIT" half of plan 4.7's
// contract that TestUpsertFeedback_RollsBackOnSkillInsertError already
// proves for UpsertFeedback but nothing previously proved for DeleteIdentity
// itself.
func TestDeleteIdentity_RollsBackJobAndDeleteOnMidTransactionFailure(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-a")
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, identity, mustInput(t, "anthropic/claude", 5, []feedback.SkillRating{{Key: "coding", Rating: 5}}, "hello"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER force_delete_failure BEFORE DELETE ON model_feedback
		BEGIN SELECT RAISE(ABORT, 'forced test failure'); END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(ctx, `DROP TRIGGER IF EXISTS force_delete_failure`)
	})

	_, created, err := store.DeleteIdentity(ctx, identity, now.Add(time.Hour))
	if err == nil {
		t.Fatal("DeleteIdentity succeeded despite the forced mid-transaction failure, want an error")
	}
	if created {
		t.Fatal("created=true despite the transaction failing")
	}

	// The job must not exist: it was inserted in the same, now rolled-back,
	// transaction as the failed delete -- an uncommitted job is not durable.
	var jobCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM privacy_cleanup_jobs`).Scan(&jobCount); err != nil {
		t.Fatalf("count privacy_cleanup_jobs: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("privacy_cleanup_jobs has %d rows after a rolled-back delete, want 0", jobCount)
	}

	// The identity's data must be fully intact -- both feedback and skills.
	own, found, err := store.OwnFeedback(ctx, identity, feedback.ModelKey("anthropic/claude"))
	if err != nil || !found {
		t.Fatalf("OwnFeedback after rolled-back delete: found=%v err=%v", found, err)
	}
	if own.Overall != 5 || len(own.Skills) != 1 {
		t.Fatalf("feedback changed despite rollback: %+v", own)
	}

	var identityCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM identities WHERE id = ?`, identityBytes(identity)).Scan(&identityCount); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if identityCount != 1 {
		t.Fatal("identities row missing after a rolled-back delete")
	}

	// No lock left behind: the identity remains available for a repeated,
	// equally all-or-nothing DELETE call.
	if _, foundActive, err := store.ActiveCleanupJob(ctx); err != nil || foundActive {
		t.Fatalf("ActiveCleanupJob after a rolled-back delete: found=%v err=%v, want no lock", foundActive, err)
	}
}

// TestDeleteIdentity_SurvivesRestartAndResumesCleanup runs the real
// DeleteIdentity -> Close -> Open sequence (not a job row inserted directly,
// unlike TestRunCleanup_ResumesAfterSimulatedCrash) and proves every step of
// the resume path plan 4.7/10.2 describes: the committed delete survives the
// restart, the reopened Store observes a durable cleanup_pending job,
// ordinary writes stay blocked with ErrMaintenanceLocked until cleanup
// finishes, and RunCleanup then completes cleanly and releases the lock.
func TestDeleteIdentity_SurvivesRestartAndResumesCleanup(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "feedback.sqlite")

	store, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.Migrate(ctx, nil); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	now := time.Now()
	identity := feedback.IdentityID("user-a")
	if _, err := store.UpsertFeedback(ctx, identity, mustInput(t, "anthropic/claude", 5, nil, "secret"), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, created, err := store.DeleteIdentity(ctx, identity, now.Add(time.Minute)); err != nil || !created {
		t.Fatalf("DeleteIdentity: created=%v err=%v", created, err)
	}

	// Simulate the process crashing/restarting right after the committed
	// delete, before any post-commit cleanup ran: close and reopen against
	// the same file, with no RunCleanup call in between.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	restarted, err := Open(ctx, dbPath, Config{})
	if err != nil {
		t.Fatalf("Open (restart): %v", err)
	}
	defer restarted.Close()

	// The committed delete must have survived the restart.
	if _, found, err := restarted.OwnFeedback(ctx, identity, feedback.ModelKey("anthropic/claude")); err != nil || found {
		t.Fatalf("OwnFeedback after restart: found=%v err=%v, want gone", found, err)
	}

	// The durable job must still be there, in cleanup_pending -- exactly the
	// state a real restart must observe and resume (plan: "сервер видит
	// durable cleanup_pending... и возобновляет cleanup").
	active, found, err := restarted.ActiveCleanupJob(ctx)
	if err != nil || !found {
		t.Fatalf("ActiveCleanupJob after restart: found=%v err=%v", found, err)
	}
	if active.State != CleanupJobPending {
		t.Fatalf("active job state after restart = %s, want %s", active.State, CleanupJobPending)
	}

	// Writes stay blocked while the job is unresolved, restart included.
	if _, err := restarted.UpsertFeedback(ctx, "user-b", mustInput(t, "anthropic/claude", 3, nil, ""), now.Add(2*time.Minute)); !errors.Is(err, ErrMaintenanceLocked) {
		t.Fatalf("UpsertFeedback after restart while job is still pending: err = %v, want ErrMaintenanceLocked", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	done, err := restarted.RunCleanup(ctx, backupDir, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("RunCleanup after restart: %v", err)
	}
	if done.State != CleanupJobDone {
		t.Fatalf("job state after RunCleanup = %s, want %s", done.State, CleanupJobDone)
	}

	if _, found, err := restarted.ActiveCleanupJob(ctx); err != nil || found {
		t.Fatalf("ActiveCleanupJob after RunCleanup: found=%v err=%v, want none", found, err)
	}

	// The lock is released: a normal write now succeeds.
	if _, err := restarted.UpsertFeedback(ctx, "user-b", mustInput(t, "anthropic/claude", 3, nil, ""), now.Add(4*time.Minute)); err != nil {
		t.Fatalf("UpsertFeedback after RunCleanup: %v", err)
	}
}

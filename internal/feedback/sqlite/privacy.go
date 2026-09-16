package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// CleanupJobState is privacy_cleanup_jobs.state (plan 4.7/5.2/10.2): the
// three-state lifecycle of one self-service identity deletion's post-commit
// backup cleanup.
type CleanupJobState string

const (
	// CleanupJobPending means the DB delete this job was created alongside
	// already committed, and the post-commit backup cleanup has not yet
	// finished (or has not been attempted at all yet).
	CleanupJobPending CleanupJobState = "cleanup_pending"
	// CleanupJobDone means both the DB delete and the post-commit backup
	// cleanup completed successfully. Terminal.
	CleanupJobDone CleanupJobState = "done"
	// CleanupJobFailed means the post-commit backup cleanup phase failed —
	// the DB delete it followed is untouched and stays committed. RunCleanup
	// moves a failed job back to CleanupJobPending before retrying it.
	CleanupJobFailed CleanupJobState = "failed"
)

// CleanupJob is one durable privacy_cleanup_jobs row. It deliberately never
// carries an identity, review, or token (plan 5.2: "privacy_cleanup_jobs —
// durable maintenance state без identity/review/token") — by the time a job
// exists, the identity it was for has already been deleted in the same
// transaction.
type CleanupJob struct {
	ID        string
	State     CleanupJobState
	CreatedAt time.Time
	UpdatedAt time.Time
}

// deleteIdentityResult is DeleteIdentity's withTx payload: the job plus
// whether this call is the one that created it (as opposed to finding one
// already active and declining to start a second delete).
type deleteIdentityResult struct {
	job     CleanupJob
	created bool
}

// DeleteIdentity implements the DB half of DELETE /v1/me/feedback (plan
// 4.7/10.2). In one transaction (BEGIN IMMEDIATE, via Open's _txlock=immediate)
// it: checks for an already-active cleanup job and, if found, returns it
// with created=false instead of starting a second delete (plan 5.2: "не
// более одной активной job; активная job блокирует ... privacy operations");
// otherwise creates a new job row (state cleanup_pending, no identity/
// review/token), deletes every model_feedback row for identity (cascading
// to skill_ratings via ON DELETE CASCADE), deletes the identities row, and
// commits. The job becomes durable only together with the committed delete:
// any error before COMMIT (via withTx's rollback) leaves neither behind, so
// identity remains available for a repeated, equally all-or-nothing call —
// this method's own idempotency. It does not distinguish "identity had no
// rows" from "identity had rows": both delete 0-or-more rows and commit the
// same job, matching the endpoint's idempotent contract.
//
// Deciding what HTTP status this maps to (204 vs 202 cleanup_pending vs 503)
// is Task 4's job; this method only reports what happened at the DB layer.
func (s *Store) DeleteIdentity(ctx context.Context, identity feedback.IdentityID, now time.Time) (job CleanupJob, created bool, err error) {
	if identity == "" {
		return CleanupJob{}, false, fmt.Errorf("sqlite: DeleteIdentity: empty identity")
	}

	result, err := withTx(ctx, s.db, func(tx *sql.Tx) (deleteIdentityResult, error) {
		existing, found, err := activeJobTx(ctx, tx)
		if err != nil {
			return deleteIdentityResult{}, err
		}
		if found {
			return deleteIdentityResult{job: existing, created: false}, nil
		}

		newJob := CleanupJob{ID: newJobID(), State: CleanupJobPending, CreatedAt: now, UpdatedAt: now}
		nowText := formatTime(now)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO privacy_cleanup_jobs (id, state, created_at, updated_at) VALUES (?, ?, ?, ?)
		`, newJob.ID, string(newJob.State), nowText, nowText); err != nil {
			return deleteIdentityResult{}, fmt.Errorf("sqlite: create cleanup job: %w", err)
		}

		idBytes := identityBytes(identity)
		if _, err := tx.ExecContext(ctx, `DELETE FROM model_feedback WHERE identity_id = ?`, idBytes); err != nil {
			return deleteIdentityResult{}, fmt.Errorf("sqlite: delete model_feedback: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM identities WHERE id = ?`, idBytes); err != nil {
			return deleteIdentityResult{}, fmt.Errorf("sqlite: delete identity: %w", err)
		}

		return deleteIdentityResult{job: newJob, created: true}, nil
	})
	if err != nil {
		return CleanupJob{}, false, err
	}
	return result.job, result.created, nil
}

// ActiveCleanupJob returns the current active (state cleanup_pending or
// failed — anything not done) privacy_cleanup_jobs row, if any. It is the
// read-only form of the same lookup UpsertFeedback/DeleteIdentity make
// inside their own transactions, exposed for a caller (Task 4's server
// startup, or a health check) that needs to know whether the maintenance
// lock is held without starting a write.
func (s *Store) ActiveCleanupJob(ctx context.Context) (CleanupJob, bool, error) {
	return activeJobTx(ctx, s.db)
}

// txQueryer is the subset of *sql.DB/*sql.Tx activeJobTx needs, so it can
// run identically inside a transaction (the maintenance-lock check
// UpsertFeedback/DeleteIdentity make) or standalone (ActiveCleanupJob).
type txQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func activeJobTx(ctx context.Context, q txQueryer) (CleanupJob, bool, error) {
	var job CleanupJob
	var state, createdAtText, updatedAtText string
	err := q.QueryRowContext(ctx, `
		SELECT id, state, created_at, updated_at
		FROM privacy_cleanup_jobs WHERE state <> 'done'
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&job.ID, &state, &createdAtText, &updatedAtText)
	if errors.Is(err, sql.ErrNoRows) {
		return CleanupJob{}, false, nil
	}
	if err != nil {
		return CleanupJob{}, false, fmt.Errorf("sqlite: read active cleanup job: %w", err)
	}
	job.State = CleanupJobState(state)
	if job.CreatedAt, err = parseTime(createdAtText); err != nil {
		return CleanupJob{}, false, err
	}
	if job.UpdatedAt, err = parseTime(updatedAtText); err != nil {
		return CleanupJob{}, false, err
	}
	return job, true, nil
}

func (s *Store) setJobState(ctx context.Context, jobID string, state CleanupJobState, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE privacy_cleanup_jobs SET state = ?, updated_at = ? WHERE id = ?
	`, string(state), formatTime(now), jobID)
	if err != nil {
		return fmt.Errorf("sqlite: set cleanup job %s state %s: %w", jobID, state, err)
	}
	return nil
}

// RunCleanup performs the post-commit cleanup phase described in plan
// 4.7/10.2, as a separate, idempotent, resumable operation from the DELETE
// transaction DeleteIdentity already committed: it takes a fresh backup
// (already without the deleted identity, since the delete already
// committed), verifies that backup is readable, prunes old backups per
// retain, and only then marks the active job done — releasing the
// maintenance lock. It returns ErrNoActiveCleanupJob if there is no job in
// cleanup_pending or failed state to resume (plan: "при crash/restart
// сервер видит durable cleanup_pending... и возобновляет cleanup").
//
// A prior failure (job state failed) is retried by first durably moving the
// job back to cleanup_pending — a separate, already-committed step before
// this call attempts the backup again — and then re-running the same
// backup+verify+prune sequence; a crash between that state change and this
// call finishing still leaves a durable cleanup_pending for the next call to
// resume, never a lost or ambiguous state. On failure, the job is marked
// failed and the error returned; the committed DB delete this job followed
// is never touched by this method.
func (s *Store) RunCleanup(ctx context.Context, backupDir string, retain int, now time.Time) (CleanupJob, error) {
	job, found, err := s.ActiveCleanupJob(ctx)
	if err != nil {
		return CleanupJob{}, err
	}
	if !found {
		return CleanupJob{}, ErrNoActiveCleanupJob
	}

	if job.State == CleanupJobFailed {
		if err := s.setJobState(ctx, job.ID, CleanupJobPending, now); err != nil {
			return CleanupJob{}, err
		}
		job.State = CleanupJobPending
		job.UpdatedAt = now
	}

	if _, backupErr := s.Backup(ctx, backupDir, retain); backupErr != nil {
		if setErr := s.setJobState(ctx, job.ID, CleanupJobFailed, now); setErr != nil {
			return CleanupJob{}, fmt.Errorf("sqlite: post-commit cleanup failed (%v) and marking job failed also failed: %w", backupErr, setErr)
		}
		job.State = CleanupJobFailed
		job.UpdatedAt = now
		return job, fmt.Errorf("sqlite: post-commit cleanup: %w", backupErr)
	}

	if err := s.setJobState(ctx, job.ID, CleanupJobDone, now); err != nil {
		return CleanupJob{}, err
	}
	job.State = CleanupJobDone
	job.UpdatedAt = now
	return job, nil
}

// newJobID generates a random 128-bit hex identifier for a new
// privacy_cleanup_jobs row. It carries no meaning beyond uniqueness — no
// timestamp encoding, no identity fragment — matching the job's own "no
// identity" contract.
func newJobID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failing is effectively unreachable on every
		// platform this project targets; fall back rather than panicking.
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

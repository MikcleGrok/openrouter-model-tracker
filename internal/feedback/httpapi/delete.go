package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

// handleDeleteMe implements DELETE /v1/me/feedback (plan 4.7). Called only
// from behind authUser (routes.go), so identityFromContext is always
// populated here.
//
// Store.DeleteIdentity's own contract (sqlite/privacy.go) returns
// ErrMaintenanceLocked unconditionally whenever a privacy_cleanup_jobs row
// is already active — including when it is this exact identity's own
// prior, already-committed delete whose post-commit cleanup has not yet
// finished, since a job carries no identity and there is no safe way to
// tell the two cases apart at that layer. Rather than mapping that
// straight to a bare 503 (which would tell an idempotent retrying caller
// their delete failed, when in fact their data may already be gone), this
// handler treats ErrMaintenanceLocked as "something is blocking; drain it
// and try my own delete again": it runs RunCleanup to advance whatever job
// is currently active, then retries DeleteIdentity. Once the lock clears —
// whether the job it drained was this identity's own prior delete or a
// completely unrelated one — DeleteIdentity's next attempt performs this
// identity's actual delete for real (it is idempotent: even if this
// identity's rows were already gone, it still creates a fresh job, deletes
// zero rows, and commits, exactly matching its own documented "does not
// distinguish 'had no rows' from 'had rows'" contract), so a 204 or 202
// this handler returns is always backed by a delete this exact request
// caused or confirmed, never a borrowed report of a stranger's job.
func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	ctx := r.Context()
	now := s.now()

	for attempt := 0; attempt < s.deleteRetryAttempts; attempt++ {
		_, _, err := s.store.DeleteIdentity(ctx, identity, now)
		if err == nil {
			s.finishCleanupOrPending(w, ctx, now)
			return
		}
		if errors.Is(err, sqlite.ErrMaintenanceLocked) {
			if _, runErr := s.store.RunCleanup(ctx, s.backupDir, now); runErr != nil && !errors.Is(runErr, sqlite.ErrNoActiveCleanupJob) {
				s.logger.Warn("delete: could not drain active cleanup job this attempt", "error", runErr.Error())
				writeJSON(w, http.StatusAccepted, deleteStatusResponseDTO{Status: "cleanup_pending"})
				return
			}
			// The blocking job is now done (or had already finished by the
			// time we looked, ErrNoActiveCleanupJob) — retry our own delete
			// now that the maintenance lock is clear.
			continue
		}
		// Any other error means the transaction failed before COMMIT (plan
		// 4.7: "внутренний сбой до COMMIT — 503 без identity/token в теле").
		s.logger.Error("delete identity failed before commit", "error", err.Error())
		writeError(w, http.StatusServiceUnavailable, "internal error")
		return
	}

	// Exhausted local retries: the maintenance lock is still held by a job
	// this handler could not drain within s.deleteRetryAttempts attempts.
	// This is still an honest "cleanup_pending" — the system genuinely is
	// in that state — never a bare 503; a client-side retry (or an
	// operator noticing many consecutive 202s, which is what would
	// eventually reveal a genuinely stuck job) will keep making progress.
	writeJSON(w, http.StatusAccepted, deleteStatusResponseDTO{Status: "cleanup_pending"})
}

// finishCleanupOrPending runs the post-commit cleanup phase once, right
// after this request's own DeleteIdentity call committed. Success (or
// ErrNoActiveCleanupJob, meaning some other caller — e.g. a concurrent
// request, or ResumePendingCleanup at startup — already finished it first)
// answers 204: the delete and its cleanup are both done. Any other failure
// leaves the already-committed delete untouched (RunCleanup never rolls
// back a delete) and answers 202 cleanup_pending, matching plan 4.7 exactly
// ("Ошибка этой фазы оставляет committed delete и переводит job в failed").
func (s *Server) finishCleanupOrPending(w http.ResponseWriter, ctx context.Context, now time.Time) {
	_, err := s.store.RunCleanup(ctx, s.backupDir, now)
	if err == nil || errors.Is(err, sqlite.ErrNoActiveCleanupJob) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.logger.Warn("post-commit cleanup failed; will resume on a later attempt", "error", err.Error())
	writeJSON(w, http.StatusAccepted, deleteStatusResponseDTO{Status: "cleanup_pending"})
}

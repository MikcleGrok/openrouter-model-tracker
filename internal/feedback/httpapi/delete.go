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
// distinguish 'had no rows' from 'had rows'" contract).
//
// The line this handler must never cross (review round 1, finding #2):
// plan §4.7 defines 202 strictly — "только после успешного COMMIT" — a
// promise that THIS request's own DeleteIdentity call actually committed a
// delete, with cleanup still finishing in the background. It is never "the
// system is busy, try later" spelled as 202. So the *only* way this
// handler reaches finishCleanupOrPending (which can answer 204 or 202) is
// the `err == nil` branch below, i.e. an actual successful DeleteIdentity
// call in this exact request — whether on the first attempt or after a
// drain-and-retry. Every other path (a drain that fails to clear the lock,
// or exhausting the retry budget without ever getting past
// ErrMaintenanceLocked) means this identity's own delete never committed
// in this request, and answers 503 instead, via the same writeServiceError
// shape already used elsewhere for ErrMaintenanceLocked.
func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	ctx := r.Context()
	now := s.now()

	for attempt := 0; attempt < s.deleteRetryAttempts; attempt++ {
		_, _, err := s.store.DeleteIdentity(ctx, identity, now)
		if err == nil {
			// This request's own delete just committed — only from this
			// point on may the response be 204/202 (plan 4.7).
			s.finishCleanupOrPending(w, ctx, now)
			return
		}
		if errors.Is(err, sqlite.ErrMaintenanceLocked) {
			if _, runErr := s.store.RunCleanup(ctx, s.backupDir, now); runErr != nil && !errors.Is(runErr, sqlite.ErrNoActiveCleanupJob) {
				// The blocking job could not be drained this attempt, and
				// this identity's own DeleteIdentity call above never even
				// started (it was rejected before any transaction began) —
				// nothing committed for this identity in this request, so
				// this is a plain pre-COMMIT failure, not "cleanup_pending".
				s.logger.Error("delete: could not drain active cleanup job; this identity's own delete was never attempted", "error", runErr.Error())
				s.writeServiceError(w, sqlite.ErrMaintenanceLocked)
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

	// Exhausted local retries without this identity's own DeleteIdentity
	// call ever succeeding (some other job keeps re-taking the maintenance
	// lock faster than this handler can drain it): this identity's delete
	// never committed in this request, so plan 4.7's 202 — a promise that
	// it did — does not apply here. 503, the same as any other pre-COMMIT
	// failure; a client retry (or an operator noticing repeated 503s,
	// which is what would eventually reveal a genuinely stuck job) is what
	// makes further progress, not a misleading "cleanup_pending".
	s.logger.Error("delete: exhausted retry attempts without this identity's own delete ever committing")
	s.writeServiceError(w, sqlite.ErrMaintenanceLocked)
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

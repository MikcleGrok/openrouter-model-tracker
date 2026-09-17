package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

func TestDelete_NoIdentityYet_Returns204(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE with no prior identity: status = %d, want 204", resp.StatusCode)
	}
}

func TestDelete_ExistingIdentity_DeletesDataAndReturns204(t *testing.T) {
	env := newTestEnv(t, nil)
	env.seedFeedback(t, identityA, "acme/model-1", 4, nil, env.now)

	resp := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE existing identity: status = %d, want 204", resp.StatusCode)
	}

	getResp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	got := decodeJSON[ownFeedbackResponseDTO](t, getResp)
	if got.OwnFeedback != nil {
		t.Errorf("own_feedback after delete = %+v, want nil", got.OwnFeedback)
	}
}

func TestDelete_RequiresUserAuth(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodDelete, "/v1/me/feedback", map[string]string{}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("DELETE with no auth: status = %d, want 401", resp.StatusCode)
	}
}

// TestDelete_SecondDeleteWhileCleanupPending_SafelyContinuesAndReturns204
// is the case the task's own brief called out by name: a repeated DELETE
// arriving while a privacy_cleanup_jobs row from the first call's own
// DeleteIdentity is still active (its post-commit RunCleanup deliberately
// not yet run) must not just become a bare 503 — it must safely finish the
// same cleanup and answer 204 or 202, never crash and never lie about
// nothing having happened.
func TestDelete_SecondDeleteWhileCleanupPending_SafelyContinuesAndReturns204(t *testing.T) {
	env := newTestEnv(t, nil)
	env.seedFeedback(t, identityA, "acme/model-1", 4, nil, env.now)

	// Manually create the committed delete + active job, WITHOUT running
	// the post-commit cleanup — reproducing exactly the state a crash
	// between COMMIT and RunCleanup (or a first HTTP request that never
	// got to call RunCleanup) would leave behind.
	if _, _, err := env.store.DeleteIdentity(context.Background(), feedback.IdentityID(identityA), env.now); err != nil {
		t.Fatalf("seed DeleteIdentity: %v", err)
	}
	if job, found, err := env.store.ActiveCleanupJob(context.Background()); err != nil || !found {
		t.Fatalf("expected an active cleanup job after DeleteIdentity, got found=%v err=%v job=%+v", found, err, job)
	}

	// The HTTP DELETE now arrives on top of that already-active job.
	resp := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		// Drained the job and confirmed/re-did the delete: fully done.
	case http.StatusAccepted:
		got := decodeJSON[deleteStatusResponseDTO](t, resp)
		if got.Status != "cleanup_pending" {
			t.Errorf("202 body = %+v, want status=cleanup_pending", got)
		}
	default:
		t.Fatalf("second DELETE while cleanup pending: status = %d, want 204 or 202 (never a bare 503)", resp.StatusCode)
	}

	// Either way, the maintenance lock must have cleared by the time this
	// handler returns (RunCleanup succeeds trivially against a real,
	// writable temp backup dir), so a normal write works again immediately
	// afterward.
	putResp := env.do(t, http.MethodPut, "/v1/models/acme/model-2/feedback", userHeaders(identityB), bytes.NewBufferString(`{"overall":3,"skills":[],"review":""}`))
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Errorf("write after resolved cleanup: status = %d, want 200", putResp.StatusCode)
	}
}

// TestDelete_DrainFailureLeavesDataUntouched_Returns503 is review round 1
// finding #2's first scenario: another identity's cleanup job is active,
// and this request's attempt to drain it (RunCleanup) fails outright (here,
// a broken backup-dir path standing in for "bad --backup-dir, full disk, a
// permanently stuck job"). This identity's own DeleteIdentity call never
// even ran, so plan §4.7's 202 — a promise that THIS delete already
// committed — must not fire; the correct answer is 503, and identity A's
// data must be provably untouched.
func TestDelete_DrainFailureLeavesDataUntouched_Returns503(t *testing.T) {
	// A regular file where the backup dir's parent segment should be
	// forces every RunCleanup attempt to fail at Backup's own
	// os.MkdirAll — deterministic and reproducible, no real disk-full or
	// permission setup needed.
	blockerFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockerFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	brokenBackupDir := filepath.Join(blockerFile, "backups")

	env := newTestEnv(t, func(cfg *Config) { cfg.BackupDir = brokenBackupDir })
	env.seedFeedback(t, identityA, "acme/model-1", 4, nil, env.now)

	// Identity B's delete already committed and left an active job that
	// identity A's own DELETE must drain before it can even attempt its
	// own delete — and draining is exactly what the broken backup dir
	// prevents.
	if _, _, err := env.store.DeleteIdentity(context.Background(), feedback.IdentityID(identityB), env.now); err != nil {
		t.Fatalf("seed DeleteIdentity (identity B): %v", err)
	}

	resp := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("DELETE blocked by an undrainable job: status = %d, want 503 (never 202 — this identity's own delete never committed)", resp.StatusCode)
	}

	getResp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	got := decodeJSON[ownFeedbackResponseDTO](t, getResp)
	if got.OwnFeedback == nil {
		t.Error("identity A's own_feedback is gone after a 503, want untouched — their own delete never committed")
	}
}

// TestDelete_RetryExhaustion_Returns503 is review round 1 finding #2's
// second scenario: the bounded retry loop exhausts itself without this
// identity's own DeleteIdentity call ever succeeding. Configuring
// DeleteRetryAttempts=1 reproduces this deterministically: the single
// allowed attempt hits ErrMaintenanceLocked, successfully drains the
// blocking job, and then the loop's own bound (attempt < 1) is already
// exhausted before a second attempt can retry this identity's actual
// delete — exactly the "retry budget exhausted before this identity's own
// delete committed" case, without needing to simulate a real concurrent
// process racing to re-take the lock.
func TestDelete_RetryExhaustion_Returns503(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) { cfg.DeleteRetryAttempts = 1 })
	env.seedFeedback(t, identityA, "acme/model-1", 4, nil, env.now)

	if _, _, err := env.store.DeleteIdentity(context.Background(), feedback.IdentityID(identityB), env.now); err != nil {
		t.Fatalf("seed DeleteIdentity (identity B): %v", err)
	}

	resp := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("DELETE exhausting its retry budget: status = %d, want 503 (never 202 — this identity's own delete never committed in this request)", resp.StatusCode)
	}

	getResp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	got := decodeJSON[ownFeedbackResponseDTO](t, getResp)
	if got.OwnFeedback == nil {
		t.Error("identity A's own_feedback is gone after a 503, want untouched — their delete never committed within the retry budget")
	}
}

// TestDelete_IsIdempotent covers plan 4.7's "идемпотентный DELETE": calling
// it twice in the ordinary (no artificially-injected pending job) case
// always answers 204 both times.
func TestDelete_IsIdempotent(t *testing.T) {
	env := newTestEnv(t, nil)
	env.seedFeedback(t, identityA, "acme/model-1", 4, nil, env.now)

	first := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	first.Body.Close()
	if first.StatusCode != http.StatusNoContent {
		t.Fatalf("first DELETE: status = %d, want 204", first.StatusCode)
	}

	second := env.do(t, http.MethodDelete, "/v1/me/feedback", userHeaders(identityA), nil)
	second.Body.Close()
	if second.StatusCode != http.StatusNoContent {
		t.Errorf("second DELETE (already gone): status = %d, want 204", second.StatusCode)
	}
}

package httpapi

import (
	"bytes"
	"context"
	"net/http"
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

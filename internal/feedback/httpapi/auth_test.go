package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestAuth_UserScope_ValidCredentialsReachHandler proves the ordinary
// success path so the failure-path tests below are meaningful contrasts.
func TestAuth_UserScope_ValidCredentialsReachHandler(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET .../me with valid user auth: status = %d, want 200", resp.StatusCode)
	}
}

func TestAuth_UserScope_MissingBearerToken(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{"X-Identity-Id": identityA}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing bearer token: status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_UserScope_WrongBearerToken(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{"Authorization": "Bearer not-the-real-token", "X-Identity-Id": identityA}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong bearer token: status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_UserScope_MissingIdentity(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{"Authorization": "Bearer " + testUserToken}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing X-Identity-Id: status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_UserScope_MalformedIdentityRejected(t *testing.T) {
	env := newTestEnv(t, nil)
	cases := []string{
		"short",
		"UPPERCASE0000000000000000000000000000000000000000000000000000",
		identityA + "x", // one character too long
		identityA[:63],  // one character too short
		"not-hex-" + identityA[8:],
	}
	for _, id := range cases {
		headers := map[string]string{"Authorization": "Bearer " + testUserToken, "X-Identity-Id": id}
		resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", headers, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("X-Identity-Id=%q: status = %d, want 401", id, resp.StatusCode)
		}
	}
}

// TestAuth_ConsumerTokenOnUserEndpoint_Gets403 proves the consumer
// credential cannot reach a user-scoped endpoint, even though it is a
// perfectly valid *known* secret — just for the other scope.
func TestAuth_ConsumerTokenOnUserEndpoint_Gets403(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{"Authorization": "Bearer " + testConsumerToken, "X-Identity-Id": identityA}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("consumer token on user endpoint: status = %d, want 403", resp.StatusCode)
	}
}

// TestAuth_ConsumerScope_ValidCredentialReachesHandler is the consumer-side
// success path.
func TestAuth_ConsumerScope_ValidCredentialReachesHandler(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/signal", consumerHeaders(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET .../signal with valid consumer auth: status = %d, want 200", resp.StatusCode)
	}
}

// TestAuth_UserTokenOnConsumerEndpoint_Gets403IsTheHeadlineRequirement is
// exactly what the task brief calls out as the security-critical case: a
// user/TUI token must never reach the trusted-consumer-only signal
// endpoint, even though it is a valid, currently-live secret.
func TestAuth_UserTokenOnConsumerEndpoint_Gets403IsTheHeadlineRequirement(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{"Authorization": "Bearer " + testUserToken}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/signal", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("user token on consumer endpoint: status = %d, want 403", resp.StatusCode)
	}
}

// TestAuth_ConsumerEndpoint_ClientClaimedScopeHeadersAreIgnored proves that
// no client-supplied header can manufacture consumer authority: a request
// carrying every plausible "I am the trusted consumer" header, but no
// valid consumer bearer token, is still rejected.
func TestAuth_ConsumerEndpoint_ClientClaimedScopeHeadersAreIgnored(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{
		"X-Scope":       "feedback:signal:read",
		"X-Audience":    "assistant-runtime",
		"Authorization": "Bearer garbage-not-a-real-token",
	}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/signal", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("claimed scope/audience headers with no real token: status = %d, want 401", resp.StatusCode)
	}
}

// TestAuth_ConsumerEndpoint_ClientClaimedScopeCannotUpgradeUserToken proves
// the same thing from the other direction: a *user* token plus claimed
// consumer scope/audience headers still only gets 403, never through.
func TestAuth_ConsumerEndpoint_ClientClaimedScopeCannotUpgradeUserToken(t *testing.T) {
	env := newTestEnv(t, nil)
	headers := map[string]string{
		"X-Scope":       "feedback:signal:read",
		"X-Audience":    "assistant-runtime",
		"Authorization": "Bearer " + testUserToken,
	}
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/signal", headers, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("user token + claimed consumer headers: status = %d, want 403 (claims must never upgrade scope)", resp.StatusCode)
	}
}

// TestAuth_Impersonation_SameTokenDifferentIdentityIsolatesData documents
// (and proves) the MVP's known, accepted trust model (plan 6.1): anyone
// holding the one shared user token can act as any identity simply by
// choosing X-Identity-Id — this is a local/trusted-OS-user limitation, not
// a bug — but two different identities using that same token must still
// see fully isolated data from each other.
func TestAuth_Impersonation_SameTokenDifferentIdentityIsolatesData(t *testing.T) {
	env := newTestEnv(t, nil)
	body := bytes.NewBufferString(`{"overall":5,"skills":[],"review":"from A"}`)
	putResp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), body)
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT as identity A: status = %d, want 200", putResp.StatusCode)
	}

	getResp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityB), nil)
	defer getResp.Body.Close()
	var got ownFeedbackResponseDTO
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OwnFeedback != nil {
		t.Errorf("identity B's own_feedback = %+v, want nil (A's write must not be visible to B)", got.OwnFeedback)
	}
}

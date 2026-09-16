package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// TestContract_SignalResponse_ExactFieldNames decodes the raw JSON into a
// generic map (never through signalResponseDTO's own struct tags) so a typo
// in this package's own tags cannot silently "match itself" — it checks the
// literal field names plan 4.6's worked example specifies, since Task 7's
// consumer adapter depends on this shape byte-for-byte.
func TestContract_SignalResponse_ExactFieldNames(t *testing.T) {
	env := newTestEnv(t, nil)
	const modelKey = feedback.ModelKey("acme/contract-model")
	for i := 0; i < 20; i++ {
		env.seedFeedback(t, feedback.IdentityID(paddedIdentity(i)), modelKey, 4, []feedback.SkillRating{{Key: "reasoning", Rating: 4}}, env.now)
	}

	resp := env.do(t, http.MethodGet, "/v1/models/acme/contract-model/feedback/signal", consumerHeaders(), nil)
	defer resp.Body.Close()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode raw JSON: %v", err)
	}

	requireKey(t, raw, "model_key")
	requireKey(t, raw, "signal_scope")
	requireKey(t, raw, "position")
	requireKey(t, raw, "overall")
	requireKey(t, raw, "skills")
	requireKey(t, raw, "status")
	requireKey(t, raw, "freshness")
	requireKey(t, raw, "schema_version")
	requireKey(t, raw, "policy_version")

	if raw["signal_scope"] != "community" {
		t.Errorf(`signal_scope = %v, want "community"`, raw["signal_scope"])
	}

	position, ok := raw["position"].(map[string]any)
	if !ok {
		t.Fatalf("position is not an object: %#v", raw["position"])
	}
	requireKey(t, position, "community_position")

	overall, ok := raw["overall"].(map[string]any)
	if !ok {
		t.Fatalf("overall is not an object: %#v", raw["overall"])
	}
	for _, key := range []string{"value", "status", "confidence", "sample_count"} {
		requireKey(t, overall, key)
	}

	skills, ok := raw["skills"].(map[string]any)
	if !ok {
		t.Fatalf("skills is not an object (map keyed by skill): %#v", raw["skills"])
	}
	reasoning, ok := skills["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf(`skills["reasoning"] is not an object: %#v`, skills["reasoning"])
	}
	for _, key := range []string{"value", "status", "confidence", "sample_count"} {
		requireKey(t, reasoning, key)
	}

	freshness, ok := raw["freshness"].(map[string]any)
	if !ok {
		t.Fatalf("freshness is not an object: %#v", raw["freshness"])
	}
	for _, key := range []string{"as_of", "computed_at", "ttl_seconds", "stale"} {
		requireKey(t, freshness, key)
	}

	if raw["schema_version"] != "feedback-signal.v1" {
		t.Errorf("schema_version = %v, want feedback-signal.v1", raw["schema_version"])
	}
	if raw["policy_version"] != "feedback-signal-policy.v1" {
		t.Errorf("policy_version = %v, want feedback-signal-policy.v1", raw["policy_version"])
	}

	// Never a personal/identity-scoped field of any kind.
	for _, forbidden := range []string{"mine", "others", "personal_position", "identity_id", "identity", "token", "review"} {
		if _, present := raw[forbidden]; present {
			t.Errorf("signal response unexpectedly contains top-level field %q", forbidden)
		}
	}
}

func requireKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("missing required JSON field %q in %#v", key, m)
	}
}

// TestContract_SummaryResponse_ExactFieldNames does the same raw-JSON check
// for GET .../feedback/summary's response shape (plan 4.4/contract §5).
func TestContract_SummaryResponse_ExactFieldNames(t *testing.T) {
	env := newTestEnv(t, nil)
	env.seedFeedback(t, identityA, "acme/model-1", 4, nil, env.now)

	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/summary", userHeaders(identityA), nil)
	defer resp.Body.Close()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode raw JSON: %v", err)
	}

	for _, key := range []string{"model_key", "mine", "community", "base_position", "personal_position", "community_position"} {
		requireKey(t, raw, key)
	}
	if _, present := raw["others"]; present {
		t.Errorf(`"others" present without ?others=true: %#v`, raw["others"])
	}

	mine, ok := raw["mine"].(map[string]any)
	if !ok {
		t.Fatalf("mine is not an object: %#v", raw["mine"])
	}
	for _, key := range []string{"overall", "skills", "review", "created_at", "updated_at"} {
		requireKey(t, mine, key)
	}
	if _, present := mine["identity_id"]; present {
		t.Error(`mine unexpectedly contains "identity_id"`)
	}

	community, ok := raw["community"].(map[string]any)
	if !ok {
		t.Fatalf("community is not an object: %#v", raw["community"])
	}
	for _, key := range []string{"count", "average", "distribution", "skills", "computed_at"} {
		requireKey(t, community, key)
	}
}

// TestContract_OwnFeedbackResponse_ExactFieldNames covers GET
// .../feedback/me's response shape (plan 4.3).
func TestContract_OwnFeedbackResponse_ExactFieldNames(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	defer resp.Body.Close()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode raw JSON: %v", err)
	}
	requireKey(t, raw, "model_key")
	requireKey(t, raw, "own_feedback")
	if raw["own_feedback"] != nil {
		t.Errorf("own_feedback = %v, want null (no rating yet)", raw["own_feedback"])
	}
}

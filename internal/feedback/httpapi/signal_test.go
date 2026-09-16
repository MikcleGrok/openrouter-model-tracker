package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

func TestSignal_NoDataAtAll_Unavailable(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodGet, "/v1/models/acme/never-rated/feedback/signal", consumerHeaders(), nil)
	got := decodeJSON[signalResponseDTO](t, resp)

	if got.Status != "unavailable" {
		t.Errorf("status = %q, want unavailable", got.Status)
	}
	if got.Overall.Status != "unavailable" || got.Overall.Value != nil {
		t.Errorf("overall = %+v, want unavailable/nil value", got.Overall)
	}
	for _, key := range feedback.AllowedSkills() {
		dim, ok := got.Skills[key]
		if !ok {
			t.Fatalf("skills missing key %q", key)
		}
		if dim.Status != "unavailable" || dim.Value != nil {
			t.Errorf("skills[%q] = %+v, want unavailable/nil value", key, dim)
		}
	}
	if got.Freshness.AsOf != nil {
		t.Errorf("freshness.as_of = %v, want nil", got.Freshness.AsOf)
	}
	if got.SchemaVersion != "feedback-signal.v1" || got.PolicyVersion != "feedback-signal-policy.v1" {
		t.Errorf("versions = %q/%q, want feedback-signal.v1/feedback-signal-policy.v1", got.SchemaVersion, got.PolicyVersion)
	}
	if got.SignalScope != "community" {
		t.Errorf("signal_scope = %q, want community", got.SignalScope)
	}
	if got.Position.CommunityPosition != nil {
		t.Errorf("position.community_position = %v, want nil", *got.Position.CommunityPosition)
	}
}

// TestSignal_OverallDimensionBoundaries checks the deterministic policy
// (contract §6, plan 4.6) at the required counts 1/4/5/19/20 for the
// OVERALL dimension. count=0 is covered separately by
// TestSignal_NoDataAtAll_Unavailable, since a whole-model zero count is
// "unavailable" (no signal at all), not "insufficient" (some data, below
// threshold) — see handlers_signal.go's own doc comment for why those are
// different states here.
func TestSignal_OverallDimensionBoundaries(t *testing.T) {
	cases := []struct {
		count          int
		wantStatus     string
		wantConfidence string
		wantValue      bool
	}{
		{count: 1, wantStatus: "insufficient", wantConfidence: "insufficient", wantValue: false},
		{count: 4, wantStatus: "insufficient", wantConfidence: "insufficient", wantValue: false},
		{count: 5, wantStatus: "provisional", wantConfidence: "provisional", wantValue: true},
		{count: 19, wantStatus: "provisional", wantConfidence: "provisional", wantValue: true},
		{count: 20, wantStatus: "established", wantConfidence: "established", wantValue: true},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.count), func(t *testing.T) {
			env := newTestEnv(t, nil)
			const modelKey = feedback.ModelKey("acme/boundary-model")
			for i := 0; i < tc.count; i++ {
				identity := feedback.IdentityID(paddedIdentity(i))
				env.seedFeedback(t, identity, modelKey, 4, nil, env.now)
			}

			resp := env.do(t, http.MethodGet, "/v1/models/acme/boundary-model/feedback/signal", consumerHeaders(), nil)
			got := decodeJSON[signalResponseDTO](t, resp)

			if got.Status != "usable" {
				t.Fatalf("top-level status = %q, want usable", got.Status)
			}
			if got.Overall.Status != tc.wantStatus {
				t.Errorf("overall.status = %q, want %q", got.Overall.Status, tc.wantStatus)
			}
			if got.Overall.Confidence == nil || *got.Overall.Confidence != tc.wantConfidence {
				t.Errorf("overall.confidence = %v, want %q", got.Overall.Confidence, tc.wantConfidence)
			}
			if got.Overall.SampleCount != tc.count {
				t.Errorf("overall.sample_count = %d, want %d", got.Overall.SampleCount, tc.count)
			}
			if tc.wantValue {
				if got.Overall.Value == nil || *got.Overall.Value != 4 {
					t.Errorf("overall.value = %v, want 4", got.Overall.Value)
				}
			} else if got.Overall.Value != nil {
				t.Errorf("overall.value = %v, want nil", *got.Overall.Value)
			}
		})
	}
}

// TestSignal_SkillDimensionIndependentOfOverall proves the "mixed states"
// requirement: an established overall does not make an insufficient/stale
// skill usable, and each skill is judged purely on its own count.
func TestSignal_SkillDimensionIndependentOfOverall(t *testing.T) {
	env := newTestEnv(t, nil)
	const modelKey = feedback.ModelKey("acme/mixed-model")
	for i := 0; i < 20; i++ {
		identity := feedback.IdentityID(paddedIdentity(i))
		var skills []feedback.SkillRating
		if i < 12 {
			skills = append(skills, feedback.SkillRating{Key: "reasoning", Rating: 4})
		}
		if i < 3 {
			skills = append(skills, feedback.SkillRating{Key: "coding", Rating: 5})
		}
		env.seedFeedback(t, identity, modelKey, 4, skills, env.now)
	}

	resp := env.do(t, http.MethodGet, "/v1/models/acme/mixed-model/feedback/signal", consumerHeaders(), nil)
	got := decodeJSON[signalResponseDTO](t, resp)

	if got.Overall.Status != "established" || got.Overall.SampleCount != 20 {
		t.Fatalf("overall = %+v, want established/20", got.Overall)
	}
	reasoning := got.Skills["reasoning"]
	if reasoning.Status != "provisional" || reasoning.SampleCount != 12 {
		t.Errorf("skills[reasoning] = %+v, want provisional/12", reasoning)
	}
	coding := got.Skills["coding"]
	if coding.Status != "insufficient" || coding.SampleCount != 3 || coding.Value != nil {
		t.Errorf("skills[coding] = %+v, want insufficient/3/nil-value", coding)
	}
	longContext := got.Skills["long_context"]
	if longContext.Status != "insufficient" || longContext.SampleCount != 0 {
		t.Errorf("skills[long_context] = %+v, want insufficient/0 (never rated)", longContext)
	}
}

// TestSignal_Freshness_ExactlyTTLIsFresh_OneNanosecondOverIsStale is the
// exact boundary contract §6/11.2 calls out by name, driven with a fake
// clock so it never depends on real wall-clock timing.
func TestSignal_Freshness_ExactlyTTLIsFresh_OneNanosecondOverIsStale(t *testing.T) {
	env := newTestEnv(t, nil)
	const modelKey = feedback.ModelKey("acme/freshness-model")
	seededAt := env.now
	for i := 0; i < 20; i++ {
		env.seedFeedback(t, feedback.IdentityID(paddedIdentity(i)), modelKey, 4, nil, seededAt)
	}

	// Exactly 24h later: fresh.
	env.now = seededAt.Add(24 * time.Hour)
	freshResp := env.do(t, http.MethodGet, "/v1/models/acme/freshness-model/feedback/signal", consumerHeaders(), nil)
	fresh := decodeJSON[signalResponseDTO](t, freshResp)
	if fresh.Status != "usable" || fresh.Freshness.Stale {
		t.Errorf("at age=24h: status=%q stale=%v, want usable/false", fresh.Status, fresh.Freshness.Stale)
	}
	if fresh.Overall.Status != "established" {
		t.Errorf("at age=24h: overall.status = %q, want established", fresh.Overall.Status)
	}
	if fresh.Freshness.AsOf == nil || !fresh.Freshness.AsOf.Equal(seededAt) {
		t.Errorf("freshness.as_of = %v, want %v (MAX(updated_at), not GET time)", fresh.Freshness.AsOf, seededAt)
	}
	if !fresh.Freshness.ComputedAt.Equal(env.now) {
		t.Errorf("freshness.computed_at = %v, want %v", fresh.Freshness.ComputedAt, env.now)
	}
	if fresh.Freshness.TTLSeconds != 86400 {
		t.Errorf("freshness.ttl_seconds = %d, want 86400", fresh.Freshness.TTLSeconds)
	}

	// One nanosecond further: stale.
	env.now = seededAt.Add(24*time.Hour + time.Nanosecond)
	staleResp := env.do(t, http.MethodGet, "/v1/models/acme/freshness-model/feedback/signal", consumerHeaders(), nil)
	stale := decodeJSON[signalResponseDTO](t, staleResp)
	if stale.Status != "stale" || !stale.Freshness.Stale {
		t.Fatalf("at age=24h+1ns: status=%q stale=%v, want stale/true", stale.Status, stale.Freshness.Stale)
	}
	if stale.Overall.Value != nil || stale.Overall.Confidence != nil {
		t.Errorf("stale overall = %+v, want nil value and nil confidence", stale.Overall)
	}
	if stale.Overall.SampleCount != 20 {
		t.Errorf("stale overall.sample_count = %d, want 20 (count still reported, only value/confidence null)", stale.Overall.SampleCount)
	}
	for _, key := range feedback.AllowedSkills() {
		dim := stale.Skills[key]
		if dim.Status != "stale" || dim.Value != nil || dim.Confidence != nil {
			t.Errorf("stale skills[%q] = %+v, want stale/nil/nil", key, dim)
		}
	}
}

// TestSignal_CommunityPositionEligibility mirrors the n>=5 eligibility
// boundary specifically for the signal endpoint's position.community_position
// field (contract §3, plan 4.6).
func TestSignal_CommunityPositionEligibility(t *testing.T) {
	for _, count := range []int{4, 5} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			env := newTestEnv(t, nil)
			const modelKey = feedback.ModelKey("acme/position-model")
			for i := 0; i < count; i++ {
				env.seedFeedback(t, feedback.IdentityID(paddedIdentity(i)), modelKey, 4, nil, env.now)
			}
			resp := env.do(t, http.MethodGet, "/v1/models/acme/position-model/feedback/signal", consumerHeaders(), nil)
			got := decodeJSON[signalResponseDTO](t, resp)

			if count < 5 {
				if got.Position.CommunityPosition != nil {
					t.Errorf("count=%d: community_position = %v, want nil", count, *got.Position.CommunityPosition)
				}
			} else if got.Position.CommunityPosition == nil || *got.Position.CommunityPosition != 1 {
				t.Errorf("count=%d: community_position = %v, want 1", count, got.Position.CommunityPosition)
			}
		})
	}
}

// TestSignal_NeverLeaksReviewIdentityOrToken proves an injection-like
// review comment never surfaces anywhere in the signal response, and
// neither does identity or the tokens.
func TestSignal_NeverLeaksReviewIdentityOrToken(t *testing.T) {
	env := newTestEnv(t, nil)
	input, err := feedback.NewFeedbackInput("acme/injection-model", 4, nil, "\x1b]0;pwned\x07 <script>alert(1)</script>")
	if err != nil {
		t.Fatalf("NewFeedbackInput: %v", err)
	}
	if _, err := env.store.UpsertFeedback(t.Context(), identityA, input, env.now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	resp := env.do(t, http.MethodGet, "/v1/models/acme/injection-model/feedback/signal", consumerHeaders(), nil)
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	text := body.String()

	for _, forbidden := range []string{"pwned", "<script>", "alert(1)", identityA, testUserToken, testConsumerToken} {
		if strings.Contains(text, forbidden) {
			t.Errorf("signal response leaks %q: %s", forbidden, text)
		}
	}
}

// TestSignal_InvalidModelKeyRejected400 confirms model_key validation
// (plan 10.1) still applies on the consumer route — a key containing ".."
// (contract §7's own defense-in-depth rule against anything path-
// traversal-shaped) is rejected by this layer's own NormalizeModelKey,
// not merely redirected away by net/http's unrelated path cleaning.
func TestSignal_InvalidModelKeyRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodGet, "/v1/models/acme/mod..el/feedback/signal", consumerHeaders(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("model_key containing \"..\": status = %d, want 400", resp.StatusCode)
	}
}

// paddedIdentity returns a syntactically valid (64 lowercase hex chars)
// identity distinct for each n, used only to seed distinct rows in a
// matrix/boundary test — never asserted against as a "real" identity.
func paddedIdentity(n int) string {
	return fmt.Sprintf("%064d", n)
}

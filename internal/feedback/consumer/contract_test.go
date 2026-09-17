package consumer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/httpapi"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

// This file proves HTTPReferenceSignalProvider against the REAL, shipped
// signal endpoint (internal/feedback/httpapi's handleGetSignal, Task 4) —
// not merely this package's own understanding of its shape — by wiring a
// real *sqlite.Store + real httpapi.Server behind an httptest.Server, the
// same pattern httpapi/testhelpers_test.go itself uses. Production code in
// this package never imports internal/feedback or internal/feedback/httpapi
// (doc.go); only this test file does, deliberately, to close the loop
// against the actual contract instead of trusting a hand-duplicated DTO.

const (
	testUserToken     = "user-secret-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testConsumerToken = "consumer-secret-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type realServerEnv struct {
	store    *sqlite.Store
	http     *httptest.Server
	provider *HTTPReferenceSignalProvider
	now      time.Time
}

func newRealServerEnv(t *testing.T) *realServerEnv {
	t.Helper()
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "feedback.db")
	store, err := sqlite.Open(ctx, dbPath, sqlite.Config{})
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Migrate(ctx, io.Discard); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	env := &realServerEnv{store: store, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}

	server, err := httpapi.New(store, httpapi.Config{
		UserToken:     []byte(testUserToken),
		ConsumerToken: []byte(testConsumerToken),
		BackupDir:     filepath.Join(t.TempDir(), "backups"),
		Now:           func() time.Time { return env.now },
	})
	if err != nil {
		t.Fatalf("httpapi.New: %v", err)
	}
	env.http = httptest.NewServer(server.Handler())
	t.Cleanup(env.http.Close)

	provider, err := NewHTTPReferenceSignalProvider(HTTPReferenceSignalProviderConfig{
		BaseURL: env.http.URL,
		Token:   testConsumerToken,
	})
	if err != nil {
		t.Fatalf("NewHTTPReferenceSignalProvider: %v", err)
	}
	env.provider = provider

	return env
}

func (env *realServerEnv) seed(t *testing.T, identity feedback.IdentityID, modelKey feedback.ModelKey, overall int, skills []feedback.SkillRating, at time.Time) {
	t.Helper()
	input, err := feedback.NewFeedbackInput(string(modelKey), overall, skills, "")
	if err != nil {
		t.Fatalf("NewFeedbackInput: %v", err)
	}
	if _, err := env.store.UpsertFeedback(context.Background(), identity, input, at); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
}

// paddedIdentity returns a syntactically valid (64 lowercase hex chars)
// identity distinct for each n — matching
// httpapi/signal_test.go's own paddedIdentity, duplicated here since that
// helper is unexported to its own package.
func paddedIdentity(n int) feedback.IdentityID {
	return feedback.IdentityID(fmt.Sprintf("%064d", n))
}

// TestGetSignal_RealServer_BoundaryCounts drives the real signal endpoint
// at the required boundary counts (0/1/4/5/19/20 — plan 11.2) and checks
// HTTPReferenceSignalProvider maps every field correctly.
func TestGetSignal_RealServer_BoundaryCounts(t *testing.T) {
	cases := []struct {
		count          int
		wantTopStatus  SignalStatus
		wantStatus     DimensionStatus
		wantConfidence Confidence
		wantValue      bool
	}{
		{0, SignalUnavailable, DimensionInsufficient, ConfidenceInsufficient, false},
		{1, SignalUsable, DimensionInsufficient, ConfidenceInsufficient, false},
		{4, SignalUsable, DimensionInsufficient, ConfidenceInsufficient, false},
		{5, SignalUsable, DimensionProvisional, ConfidenceProvisional, true},
		{19, SignalUsable, DimensionProvisional, ConfidenceProvisional, true},
		{20, SignalUsable, DimensionEstablished, ConfidenceEstablished, true},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.count), func(t *testing.T) {
			env := newRealServerEnv(t)
			const modelKey = feedback.ModelKey("acme/boundary-model")
			for i := 0; i < tc.count; i++ {
				env.seed(t, paddedIdentity(i), modelKey, 4, nil, env.now)
			}

			signal, err := env.provider.GetSignal(context.Background(), string(modelKey))
			if err != nil {
				t.Fatalf("GetSignal: %v", err)
			}
			if signal.Status != tc.wantTopStatus {
				t.Errorf("status = %q, want %q", signal.Status, tc.wantTopStatus)
			}
			if signal.Overall.Status != tc.wantStatus {
				t.Errorf("overall.status = %q, want %q", signal.Overall.Status, tc.wantStatus)
			}
			if signal.Overall.Confidence == nil || *signal.Overall.Confidence != tc.wantConfidence {
				t.Errorf("overall.confidence = %v, want %q", signal.Overall.Confidence, tc.wantConfidence)
			}
			if signal.Overall.SampleCount != tc.count {
				t.Errorf("overall.sample_count = %d, want %d", signal.Overall.SampleCount, tc.count)
			}
			if tc.wantValue {
				if signal.Overall.Value == nil || *signal.Overall.Value != 4 {
					t.Errorf("overall.value = %v, want 4", signal.Overall.Value)
				}
			} else if signal.Overall.Value != nil {
				t.Errorf("overall.value = %v, want nil", *signal.Overall.Value)
			}
			if signal.SchemaVersion != SupportedSchemaVersion {
				t.Errorf("schema_version = %q, want %q", signal.SchemaVersion, SupportedSchemaVersion)
			}
			if signal.SignalScope != "community" {
				t.Errorf("signal_scope = %q, want community", signal.SignalScope)
			}
		})
	}
}

// TestGetSignal_RealServer_FreshnessBoundary is the exact 24h/24h+1ns
// boundary contract §6/11.2 calls out by name, driven with the real
// server's own fake clock.
func TestGetSignal_RealServer_FreshnessBoundary(t *testing.T) {
	env := newRealServerEnv(t)
	const modelKey = feedback.ModelKey("acme/freshness-model")
	seededAt := env.now
	for i := 0; i < 20; i++ {
		env.seed(t, paddedIdentity(i), modelKey, 4, nil, seededAt)
	}

	env.now = seededAt.Add(24 * time.Hour)
	fresh, err := env.provider.GetSignal(context.Background(), string(modelKey))
	if err != nil {
		t.Fatalf("GetSignal (fresh): %v", err)
	}
	if fresh.Status != SignalUsable || fresh.Freshness.Stale {
		t.Errorf("at age=24h: status=%q stale=%v, want usable/false", fresh.Status, fresh.Freshness.Stale)
	}
	if fresh.Freshness.AsOf == nil || !fresh.Freshness.AsOf.Equal(seededAt) {
		t.Errorf("freshness.as_of = %v, want %v (MAX(updated_at), not GET time)", fresh.Freshness.AsOf, seededAt)
	}
	if fresh.Freshness.TTLSeconds != 86400 {
		t.Errorf("freshness.ttl_seconds = %d, want 86400", fresh.Freshness.TTLSeconds)
	}

	env.now = seededAt.Add(24*time.Hour + time.Nanosecond)
	stale, err := env.provider.GetSignal(context.Background(), string(modelKey))
	if err != nil {
		t.Fatalf("GetSignal (stale): %v", err)
	}
	if stale.Status != SignalStale || !stale.Freshness.Stale {
		t.Fatalf("at age=24h+1ns: status=%q stale=%v, want stale/true", stale.Status, stale.Freshness.Stale)
	}
	if stale.Overall.Value != nil || stale.Overall.Confidence != nil {
		t.Errorf("stale overall = %+v, want nil value and nil confidence", stale.Overall)
	}
	if stale.Overall.Status != DimensionStale {
		t.Errorf("stale overall.status = %q, want stale", stale.Overall.Status)
	}
}

// TestGetSignal_RealServer_CommunityPositionEligibility mirrors the n>=5
// eligibility boundary for position.community_position (contract §3, plan
// 4.6).
func TestGetSignal_RealServer_CommunityPositionEligibility(t *testing.T) {
	for _, count := range []int{4, 5} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			env := newRealServerEnv(t)
			const modelKey = feedback.ModelKey("acme/position-model")
			for i := 0; i < count; i++ {
				env.seed(t, paddedIdentity(i), modelKey, 4, nil, env.now)
			}
			signal, err := env.provider.GetSignal(context.Background(), string(modelKey))
			if err != nil {
				t.Fatalf("GetSignal: %v", err)
			}
			if count < 5 {
				if signal.CommunityPosition != nil {
					t.Errorf("count=%d: community_position = %v, want nil", count, *signal.CommunityPosition)
				}
			} else if signal.CommunityPosition == nil || *signal.CommunityPosition != 1 {
				t.Errorf("count=%d: community_position = %v, want 1", count, signal.CommunityPosition)
			}
		})
	}
}

// TestGetSignal_RealServer_MixedSkillStates proves the "mixed states"
// requirement end-to-end: an established overall never makes an
// insufficient skill usable in the decoded FeedbackSignal, and a
// never-rated skill still decodes as an ordinary insufficient/0 dimension.
func TestGetSignal_RealServer_MixedSkillStates(t *testing.T) {
	env := newRealServerEnv(t)
	const modelKey = feedback.ModelKey("acme/mixed-model")
	for i := 0; i < 20; i++ {
		var skills []feedback.SkillRating
		if i < 12 {
			skills = append(skills, feedback.SkillRating{Key: "reasoning", Rating: 4})
		}
		if i < 3 {
			skills = append(skills, feedback.SkillRating{Key: "coding", Rating: 5})
		}
		env.seed(t, paddedIdentity(i), modelKey, 4, skills, env.now)
	}

	signal, err := env.provider.GetSignal(context.Background(), string(modelKey))
	if err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	if signal.Overall.Status != DimensionEstablished || signal.Overall.SampleCount != 20 {
		t.Fatalf("overall = %+v, want established/20", signal.Overall)
	}
	reasoning := signal.Skills["reasoning"]
	if reasoning.Status != DimensionProvisional || reasoning.SampleCount != 12 {
		t.Errorf("skills[reasoning] = %+v, want provisional/12", reasoning)
	}
	coding := signal.Skills["coding"]
	if coding.Status != DimensionInsufficient || coding.SampleCount != 3 || coding.Value != nil {
		t.Errorf("skills[coding] = %+v, want insufficient/3/nil-value", coding)
	}
	longContext := signal.Skills["long_context"]
	if longContext.Status != DimensionInsufficient || longContext.SampleCount != 0 {
		t.Errorf("skills[long_context] = %+v, want insufficient/0 (never rated)", longContext)
	}

	// Policy layer must read only the named skill's own status, never
	// "rescued" by the established overall (plan 4.6).
	if got := EvaluateSkill(signal, nil, "coding"); got.Mode != ModeBaseline || got.ReasonCode != ReasonInsufficientSample {
		t.Errorf("EvaluateSkill(coding) = %+v, want baseline/insufficient_sample", got)
	}
	if got := EvaluateOverall(signal, nil); got.Mode != ModeFeedback || got.Action != ActionRouting {
		t.Errorf("EvaluateOverall = %+v, want feedback/routing (overall is established)", got)
	}
}

// TestGetSignal_RealServer_NeverLeaksReviewIdentityOrToken proves an
// injection-like review comment never surfaces anywhere in the decoded
// FeedbackSignal.
func TestGetSignal_RealServer_NeverLeaksReviewIdentityOrToken(t *testing.T) {
	env := newRealServerEnv(t)
	input, err := feedback.NewFeedbackInput("acme/injection-model", 4, nil, "\x1b]0;pwned\x07 <script>alert(1)</script>")
	if err != nil {
		t.Fatalf("NewFeedbackInput: %v", err)
	}
	identity := paddedIdentity(0)
	if _, err := env.store.UpsertFeedback(context.Background(), identity, input, env.now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	signal, err := env.provider.GetSignal(context.Background(), "acme/injection-model")
	if err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	text := fmt.Sprintf("%+v", signal)
	for _, forbidden := range []string{"pwned", "<script>", "alert(1)", string(identity), testUserToken, testConsumerToken} {
		if strings.Contains(text, forbidden) {
			t.Errorf("decoded FeedbackSignal leaks %q: %s", forbidden, text)
		}
	}
}

// TestGetSignal_RealServer_UserTokenGets403MappedToUnauthorized proves the
// real server's own "wrong scope" 403 (auth.go's authConsumer) maps to
// ErrUnauthorized end-to-end, not merely in a hand-rolled fake.
func TestGetSignal_RealServer_UserTokenGets403MappedToUnauthorized(t *testing.T) {
	env := newRealServerEnv(t)
	env.seed(t, paddedIdentity(0), "acme/model-1", 4, nil, env.now)

	badProvider, err := NewHTTPReferenceSignalProvider(HTTPReferenceSignalProviderConfig{
		BaseURL: env.http.URL,
		Token:   testUserToken, // the user-scope secret, deliberately wrong scope
	})
	if err != nil {
		t.Fatalf("NewHTTPReferenceSignalProvider: %v", err)
	}
	if _, err := badProvider.GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("GetSignal with user-scope token: err = %v, want ErrUnauthorized", err)
	}
}

// TestGetSignal_RealServer_GarbageTokenIsUnauthorized proves a token
// matching neither configured secret is still ErrUnauthorized (401), not
// confused with the wrong-scope 403 case above.
func TestGetSignal_RealServer_GarbageTokenIsUnauthorized(t *testing.T) {
	env := newRealServerEnv(t)
	badProvider, err := NewHTTPReferenceSignalProvider(HTTPReferenceSignalProviderConfig{
		BaseURL: env.http.URL,
		Token:   "not-a-real-token-at-all",
	})
	if err != nil {
		t.Fatalf("NewHTTPReferenceSignalProvider: %v", err)
	}
	if _, err := badProvider.GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("GetSignal with garbage token: err = %v, want ErrUnauthorized", err)
	}
}

// TestGetSignal_RealServer_InvalidModelKeyMapsToUnavailable proves the real
// server's own model_key validation 400 (contract §7's path-traversal
// defense) is handled, not left to panic or decode nonsense.
func TestGetSignal_RealServer_InvalidModelKeyMapsToUnavailable(t *testing.T) {
	env := newRealServerEnv(t)
	if _, err := env.provider.GetSignal(context.Background(), "acme/mod..el"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("GetSignal with invalid model key: err = %v, want ErrUnavailable (real server 400)", err)
	}
}

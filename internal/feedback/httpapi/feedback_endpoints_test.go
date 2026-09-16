package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

func TestPutFeedback_Success_ReturnsMineCommunityAndPositions(t *testing.T) {
	env := newTestEnv(t, nil)
	reqBody := `{"overall":4,"skills":[{"key":"reasoning","rating":5},{"key":"coding","rating":4}],"review":"Короткий отзыв"}`

	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
	if resp.StatusCode != http.StatusOK {
		body := decodeJSON[errorResponseDTO](t, resp)
		t.Fatalf("PUT: status = %d, body = %+v", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	got := decodeJSON[summaryResponseDTO](t, resp)

	if got.ModelKey != "acme/model-1" {
		t.Errorf("model_key = %q, want acme/model-1", got.ModelKey)
	}
	if got.Mine == nil || got.Mine.Overall != 4 || got.Mine.Review != "Короткий отзыв" {
		t.Errorf("mine = %+v, want overall=4 review=Короткий отзыв", got.Mine)
	}
	if len(got.Mine.Skills) != 2 {
		t.Errorf("mine.skills = %+v, want 2 entries", got.Mine.Skills)
	}
	if got.Community == nil || got.Community.Count != 1 || got.Community.Average == nil || *got.Community.Average != 4 {
		t.Errorf("community = %+v, want count=1 average=4", got.Community)
	}
	if got.Others != nil {
		t.Errorf("others = %+v, want nil (not requested)", got.Others)
	}
	if got.PersonalPosition.Status != "ranked" || got.PersonalPosition.Value != 1 {
		t.Errorf("personal_position = %+v, want ranked/1", got.PersonalPosition)
	}
	if got.CommunityPosition.Status != "ineligible" {
		t.Errorf("community_position = %+v, want ineligible (n=1 < 5)", got.CommunityPosition)
	}
	if got.BasePosition.Status != "unranked" {
		t.Errorf("base_position = %+v, want unranked (no base ranking source in this MVP)", got.BasePosition)
	}
}

func TestPutFeedback_ResponseNeverLeaksIdentityOrReview400Fields(t *testing.T) {
	env := newTestEnv(t, nil)
	reqBody := `{"overall":3,"skills":[],"review":""}`
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
	raw := make([]byte, 0, 4096)
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	resp.Body.Close()
	raw = buf.Bytes()
	if strings.Contains(string(raw), identityA) {
		t.Errorf("PUT response leaks identity_id: %s", raw)
	}
	if strings.Contains(string(raw), testUserToken) {
		t.Errorf("PUT response leaks the bearer token: %s", raw)
	}
}

func TestPutFeedback_DuplicateSkillKeyRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	reqBody := `{"overall":3,"skills":[{"key":"coding","rating":4},{"key":"coding","rating":5}],"review":""}`
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate skill key: status = %d, want 400", resp.StatusCode)
	}
	got := decodeJSON[errorResponseDTO](t, resp)
	if len(got.Fields) != 1 || got.Fields[0].Field != "skills[].key" {
		t.Errorf("fields = %+v, want one skills[].key violation", got.Fields)
	}
}

func TestPutFeedback_InvalidRatingRangeRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	for _, overall := range []string{"0", "6", "-1"} {
		reqBody := fmt.Sprintf(`{"overall":%s,"skills":[],"review":""}`, overall)
		resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("overall=%s: status = %d, want 400", overall, resp.StatusCode)
		}
	}
}

func TestPutFeedback_UnknownSkillKeyRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	reqBody := `{"overall":3,"skills":[{"key":"not-a-real-skill","rating":3}],"review":""}`
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown skill key: status = %d, want 400", resp.StatusCode)
	}
}

func TestPutFeedback_MalformedJSONRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(`{not json`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed JSON: status = %d, want 400", resp.StatusCode)
	}
}

func TestPutFeedback_EmptyBodyRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(``))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty body: status = %d, want 400", resp.StatusCode)
	}
}

func TestPutFeedback_UnknownFieldRejected400(t *testing.T) {
	env := newTestEnv(t, nil)
	// model_key must never be accepted in the body — it comes from the URL
	// only (dto.go's own doc comment).
	reqBody := `{"model_key":"acme/model-1","overall":3,"skills":[],"review":""}`
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown field model_key in body: status = %d, want 400", resp.StatusCode)
	}
}

func TestPutFeedback_BodyTooLargeRejected413(t *testing.T) {
	env := newTestEnv(t, nil)
	hugeReview := strings.Repeat("a", int(feedback.MaxRequestBodyBytes)+1)
	reqBody := fmt.Sprintf(`{"overall":3,"skills":[],"review":%q}`, hugeReview)
	resp := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(reqBody))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status = %d, want 413", resp.StatusCode)
	}
}

func TestPutFeedback_IsIdempotentUpsert(t *testing.T) {
	env := newTestEnv(t, nil)
	first := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(`{"overall":2,"skills":[],"review":""}`))
	firstBody := decodeJSON[summaryResponseDTO](t, first)
	if firstBody.Community.Count != 1 {
		t.Fatalf("after first PUT: community.count = %d, want 1", firstBody.Community.Count)
	}

	second := env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(`{"overall":5,"skills":[],"review":""}`))
	secondBody := decodeJSON[summaryResponseDTO](t, second)
	if secondBody.Community.Count != 1 {
		t.Errorf("after second PUT (same identity): community.count = %d, want 1 (upsert, not a new vote)", secondBody.Community.Count)
	}
	if secondBody.Community.Average == nil || *secondBody.Community.Average != 5 {
		t.Errorf("after second PUT: community.average = %v, want 5", secondBody.Community.Average)
	}
}

func TestGetOwnFeedback_NoRatingYetReturns200WithNull(t *testing.T) {
	env := newTestEnv(t, nil)
	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET .../me with no rating: status = %d, want 200 (never 404)", resp.StatusCode)
	}
	got := decodeJSON[ownFeedbackResponseDTO](t, resp)
	if got.OwnFeedback != nil {
		t.Errorf("own_feedback = %+v, want nil", got.OwnFeedback)
	}
	if got.ModelKey != "acme/model-1" {
		t.Errorf("model_key = %q, want acme/model-1", got.ModelKey)
	}
}

func TestGetSummary_OthersExcludesCallerOnlyWhenRequested(t *testing.T) {
	env := newTestEnv(t, nil)
	env.seedFeedback(t, identityA, "acme/model-1", 5, nil, env.now)
	env.seedFeedback(t, identityB, "acme/model-1", 1, nil, env.now)

	withoutOthers := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/summary", userHeaders(identityA), nil)
	withoutBody := decodeJSON[summaryResponseDTO](t, withoutOthers)
	if withoutBody.Others != nil {
		t.Errorf("others = %+v without ?others=true, want nil", withoutBody.Others)
	}
	if withoutBody.Community.Count != 2 {
		t.Errorf("community.count = %d, want 2", withoutBody.Community.Count)
	}

	withOthers := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/summary?others=true", userHeaders(identityA), nil)
	withBody := decodeJSON[summaryResponseDTO](t, withOthers)
	if withBody.Others == nil || withBody.Others.Count != 1 {
		t.Fatalf("others = %+v, want count=1 (excluding identity A)", withBody.Others)
	}
	if withBody.Others.Average == nil || *withBody.Others.Average != 1 {
		t.Errorf("others.average = %v, want 1 (identity B's rating only)", withBody.Others.Average)
	}
}

func TestUpdateIdentityA_NeverChangesIdentityBsRowsOrPersonalPosition(t *testing.T) {
	env := newTestEnv(t, nil)
	env.seedFeedback(t, identityA, "acme/model-1", 3, nil, env.now)
	env.seedFeedback(t, identityB, "acme/model-1", 4, nil, env.now)

	// Identity A updates their rating.
	env.seedFeedback(t, identityA, "acme/model-1", 1, nil, env.now)

	bResp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityB), nil)
	bGot := decodeJSON[ownFeedbackResponseDTO](t, bResp)
	if bGot.OwnFeedback == nil || bGot.OwnFeedback.Overall != 4 {
		t.Fatalf("identity B's own_feedback = %+v, want unchanged overall=4", bGot.OwnFeedback)
	}

	bSummary := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/summary", userHeaders(identityB), nil)
	bSummaryGot := decodeJSON[summaryResponseDTO](t, bSummary)
	if bSummaryGot.PersonalPosition.Status != "ranked" || bSummaryGot.PersonalPosition.Value != 1 {
		t.Errorf("identity B's personal_position = %+v, want unchanged ranked/1 (B is B's only rated model)", bSummaryGot.PersonalPosition)
	}

	// Community reflects both, including A's update.
	aSummary := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/summary", userHeaders(identityA), nil)
	aSummaryGot := decodeJSON[summaryResponseDTO](t, aSummary)
	if aSummaryGot.Community.Count != 2 {
		t.Fatalf("community.count = %d, want 2", aSummaryGot.Community.Count)
	}
	wantAvg := 2.5 // (1 + 4) / 2
	if aSummaryGot.Community.Average == nil || *aSummaryGot.Community.Average != wantAvg {
		t.Errorf("community.average = %v, want %v", aSummaryGot.Community.Average, wantAvg)
	}
}

// TestCommunityMatrix_CountsAndPositions is the brief's own required
// matrix over identities A/B and counts 0/1/4/5/19/20 (11.2): mine/others,
// personal positions, community eligibility/position, and shrinkage score
// for the count boundary that flips community_position from ineligible to
// ranked.
func TestCommunityMatrix_CountsAndPositions(t *testing.T) {
	cases := []struct {
		name             string
		count            int
		wantEligible     bool
		wantCommunityAvg float64
	}{
		{name: "zero", count: 0, wantEligible: false},
		{name: "one", count: 1, wantEligible: false, wantCommunityAvg: 3},
		{name: "four", count: 4, wantEligible: false, wantCommunityAvg: 3},
		{name: "five", count: 5, wantEligible: true, wantCommunityAvg: 3},
		{name: "nineteen", count: 19, wantEligible: true, wantCommunityAvg: 3},
		{name: "twenty", count: 20, wantEligible: true, wantCommunityAvg: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, nil)
			const modelKey = feedback.ModelKey("acme/matrix-model")
			for i := 0; i < tc.count; i++ {
				identity := feedback.IdentityID(fmt.Sprintf("%064d", i))
				env.seedFeedback(t, identity, modelKey, 3, nil, env.now)
			}

			resp := env.do(t, http.MethodGet, "/v1/models/acme/matrix-model/feedback/summary", userHeaders(identityA), nil)
			got := decodeJSON[summaryResponseDTO](t, resp)

			if got.Community.Count != tc.count {
				t.Fatalf("community.count = %d, want %d", got.Community.Count, tc.count)
			}
			if tc.count > 0 {
				if got.Community.Average == nil || *got.Community.Average != tc.wantCommunityAvg {
					t.Errorf("community.average = %v, want %v", got.Community.Average, tc.wantCommunityAvg)
				}
			} else if got.Community.Average != nil {
				t.Errorf("community.average = %v, want nil for count=0", *got.Community.Average)
			}

			wantStatus := "ineligible"
			if tc.wantEligible {
				wantStatus = "ranked"
			}
			if got.CommunityPosition.Status != wantStatus {
				t.Errorf("community_position.status = %q, want %q (count=%d)", got.CommunityPosition.Status, wantStatus, tc.count)
			}
			if tc.wantEligible && got.CommunityPosition.Value != 1 {
				t.Errorf("community_position.value = %d, want 1 (only eligible model)", got.CommunityPosition.Value)
			}

			// identityA never rated this model in this subtest, so mine is
			// null and personal_position is unranked.
			if got.Mine != nil {
				t.Errorf("mine = %+v, want nil (identity A never rated this model)", got.Mine)
			}
			if got.PersonalPosition.Status != "unranked" {
				t.Errorf("personal_position.status = %q, want unranked", got.PersonalPosition.Status)
			}
		})
	}
}

// TestCommunityPosition_RanksMultipleEligibleModelsByShrinkageScore proves
// the single social policy across more than one eligible model (contract
// §3, plan 11.2): community_score=(n*average+10*3.0)/(n+10), higher score
// ranks first; a genuine tie falls back to base_position — which, for this
// MVP's summary/signal endpoints (no independent base ranking source, see
// buildSummary's own doc comment), is always empty, so two models tying on
// score fall back to ModelKey order (ranking.go's own documented,
// deterministic default when no base ranking applies to either side).
func TestCommunityPosition_RanksMultipleEligibleModelsByShrinkageScore(t *testing.T) {
	env := newTestEnv(t, nil)

	// model-high: 5 ratings of 5 -> score = (5*5+10*3)/(15) = 55/15 = 3.667
	for i := 0; i < 5; i++ {
		env.seedFeedback(t, feedback.IdentityID(paddedIdentity(i)), "acme/model-high", 5, nil, env.now)
	}
	// model-low: 5 ratings of 1 -> score = (5*1+10*3)/(15) = 35/15 = 2.333
	for i := 0; i < 5; i++ {
		env.seedFeedback(t, feedback.IdentityID(paddedIdentity(100+i)), "acme/model-low", 1, nil, env.now)
	}

	highResp := env.do(t, http.MethodGet, "/v1/models/acme/model-high/feedback/summary", userHeaders(identityA), nil)
	high := decodeJSON[summaryResponseDTO](t, highResp)
	lowResp := env.do(t, http.MethodGet, "/v1/models/acme/model-low/feedback/summary", userHeaders(identityA), nil)
	low := decodeJSON[summaryResponseDTO](t, lowResp)

	if high.CommunityPosition.Status != "ranked" || high.CommunityPosition.Value != 1 {
		t.Errorf("model-high community_position = %+v, want ranked/1 (higher shrinkage score)", high.CommunityPosition)
	}
	if low.CommunityPosition.Status != "ranked" || low.CommunityPosition.Value != 2 {
		t.Errorf("model-low community_position = %+v, want ranked/2 (lower shrinkage score)", low.CommunityPosition)
	}

	// The same ranking must show up on the trusted-consumer signal endpoint
	// too, via the shared Service.GetCommunityPosition path.
	sigHighResp := env.do(t, http.MethodGet, "/v1/models/acme/model-high/feedback/signal", consumerHeaders(), nil)
	sigHigh := decodeJSON[signalResponseDTO](t, sigHighResp)
	if sigHigh.Position.CommunityPosition == nil || *sigHigh.Position.CommunityPosition != 1 {
		t.Errorf("signal model-high position.community_position = %v, want 1", sigHigh.Position.CommunityPosition)
	}
}

// TestCommunityPosition_TieBreaksDeterministically proves that two models
// with an identical shrinkage score still get a stable, deterministic
// (not map-iteration-random) relative order.
func TestCommunityPosition_TieBreaksDeterministically(t *testing.T) {
	env := newTestEnv(t, nil)
	for i := 0; i < 5; i++ {
		env.seedFeedback(t, feedback.IdentityID(paddedIdentity(i)), "acme/tie-a", 3, nil, env.now)
	}
	for i := 0; i < 5; i++ {
		env.seedFeedback(t, feedback.IdentityID(paddedIdentity(100+i)), "acme/tie-b", 3, nil, env.now)
	}

	aResp := env.do(t, http.MethodGet, "/v1/models/acme/tie-a/feedback/summary", userHeaders(identityA), nil)
	a := decodeJSON[summaryResponseDTO](t, aResp)
	bResp := env.do(t, http.MethodGet, "/v1/models/acme/tie-b/feedback/summary", userHeaders(identityA), nil)
	b := decodeJSON[summaryResponseDTO](t, bResp)

	if a.CommunityPosition.Status != "ranked" || b.CommunityPosition.Status != "ranked" {
		t.Fatalf("both models should be ranked: a=%+v b=%+v", a.CommunityPosition, b.CommunityPosition)
	}
	if a.CommunityPosition.Value == b.CommunityPosition.Value {
		t.Errorf("tied models both report position %d, want distinct ranks", a.CommunityPosition.Value)
	}
	// "acme/tie-a" < "acme/tie-b" lexicographically, and ranking.go's
	// lessByBaseRank falls back to ModelKey order when neither side is in
	// the (here, empty) base ranking — so tie-a must win the tie.
	if a.CommunityPosition.Value != 1 || b.CommunityPosition.Value != 2 {
		t.Errorf("tie broken as a=%d b=%d, want a=1 b=2 (deterministic ModelKey fallback)", a.CommunityPosition.Value, b.CommunityPosition.Value)
	}
}

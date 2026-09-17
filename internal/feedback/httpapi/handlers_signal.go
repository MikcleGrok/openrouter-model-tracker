package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// Consumer-signal policy constants (plan 4.6, contract.md §6). These are
// server-side policy, never client config or a request parameter (plan
// 4.6: "Threshold, TTL, confidence enum и допустимые schema/policy versions
// являются кодом server-side policy, а не client config").
const (
	signalSchemaVersion = "feedback-signal.v1"
	signalPolicyVersion = "feedback-signal-policy.v1"

	// signalFreshnessTTL is the age past which a signal is stale (plan
	// 4.6: TTL 24h; contract §6 confirms "TTL свежести (freshness): 24ч").
	// Exactly 24h is fresh; anything strictly greater is stale (contract
	// §6: "age 24h как fresh и age 24h+1ns как stale").
	signalFreshnessTTL = 24 * time.Hour
	signalTTLSeconds   = int(signalFreshnessTTL / time.Second)

	// signalEstablishedThreshold is the sample_count at and above which a
	// dimension is "established" (plan 4.6: ">= 20"). Below it down to
	// signalProvisionalThreshold, a dimension is "provisional"; below that,
	// "insufficient".
	signalEstablishedThreshold = 20
)

// signalProvisionalThreshold is the sample_count at and above which a
// dimension is at least "provisional" (plan 4.6: "5-19 — provisional"). It
// is the same number as feedback.CommunityMinSampleForPosition by contract
// (contract.md §6: "то же число, что и CommunityMinSampleForPosition... plan
// не выделяет отдельный порог для consumer API"), referenced by name
// rather than restated as a literal 5 so the two can never silently drift
// apart.
const signalProvisionalThreshold = feedback.CommunityMinSampleForPosition

// handleGetSignal implements GET /v1/models/{model_key}/feedback/signal
// (plan 4.6). Called only from behind authConsumer (routes.go).
func (s *Server) handleGetSignal(w http.ResponseWriter, r *http.Request, rawModelKey string) {
	modelKey, err := feedback.NormalizeModelKey(rawModelKey)
	if err != nil {
		writeInputError(w, err)
		return
	}

	resp, err := s.buildSignal(r.Context(), modelKey)
	if err != nil {
		s.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildSignal computes the full community-only consumer signal for
// modelKey (plan 4.6). It never reads or reports mine, others,
// personal_position, identity, a token, or raw review text — the aggregate
// this reads (community, exclude=nil) is already identity-blind by
// construction (Service.GetAggregate's own contract), and this function
// only ever converts count/average/skill data into the dimension DTOs
// below.
func (s *Server) buildSignal(ctx context.Context, modelKey feedback.ModelKey) (signalResponseDTO, error) {
	agg, err := s.service.GetAggregate(ctx, modelKey, nil)
	if err != nil {
		return signalResponseDTO{}, err
	}
	asOf, hasAsOf, err := s.store.LastUpdatedAt(ctx, modelKey)
	if err != nil {
		return signalResponseDTO{}, err
	}
	now := s.now()

	resp := signalResponseDTO{
		ModelKey:      string(modelKey),
		SignalScope:   "community",
		SchemaVersion: signalSchemaVersion,
		PolicyVersion: signalPolicyVersion,
		Skills:        make(map[string]signalDimensionDTO, len(feedback.AllowedSkills())),
	}

	switch {
	case !hasAsOf:
		// No ratings at all for this model (contract §6: "при отсутствии
		// сохранённых оценок as_of=null"). The whole signal is reported
		// unavailable at the top level (there is nothing to serve), but
		// plan §4.6's dimension policy is unconditional on that: "При
		// count < 5 dimension value равно null, status/confidence=
		// insufficient" — and §11.2's own boundary matrix explicitly puts
		// 0 inside the "<5 → insufficient" case, not a separate state.
		// Every dimension therefore still takes the ordinary
		// dimensionSignal(0, 0) path (review round 1 finding #1): a
		// consumer keyed on confidence=="insufficient" for "brand new
		// model, not enough votes yet" must not instead see a null
		// confidence here, which would misread it as a provider outage.
		resp.Status = "unavailable"
		resp.Freshness = signalFreshnessDTO{AsOf: nil, ComputedAt: now, TTLSeconds: signalTTLSeconds, Stale: false}
		resp.Overall = dimensionSignal(0, 0)
		for _, key := range feedback.AllowedSkills() {
			resp.Skills[key] = dimensionSignal(0, 0)
		}
	default:
		age := now.Sub(asOf)
		stale := age > signalFreshnessTTL
		asOfCopy := asOf
		resp.Freshness = signalFreshnessDTO{AsOf: &asOfCopy, ComputedAt: now, TTLSeconds: signalTTLSeconds, Stale: stale}
		if stale {
			resp.Status = "stale"
			resp.Overall = staleDimension(agg.Count)
			for _, key := range feedback.AllowedSkills() {
				resp.Skills[key] = staleDimension(skillCount(agg, key))
			}
		} else {
			resp.Status = "usable"
			resp.Overall = dimensionSignal(agg.Count, agg.Average)
			for _, key := range feedback.AllowedSkills() {
				count, average := skillCountAverage(agg, key)
				resp.Skills[key] = dimensionSignal(count, average)
			}
		}
	}

	// position.community_position is a separate, purely count/score-based
	// eligibility computation (contract §3) — never affected by the
	// value/freshness policy above, so it is always computed live
	// regardless of which branch was taken.
	pos, err := s.service.GetCommunityPosition(ctx, modelKey, nil)
	if err != nil {
		return signalResponseDTO{}, err
	}
	resp.Position = signalPositionDTO{CommunityPosition: positionValueOrNil(pos)}

	return resp, nil
}

// skillCountAverage returns the count/average for one allowed skill within
// agg, or (0, 0) if that skill has no ratings — matching Aggregate.Skills'
// own "absent means zero" contract (model.go).
func skillCountAverage(agg feedback.Aggregate, key string) (int, float64) {
	for _, sa := range agg.Skills {
		if sa.Key == key {
			return sa.Count, sa.Average
		}
	}
	return 0, 0
}

func skillCount(agg feedback.Aggregate, key string) int {
	count, _ := skillCountAverage(agg, key)
	return count
}

// dimensionSignal classifies one dimension's (overall's, or one skill's)
// sample_count/average into a signalDimensionDTO per the MVP policy (plan
// 4.6): <5 insufficient (value null), 5-19 provisional, >=20 established —
// provisional and established both carry the real average as value.
func dimensionSignal(count int, average float64) signalDimensionDTO {
	switch {
	case count < signalProvisionalThreshold:
		return signalDimensionDTO{Value: nil, Status: "insufficient", Confidence: strPtr("insufficient"), SampleCount: count}
	case count < signalEstablishedThreshold:
		v := average
		return signalDimensionDTO{Value: &v, Status: "provisional", Confidence: strPtr("provisional"), SampleCount: count}
	default:
		v := average
		return signalDimensionDTO{Value: &v, Status: "established", Confidence: strPtr("established"), SampleCount: count}
	}
}

// staleDimension is one dimension's shape when the whole signal is stale:
// null value, null confidence, but the real sample_count is still reported
// (informational — plan 4.6 nulls "values", not counts).
func staleDimension(count int) signalDimensionDTO {
	return signalDimensionDTO{Value: nil, Status: "stale", Confidence: nil, SampleCount: count}
}

func strPtr(s string) *string { return &s }

// positionValueOrNil converts a domain Position into the signal's simpler
// nullable-int shape (plan 4.6's "position": {"community_position": 12}),
// nil unless the position is actually ranked.
func positionValueOrNil(p feedback.Position) *int {
	if p.Status != feedback.PositionStatusRanked {
		return nil
	}
	v := p.Value
	return &v
}

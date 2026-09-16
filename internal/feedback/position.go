package feedback

// CommunityPriorWeight, CommunityPriorMean and CommunityMinSampleForPosition
// fix the single deterministic community-position policy from plan 4.1:
//
//	community_score = (n*average + prior_weight*3.0) / (n + prior_weight)
//	prior_weight = 10
//
// A model's raw Aggregate (Count/Average/Distribution/Skills) always
// reflects the real numbers; CommunityScore/CommunityPositionEligible only
// govern how those numbers translate into a *position* — a small sample
// (n<5) never outranks a large one on a lucky average, and never gets a
// position at all (see CommunityPositionEligible and
// ModelPositions.CommunityPosition's PositionStatusIneligible case).
const (
	CommunityPriorWeight          = 10.0
	CommunityPriorMean            = 3.0
	CommunityMinSampleForPosition = 5
)

// CommunityScore computes the Bayesian-shrinkage score a model's community
// position is ordered by, given its Aggregate.Count and Aggregate.Average.
// It takes the already-known count/average rather than an Aggregate or a
// full model list on purpose: computing them is a repository/service
// concern (Task 2+), this package only fixes the pure formula those layers
// must use, so the number can never drift between call sites.
//
// The result is meaningful for ordering only when
// CommunityPositionEligible(count) is true; call CommunityScore on an
// ineligible count only if a caller has some other reason to want the raw
// shrinkage value (e.g. diagnostics) — ModelPositions.CommunityPosition
// must report PositionStatusIneligible instead of a value in that case.
func CommunityScore(count int, average float64) float64 {
	n := float64(count)
	return (n*average + CommunityPriorWeight*CommunityPriorMean) / (n + CommunityPriorWeight)
}

// CommunityPositionEligible reports whether count many valid community
// ratings are enough to assign a community_position at all (plan 4.1: "при
// n<5 community_position=null, модель идёт после eligible community
// models, а внутри неeligible-группы используется base_position"). An
// ineligible model is not absent from ordering altogether — it sorts after
// every eligible model, by base_position among its own ineligible group —
// but assigning that group ordering needs the full model set and so
// belongs to the service/repository layer, not this package.
func CommunityPositionEligible(count int) bool {
	return count >= CommunityMinSampleForPosition
}

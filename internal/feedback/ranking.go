package feedback

import "sort"

// This file computes the three ModelPositions signals across a full model
// set (plan 4.1/5.2's "алгоритм агрегации/позиционирования, работающий по
// всему набору моделей — нужен repository"). position.go fixes the pure,
// single-model shrinkage formula (CommunityScore/CommunityPositionEligible);
// this file is what turns that formula, plus a whole set of models' data,
// into one model's rank among its peers. All three functions are pure and
// unexported: Service.GetModelPositions is their only caller, feeding them
// data already fetched from Repository.

// basePosition finds modelKey's 1-based index within baseRanking, the
// caller-supplied existing benchmark/ranking order (index 0 = rank 1, the
// best-ranked model; this package never computes that order itself — see
// GetModelPositions). A modelKey absent from baseRanking is
// PositionStatusUnranked: feedback never fabricates a base rank for a
// model outside the current benchmark ranking (model.go's
// ModelPositions.BasePosition doc comment).
func basePosition(modelKey ModelKey, baseRanking []ModelKey) Position {
	if rank, ok := baseRank(modelKey, baseRanking); ok {
		return Position{Value: rank + 1, Status: PositionStatusRanked}
	}
	return Position{Status: PositionStatusUnranked}
}

// baseRank returns modelKey's 0-based index within baseRanking and whether
// it was found, for use as the tie-breaker in personalPosition and
// communityPosition below.
func baseRank(modelKey ModelKey, baseRanking []ModelKey) (rank int, ok bool) {
	for i, mk := range baseRanking {
		if mk == modelKey {
			return i, true
		}
	}
	return 0, false
}

// lessByBaseRank reports whether a should sort before b, used as the tie
// -breaker once two candidates score equally: a model present in
// baseRanking always beats one absent from it; between two present models
// the lower (better) base rank wins; between two absent models, ModelKey
// order breaks the tie so the result is deterministic instead of depending
// on Go's randomized map iteration order.
func lessByBaseRank(a, b ModelKey, baseRanking []ModelKey) bool {
	ra, aok := baseRank(a, baseRanking)
	rb, bok := baseRank(b, baseRanking)
	switch {
	case aok && bok:
		return ra < rb
	case aok:
		return true
	case bok:
		return false
	default:
		return a < b
	}
}

// personalPosition ranks modelKey within identity's own "Мои оценки"
// ordering (plan 1.4/4.1): only models present in ownRatings (identity's
// own feedback, keyed by ModelKey) participate, ordered by Overall rating
// descending, ties broken by lessByBaseRank. modelKey absent from
// ownRatings is PositionStatusUnranked and never receives a fabricated
// score — it is not sorted in among rated models at all.
func personalPosition(modelKey ModelKey, ownRatings map[ModelKey]Feedback, baseRanking []ModelKey) Position {
	if _, rated := ownRatings[modelKey]; !rated {
		return Position{Status: PositionStatusUnranked}
	}
	keys := make([]ModelKey, 0, len(ownRatings))
	for mk := range ownRatings {
		keys = append(keys, mk)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if ra, rb := ownRatings[a].Overall, ownRatings[b].Overall; ra != rb {
			return ra > rb
		}
		return lessByBaseRank(a, b, baseRanking)
	})
	for i, mk := range keys {
		if mk == modelKey {
			return Position{Value: i + 1, Status: PositionStatusRanked}
		}
	}
	// keys is exactly the key set of ownRatings, and modelKey was
	// confirmed present in ownRatings above, so it is always found.
	panic("feedback: personalPosition: modelKey missing from its own ownRatings key set")
}

// communityPosition ranks modelKey within the community-position ordering
// (plan 4.1, contract §3): only models whose Aggregate.Count satisfies
// CommunityPositionEligible participate, ordered by CommunityScore
// descending, ties broken by lessByBaseRank. modelKey below the
// eligibility threshold — including one with no aggregate entry at all,
// i.e. zero ratings — is PositionStatusIneligible and never receives a
// value: contract §3 places every ineligible model after every eligible
// one, ordered by base_position within its own group, but that full-list
// rendering is a caller concern (e.g. a TUI sorting a whole table), not
// something this single-model computation needs to resolve.
func communityPosition(modelKey ModelKey, aggregates map[ModelKey]Aggregate, baseRanking []ModelKey) Position {
	agg, ok := aggregates[modelKey]
	if !ok || !CommunityPositionEligible(agg.Count) {
		return Position{Status: PositionStatusIneligible}
	}
	keys := make([]ModelKey, 0, len(aggregates))
	for mk, a := range aggregates {
		if CommunityPositionEligible(a.Count) {
			keys = append(keys, mk)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		sa := CommunityScore(aggregates[a].Count, aggregates[a].Average)
		sb := CommunityScore(aggregates[b].Count, aggregates[b].Average)
		if sa != sb {
			return sa > sb
		}
		return lessByBaseRank(a, b, baseRanking)
	})
	for i, mk := range keys {
		if mk == modelKey {
			return Position{Value: i + 1, Status: PositionStatusRanked}
		}
	}
	// keys always includes every eligible modelKey from aggregates, and
	// modelKey was confirmed eligible above, so it is always found.
	panic("feedback: communityPosition: modelKey missing from its own eligible key set")
}

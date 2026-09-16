package feedback

import "testing"

func TestBasePositionFoundAndAbsent(t *testing.T) {
	ranking := []ModelKey{"a/one", "b/two", "c/three"}

	tests := []struct {
		name     string
		modelKey ModelKey
		want     Position
	}{
		{"first in ranking is position 1", "a/one", Position{Value: 1, Status: PositionStatusRanked}},
		{"middle of ranking", "b/two", Position{Value: 2, Status: PositionStatusRanked}},
		{"last of ranking", "c/three", Position{Value: 3, Status: PositionStatusRanked}},
		{"absent from ranking is unranked, not a fabricated value", "z/unknown", Position{Status: PositionStatusUnranked}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := basePosition(tt.modelKey, ranking); got != tt.want {
				t.Errorf("basePosition(%q, %v) = %+v, want %+v", tt.modelKey, ranking, got, tt.want)
			}
		})
	}
}

func TestBasePositionEmptyRanking(t *testing.T) {
	if got, want := basePosition("a/one", nil), (Position{Status: PositionStatusUnranked}); got != want {
		t.Errorf("basePosition with empty ranking = %+v, want %+v", got, want)
	}
}

func TestPersonalPositionUnratedModelIsUnranked(t *testing.T) {
	ownRatings := map[ModelKey]Feedback{
		"a/one": {FeedbackInput: FeedbackInput{ModelKey: "a/one", Overall: 4}},
	}
	got := personalPosition("z/never-rated", ownRatings, nil)
	if want := (Position{Status: PositionStatusUnranked}); got != want {
		t.Errorf("personalPosition(unrated) = %+v, want %+v", got, want)
	}
}

func TestPersonalPositionOrdersByOwnRatingDescendingThenBaseRank(t *testing.T) {
	// b/two has the highest personal rating, so it must rank 1 despite
	// sitting last in the base ranking; a/one and c/three tie on rating and
	// are split by base rank (a/one is earlier in baseRanking).
	ownRatings := map[ModelKey]Feedback{
		"a/one":   {FeedbackInput: FeedbackInput{ModelKey: "a/one", Overall: 3}},
		"b/two":   {FeedbackInput: FeedbackInput{ModelKey: "b/two", Overall: 5}},
		"c/three": {FeedbackInput: FeedbackInput{ModelKey: "c/three", Overall: 3}},
	}
	baseRanking := []ModelKey{"a/one", "c/three", "b/two"}

	tests := []struct {
		modelKey ModelKey
		want     int
	}{
		{"b/two", 1},
		{"a/one", 2},
		{"c/three", 3},
	}
	for _, tt := range tests {
		got := personalPosition(tt.modelKey, ownRatings, baseRanking)
		if got.Status != PositionStatusRanked || got.Value != tt.want {
			t.Errorf("personalPosition(%q) = %+v, want Value=%d Status=ranked", tt.modelKey, got, tt.want)
		}
	}
}

func TestPersonalPositionUnratedModelDoesNotShiftRatedRanks(t *testing.T) {
	// An unrated model must never occupy a slot in the rated ordering: with
	// only one rated model, that model is always personal position 1,
	// regardless of how many other models exist unrated.
	ownRatings := map[ModelKey]Feedback{
		"only-rated": {FeedbackInput: FeedbackInput{ModelKey: "only-rated", Overall: 1}},
	}
	got := personalPosition("only-rated", ownRatings, []ModelKey{"unrated-a", "unrated-b", "only-rated"})
	if want := (Position{Value: 1, Status: PositionStatusRanked}); got != want {
		t.Errorf("personalPosition(only rated model) = %+v, want %+v", got, want)
	}
}

func TestCommunityPositionIneligibleBelowThreshold(t *testing.T) {
	aggregates := map[ModelKey]Aggregate{
		"low-n": {Count: 4, Average: 5},
	}
	got := communityPosition("low-n", aggregates, nil)
	if want := (Position{Status: PositionStatusIneligible}); got != want {
		t.Errorf("communityPosition(n=4) = %+v, want %+v (no Value)", got, want)
	}
}

func TestCommunityPositionAbsentFromAggregatesIsIneligible(t *testing.T) {
	got := communityPosition("never-rated", map[ModelKey]Aggregate{}, nil)
	if want := (Position{Status: PositionStatusIneligible}); got != want {
		t.Errorf("communityPosition(never rated) = %+v, want %+v", got, want)
	}
}

func TestCommunityPositionRanksEligibleByShrinkageScoreDescending(t *testing.T) {
	aggregates := map[ModelKey]Aggregate{
		"best":       {Count: 20, Average: 5},
		"middle":     {Count: 10, Average: 4},
		"worst":      {Count: 10, Average: 3.2},
		"ineligible": {Count: 3, Average: 5}, // high average, but too few samples to compete
	}
	tests := []struct {
		modelKey ModelKey
		want     Position
	}{
		{"best", Position{Value: 1, Status: PositionStatusRanked}},
		{"middle", Position{Value: 2, Status: PositionStatusRanked}},
		{"worst", Position{Value: 3, Status: PositionStatusRanked}},
		{"ineligible", Position{Status: PositionStatusIneligible}},
	}
	for _, tt := range tests {
		if got := communityPosition(tt.modelKey, aggregates, nil); got != tt.want {
			t.Errorf("communityPosition(%q) = %+v, want %+v", tt.modelKey, got, tt.want)
		}
	}
}

func TestCommunityPositionTiesBrokenByBaseRank(t *testing.T) {
	// Identical count/average produce an identical CommunityScore for both
	// models, so the tie must be broken by base ranking, not by map order.
	aggregates := map[ModelKey]Aggregate{
		"tie-a": {Count: 10, Average: 4},
		"tie-b": {Count: 10, Average: 4},
	}
	baseRanking := []ModelKey{"tie-b", "tie-a"}

	if got := communityPosition("tie-b", aggregates, baseRanking); got != (Position{Value: 1, Status: PositionStatusRanked}) {
		t.Errorf("communityPosition(tie-b) = %+v, want position 1 (better base rank)", got)
	}
	if got := communityPosition("tie-a", aggregates, baseRanking); got != (Position{Value: 2, Status: PositionStatusRanked}) {
		t.Errorf("communityPosition(tie-a) = %+v, want position 2", got)
	}
}

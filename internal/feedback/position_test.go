package feedback

import "testing"

func TestCommunityScoreMatchesFixedShrinkageFormula(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		average float64
		want    float64
	}{
		// n=0: score collapses to the prior mean regardless of average.
		{name: "zero samples returns prior mean", count: 0, average: 0, want: 3.0},
		{name: "zero samples ignores a nonzero average", count: 0, average: 5, want: 3.0},
		// A perfect score with few samples is pulled toward the prior mean,
		// not reported as a perfect 5.
		{name: "n=5 perfect average pulled toward prior", count: 5, average: 5, want: (5*5.0 + 10*3.0) / 15},
		// Large n dominates the prior, converging toward the raw average.
		{name: "large n approaches raw average", count: 1000, average: 4.5, want: (1000*4.5 + 10*3.0) / 1010},
		{name: "average equal to prior mean returns prior mean regardless of n", count: 50, average: 3.0, want: 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CommunityScore(tt.count, tt.average); !floatsClose(got, tt.want) {
				t.Errorf("CommunityScore(%d, %v) = %v, want %v", tt.count, tt.average, got, tt.want)
			}
		})
	}
}

func TestCommunityPositionEligibleBoundary(t *testing.T) {
	tests := []struct {
		count int
		want  bool
	}{
		{0, false},
		{1, false},
		{4, false},
		{5, true},
		{6, true},
		{1000, true},
	}
	for _, tt := range tests {
		if got := CommunityPositionEligible(tt.count); got != tt.want {
			t.Errorf("CommunityPositionEligible(%d) = %v, want %v", tt.count, got, tt.want)
		}
	}
}

func floatsClose(a, b float64) bool {
	const epsilon = 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

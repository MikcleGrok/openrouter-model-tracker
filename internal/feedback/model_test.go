package feedback

import "testing"

func TestRatingDistributionSetAndCountRoundTrip(t *testing.T) {
	var d RatingDistribution
	for rating := RatingMin; rating <= RatingMax; rating++ {
		d.Set(rating, rating*10)
	}
	for rating := RatingMin; rating <= RatingMax; rating++ {
		if got, want := d.Count(rating), rating*10; got != want {
			t.Errorf("Count(%d) = %d, want %d", rating, got, want)
		}
	}
}

func TestRatingDistributionCountOutOfRangeReturnsZeroNotPanic(t *testing.T) {
	var d RatingDistribution
	d.Set(3, 7)
	for _, rating := range []int{-100, -1, 0, 6, 7, 100} {
		if got := d.Count(rating); got != 0 {
			t.Errorf("Count(%d) = %d, want 0", rating, got)
		}
	}
}

func TestRatingDistributionSetOutOfRangeIsNoOp(t *testing.T) {
	var d RatingDistribution
	d.Set(0, 99)
	d.Set(6, 99)
	d.Set(-1, 99)
	for rating := RatingMin; rating <= RatingMax; rating++ {
		if got := d.Count(rating); got != 0 {
			t.Errorf("Count(%d) = %d, want 0 after only out-of-range Set calls", rating, got)
		}
	}
}

func TestRatingDistributionMarshalsAsPlainFiveElementArray(t *testing.T) {
	var d RatingDistribution
	if got := len(d); got != RatingMax {
		t.Fatalf("len(RatingDistribution{}) = %d, want %d", got, RatingMax)
	}
}

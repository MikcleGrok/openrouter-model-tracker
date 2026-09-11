package tier

import (
	"reflect"
	"testing"
)

func TestFilterValuesExcludeFree(t *testing.T) {
	if got, want := FilterValues(), []string{"opus", "sonnet", "haiku"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterValues() = %v, want %v", got, want)
	}
}

func TestAtLeastUsesMinimumSemanticsForPaidTiersAndExactLegacyFree(t *testing.T) {
	for _, test := range []struct {
		value, minimum string
		want           bool
	}{
		{"opus", "opus", true}, {"opus", "sonnet", true}, {"opus", "haiku", true}, {"opus", "free", false},
		{"sonnet", "opus", false}, {"sonnet", "sonnet", true}, {"sonnet", "haiku", true}, {"sonnet", "free", false},
		{"haiku", "opus", false}, {"haiku", "sonnet", false}, {"haiku", "haiku", true}, {"haiku", "free", false},
		{"free", "opus", false}, {"free", "sonnet", false}, {"free", "haiku", false}, {"free", "free", true},
	} {
		if got := AtLeast(test.value, test.minimum); got != test.want {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", test.value, test.minimum, got, test.want)
		}
	}
}

func TestAtLeastRejectsEmptyAndUnknownTiers(t *testing.T) {
	for _, test := range []struct {
		value, minimum string
	}{
		{"", ""}, {"unknown", "unknown"}, {"opus", ""}, {"", "opus"}, {"unknown", "haiku"}, {"free", "unknown"},
	} {
		if got := AtLeast(test.value, test.minimum); got {
			t.Errorf("AtLeast(%q, %q) = true, want false", test.value, test.minimum)
		}
	}
}

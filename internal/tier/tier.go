// Package tier defines the model tiers accepted throughout the application.
package tier

import "strings"

var values = []string{"opus", "sonnet", "haiku", "free"}

// FilterValues returns the paid tiers offered by the minimum-tier filter.
func FilterValues() []string {
	return append([]string(nil), values[:len(values)-1]...)
}

// Values returns the accepted tier values in display order.
func Values() []string {
	return append([]string(nil), values...)
}

// IsValid reports whether value is an accepted tier, ignoring case.
func IsValid(value string) bool {
	for _, valid := range values {
		if strings.EqualFold(value, valid) {
			return true
		}
	}
	return false
}

// AtLeast reports whether value is at or above the selected minimum tier.
// The legacy free filter is exact rather than a minimum paid-tier threshold.
func AtLeast(value, minimum string) bool {
	valueRank, minimumRank := rank(value), rank(minimum)
	if valueRank < 0 || minimumRank < 0 {
		return false
	}
	if strings.EqualFold(minimum, "free") {
		return strings.EqualFold(value, "free")
	}
	return valueRank >= minimumRank
}

// Rank returns the established tier order, with higher tiers having larger values.
func Rank(value string) int {
	return rank(value)
}

func rank(value string) int {
	for index, valid := range values {
		if strings.EqualFold(value, valid) {
			return len(values) - index - 1
		}
	}
	return -1
}

// ValuesString returns the accepted values for user-facing errors and help.
func ValuesString() string {
	return strings.Join(values, ", ")
}

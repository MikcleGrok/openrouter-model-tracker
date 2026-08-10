// Package tier defines the model tiers accepted throughout the application.
package tier

import "strings"

var values = []string{"opus", "sonnet", "haiku", "free"}

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

// ValuesString returns the accepted values for user-facing errors and help.
func ValuesString() string {
	return strings.Join(values, ", ")
}

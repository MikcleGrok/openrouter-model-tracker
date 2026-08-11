// Package filter contains validation shared by config loading and filter parsing.
package filter

import (
	"fmt"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/tier"
)

// ValidateTiers validates tier predicates in a comma-separated filter.
func ValidateTiers(value string) error {
	for _, raw := range strings.Split(value, ",") {
		predicate := strings.TrimSpace(raw)
		lower := strings.ToLower(predicate)
		if !strings.HasPrefix(lower, "tier:") {
			continue
		}
		value := strings.TrimSpace(predicate[len("tier:"):])
		if value == "" {
			return fmt.Errorf("tier must not be empty")
		}
		if !tier.IsValid(value) {
			return fmt.Errorf("unknown tier %q; allowed values: %s", value, tier.ValuesString())
		}
	}
	return nil
}

// ValidateAvailability validates the persisted tri-state pricing predicate.
func ValidateAvailability(value string) error {
	for _, raw := range strings.Split(value, ",") {
		predicate := strings.ToLower(strings.TrimSpace(raw))
		if !strings.HasPrefix(predicate, "availability:") {
			continue
		}
		choice := strings.TrimSpace(strings.TrimPrefix(predicate, "availability:"))
		if choice != "any" && choice != "free" && choice != "paid" {
			return fmt.Errorf("availability must be any, free, or paid")
		}
	}
	return nil
}

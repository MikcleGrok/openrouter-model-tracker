// Package filter contains validation shared by config loading and filter parsing.
package filter

import (
	"fmt"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/tier"
)

type taskFitDefinition struct {
	keyword string
	code    string
}

var taskFitDefinitions = []taskFitDefinition{
	{keyword: "implement", code: "I"},
	{keyword: "plan", code: "P"},
	{keyword: "research", code: "R"},
	{keyword: "debug", code: "D"},
	{keyword: "audit", code: "A"},
	{keyword: "refactor", code: "F"},
	{keyword: "test", code: "T"},
}

// TaskFitKeywords returns the canonical task-fit keyword order.
func TaskFitKeywords() []string {
	keywords := make([]string, 0, len(taskFitDefinitions))
	for _, definition := range taskFitDefinitions {
		keywords = append(keywords, definition.keyword)
	}
	return keywords
}

// TaskFitCode returns the canonical short display code for a task-fit keyword.
func TaskFitCode(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, definition := range taskFitDefinitions {
		if definition.keyword == value {
			return definition.code, true
		}
	}
	return "", false
}

// IsTaskFitKeyword reports whether value is a valid task-fit keyword.
func IsTaskFitKeyword(value string) bool {
	_, ok := TaskFitCode(value)
	return ok
}

// Split separates comma-delimited predicates while keeping CSV-valued predicates in one predicate.
func Split(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if len(result) > 0 && isCopyrightGuardrailValue(trimmed) && strings.HasPrefix(strings.ToLower(strings.TrimSpace(result[len(result)-1])), "copyright_guardrail:") {
			result[len(result)-1] += "," + trimmed
			continue
		}
		if len(result) > 0 && !IsPredicateStart(trimmed) && strings.HasPrefix(strings.ToLower(strings.TrimSpace(result[len(result)-1])), "task_fit:") {
			result[len(result)-1] += "," + trimmed
			continue
		}
		if len(result) > 0 && continuesTierCSV(trimmed) && strings.HasPrefix(strings.ToLower(strings.TrimSpace(result[len(result)-1])), "tier:") {
			result[len(result)-1] += "," + trimmed
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

// IsPredicateStart reports whether value starts a new structured filter predicate.
func IsPredicateStart(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if isStructuredPredicateStart(lower) {
		return true
	}
	return lower == "paid" || lower == "free" || lower == "scored" || lower == "has-q/p"
}

func continuesTierCSV(value string) bool {
	return tier.IsValid(value) || !IsPredicateStart(value)
}

func isStructuredPredicateStart(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"availability:", "tier:", "task_fit:", "copyright_guardrail:", "quality>=", "context>=", "input<=", "output<="} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// ValidateTiers validates tier predicates in a comma-separated filter.
func ValidateTiers(value string) error {
	for _, raw := range Split(value) {
		predicate := strings.TrimSpace(raw)
		lower := strings.ToLower(predicate)
		if !strings.HasPrefix(lower, "tier:") {
			continue
		}
		for _, rawValue := range strings.Split(strings.TrimSpace(predicate[len("tier:"):]), ",") {
			value := strings.TrimSpace(rawValue)
			if value == "" {
				return fmt.Errorf("tier must not be empty")
			}
			if !tier.IsValid(value) {
				return fmt.Errorf("unknown tier %q; allowed values: %s", value, tier.ValuesString())
			}
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

// ValidateTaskFit validates task-fit predicates in a comma-separated filter.
func ValidateTaskFit(value string) error {
	for _, raw := range Split(value) {
		predicate := strings.TrimSpace(raw)
		lower := strings.ToLower(predicate)
		if !strings.HasPrefix(lower, "task_fit:") {
			continue
		}
		body := strings.TrimSpace(predicate[len("task_fit:"):])
		if body == "" {
			continue
		}
		for _, rawKeyword := range strings.Split(body, ",") {
			keyword := strings.TrimSpace(rawKeyword)
			if keyword == "" || !IsTaskFitKeyword(keyword) {
				return fmt.Errorf("unknown task fit keyword %q; allowed values: %s", keyword, strings.Join(TaskFitKeywords(), ", "))
			}
		}
	}
	return nil
}

func isCopyrightGuardrailValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enforces", "bypasses", "unknown":
		return true
	default:
		return false
	}
}

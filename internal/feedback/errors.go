package feedback

import "fmt"

// ValidationError reports one field-scoped input problem. Field names the
// offending field using the JSON DTO's own names ("model_key", "overall",
// "skills", "skills[].key", "skills[].rating", "review"), so a later
// httpapi decoder can map an error straight onto a structured 400 response
// without re-deriving which field was rejected from error-string parsing.
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("feedback: invalid %s (%q): %s", e.Field, e.Value, e.Message)
}

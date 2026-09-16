package feedback

import (
	"errors"
	"strings"
	"testing"
)

func TestValidationErrorImplementsError(t *testing.T) {
	var err error = &ValidationError{Field: "model_key", Value: "bad key", Message: "must not be empty"}
	msg := err.Error()
	for _, want := range []string{"model_key", "bad key", "must not be empty"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ValidationError.Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestValidationErrorIsExtractableWithErrorsAs(t *testing.T) {
	err := error(&ValidationError{Field: "review", Value: "", Message: "too long"})
	var target *ValidationError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed to extract *ValidationError from %v", err)
	}
	if target.Field != "review" {
		t.Errorf("target.Field = %q, want %q", target.Field, "review")
	}
}

package assert

import (
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/tests/support/act"
)

func Success(t *testing.T, result act.Result) {
	t.Helper()
	if result.Err != nil {
		t.Fatalf("command failed: %v\nstderr: %s\nstdout: %s", result.Err, result.Stderr, result.Stdout)
	}
}

func Failure(t *testing.T, result act.Result, message string) {
	t.Helper()
	if result.Err == nil {
		t.Fatal("command succeeded, want failure")
	}
	if !strings.Contains(result.Stderr, message) {
		t.Fatalf("stderr = %q, want %q", result.Stderr, message)
	}
}

func Contains(t *testing.T, result act.Result, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(result.Stdout, value) {
			t.Errorf("stdout does not contain %q:\n%s", value, result.Stdout)
		}
	}
}

func Version(t *testing.T, result act.Result, version string) {
	t.Helper()
	Success(t, result)
	if result.Stdout != "openrouter version "+version+"\n" {
		t.Fatalf("version output = %q", result.Stdout)
	}
}

func Help(t *testing.T, result act.Result) {
	t.Helper()
	Success(t, result)
	Contains(t, result, "Show local model data as a plain-text table", "path to config.yaml")
}

func Table(t *testing.T, result act.Result) {
	t.Helper()
	Success(t, result)
	Contains(t, result, "Task fit")
}

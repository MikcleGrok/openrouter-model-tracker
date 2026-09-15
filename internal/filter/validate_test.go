package filter

import (
	"reflect"
	"testing"
)

func TestTaskFitKeywordsAreOrderedAndCopied(t *testing.T) {
	want := []string{"implement", "plan", "research", "debug", "audit", "refactor", "test"}
	got := TaskFitKeywords()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TaskFitKeywords = %v, want %v", got, want)
	}
	got[0] = "changed"
	if TaskFitKeywords()[0] != want[0] {
		t.Fatalf("TaskFitKeywords returned mutable backing storage")
	}
	for _, keyword := range want {
		if !IsTaskFitKeyword(keyword) {
			t.Errorf("IsTaskFitKeyword(%q) = false, want true", keyword)
		}
	}
	if code, ok := TaskFitCode(" IMPLEMENT "); !ok || code != "I" {
		t.Fatalf("TaskFitCode(IMPLEMENT) = %q, %t; want I, true", code, ok)
	}
}

func TestValidateTaskFitBoundaries(t *testing.T) {
	for _, value := range []string{
		"task_fit:implement,debug",
		"task_fit:implement,task_fit:debug",
		"task_fit:,paid",
		"paid, task_fit:implement, debug",
		"TASK_FIT: IMPLEMENT , DEBUG",
	} {
		if err := ValidateTaskFit(value); err != nil {
			t.Errorf("ValidateTaskFit(%q) = %v, want nil", value, err)
		}
	}
	for _, value := range []string{"task_fit:implement,freebie", "task_fit:paid-extra", "task_fit:,debug", "paid, task_fit:implement, unknown"} {
		if err := ValidateTaskFit(value); err == nil {
			t.Errorf("ValidateTaskFit(%q) = nil, want error", value)
		}
	}
}

func TestSplitKeepsTaskFitCSVAndRepeatedPredicates(t *testing.T) {
	got := Split("paid, task_fit:implement, debug, task_fit:research, free")
	want := []string{"paid", "task_fit:implement,debug", "task_fit:research", "free"}
	if len(got) != len(want) {
		t.Fatalf("Split = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Split[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

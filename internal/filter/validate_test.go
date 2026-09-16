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

func TestValidateTiersAcceptsCSVAndRejectsEmptyOrUnknownValues(t *testing.T) {
	for _, value := range []string{"tier:opus,haiku", "tier:free", "tier:opus,opus", "tier:opus,has-q/p", "tier:opus,paid", "tier:opus,scored"} {
		if err := ValidateTiers(value); err != nil {
			t.Errorf("ValidateTiers(%q) = %v, want nil", value, err)
		}
	}
	for _, value := range []string{"tier:", "tier:opus,", "tier:,opus", "tier:unknown"} {
		if err := ValidateTiers(value); err == nil {
			t.Errorf("ValidateTiers(%q) = nil, want error", value)
		}
	}
}

func TestSplitKeepsTierCSVAndSeparatesRepeatedPredicates(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "free remains a tier value", value: "tier:opus,free", want: []string{"tier:opus,free"}},
		{name: "has-q/p starts a bare predicate", value: "tier:opus,has-q/p", want: []string{"tier:opus", "has-q/p"}},
		{name: "paid starts a bare predicate", value: "tier:opus,paid", want: []string{"tier:opus", "paid"}},
		{name: "scored starts a bare predicate", value: "tier:opus,scored", want: []string{"tier:opus", "scored"}},
		{name: "repeated tier predicates split", value: "tier:opus,haiku,tier:sonnet", want: []string{"tier:opus,haiku", "tier:sonnet"}},
		{name: "bare legacy predicates remain separate", value: "free,paid,scored", want: []string{"free", "paid", "scored"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Split(tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Split(%q) = %#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}

package feedback

import (
	"reflect"
	"testing"
)

func TestAllowedSkillsReturnsFixedOrderAndIsDefensivelyCopied(t *testing.T) {
	want := []string{"reasoning", "coding", "instruction_following", "long_context"}
	got := AllowedSkills()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedSkills() = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if AllowedSkills()[0] != want[0] {
		t.Fatalf("AllowedSkills() returned mutable backing storage")
	}
}

func TestIsAllowedSkill(t *testing.T) {
	for _, key := range []string{"reasoning", "coding", "instruction_following", "long_context"} {
		if !IsAllowedSkill(key) {
			t.Errorf("IsAllowedSkill(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"", "Reasoning", "REASONING", "debugging", "coding ", " coding", "unknown"} {
		if IsAllowedSkill(key) {
			t.Errorf("IsAllowedSkill(%q) = true, want false", key)
		}
	}
}

func TestAllowedSkillsStringListsEveryAllowedSkill(t *testing.T) {
	want := "reasoning, coding, instruction_following, long_context"
	if got := AllowedSkillsString(); got != want {
		t.Fatalf("AllowedSkillsString() = %q, want %q", got, want)
	}
}

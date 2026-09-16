package feedback

import "strings"

// allowedSkills is the MVP's closed, documented skill vocabulary (plan
// section 4.1: "начальный закрытый словарь навыков должен быть небольшим и
// явно документированным"). A skill key outside this list is rejected by
// ValidateSkillRatings, never silently accepted.
//
// Adding, removing, or renaming a skill here is a contract change: it
// changes what a feedback submission may contain and what an Aggregate can
// report. Update .task/model-feedback-plan/contract.md's skills section in
// the same change, and treat it as a breaking change for anything already
// storing feedback keyed by the old vocabulary.
var allowedSkills = [...]string{
	"reasoning",
	"coding",
	"instruction_following",
	"long_context",
}

// AllowedSkills returns the closed skill vocabulary, in the fixed order
// declared above. Each call returns a fresh copy; mutating the result never
// affects the package's own list or any other caller.
func AllowedSkills() []string {
	return append([]string(nil), allowedSkills[:]...)
}

// IsAllowedSkill reports whether key is exactly one of AllowedSkills. The
// comparison is case-sensitive: a skill key is a stable machine identifier
// used in storage and API payloads, not a user-facing label that should
// tolerate casing variation.
func IsAllowedSkill(key string) bool {
	for _, allowed := range allowedSkills {
		if key == allowed {
			return true
		}
	}
	return false
}

// AllowedSkillsString formats AllowedSkills as a comma-separated list, for
// validation-error and help text.
func AllowedSkillsString() string {
	return strings.Join(allowedSkills[:], ", ")
}

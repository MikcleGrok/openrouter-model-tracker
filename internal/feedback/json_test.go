package feedback

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSkillsMarshalAsArrayNeverAsMap locks in the load-bearing wire shape
// plan 10.1 mandates: "skills" is always a JSON array of {key, rating}
// objects, never an object/map keyed by skill. A later httpapi package
// depends on this shape to be able to detect a duplicate key at all (see
// ValidateSkillRatings) — a map would already have silently dropped it
// during decoding.
func TestSkillsMarshalAsArrayNeverAsMap(t *testing.T) {
	input := FeedbackInput{
		ModelKey: "anthropic/claude-sonnet-5",
		Overall:  4,
		Skills:   []SkillRating{{Key: "coding", Rating: 4}, {Key: "reasoning", Rating: 5}},
		Review:   "solid",
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal(FeedbackInput) error: %v", err)
	}

	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("json.Unmarshal into map[string]json.RawMessage error: %v", err)
	}
	skillsRaw, ok := generic["skills"]
	if !ok {
		t.Fatalf("marshaled FeedbackInput has no top-level %q key: %s", "skills", raw)
	}
	trimmed := strings.TrimSpace(string(skillsRaw))
	if !strings.HasPrefix(trimmed, "[") {
		t.Fatalf("skills field is not a JSON array: %s", trimmed)
	}

	var decoded []SkillRating
	if err := json.Unmarshal(skillsRaw, &decoded); err != nil {
		t.Fatalf("skills field does not decode as []SkillRating: %v (raw: %s)", err, trimmed)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded %d skill entries, want 2 (a map would have kept only unique keys)", len(decoded))
	}
}

// TestSkillRatingsDecodeFromJSONArrayPreservingDuplicates confirms the
// decode half of the same contract: decoding a JSON body whose skills
// array repeats a key produces a Go slice that still contains both
// entries, so ValidateSkillRatings (called on that slice, per its own
// doc comment) can actually see and reject the duplicate.
func TestSkillRatingsDecodeFromJSONArrayPreservingDuplicates(t *testing.T) {
	body := []byte(`{"model_key":"anthropic/claude-sonnet-5","overall":4,"skills":[{"key":"coding","rating":3},{"key":"coding","rating":5}],"review":""}`)
	var input FeedbackInput
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatalf("json.Unmarshal(body, &FeedbackInput) error: %v", err)
	}
	if len(input.Skills) != 2 {
		t.Fatalf("decoded %d skills, want 2 (duplicate key must survive decoding)", len(input.Skills))
	}
	if err := ValidateSkillRatings(input.Skills); err == nil {
		t.Fatalf("ValidateSkillRatings(decoded slice) = nil, want a duplicate-key error")
	}
}

// TestFeedbackFlattensFeedbackInputFields checks the embedded-struct JSON
// shape: Feedback = FeedbackInput plus created_at/updated_at, all at one
// flat level, matching plan 4.1's field list for Feedback exactly (no
// nested "feedback_input" wrapper key).
func TestFeedbackFlattensFeedbackInputFields(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := Feedback{
		FeedbackInput: FeedbackInput{
			ModelKey: "anthropic/claude-sonnet-5",
			Overall:  5,
			Skills:   []SkillRating{{Key: "coding", Rating: 5}},
			Review:   "excellent",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("json.Marshal(Feedback) error: %v", err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	for _, key := range []string{"model_key", "overall", "skills", "review", "created_at", "updated_at"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("marshaled Feedback missing top-level key %q: %s", key, raw)
		}
	}
	if _, ok := generic["feedback_input"]; ok {
		t.Errorf("marshaled Feedback has an unexpected nested %q key: %s", "feedback_input", raw)
	}
}

package feedback

import (
	"strings"
	"testing"
)

func TestNormalizeModelKeyAcceptsRealisticSlugsAndTrims(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want ModelKey
	}{
		{name: "plain namespace/name slug", raw: "anthropic/claude-sonnet-5", want: "anthropic/claude-sonnet-5"},
		{name: "surrounding whitespace trimmed", raw: "  anthropic/claude-opus-4.6  ", want: "anthropic/claude-opus-4.6"},
		{name: "colon variant suffix", raw: "dots-studio/dots-3-note-preview:free", want: "dots-studio/dots-3-note-preview:free"},
		{name: "mixed case and digits", raw: "minimax/MiniMax-M2.5", want: "minimax/MiniMax-M2.5"},
		{name: "underscore allowed", raw: "a_b/c_d", want: "a_b/c_d"},
		{name: "single segment key with no slash", raw: "standalone-key", want: "standalone-key"},
		{name: "exactly max length is accepted", raw: strings.Repeat("a", ModelKeyMaxLength), want: ModelKey(strings.Repeat("a", ModelKeyMaxLength))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeModelKey(tt.raw)
			if err != nil {
				t.Fatalf("NormalizeModelKey(%q) returned error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeModelKey(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeModelKeyRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty string", raw: ""},
		{name: "whitespace only", raw: "   "},
		{name: "too long", raw: strings.Repeat("a", ModelKeyMaxLength+1)},
		{name: "contains parent-directory traversal", raw: "foo/../bar"},
		{name: "leading slash", raw: "/foo/bar"},
		{name: "trailing slash", raw: "foo/bar/"},
		{name: "double slash", raw: "foo//bar"},
		{name: "internal space", raw: "foo bar"},
		{name: "backslash", raw: "foo\\bar"},
		{name: "non-ASCII letters", raw: "модель/пример"},
		{name: "tab control character", raw: "foo\tbar"},
		{name: "newline control character", raw: "foo\nbar"},
		{name: "NUL byte", raw: "foo\x00bar"},
		{name: "question mark", raw: "foo?bar"},
		{name: "at sign", raw: "foo@bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeModelKey(tt.raw)
			if err == nil {
				t.Fatalf("NormalizeModelKey(%q) = %q, nil; want an error", tt.raw, got)
			}
			var ve *ValidationError
			if !isValidationError(err, &ve) {
				t.Fatalf("NormalizeModelKey(%q) error is not a *ValidationError: %v (%T)", tt.raw, err, err)
			}
			if ve.Field != "model_key" {
				t.Errorf("NormalizeModelKey(%q) error field = %q, want %q", tt.raw, ve.Field, "model_key")
			}
		})
	}
}

func TestValidateRatingBoundaries(t *testing.T) {
	for _, rating := range []int{RatingMin, 2, 3, 4, RatingMax} {
		if err := ValidateRating("overall", rating); err != nil {
			t.Errorf("ValidateRating(%q, %d) = %v, want nil", "overall", rating, err)
		}
	}
	for _, rating := range []int{-100, -1, RatingMin - 1, RatingMax + 1, 6, 100} {
		err := ValidateRating("overall", rating)
		if err == nil {
			t.Fatalf("ValidateRating(%q, %d) = nil, want error", "overall", rating)
		}
		var ve *ValidationError
		if !isValidationError(err, &ve) {
			t.Fatalf("ValidateRating(%q, %d) error is not a *ValidationError: %v", "overall", rating, err)
		}
		if ve.Field != "overall" {
			t.Errorf("ValidateRating(%q, %d) error field = %q, want %q", "overall", rating, ve.Field, "overall")
		}
	}
}

func TestValidateSkillRatingsAcceptsValidCombinations(t *testing.T) {
	tests := []struct {
		name   string
		skills []SkillRating
	}{
		{name: "nil slice", skills: nil},
		{name: "empty slice", skills: []SkillRating{}},
		{name: "single valid skill", skills: []SkillRating{{Key: "coding", Rating: 4}}},
		{name: "all allowed skills, each once, at rating boundaries", skills: []SkillRating{
			{Key: "reasoning", Rating: RatingMin},
			{Key: "coding", Rating: RatingMax},
			{Key: "instruction_following", Rating: 3},
			{Key: "long_context", Rating: 2},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSkillRatings(tt.skills); err != nil {
				t.Errorf("ValidateSkillRatings(%v) = %v, want nil", tt.skills, err)
			}
		})
	}
}

func TestValidateSkillRatingsRejectsTooManyEntries(t *testing.T) {
	skills := make([]SkillRating, MaxSkillsPerFeedback+1)
	for i := range skills {
		// Values are intentionally invalid in other ways too (repeated key,
		// unknown key) — the length check must fire first, before either of
		// those is ever reached, so this table isolates that ordering.
		skills[i] = SkillRating{Key: "coding", Rating: 3}
	}
	err := ValidateSkillRatings(skills)
	if err == nil {
		t.Fatalf("ValidateSkillRatings(%d entries) = nil, want error", len(skills))
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("error is not a *ValidationError: %v", err)
	}
	if ve.Field != "skills" {
		t.Errorf("error field = %q, want %q", ve.Field, "skills")
	}
}

// TestValidateSkillRatingsDetectsDuplicateKeyBeforeMapConversion is the
// dedicated duplicate-key test plan 10.1 requires: a repeated skill key
// must be rejected by walking the decoded slice itself, the way a later
// HTTP decoder will call this function directly on the JSON-array result —
// never by first collapsing into a map, which would silently keep only the
// last entry and make the duplicate undetectable.
func TestValidateSkillRatingsDetectsDuplicateKeyBeforeMapConversion(t *testing.T) {
	skills := []SkillRating{
		{Key: "coding", Rating: 3},
		{Key: "reasoning", Rating: 5},
		{Key: "coding", Rating: 4}, // duplicate of the first entry
	}
	err := ValidateSkillRatings(skills)
	if err == nil {
		t.Fatalf("ValidateSkillRatings(%v) = nil, want a duplicate-key error", skills)
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("error is not a *ValidationError: %v", err)
	}
	if ve.Field != "skills[].key" {
		t.Errorf("error field = %q, want %q", ve.Field, "skills[].key")
	}
	if ve.Value != "coding" {
		t.Errorf("error value = %q, want %q", ve.Value, "coding")
	}
	if !strings.Contains(strings.ToLower(ve.Message), "duplicate") {
		t.Errorf("error message = %q, want it to mention duplicate", ve.Message)
	}

	// Sanity check on the premise: naively decoding the same skills into a
	// map would lose the duplicate instead of exposing it, which is exactly
	// why ValidateSkillRatings must not do that internally.
	asMap := make(map[string]int, len(skills))
	for _, s := range skills {
		asMap[s.Key] = s.Rating
	}
	if len(asMap) == len(skills) {
		t.Fatalf("test premise broken: map conversion did not lose the duplicate (got %d keys for %d entries)", len(asMap), len(skills))
	}
}

func TestValidateSkillRatingsRejectsUnknownSkillKey(t *testing.T) {
	tests := []struct {
		name   string
		skills []SkillRating
	}{
		{name: "single unknown skill", skills: []SkillRating{{Key: "debugging", Rating: 3}}},
		{name: "case mismatch is unknown, not a match", skills: []SkillRating{{Key: "Coding", Rating: 3}}},
		{name: "empty key", skills: []SkillRating{{Key: "", Rating: 3}}},
		{name: "unknown key after a valid one", skills: []SkillRating{{Key: "coding", Rating: 3}, {Key: "typing-speed", Rating: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSkillRatings(tt.skills)
			if err == nil {
				t.Fatalf("ValidateSkillRatings(%v) = nil, want error", tt.skills)
			}
			var ve *ValidationError
			if !isValidationError(err, &ve) {
				t.Fatalf("error is not a *ValidationError: %v", err)
			}
			if ve.Field != "skills[].key" {
				t.Errorf("error field = %q, want %q", ve.Field, "skills[].key")
			}
		})
	}
}

func TestValidateSkillRatingsRejectsOutOfRangeRating(t *testing.T) {
	tests := []struct {
		name   string
		skills []SkillRating
	}{
		{name: "zero rating", skills: []SkillRating{{Key: "coding", Rating: 0}}},
		{name: "negative rating", skills: []SkillRating{{Key: "coding", Rating: -1}}},
		{name: "rating above max", skills: []SkillRating{{Key: "coding", Rating: RatingMax + 1}}},
		{name: "valid first entry, invalid second entry", skills: []SkillRating{{Key: "coding", Rating: 3}, {Key: "reasoning", Rating: 9}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSkillRatings(tt.skills)
			if err == nil {
				t.Fatalf("ValidateSkillRatings(%v) = nil, want error", tt.skills)
			}
			var ve *ValidationError
			if !isValidationError(err, &ve) {
				t.Fatalf("error is not a *ValidationError: %v", err)
			}
			if ve.Field != "skills[].rating" {
				t.Errorf("error field = %q, want %q", ve.Field, "skills[].rating")
			}
		})
	}
}

func TestNormalizeReviewTrimsAndPreservesInternalContent(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty stays empty", raw: "", want: ""},
		{name: "whitespace only becomes empty", raw: "   \t\n  ", want: ""},
		{name: "outer whitespace trimmed", raw: "  hello world  ", want: "hello world"},
		{name: "internal whitespace and newlines preserved", raw: "  line one\nline two   with   spaces  \n  ", want: "line one\nline two   with   spaces"},
		{name: "unicode content preserved verbatim", raw: "  Привет, мир! 😀  ", want: "Привет, мир! 😀"},
		{name: "internal case preserved", raw: "  MiXeD CaSe Review  ", want: "MiXeD CaSe Review"},
		{name: "exactly max length rune count is accepted", raw: strings.Repeat("字", ReviewMaxLength), want: strings.Repeat("字", ReviewMaxLength)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeReview(tt.raw)
			if err != nil {
				t.Fatalf("NormalizeReview(%q) returned error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeReview(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeReviewRejectsTooLong(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "one over the limit, ASCII", raw: strings.Repeat("a", ReviewMaxLength+1)},
		{name: "one over the limit, multi-byte runes", raw: strings.Repeat("字", ReviewMaxLength+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeReview(tt.raw)
			if err == nil {
				t.Fatalf("NormalizeReview(%d runes) = nil, want error", len([]rune(tt.raw)))
			}
			var ve *ValidationError
			if !isValidationError(err, &ve) {
				t.Fatalf("error is not a *ValidationError: %v", err)
			}
			if ve.Field != "review" {
				t.Errorf("error field = %q, want %q", ve.Field, "review")
			}
		})
	}
}

func TestNewFeedbackInputValidCompositeAndDefensiveCopy(t *testing.T) {
	skills := []SkillRating{{Key: "coding", Rating: 4}, {Key: "reasoning", Rating: 5}}
	got, err := NewFeedbackInput("  anthropic/claude-sonnet-5  ", 4, skills, "  great model  ")
	if err != nil {
		t.Fatalf("NewFeedbackInput(...) returned error: %v", err)
	}
	want := FeedbackInput{
		ModelKey: "anthropic/claude-sonnet-5",
		Overall:  4,
		Skills:   []SkillRating{{Key: "coding", Rating: 4}, {Key: "reasoning", Rating: 5}},
		Review:   "great model",
	}
	if got.ModelKey != want.ModelKey || got.Overall != want.Overall || got.Review != want.Review {
		t.Fatalf("NewFeedbackInput(...) = %+v, want %+v", got, want)
	}
	if len(got.Skills) != len(want.Skills) {
		t.Fatalf("NewFeedbackInput(...).Skills = %v, want %v", got.Skills, want.Skills)
	}
	for i := range want.Skills {
		if got.Skills[i] != want.Skills[i] {
			t.Errorf("NewFeedbackInput(...).Skills[%d] = %v, want %v", i, got.Skills[i], want.Skills[i])
		}
	}

	// The returned Skills must not alias the caller's slice: mutating the
	// input after the call must not change the result.
	skills[0].Rating = 1
	if got.Skills[0].Rating != 4 {
		t.Errorf("NewFeedbackInput(...) result aliases the caller's skills slice: got.Skills[0].Rating = %d after caller mutation, want unaffected 4", got.Skills[0].Rating)
	}
}

func TestNewFeedbackInputStopsAtFirstFailingFieldInFixedOrder(t *testing.T) {
	validSkills := []SkillRating{{Key: "coding", Rating: 3}}

	t.Run("invalid model_key reported even when overall is also invalid", func(t *testing.T) {
		_, err := NewFeedbackInput("", 0, validSkills, "")
		var ve *ValidationError
		if !isValidationError(err, &ve) {
			t.Fatalf("error is not a *ValidationError: %v", err)
		}
		if ve.Field != "model_key" {
			t.Errorf("error field = %q, want %q", ve.Field, "model_key")
		}
	})

	t.Run("invalid overall reported when model_key is valid", func(t *testing.T) {
		_, err := NewFeedbackInput("anthropic/claude-sonnet-5", 0, validSkills, "")
		var ve *ValidationError
		if !isValidationError(err, &ve) {
			t.Fatalf("error is not a *ValidationError: %v", err)
		}
		if ve.Field != "overall" {
			t.Errorf("error field = %q, want %q", ve.Field, "overall")
		}
	})

	t.Run("invalid skills reported when model_key and overall are valid", func(t *testing.T) {
		_, err := NewFeedbackInput("anthropic/claude-sonnet-5", 4, []SkillRating{{Key: "unknown", Rating: 3}}, "")
		var ve *ValidationError
		if !isValidationError(err, &ve) {
			t.Fatalf("error is not a *ValidationError: %v", err)
		}
		if ve.Field != "skills[].key" {
			t.Errorf("error field = %q, want %q", ve.Field, "skills[].key")
		}
	})

	t.Run("invalid review reported only once everything else is valid", func(t *testing.T) {
		_, err := NewFeedbackInput("anthropic/claude-sonnet-5", 4, validSkills, strings.Repeat("a", ReviewMaxLength+1))
		var ve *ValidationError
		if !isValidationError(err, &ve) {
			t.Fatalf("error is not a *ValidationError: %v", err)
		}
		if ve.Field != "review" {
			t.Errorf("error field = %q, want %q", ve.Field, "review")
		}
	})

	t.Run("all valid returns no error", func(t *testing.T) {
		if _, err := NewFeedbackInput("anthropic/claude-sonnet-5", 4, validSkills, "fine"); err != nil {
			t.Errorf("NewFeedbackInput(...) = %v, want nil", err)
		}
	})
}

// isValidationError is a small errors.As wrapper so every test above can
// assert both "is a *ValidationError" and inspect its fields in one call.
func isValidationError(err error, target **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if !ok {
		return false
	}
	*target = ve
	return true
}

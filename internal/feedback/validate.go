package feedback

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// NewFeedbackInput normalizes and validates raw, caller-supplied feedback
// fields into a FeedbackInput. It is the single entry point a later decoder
// (an HTTP JSON body, a TUI form) should call: it runs every rule this
// package defines, in the fixed order model_key, overall, skills, review,
// and returns the first error encountered rather than trying to collect
// them all — so behavior stays deterministic and easy to test, and no
// caller can accidentally reach the SQL/HTTP layer with an unvalidated
// skills slice by going through some other path.
//
// The returned FeedbackInput never aliases the caller's skills slice.
func NewFeedbackInput(rawModelKey string, overall int, skills []SkillRating, rawReview string) (FeedbackInput, error) {
	modelKey, err := NormalizeModelKey(rawModelKey)
	if err != nil {
		return FeedbackInput{}, err
	}
	if err := ValidateRating("overall", overall); err != nil {
		return FeedbackInput{}, err
	}
	if err := ValidateSkillRatings(skills); err != nil {
		return FeedbackInput{}, err
	}
	review, err := NormalizeReview(rawReview)
	if err != nil {
		return FeedbackInput{}, err
	}
	return FeedbackInput{
		ModelKey: modelKey,
		Overall:  overall,
		Skills:   append([]SkillRating(nil), skills...),
		Review:   review,
	}, nil
}

// NormalizeModelKey trims raw and validates it against the model_key
// policy (plan 10.1: "trim, Unicode/ASCII policy, длину и отсутствие
// control chars/path separators"), returning the normalized ModelKey.
//
// The policy fixed here: trim surrounding whitespace; reject empty; reject
// longer than ModelKeyMaxLength runes; allow only ASCII letters, digits,
// and the punctuation "-_.:/" (the observed charset of real OpenRouter
// slugs, "namespace/name[:variant]") — which rejects every control
// character and every non-ASCII rune as a side effect of being an
// allow-list, not a separate check; and reject "..", a leading or trailing
// "/", and "//", so a model_key can never look like a path-traversal or
// malformed-path string even though a single internal "/" is part of the
// normal slug grammar.
func NormalizeModelKey(raw string) (ModelKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", &ValidationError{Field: "model_key", Value: raw, Message: "must not be empty"}
	}
	if utf8.RuneCountInString(trimmed) > ModelKeyMaxLength {
		return "", &ValidationError{Field: "model_key", Value: trimmed, Message: fmt.Sprintf("must be at most %d characters", ModelKeyMaxLength)}
	}
	if strings.Contains(trimmed, "..") {
		return "", &ValidationError{Field: "model_key", Value: trimmed, Message: `must not contain ".."`}
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasSuffix(trimmed, "/") || strings.Contains(trimmed, "//") {
		return "", &ValidationError{Field: "model_key", Value: trimmed, Message: `must not start or end with "/" or contain "//"`}
	}
	for _, r := range trimmed {
		if !isAllowedModelKeyRune(r) {
			return "", &ValidationError{Field: "model_key", Value: trimmed, Message: fmt.Sprintf("contains disallowed character %q", r)}
		}
	}
	return ModelKey(trimmed), nil
}

func isAllowedModelKeyRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '-' || r == '_' || r == '.' || r == ':' || r == '/':
		return true
	default:
		return false
	}
}

// ValidateRating reports whether value is a valid RatingMin..RatingMax
// integer rating, returning a *ValidationError naming field ("overall" or
// "skills[].rating") when it is not.
func ValidateRating(field string, value int) error {
	if value < RatingMin || value > RatingMax {
		return &ValidationError{
			Field:   field,
			Value:   strconv.Itoa(value),
			Message: fmt.Sprintf("must be an integer between %d and %d", RatingMin, RatingMax),
		}
	}
	return nil
}

// ValidateSkillRatings validates a decoded skills slice in place: it is the
// function a later HTTP decoder calls directly on the []SkillRating that
// encoding/json produced from the request's JSON array, before any
// conversion to a map. That ordering is load-bearing (plan 10.1): decoding
// duplicate-keyed JSON objects into a Go map silently keeps only the last
// one, destroying the very evidence a duplicate-key rejection needs, so
// this function walks the slice itself with a seen-set used only for
// detection, never for storage.
//
// It checks, per element in slice order: the MaxSkillsPerFeedback length
// bound; whether this Key repeats an earlier element (duplicate key,
// rejected outright, regardless of whether either rating is otherwise
// valid); whether Key is in AllowedSkills; and whether Rating is a valid
// RatingMin..RatingMax integer. It returns the first violation found.
func ValidateSkillRatings(skills []SkillRating) error {
	if len(skills) > MaxSkillsPerFeedback {
		return &ValidationError{
			Field:   "skills",
			Value:   strconv.Itoa(len(skills)),
			Message: fmt.Sprintf("at most %d skill ratings are allowed", MaxSkillsPerFeedback),
		}
	}
	seen := make(map[string]bool, len(skills))
	for _, skill := range skills {
		if seen[skill.Key] {
			return &ValidationError{Field: "skills[].key", Value: skill.Key, Message: "duplicate skill key"}
		}
		seen[skill.Key] = true
		if !IsAllowedSkill(skill.Key) {
			return &ValidationError{
				Field:   "skills[].key",
				Value:   skill.Key,
				Message: "unknown skill key; allowed: " + AllowedSkillsString(),
			}
		}
		if err := ValidateRating("skills[].rating", skill.Rating); err != nil {
			return err
		}
	}
	return nil
}

// NormalizeReview trims raw's surrounding whitespace and validates its
// length, returning the normalized review text. review is optional (plan
// 4.1: "необязательный review"), so an empty result after trimming is
// valid.
//
// Beyond trimming, the Unicode content of the review is preserved exactly
// as submitted (plan 10.1: "сохранить исходную Unicode-текстовость отзыва
// после trim") — no internal whitespace collapsing, no case folding, no
// script normalization. Two things plan 10.1 also mentions are explicitly
// NOT done here, and remain open work for the tasks that own them: escaping
// for HTML rendering ("не HTML-renderить отзыв без экранирования" is a
// rendering-layer responsibility, not a storage-normalization one) and
// terminal-safe sanitization of stray ANSI/control sequences before display
// (plan section 15 step 1's "terminal-safe sanitizer" is its own, more
// involved piece of work for whichever task renders review text in the
// TUI). A stored/validated review here may still contain characters that
// are unsafe to print to a terminal or to a browser unescaped.
func NormalizeReview(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if utf8.RuneCountInString(trimmed) > ReviewMaxLength {
		return "", &ValidationError{
			Field:   "review",
			Value:   "",
			Message: fmt.Sprintf("must be at most %d characters", ReviewMaxLength),
		}
	}
	return trimmed, nil
}

package feedback

// RatingMin and RatingMax bound overall and every skill rating: both must
// be an integer in this closed range (plan 10.1: "overall и skill ratings
// должны быть integer 1-5"). Decoding into the Go `int` fields on
// FeedbackInput/SkillRating already rejects a fractional or string JSON
// value (encoding/json errors on those type mismatches for an int target);
// a JSON null or an omitted field decodes to the Go zero value 0, which
// this range then rejects too, since 0 < RatingMin. See ValidateRating.
const (
	RatingMin = 1
	RatingMax = 5
)

// ModelKeyMaxLength is the maximum rune length of a normalized ModelKey.
// Real OpenRouter slugs ("namespace/name[:variant]") observed in this repo
// run well under 100 characters; 200 leaves headroom for longer future
// namespaces without opening the field up to arbitrary-size input.
const ModelKeyMaxLength = 200

// ReviewMaxLength is the maximum rune length (not byte length — a review is
// free text and may be in any script) of a normalized review. 2000 runes is
// generous for a model review while keeping a single review bounded well
// under MaxRequestBodyBytes.
const ReviewMaxLength = 2000

// MaxSkillsPerFeedback bounds how many skill ratings one Feedback may
// carry. It equals the size of the allowed-skills vocabulary: since
// ValidateSkillRatings rejects a duplicate skill key, no valid submission
// can ever need more entries than there are skills to rate, so the limit is
// derived rather than a second number to keep in sync by hand.
const MaxSkillsPerFeedback = len(allowedSkills)

// MaxRequestBodyBytes bounds the HTTP request body a later httpapi decoder
// should accept for a feedback submission, before this package's field
// limits are even checked. 16 KiB comfortably covers ReviewMaxLength (2000
// runes is at most 8000 UTF-8 bytes) plus MaxSkillsPerFeedback entries and
// JSON overhead, with headroom to spare.
//
// This constant exists here, not in a config file, because it is a
// contract limit (what a well-formed request looks like), not an
// operational tuning knob — matching plan 10.1's "лимиты вынести в
// доменные константы, а не в конфиг без необходимости". It is unused by
// this package itself: internal/feedback has no HTTP layer. It is defined
// here so the httpapi task does not have to invent or re-derive the number.
const MaxRequestBodyBytes = 16 * 1024

// Package feedback defines the feedback domain: the types, the closed
// allowed-skill vocabulary, and the validation rules for user ratings of a
// model, independent of how they are stored (internal/feedback/sqlite) or
// served over HTTP (internal/feedback/httpapi) — neither of which exists
// yet; this package has no dependency on either.
//
// Every exact value this package fixes — the skill vocabulary, the numeric
// limits, the community-position formula, and the DTO field names — is
// restated in one place at .task/model-feedback-plan/contract.md. Later
// tasks should read values from there instead of re-deriving them from the
// full plan or from this package's source.
package feedback

import "time"

// ModelKey is a normalized, stable model identifier. It is initially the
// OpenRouter catalogue slug (internal/model.Model.Slug), but is kept as a
// distinct type rather than a bare string so a feedback record can never be
// accidentally keyed by a display name or a price, both of which change
// over time while the identity of "which model this is" does not.
//
// A ModelKey is never the zero value once normalized: NormalizeModelKey
// rejects an empty key. The zero value of the type ("") exists only as an
// unnormalized placeholder and must not be treated as a valid key by any
// caller.
type ModelKey string

// SkillRating is one rating for one skill: a stable, closed-vocabulary skill
// key (see AllowedSkills) and a 1-5 integer rating. It is also the wire
// shape for the "skills" JSON field: always an array of these objects,
// [{"key":"...","rating":N}, ...], never a JSON object/map keyed by skill.
// A map would let encoding/json silently drop a duplicate key before this
// package ever sees it; the array shape is what lets ValidateSkillRatings
// detect that duplicate and reject it instead.
type SkillRating struct {
	Key    string `json:"key"`
	Rating int    `json:"rating"`
}

// FeedbackInput is what a caller submits: everything about a Feedback that
// the client controls. It deliberately excludes CreatedAt/UpdatedAt — those
// are server time, never trusted from client input (see Feedback) — and it
// carries no identity: which identity is submitting is a service-layer
// concern (upsert key (identity_id, model_key), Task 2+), not a domain-type
// field.
//
// Construct a FeedbackInput through NewFeedbackInput, which runs every
// validation/normalization rule this package defines in one deterministic
// order. Do not build one by hand from unvalidated data and skip that call.
type FeedbackInput struct {
	ModelKey ModelKey      `json:"model_key"`
	Overall  int           `json:"overall"`
	Skills   []SkillRating `json:"skills"`
	Review   string        `json:"review,omitempty"`
}

// Feedback is a stored feedback record: a validated FeedbackInput plus the
// server-assigned timestamps. CreatedAt and UpdatedAt are always server
// time — a client-supplied timestamp is never a source of truth for either
// field, per the plan's upsert semantics (repeated submission from the same
// identity updates one row, it does not create a vote).
type Feedback struct {
	FeedbackInput
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OwnFeedback is Feedback as returned to the identity that submitted it —
// the "mine" record in FeedbackSummary and the personal read/write
// endpoints. It is a distinct named type, not an alias, because it answers
// a different question than Feedback does ("what is *my* feedback on this
// model" vs. "a feedback record"), even though the two happen to share a
// shape today. It carries no internal identity key: Feedback itself never
// had one to leak in the first place, so nothing has to be stripped here —
// the type exists to make the "no identity key" guarantee explicit and
// checkable at the call site, not to implement it.
type OwnFeedback Feedback

// RatingDistribution counts how many ratings landed on each value 1-5.
// Index 0 holds the count for rating 1 ... index RatingMax-1 holds the
// count for rating RatingMax. It marshals as a plain 5-element JSON array;
// use Count/Set rather than indexing directly so the rating-to-index
// offset-by-one never has to be re-derived at a call site.
type RatingDistribution [RatingMax]int

// Count returns how many ratings equal the given 1-5 rating value. It
// returns 0 for any rating outside [RatingMin, RatingMax] rather than
// panicking, since a distribution is often read speculatively (e.g. by a
// renderer iterating 1..5 without re-validating the bound first).
func (d RatingDistribution) Count(rating int) int {
	if rating < RatingMin || rating > RatingMax {
		return 0
	}
	return d[rating-RatingMin]
}

// Set stores count as the number of ratings equal to the given 1-5 rating
// value. It is a no-op for a rating outside [RatingMin, RatingMax].
func (d *RatingDistribution) Set(rating, count int) {
	if rating < RatingMin || rating > RatingMax {
		return
	}
	d[rating-RatingMin] = count
}

// SkillAggregate is one allowed skill's aggregate across the ratings that
// included it. Count can differ between skills, and can differ from the
// overall Aggregate.Count, because a Feedback may rate any subset of the
// allowed skills (including none).
type SkillAggregate struct {
	Key     string  `json:"key"`
	Count   int     `json:"count"`
	Average float64 `json:"average"`
}

// Aggregate is the community rating for one model: how many valid ratings
// exist, their average, their 1-5 distribution, and the per-skill averages
// — plus ComputedAt, the timestamp of the slice/computation this snapshot
// represents. It never distinguishes one identity's contribution: an
// Aggregate answers "what does the community think", not "what does
// identity X think" (that is OwnFeedback/FeedbackInput's job).
//
// Skills lists only the allowed skills that received at least one rating in
// this aggregate's window; an allowed skill with zero ratings is simply
// absent from the slice rather than appearing with Count 0, so a caller
// does not have to distinguish "rated zero" from "never rated" by field
// value.
type Aggregate struct {
	Count        int                `json:"count"`
	Average      float64            `json:"average"`
	Distribution RatingDistribution `json:"distribution"`
	Skills       []SkillAggregate   `json:"skills"`
	ComputedAt   time.Time          `json:"computed_at"`
}

// FeedbackSummary is the user-facing view of one model's feedback: the
// caller's own rating, the community aggregate, and — only when the caller
// explicitly asked for it — the community aggregate excluding the caller's
// own contribution. The three are kept as separate nullable fields and are
// never combined into one score: a UI or API consumer that conflates
// "mine" with "the community's opinion" is exactly the bug plan section 1.4
// exists to prevent.
//
// Mine and Community are always present as JSON keys, null when there is no
// applicable data (no personal rating yet; zero community ratings yet).
// Others is populated, and should be included in the response at all, only
// when the caller explicitly requested it — that inclusion/omission
// decision belongs to the httpapi layer that does not exist yet, so this
// type only defines the field; whether omitempty applies for a given
// response is that layer's call.
type FeedbackSummary struct {
	ModelKey  ModelKey     `json:"model_key"`
	Mine      *OwnFeedback `json:"mine"`
	Community *Aggregate   `json:"community"`
	Others    *Aggregate   `json:"others,omitempty"`
}

// PositionStatus explains why a Position does or does not carry a value. A
// missing/zero position is never silently ambiguous with "position 1" —
// every Position states explicitly which of these three cases it is in.
type PositionStatus string

const (
	// PositionStatusRanked means Position.Value holds a real, usable
	// position.
	PositionStatusRanked PositionStatus = "ranked"
	// PositionStatusUnranked means there is nothing to rank: e.g. a
	// personal_position with no "mine" rating on this model yet, or a
	// base_position for a model outside the current benchmark ranking.
	PositionStatusUnranked PositionStatus = "unranked"
	// PositionStatusIneligible means a position could be computed but
	// policy withholds it: e.g. community_position when the community
	// sample is below CommunityMinSampleForPosition (n<5 per plan 4.1).
	PositionStatusIneligible PositionStatus = "ineligible"
)

// Position is one position value plus the status explaining its presence
// or absence. Value is meaningful only when Status is PositionStatusRanked;
// it is omitted from JSON otherwise (a bare 0 could otherwise be misread as
// a real rank).
type Position struct {
	Value  int            `json:"value,omitempty"`
	Status PositionStatus `json:"status"`
}

// ModelPositions bundles a model's three independent position signals.
// BasePosition is the existing benchmark/ranking position; feedback never
// replaces or blends into it. PersonalPosition is this identity's own
// "Мои оценки" ordering: only models with "mine" participate, ranked by
// that rating with the existing ranking as tie-breaker, and an unrated
// model gets no fabricated score — it is simply PositionStatusUnranked, not
// sorted in among rated models. CommunityPosition is the single deterministic
// Bayesian-shrinkage social ordering (see CommunityScore); it never changes
// the default global ranking, only an explicitly separate social sort.
type ModelPositions struct {
	ModelKey          ModelKey `json:"model_key"`
	BasePosition      Position `json:"base_position"`
	PersonalPosition  Position `json:"personal_position"`
	CommunityPosition Position `json:"community_position"`
}

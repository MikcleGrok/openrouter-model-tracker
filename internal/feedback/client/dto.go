package client

import "time"

// The types in this file mirror internal/feedback/httpapi/dto.go's own
// wire-shape types field-for-field (same JSON names, same nullability via
// pointers/omitempty) but are a distinct set of Go types, not an import of
// httpapi's (unexported) DTOs — see doc.go's "Deliberately decoupled"
// section for why. Read httpapi/dto.go before changing anything here: it is
// the actual, reviewed, tested contract; this file must always match it,
// not the plan's illustrative example.

// SkillRating is one rated skill: a closed-vocabulary key (the server
// defines and validates the allowed set; this client does not duplicate
// that list) and a 1-5 integer rating.
type SkillRating struct {
	Key    string `json:"key"`
	Rating int    `json:"rating"`
}

// FeedbackRequest is PUT /v1/models/{model_key}/feedback's request body
// (httpapi/dto.go's feedbackRequestDTO). model_key is never part of the
// body — it is always the URL path argument to Client.PutFeedback.
type FeedbackRequest struct {
	Overall int           `json:"overall"`
	Skills  []SkillRating `json:"skills"`
	Review  string        `json:"review"`
}

// OwnFeedback is one identity's own feedback on a model, as returned by both
// GetOwnFeedback ("own_feedback") and embedded in Summary ("mine"). It never
// carries an identity — matching httpapi/dto.go's ownFeedbackDTO, which
// never had one to leak.
type OwnFeedback struct {
	Overall   int           `json:"overall"`
	Skills    []SkillRating `json:"skills"`
	Review    string        `json:"review"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// SkillAggregate is one allowed skill's community aggregate. Average is a
// plain float64 (never null) because a SkillAggregate only ever exists for a
// skill with at least one rating — an unrated skill is simply absent from
// Aggregate.Skills.
type SkillAggregate struct {
	Key     string  `json:"key"`
	Count   int     `json:"count"`
	Average float64 `json:"average"`
}

// Aggregate is the community aggregate shape used by both Summary.Community
// and Summary.Others (httpapi/dto.go's aggregateDTO). Average is a pointer:
// nil ("null" on the wire) means Count is 0, so a caller can never mistake
// "no ratings yet" for a real average of 0.
type Aggregate struct {
	Count        int              `json:"count"`
	Average      *float64         `json:"average"`
	Distribution [5]int           `json:"distribution"`
	Skills       []SkillAggregate `json:"skills"`
	ComputedAt   time.Time        `json:"computed_at"`
}

// PositionStatus explains why a Position does or does not carry a Value.
type PositionStatus string

const (
	// PositionRanked means Position.Value holds a real, usable position.
	PositionRanked PositionStatus = "ranked"
	// PositionUnranked means there is nothing to rank (e.g. no personal
	// rating yet, or — for BasePosition specifically, see the field's own
	// doc comment below — the server simply has no ranking source).
	PositionUnranked PositionStatus = "unranked"
	// PositionIneligible means a position could be computed but policy
	// withholds it (e.g. too few community ratings).
	PositionIneligible PositionStatus = "ineligible"
)

// Position is one position value plus the status explaining its presence or
// absence. Value is meaningful only when Status is PositionRanked.
type Position struct {
	Value  int            `json:"value,omitempty"`
	Status PositionStatus `json:"status"`
}

// Summary is the shared response shape for both PutFeedback's 200 and
// GetSummary's 200 (httpapi/dto.go's summaryResponseDTO). Others is nil
// unless GetSummary was called with IncludeOthers.
//
// BasePosition is always reported PositionUnranked by this server (Task 4's
// own reviewed design: the feedback-server is a separate process with no
// access to the OpenRouter benchmark ranking, plan 3.1). This client passes
// that value through exactly as received — it never overrides or "fixes"
// it. A caller that wants a real base_position must merge in its own
// locally-known ranking itself; this package does not attempt that.
type Summary struct {
	ModelKey          string       `json:"model_key"`
	Mine              *OwnFeedback `json:"mine"`
	Community         *Aggregate   `json:"community"`
	Others            *Aggregate   `json:"others,omitempty"`
	BasePosition      Position     `json:"base_position"`
	PersonalPosition  Position     `json:"personal_position"`
	CommunityPosition Position     `json:"community_position"`
}

// OwnFeedbackResponse is GetOwnFeedback's 200 response shape
// (httpapi/dto.go's ownFeedbackResponseDTO): the queried model_key plus
// own_feedback, nil when the identity has never rated this model.
type OwnFeedbackResponse struct {
	ModelKey    string       `json:"model_key"`
	OwnFeedback *OwnFeedback `json:"own_feedback"`
}

// deleteStatusDTO is the fixed body of DeleteMe's 202 response
// (httpapi/dto.go's deleteStatusResponseDTO).
type deleteStatusDTO struct {
	Status string `json:"status"`
}

// ValidationField reports one field-scoped input problem from a 400
// response (httpapi/dto.go's validationFieldDTO).
type ValidationField struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// errorResponseDTO is the body of every non-2xx JSON error response the
// server writes (httpapi/dto.go's errorResponseDTO).
type errorResponseDTO struct {
	Error  string            `json:"error"`
	Fields []ValidationField `json:"fields,omitempty"`
}

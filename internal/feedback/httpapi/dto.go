package httpapi

import (
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// skillRatingDTO is the wire shape of one element of the "skills" JSON
// array, on both the request (PUT feedback) and response (own_feedback,
// mine) sides. It is a distinct type from feedback.SkillRating even though
// the two happen to share a shape today (doc.go: DTOs are never a domain
// type reused on the wire).
type skillRatingDTO struct {
	Key    string `json:"key"`
	Rating int    `json:"rating"`
}

// feedbackRequestDTO is PUT /v1/models/{model_key}/feedback's request body
// (plan 4.2). model_key is deliberately absent here — it comes from the URL
// path only, never duplicated in the body — so decoding this struct with
// DisallowUnknownFields rejects a body that redundantly (and potentially
// inconsistently) repeats it.
type feedbackRequestDTO struct {
	Overall int              `json:"overall"`
	Skills  []skillRatingDTO `json:"skills"`
	Review  string           `json:"review"`
}

// toSkillRatings converts a decoded []skillRatingDTO straight into
// []feedback.SkillRating, preserving element order and never passing
// through a map — the ordering contract.md §5 calls load-bearing for
// ValidateSkillRatings' duplicate-key detection.
func toSkillRatings(dtos []skillRatingDTO) []feedback.SkillRating {
	if dtos == nil {
		return nil
	}
	out := make([]feedback.SkillRating, len(dtos))
	for i, d := range dtos {
		out[i] = feedback.SkillRating{Key: d.Key, Rating: d.Rating}
	}
	return out
}

// skillRatingDTOs converts a domain []feedback.SkillRating into its wire
// shape, always returning a non-nil (possibly empty) slice so the field
// marshals as "[]" rather than "null" for a rating with no skills.
func skillRatingDTOs(skills []feedback.SkillRating) []skillRatingDTO {
	out := make([]skillRatingDTO, 0, len(skills))
	for _, s := range skills {
		out = append(out, skillRatingDTO{Key: s.Key, Rating: s.Rating})
	}
	return out
}

// ownFeedbackDTO is one identity's own feedback as returned by both
// GET .../feedback/me (as "own_feedback") and GET .../feedback/summary (as
// "mine"). It deliberately omits model_key: the endpoint is already scoped
// to one model_key (from the URL), and the response envelope that embeds
// this type states it once at the top level rather than repeating it here.
// It never carries an identity key — matching feedback.OwnFeedback's own
// "never had one to leak" guarantee (model.go).
type ownFeedbackDTO struct {
	Overall   int              `json:"overall"`
	Skills    []skillRatingDTO `json:"skills"`
	Review    string           `json:"review"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// newOwnFeedbackDTO maps a domain OwnFeedback to its wire shape. Returns nil
// for a nil input, matching the JSON null a caller with no rating yet sees.
func newOwnFeedbackDTO(fb *feedback.OwnFeedback) *ownFeedbackDTO {
	if fb == nil {
		return nil
	}
	return &ownFeedbackDTO{
		Overall:   fb.Overall,
		Skills:    skillRatingDTOs(fb.Skills),
		Review:    fb.Review,
		CreatedAt: fb.CreatedAt,
		UpdatedAt: fb.UpdatedAt,
	}
}

// skillAggregateDTO is one allowed skill's community aggregate. Average is
// a plain (never-null) float64 because a skillAggregateDTO only ever exists
// for a skill with at least one rating (contract §5: an unrated skill is
// simply absent from the slice) — unlike the whole aggregate's overall
// Average, which can legitimately have zero contributing ratings.
type skillAggregateDTO struct {
	Key     string  `json:"key"`
	Count   int     `json:"count"`
	Average float64 `json:"average"`
}

// aggregateDTO is the community aggregate wire shape used by "community"
// and "others" in both the PUT response and the summary response (plan
// 4.4, contract §5's Aggregate). Average is a pointer so a zero-rating
// aggregate serializes "average": null rather than a misleading "0" (plan
// 4.4: "При count == 0 средние значения должны быть null, а распределение
// нулевым, чтобы 0 не выглядело валидной оценкой").
type aggregateDTO struct {
	Count        int                 `json:"count"`
	Average      *float64            `json:"average"`
	Distribution [5]int              `json:"distribution"`
	Skills       []skillAggregateDTO `json:"skills"`
	ComputedAt   time.Time           `json:"computed_at"`
}

// newAggregateDTO maps a domain Aggregate to its wire shape. Returns nil for
// a nil input (used when "others" was not requested, or is not yet
// applicable), matching the JSON null the field carries in that case.
func newAggregateDTO(agg *feedback.Aggregate) *aggregateDTO {
	if agg == nil {
		return nil
	}
	dto := aggregateDTO{
		Count:      agg.Count,
		ComputedAt: agg.ComputedAt,
	}
	if agg.Count > 0 {
		avg := agg.Average
		dto.Average = &avg
	}
	for i := 0; i < 5; i++ {
		dto.Distribution[i] = agg.Distribution.Count(i + 1)
	}
	dto.Skills = make([]skillAggregateDTO, 0, len(agg.Skills))
	for _, s := range agg.Skills {
		dto.Skills = append(dto.Skills, skillAggregateDTO{Key: s.Key, Count: s.Count, Average: s.Average})
	}
	return &dto
}

// positionDTO mirrors feedback.Position's own shape (value present only
// when ranked) as a distinct wire type.
type positionDTO struct {
	Value  int    `json:"value,omitempty"`
	Status string `json:"status"`
}

func newPositionDTO(p feedback.Position) positionDTO {
	return positionDTO{Value: p.Value, Status: string(p.Status)}
}

// summaryResponseDTO is the shared response shape for both PUT
// .../feedback's 200 (plan 4.2: "нормализованные mine, community и
// позиционные signals") and GET .../feedback/summary's 200 (plan 4.4).
// Others is nil unless the summary endpoint's caller explicitly asked for
// it (?others=true); PUT never populates it.
type summaryResponseDTO struct {
	ModelKey          string          `json:"model_key"`
	Mine              *ownFeedbackDTO `json:"mine"`
	Community         *aggregateDTO   `json:"community"`
	Others            *aggregateDTO   `json:"others,omitempty"`
	BasePosition      positionDTO     `json:"base_position"`
	PersonalPosition  positionDTO     `json:"personal_position"`
	CommunityPosition positionDTO     `json:"community_position"`
}

// ownFeedbackResponseDTO is GET /v1/models/{model_key}/feedback/me's 200
// response shape (plan 4.3): the queried model_key plus own_feedback, null
// when identity has never rated this model. It never contains identity_id.
type ownFeedbackResponseDTO struct {
	ModelKey    string          `json:"model_key"`
	OwnFeedback *ownFeedbackDTO `json:"own_feedback"`
}

// deleteStatusResponseDTO is the fixed body of a 202 response from
// DELETE /v1/me/feedback (plan 4.7: "202 с JSON-статусом cleanup_pending").
type deleteStatusResponseDTO struct {
	Status string `json:"status"`
}

// errorResponseDTO is the body of every non-2xx JSON error response this
// package writes. Message is always a short, generic, static string chosen
// by the handler — never an underlying error's own text — so a 500/503
// response can never leak a SQL error, filesystem path, or token (plan
// 4.2's "503/500 не должны раскрывать SQL, filesystem path или token").
// Field/Value/Message-shaped validation detail is carried separately in
// Errors for a 400, so a client can programmatically tell which input was
// rejected without parsing prose.
type errorResponseDTO struct {
	Error  string               `json:"error"`
	Fields []validationFieldDTO `json:"fields,omitempty"`
}

// validationFieldDTO reports one *feedback.ValidationError on a 400
// response, using the same JSON field names the domain error already names
// (contract §5's own "Field использует те же самые имена JSON").
type validationFieldDTO struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// --- Consumer signal DTOs (plan 4.6) ---

// signalDimensionDTO is one dimension's (overall, or one skill's) signal:
// value/status/confidence/sample_count, exactly as plan 4.6's example JSON
// names them. Value and Confidence are pointers so an unusable dimension
// serializes null rather than a fabricated 0/"" (plan 4.6: "value становится
// null при недоступности dimension"; "confidence... либо null для
// error/stale status").
type signalDimensionDTO struct {
	Value       *float64 `json:"value"`
	Status      string   `json:"status"`
	Confidence  *string  `json:"confidence"`
	SampleCount int      `json:"sample_count"`
}

// signalPositionDTO is the consumer signal's own, simpler position shape:
// a bare nullable rank, not a {value,status} Position object (plan 4.6's
// example: "position": {"community_position": 12}).
type signalPositionDTO struct {
	CommunityPosition *int `json:"community_position"`
}

// signalFreshnessDTO is the shared freshness block plan 4.6 attaches once at
// the top of the signal response, covering overall and every skill alike
// (there is exactly one as_of per signal, never one per dimension).
type signalFreshnessDTO struct {
	AsOf       *time.Time `json:"as_of"`
	ComputedAt time.Time  `json:"computed_at"`
	TTLSeconds int        `json:"ttl_seconds"`
	Stale      bool       `json:"stale"`
}

// signalResponseDTO is GET /v1/models/{model_key}/feedback/signal's 200
// response (plan 4.6), field-for-field matching the brief's worked example.
// It never carries mine, others, personal_position, identity, a token, or
// raw review text — signal_scope is always the fixed literal "community".
type signalResponseDTO struct {
	ModelKey      string                        `json:"model_key"`
	SignalScope   string                        `json:"signal_scope"`
	Position      signalPositionDTO             `json:"position"`
	Overall       signalDimensionDTO            `json:"overall"`
	Skills        map[string]signalDimensionDTO `json:"skills"`
	Status        string                        `json:"status"`
	Freshness     signalFreshnessDTO            `json:"freshness"`
	SchemaVersion string                        `json:"schema_version"`
	PolicyVersion string                        `json:"policy_version"`
}

package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SignalProvider is the consumer adapter boundary (plan 3.4): the one entry
// point a future external assistant/runtime calls before generation.
// GetSignal returns either a fully-populated, validated FeedbackSignal or
// one of this package's four typed errors (ErrUnavailable/ErrUnauthorized/
// ErrIncompatibleSchema/ErrPolicyRejected, wrapped as *AdapterError) — never
// both, and never a raw review, prompt, or identity of any kind.
type SignalProvider interface {
	GetSignal(ctx context.Context, modelKey string) (FeedbackSignal, error)
}

// --- Typed adapter errors (plan 3.4) ---

var (
	// ErrUnavailable means the signal could not be obtained at all: a
	// network failure, a request timeout, or the server answering with a
	// 5xx (or any other HTTP status this adapter does not otherwise
	// recognize). It says nothing about whether the model has feedback —
	// only that this call could not find out.
	ErrUnavailable = errors.New("consumer: feedback signal unavailable")
	// ErrUnauthorized means the server rejected the presented consumer
	// credential (HTTP 401 or 403).
	ErrUnauthorized = errors.New("consumer: feedback signal credential rejected")
	// ErrIncompatibleSchema means the response could not be trusted as a
	// valid signal of the version this adapter understands: malformed JSON,
	// a missing required field, an unrecognized status/confidence/
	// signal_scope value, or a schema_version other than
	// SupportedSchemaVersion.
	ErrIncompatibleSchema = errors.New("consumer: feedback signal schema incompatible")
	// ErrPolicyRejected means the server's own top-level status was
	// "policy_rejected" — a well-formed, well-understood response that the
	// server-side policy itself refused to serve as usable.
	ErrPolicyRejected = errors.New("consumer: feedback signal rejected by server-side policy")
)

// AdapterError wraps one of the four sentinel errors above with short,
// static, technical-only detail (an HTTP status, "request timeout", ...) —
// never the response body, a review, an identity, or a token. errors.Is and
// errors.As against the wrapped sentinel work via Unwrap, so callers can
// keep matching on the package-level Err* variables without caring about
// this wrapper.
type AdapterError struct {
	// Err is always one of ErrUnavailable, ErrUnauthorized,
	// ErrIncompatibleSchema, or ErrPolicyRejected.
	Err error
	// Detail is a short, static, safe-to-log technical description (e.g.
	// "http status 503", "request timeout", "unsupported schema_version").
	// It never contains response body content or anything user-supplied.
	Detail string
}

func (e *AdapterError) Error() string {
	if e.Detail == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Err.Error(), e.Detail)
}

func (e *AdapterError) Unwrap() error { return e.Err }

// --- Signal shape (mirrors internal/feedback/httpapi/dto.go's signalResponseDTO) ---

// DimensionStatus is one dimension's (overall's, or one skill's) own status,
// exactly mirroring the wire enum plan 4.6 documents:
// insufficient|provisional|established|stale|unavailable|incompatible_schema|policy_rejected.
type DimensionStatus string

const (
	DimensionInsufficient       DimensionStatus = "insufficient"
	DimensionProvisional        DimensionStatus = "provisional"
	DimensionEstablished        DimensionStatus = "established"
	DimensionStale              DimensionStatus = "stale"
	DimensionUnavailable        DimensionStatus = "unavailable"
	DimensionIncompatibleSchema DimensionStatus = "incompatible_schema"
	DimensionPolicyRejected     DimensionStatus = "policy_rejected"
)

// Confidence mirrors the wire enum for a dimension's confidence field:
// insufficient|provisional|established, or absent (nil) for an
// error/stale-status dimension.
type Confidence string

const (
	ConfidenceInsufficient Confidence = "insufficient"
	ConfidenceProvisional  Confidence = "provisional"
	ConfidenceEstablished  Confidence = "established"
)

// SignalStatus is the whole signal's top-level status (plan 4.6):
// usable|stale|unavailable|incompatible_schema|policy_rejected. It describes
// availability of the WHOLE signal, not any one dimension's own usability —
// a "usable" signal can still carry an "insufficient" or "stale" dimension.
type SignalStatus string

const (
	SignalUsable             SignalStatus = "usable"
	SignalStale              SignalStatus = "stale"
	SignalUnavailable        SignalStatus = "unavailable"
	SignalIncompatibleSchema SignalStatus = "incompatible_schema"
	SignalPolicyRejected     SignalStatus = "policy_rejected"
)

// Dimension is one dimension's (overall's, or one named skill's) signal:
// value/status/confidence/sample_count, field-for-field matching
// httpapi/dto.go's signalDimensionDTO. Value and Confidence are pointers so
// an unusable dimension is represented as nil, never a fabricated 0/"".
type Dimension struct {
	Value       *float64
	Status      DimensionStatus
	Confidence  *Confidence
	SampleCount int
}

// Freshness mirrors httpapi/dto.go's signalFreshnessDTO: one freshness
// block shared by the whole signal (overall and every skill alike). AsOf is
// nil when the server has no saved rating at all for this model
// (MAX(updated_at) over nothing).
type Freshness struct {
	AsOf       *time.Time
	ComputedAt time.Time
	TTLSeconds int
	Stale      bool
}

// FeedbackSignal is this package's decoded, validated mirror of
// httpapi/dto.go's signalResponseDTO — the only shape
// HTTPReferenceSignalProvider.GetSignal ever returns on success. It is
// always community-only: SignalScope is always the literal "community",
// there is no personal_position, mine, others, identity, token, or raw
// review field of any kind, and there is no separate "personal" variant of
// this type (see doc.go).
type FeedbackSignal struct {
	ModelKey string
	// SignalScope is always "community" for a successfully decoded signal
	// (GetSignal rejects any other value as ErrIncompatibleSchema — see
	// http_provider.go).
	SignalScope string
	// CommunityPosition is nil unless the model is eligible for a community
	// rank under the server's fixed shrinkage policy (n>=5); this package
	// never recomputes that formula itself.
	CommunityPosition *int
	Overall           Dimension
	// Skills is keyed by the server's own skill vocabulary; this package
	// does not maintain or validate against a closed skill list of its own
	// (that vocabulary is httpapi/internal/feedback's to own and evolve).
	Skills        map[string]Dimension
	Status        SignalStatus
	Freshness     Freshness
	SchemaVersion string
	PolicyVersion string
}

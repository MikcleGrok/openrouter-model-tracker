package consumer

import "errors"

// Mode is PolicyDecision's top-level switch: whether generation should be
// influenced by feedback at all.
type Mode string

const (
	ModeBaseline Mode = "baseline"
	ModeFeedback Mode = "feedback"
)

// Action is the allowlisted way feedback may influence generation (plan
// 3.4/4.6: "разрешённое влияние ограничено структурированными решениями").
// There is no free-text action and no way to construct one outside this
// set.
type Action string

const (
	ActionBaseline Action = "baseline"
	ActionWarning  Action = "warning"
	ActionRanking  Action = "ranking"
	ActionRouting  Action = "routing"
)

// ReasonCode is PolicyDecision's fixed, closed vocabulary explaining why a
// given Mode/Action was chosen (plan 3.4's allowlist).
type ReasonCode string

const (
	// ReasonNoSignal means the server has no saved rating at all for this
	// model (the whole signal's top-level status was "unavailable") — not a
	// provider/transport failure, just genuinely no data yet.
	ReasonNoSignal ReasonCode = "no_signal"
	// ReasonUnavailable corresponds to a provider-level ErrUnavailable
	// (transport failure, timeout, or an unexpected/5xx HTTP status).
	ReasonUnavailable ReasonCode = "unavailable"
	// ReasonUnauthorized corresponds to a provider-level ErrUnauthorized.
	ReasonUnauthorized ReasonCode = "unauthorized"
	// ReasonIncompatibleSchema corresponds to a provider-level
	// ErrIncompatibleSchema.
	ReasonIncompatibleSchema ReasonCode = "incompatible_schema"
	// ReasonPolicyRejected corresponds to a provider-level
	// ErrPolicyRejected.
	ReasonPolicyRejected ReasonCode = "policy_rejected"
	// ReasonInsufficientSample means the evaluated dimension itself is
	// "insufficient" (sample_count < 5) even though the whole signal is
	// otherwise usable — e.g. a skill nobody has rated yet on an otherwise
	// well-rated model.
	ReasonInsufficientSample ReasonCode = "insufficient_sample"
	// ReasonStale means the evaluated dimension's status is "stale" (the
	// signal's age exceeded the server's freshness TTL).
	ReasonStale ReasonCode = "stale"
	// ReasonProvisional means the evaluated dimension is "provisional"
	// (5-19 samples) — eligible for a warning or ranking action, never
	// routing.
	ReasonProvisional ReasonCode = "provisional"
	// ReasonEstablished means the evaluated dimension is "established"
	// (>=20 samples) — the only status eligible for a routing action.
	ReasonEstablished ReasonCode = "established"
)

// RoutingDecision is the "safe routing decision from an allowlist" plan 3.4
// calls for: which dimension earned it (either the literal "overall" or one
// of the server's own skill keys) and its established status. It carries no
// free text, no alternative-model choice, and no raw signal data — it only
// asserts "this dimension is established enough to participate in a
// routing decision", leaving what a routing decision actually does to the
// external runtime that owns the point of use (plan 3.4: "Реальный adapter
// и точка вызова генерации принадлежат внешнему inference runtime").
type RoutingDecision struct {
	// Dimension is "overall" or a skill key, taken from the signal itself —
	// never user-supplied free text.
	Dimension string
	// Status is always DimensionEstablished when Routing is non-nil.
	Status DimensionStatus
}

// PolicyDecision is the only shape feedback is ever allowed to influence
// generation through (plan 3.4): a fixed Mode/Action/ReasonCode plus two
// optional, equally fixed-vocabulary fields. There is no free text, no raw
// review, and no identity anywhere in this type.
type PolicyDecision struct {
	Mode       Mode
	Action     Action
	ReasonCode ReasonCode
	// WarningCode is set only when Action is ActionWarning, and is always
	// one of the same ReasonCode values above — this package defines no
	// separate warning vocabulary, since the brief names none and every
	// warning this policy can currently produce already has a matching
	// reason code (ReasonProvisional today).
	WarningCode *ReasonCode
	// Routing is set only when Action is ActionRouting.
	Routing *RoutingDecision
}

func baseline(reason ReasonCode) PolicyDecision {
	return PolicyDecision{Mode: ModeBaseline, Action: ActionBaseline, ReasonCode: reason}
}

// baselineForError maps one of the four typed adapter errors (or any other
// error a non-reference SignalProvider might return) to a baseline
// PolicyDecision. It never returns anything but Mode: ModeBaseline, Action:
// ActionBaseline — an error from the provider can never produce a
// feedback-influenced decision (plan 3.4: "при ошибке provider или policy
// выбрать PolicyDecision{mode: baseline, action: baseline, ...}").
func baselineForError(err error) PolicyDecision {
	switch {
	case errors.Is(err, ErrUnavailable):
		return baseline(ReasonUnavailable)
	case errors.Is(err, ErrUnauthorized):
		return baseline(ReasonUnauthorized)
	case errors.Is(err, ErrIncompatibleSchema):
		return baseline(ReasonIncompatibleSchema)
	case errors.Is(err, ErrPolicyRejected):
		return baseline(ReasonPolicyRejected)
	default:
		// A SignalProvider implementation other than
		// HTTPReferenceSignalProvider could in principle return some other
		// error. There is no fifth typed error to fall back to, so treat
		// anything unrecognized the same as a generic availability failure
		// — never guess a more specific (and possibly wrong) reason.
		return baseline(ReasonUnavailable)
	}
}

// evaluateDimension is the shared decision core behind EvaluateOverall and
// EvaluateSkill: given the signal-level context (whole-signal status, any
// provider error) and one dimension to judge, produce the PolicyDecision.
// hasDim is false only when EvaluateSkill was asked about a skill key the
// signal does not carry at all — treated as insufficient, the same
// conclusion a real 0-count skill from the server would produce.
func evaluateDimension(signal FeedbackSignal, providerErr error, label string, dim Dimension, hasDim bool, provisionalAction Action) PolicyDecision {
	if providerErr != nil {
		return baselineForError(providerErr)
	}
	if signal.Status == SignalUnavailable {
		return baseline(ReasonNoSignal)
	}
	if !hasDim {
		return baseline(ReasonInsufficientSample)
	}

	switch dim.Status {
	case DimensionStale:
		return baseline(ReasonStale)
	case DimensionUnavailable:
		return baseline(ReasonNoSignal)
	case DimensionIncompatibleSchema:
		return baseline(ReasonIncompatibleSchema)
	case DimensionPolicyRejected:
		return baseline(ReasonPolicyRejected)
	case DimensionInsufficient:
		return baseline(ReasonInsufficientSample)
	case DimensionProvisional:
		pd := PolicyDecision{Mode: ModeFeedback, Action: provisionalAction, ReasonCode: ReasonProvisional}
		if provisionalAction == ActionWarning {
			wc := ReasonProvisional
			pd.WarningCode = &wc
		}
		return pd
	case DimensionEstablished:
		return PolicyDecision{
			Mode:       ModeFeedback,
			Action:     ActionRouting,
			ReasonCode: ReasonEstablished,
			Routing:    &RoutingDecision{Dimension: label, Status: dim.Status},
		}
	default:
		// mapDimension in http_provider.go already rejects any status
		// outside the seven known values before a FeedbackSignal is ever
		// constructed, so this is unreachable from HTTPReferenceSignalProvider
		// — kept only as a safe default for a hand-built FeedbackSignal (e.g.
		// in a test, or a future SignalProvider) carrying an unknown status.
		return baseline(ReasonIncompatibleSchema)
	}
}

// EvaluateOverall decides the model-level PolicyDecision from signal's
// OVERALL dimension (or from providerErr, when GetSignal itself failed).
// A provisional overall is eligible for ActionRanking; an established
// overall is eligible for ActionRouting. Call this once per generation to
// decide whether feedback should influence it at all — see
// fakeflow.go's GenerateWithFeedback for the reference sequencing.
func EvaluateOverall(signal FeedbackSignal, providerErr error) PolicyDecision {
	return evaluateDimension(signal, providerErr, "overall", signal.Overall, true, ActionRanking)
}

// EvaluateSkill decides a PolicyDecision scoped to one named skill
// dimension (or from providerErr, when GetSignal itself failed). A
// provisional skill is eligible for ActionWarning ("предупреждение о
// слабом навыке", plan 3.4); an established skill is eligible for
// ActionRouting. An overall status of "established" never makes an
// insufficient or stale skill usable (plan 4.6) — this function reads only
// the named skill's own status, exactly as required.
func EvaluateSkill(signal FeedbackSignal, providerErr error, skillKey string) PolicyDecision {
	dim, ok := signal.Skills[skillKey]
	return evaluateDimension(signal, providerErr, skillKey, dim, ok, ActionWarning)
}

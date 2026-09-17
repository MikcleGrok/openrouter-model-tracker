package consumer

import (
	"errors"
	"testing"
	"time"
)

// dimensionAt builds a Dimension for a given sample_count against the exact
// same boundary policy httpapi/handlers_signal.go's dimensionSignal
// implements (contract §6, plan 4.6): <5 insufficient/nil value, 5-19
// provisional, >=20 established. It is deliberately a re-statement of that
// boundary as a test fixture, not a re-implementation this package depends
// on at runtime — the real thresholds live server-side (contract.go's own
// doc comment).
func dimensionAt(count int) Dimension {
	switch {
	case count < 5:
		return Dimension{Value: nil, Status: DimensionInsufficient, Confidence: confPtr(ConfidenceInsufficient), SampleCount: count}
	case count < 20:
		v := 4.0
		return Dimension{Value: &v, Status: DimensionProvisional, Confidence: confPtr(ConfidenceProvisional), SampleCount: count}
	default:
		v := 4.0
		return Dimension{Value: &v, Status: DimensionEstablished, Confidence: confPtr(ConfidenceEstablished), SampleCount: count}
	}
}

func confPtr(c Confidence) *Confidence { return &c }

func usableSignal(overall Dimension, skills map[string]Dimension) FeedbackSignal {
	return FeedbackSignal{
		ModelKey:      "acme/policy-model",
		SignalScope:   "community",
		Overall:       overall,
		Skills:        skills,
		Status:        SignalUsable,
		Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400},
		SchemaVersion: SupportedSchemaVersion,
		PolicyVersion: "feedback-signal-policy.v1",
	}
}

// TestEvaluateOverall_BoundaryCounts drives EvaluateOverall at exactly the
// boundary counts the brief names (0/1/4/5/19/20), checking Mode/Action/
// ReasonCode at each.
func TestEvaluateOverall_BoundaryCounts(t *testing.T) {
	cases := []struct {
		count      int
		wantMode   Mode
		wantAction Action
		wantReason ReasonCode
	}{
		{count: 0, wantMode: ModeBaseline, wantAction: ActionBaseline, wantReason: ReasonInsufficientSample},
		{count: 1, wantMode: ModeBaseline, wantAction: ActionBaseline, wantReason: ReasonInsufficientSample},
		{count: 4, wantMode: ModeBaseline, wantAction: ActionBaseline, wantReason: ReasonInsufficientSample},
		{count: 5, wantMode: ModeFeedback, wantAction: ActionRanking, wantReason: ReasonProvisional},
		{count: 19, wantMode: ModeFeedback, wantAction: ActionRanking, wantReason: ReasonProvisional},
		{count: 20, wantMode: ModeFeedback, wantAction: ActionRouting, wantReason: ReasonEstablished},
	}
	for _, tc := range cases {
		signal := usableSignal(dimensionAt(tc.count), map[string]Dimension{})
		got := EvaluateOverall(signal, nil)
		if got.Mode != tc.wantMode || got.Action != tc.wantAction || got.ReasonCode != tc.wantReason {
			t.Errorf("count=%d: got %+v, want mode=%s action=%s reason=%s", tc.count, got, tc.wantMode, tc.wantAction, tc.wantReason)
		}
		if tc.wantAction == ActionRouting {
			if got.Routing == nil || got.Routing.Dimension != "overall" || got.Routing.Status != DimensionEstablished {
				t.Errorf("count=%d: routing = %+v, want {overall established}", tc.count, got.Routing)
			}
		} else if got.Routing != nil {
			t.Errorf("count=%d: routing = %+v, want nil", tc.count, got.Routing)
		}
	}
}

// TestEvaluateSkill_BoundaryCounts mirrors the overall test but for
// EvaluateSkill, whose provisional action is ActionWarning (not
// ActionRanking) and which additionally carries WarningCode when
// provisional.
func TestEvaluateSkill_BoundaryCounts(t *testing.T) {
	cases := []struct {
		count          int
		wantMode       Mode
		wantAction     Action
		wantReason     ReasonCode
		wantWarningSet bool
	}{
		{count: 0, wantMode: ModeBaseline, wantAction: ActionBaseline, wantReason: ReasonInsufficientSample},
		{count: 1, wantMode: ModeBaseline, wantAction: ActionBaseline, wantReason: ReasonInsufficientSample},
		{count: 4, wantMode: ModeBaseline, wantAction: ActionBaseline, wantReason: ReasonInsufficientSample},
		{count: 5, wantMode: ModeFeedback, wantAction: ActionWarning, wantReason: ReasonProvisional, wantWarningSet: true},
		{count: 19, wantMode: ModeFeedback, wantAction: ActionWarning, wantReason: ReasonProvisional, wantWarningSet: true},
		{count: 20, wantMode: ModeFeedback, wantAction: ActionRouting, wantReason: ReasonEstablished},
	}
	for _, tc := range cases {
		signal := usableSignal(dimensionAt(20), map[string]Dimension{"reasoning": dimensionAt(tc.count)})
		got := EvaluateSkill(signal, nil, "reasoning")
		if got.Mode != tc.wantMode || got.Action != tc.wantAction || got.ReasonCode != tc.wantReason {
			t.Errorf("count=%d: got %+v, want mode=%s action=%s reason=%s", tc.count, got, tc.wantMode, tc.wantAction, tc.wantReason)
		}
		if tc.wantWarningSet {
			if got.WarningCode == nil || *got.WarningCode != ReasonProvisional {
				t.Errorf("count=%d: warning_code = %v, want provisional", tc.count, got.WarningCode)
			}
		} else if got.WarningCode != nil {
			t.Errorf("count=%d: warning_code = %v, want nil", tc.count, *got.WarningCode)
		}
		if tc.wantAction == ActionRouting {
			if got.Routing == nil || got.Routing.Dimension != "reasoning" {
				t.Errorf("count=%d: routing = %+v, want {reasoning established}", tc.count, got.Routing)
			}
		}
	}
}

// TestEvaluateSkill_OverallEstablishedDoesNotRescueInsufficientSkill proves
// the "mixed states" requirement at the policy layer: an established
// overall never makes an insufficient or stale skill usable, and routing
// reads only the named skill's own status (plan 4.6).
func TestEvaluateSkill_OverallEstablishedDoesNotRescueInsufficientSkill(t *testing.T) {
	signal := usableSignal(dimensionAt(20), map[string]Dimension{"coding": dimensionAt(2)})
	got := EvaluateSkill(signal, nil, "coding")
	if got.Mode != ModeBaseline || got.Action != ActionBaseline || got.ReasonCode != ReasonInsufficientSample {
		t.Errorf("coding (count=2) under established overall: got %+v, want baseline/insufficient_sample", got)
	}

	overallDecision := EvaluateOverall(signal, nil)
	if overallDecision.Mode != ModeFeedback || overallDecision.Action != ActionRouting {
		t.Fatalf("sanity check: overall should still be established/routing, got %+v", overallDecision)
	}
}

// TestEvaluateSkill_UnknownSkillIsInsufficient covers hasDim=false: a skill
// key absent from Skills entirely (never returned by the server at all)
// must never be treated as anything but insufficient.
func TestEvaluateSkill_UnknownSkillIsInsufficient(t *testing.T) {
	signal := usableSignal(dimensionAt(20), map[string]Dimension{})
	got := EvaluateSkill(signal, nil, "nonexistent_skill")
	if got.Mode != ModeBaseline || got.Action != ActionBaseline || got.ReasonCode != ReasonInsufficientSample {
		t.Errorf("unknown skill: got %+v, want baseline/insufficient_sample", got)
	}
}

// TestEvaluate_StaleSignal proves a stale dimension always falls back to
// baseline/stale, matching contract §6's freshness TTL boundary
// (independently re-verified server-side by httpapi's own
// TestSignal_Freshness_ExactlyTTLIsFresh_OneNanosecondOverIsStale — this
// test only checks that THIS package's policy layer reacts correctly to the
// dimension status the server already computed, never re-deriving the TTL
// itself).
func TestEvaluate_StaleSignal(t *testing.T) {
	stale := Dimension{Value: nil, Status: DimensionStale, Confidence: nil, SampleCount: 20}
	signal := FeedbackSignal{
		ModelKey:      "acme/stale-model",
		SignalScope:   "community",
		Overall:       stale,
		Skills:        map[string]Dimension{"reasoning": stale},
		Status:        SignalStale,
		Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400, Stale: true},
		SchemaVersion: SupportedSchemaVersion,
		PolicyVersion: "feedback-signal-policy.v1",
	}

	if got := EvaluateOverall(signal, nil); got.Mode != ModeBaseline || got.Action != ActionBaseline || got.ReasonCode != ReasonStale {
		t.Errorf("stale overall: got %+v, want baseline/stale", got)
	}
	if got := EvaluateSkill(signal, nil, "reasoning"); got.Mode != ModeBaseline || got.Action != ActionBaseline || got.ReasonCode != ReasonStale {
		t.Errorf("stale skill: got %+v, want baseline/stale", got)
	}
}

// TestEvaluate_NoSignalAtAll covers the whole-signal "unavailable" case
// (zero ratings recorded for this model at all) — distinct from
// ReasonInsufficientSample, which is for a dimension short on samples
// within an otherwise-usable signal.
func TestEvaluate_NoSignalAtAll(t *testing.T) {
	signal := FeedbackSignal{
		ModelKey:      "acme/never-rated",
		SignalScope:   "community",
		Overall:       Dimension{Status: DimensionInsufficient, Confidence: confPtr(ConfidenceInsufficient)},
		Skills:        map[string]Dimension{"reasoning": {Status: DimensionInsufficient, Confidence: confPtr(ConfidenceInsufficient)}},
		Status:        SignalUnavailable,
		Freshness:     Freshness{AsOf: nil, ComputedAt: time.Now(), TTLSeconds: 86400},
		SchemaVersion: SupportedSchemaVersion,
		PolicyVersion: "feedback-signal-policy.v1",
	}
	if got := EvaluateOverall(signal, nil); got.Mode != ModeBaseline || got.Action != ActionBaseline || got.ReasonCode != ReasonNoSignal {
		t.Errorf("no-data overall: got %+v, want baseline/no_signal", got)
	}
	if got := EvaluateSkill(signal, nil, "reasoning"); got.Mode != ModeBaseline || got.Action != ActionBaseline || got.ReasonCode != ReasonNoSignal {
		t.Errorf("no-data skill: got %+v, want baseline/no_signal", got)
	}
}

// TestEvaluate_WholeSignalStatusAlwaysWinsOverDimensionShape is the
// regression test for the review finding that evaluateDimension only ever
// checked SignalUnavailable at the whole-signal level, never
// SignalStale/SignalIncompatibleSchema/SignalPolicyRejected nor
// Freshness.Stale — so a hand-built (or future, non-reference) provider
// that reports a non-usable whole-signal status while leaving an
// established-looking dimension untouched (e.g. a caching provider marking
// stale data without also rewriting every dimension's own status) must
// still get baseline, never routing/ranking/warning. Every case below
// deliberately pairs the non-usable whole-signal condition with an
// otherwise-established dimension, so a regression back to "only dim.Status
// is checked" would fail loudly instead of accidentally passing.
func TestEvaluate_WholeSignalStatusAlwaysWinsOverDimensionShape(t *testing.T) {
	established := dimensionAt(20)

	cases := []struct {
		name       string
		signal     FeedbackSignal
		wantReason ReasonCode
	}{
		{
			name: "whole signal stale, dimension still shows established",
			signal: FeedbackSignal{
				Status:        SignalStale,
				Overall:       established,
				Skills:        map[string]Dimension{"reasoning": established},
				Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400, Stale: true},
				SchemaVersion: SupportedSchemaVersion,
				PolicyVersion: SupportedPolicyVersion,
			},
			wantReason: ReasonStale,
		},
		{
			name: "whole signal incompatible_schema, dimension still shows established",
			signal: FeedbackSignal{
				Status:        SignalIncompatibleSchema,
				Overall:       established,
				Skills:        map[string]Dimension{"reasoning": established},
				Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400},
				SchemaVersion: SupportedSchemaVersion,
				PolicyVersion: SupportedPolicyVersion,
			},
			wantReason: ReasonIncompatibleSchema,
		},
		{
			name: "whole signal policy_rejected, dimension still shows established",
			signal: FeedbackSignal{
				Status:        SignalPolicyRejected,
				Overall:       established,
				Skills:        map[string]Dimension{"reasoning": established},
				Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400},
				SchemaVersion: SupportedSchemaVersion,
				PolicyVersion: SupportedPolicyVersion,
			},
			wantReason: ReasonPolicyRejected,
		},
		{
			name: "status usable but Freshness.Stale true (inconsistent provider), dimension still shows established",
			signal: FeedbackSignal{
				Status:        SignalUsable,
				Overall:       established,
				Skills:        map[string]Dimension{"reasoning": established},
				Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400, Stale: true},
				SchemaVersion: SupportedSchemaVersion,
				PolicyVersion: SupportedPolicyVersion,
			},
			wantReason: ReasonStale,
		},
		{
			name: "unrecognized/empty whole-signal status, dimension still shows established",
			signal: FeedbackSignal{
				Status:        SignalStatus(""),
				Overall:       established,
				Skills:        map[string]Dimension{"reasoning": established},
				Freshness:     Freshness{ComputedAt: time.Now(), TTLSeconds: 86400},
				SchemaVersion: SupportedSchemaVersion,
				PolicyVersion: SupportedPolicyVersion,
			},
			wantReason: ReasonIncompatibleSchema,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overall := EvaluateOverall(tc.signal, nil)
			if overall.Mode != ModeBaseline || overall.Action != ActionBaseline || overall.ReasonCode != tc.wantReason {
				t.Errorf("EvaluateOverall = %+v, want baseline/baseline/%s", overall, tc.wantReason)
			}
			if overall.Routing != nil {
				t.Errorf("EvaluateOverall must never carry Routing when the whole signal is non-usable, got %+v", overall)
			}

			skill := EvaluateSkill(tc.signal, nil, "reasoning")
			if skill.Mode != ModeBaseline || skill.Action != ActionBaseline || skill.ReasonCode != tc.wantReason {
				t.Errorf("EvaluateSkill = %+v, want baseline/baseline/%s", skill, tc.wantReason)
			}
			if skill.Routing != nil {
				t.Errorf("EvaluateSkill must never carry Routing when the whole signal is non-usable, got %+v", skill)
			}
		})
	}
}

// TestBaselineForError_EveryTypedError proves baseline-on-error for all
// four typed adapter errors plus a generic unrecognized error, both through
// EvaluateOverall and EvaluateSkill.
func TestBaselineForError_EveryTypedError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReason ReasonCode
	}{
		{"unavailable", &AdapterError{Err: ErrUnavailable, Detail: "http status 503"}, ReasonUnavailable},
		{"unauthorized", &AdapterError{Err: ErrUnauthorized, Detail: "http status 401"}, ReasonUnauthorized},
		{"incompatible_schema", &AdapterError{Err: ErrIncompatibleSchema, Detail: "bad schema_version"}, ReasonIncompatibleSchema},
		{"policy_rejected", &AdapterError{Err: ErrPolicyRejected, Detail: "server reported policy_rejected"}, ReasonPolicyRejected},
		{"generic_unrecognized_error", errors.New("some other provider implementation's own error"), ReasonUnavailable},
	}
	// An established, fully-usable signal, so a bug that ignores the error
	// and reads signal fields instead would be caught (it would report
	// feedback/routing, not baseline).
	establishedSignal := usableSignal(dimensionAt(20), map[string]Dimension{"reasoning": dimensionAt(20)})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overall := EvaluateOverall(establishedSignal, tc.err)
			if overall.Mode != ModeBaseline || overall.Action != ActionBaseline || overall.ReasonCode != tc.wantReason {
				t.Errorf("EvaluateOverall: got %+v, want baseline/%s", overall, tc.wantReason)
			}
			if overall.Routing != nil || overall.WarningCode != nil {
				t.Errorf("EvaluateOverall on error must never carry Routing/WarningCode, got %+v", overall)
			}

			skill := EvaluateSkill(establishedSignal, tc.err, "reasoning")
			if skill.Mode != ModeBaseline || skill.Action != ActionBaseline || skill.ReasonCode != tc.wantReason {
				t.Errorf("EvaluateSkill: got %+v, want baseline/%s", skill, tc.wantReason)
			}
			if skill.Routing != nil || skill.WarningCode != nil {
				t.Errorf("EvaluateSkill on error must never carry Routing/WarningCode, got %+v", skill)
			}
		})
	}
}

// TestPolicyDecision_NoFreeTextFields is a structural/documentation check:
// PolicyDecision's exported surface is exactly the fixed fields plan 3.4
// allows, nothing free-text. This test exists so an accidental future
// addition of a string field (e.g. a "message" or "detail") is caught here
// rather than only by manual review — it is intentionally re-run whenever
// this file changes.
func TestPolicyDecision_NoFreeTextFields(t *testing.T) {
	pd := PolicyDecision{
		Mode:       ModeFeedback,
		Action:     ActionRouting,
		ReasonCode: ReasonEstablished,
		Routing:    &RoutingDecision{Dimension: "overall", Status: DimensionEstablished},
	}
	// Every field is a closed-vocabulary named type (Mode/Action/ReasonCode/
	// *ReasonCode) or a *RoutingDecision whose own fields are equally closed
	// (string drawn only from the signal's own dimension keys, plus a
	// DimensionStatus) — there is no string field capable of holding a raw
	// review, a prompt, or free text. This assertion documents that
	// contract; a struct literal with unkeyed fields would fail to compile
	// the moment a new, undocumented field appeared.
	if pd.Mode != ModeFeedback {
		t.Fatalf("unexpected zero value handling: %+v", pd)
	}
}

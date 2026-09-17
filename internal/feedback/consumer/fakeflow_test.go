package consumer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// spyProvider is a SignalProvider that returns a fixed (signal, err) and
// records that it was called, in order, into a shared trace slice.
type spyProvider struct {
	signal FeedbackSignal
	err    error
	trace  *[]string
}

func (s *spyProvider) GetSignal(_ context.Context, _ string) (FeedbackSignal, error) {
	*s.trace = append(*s.trace, "provider")
	return s.signal, s.err
}

// spyGenerator returns a Generator that records that it was called into the
// same shared trace slice, and captures the exact PolicyDecision it
// received (its signature has no way to receive anything else — no
// FeedbackSignal, no error — so capturing decision is the only thing there
// is to capture from generation's own inputs).
func spyGenerator(trace *[]string, gotDecision *PolicyDecision) Generator {
	return func(_ context.Context, prompt string, decision PolicyDecision) (string, error) {
		*trace = append(*trace, "generator")
		*gotDecision = decision
		return "generated:" + prompt, nil
	}
}

// decisionsEqual compares two PolicyDecision values by content rather than
// by the identity of their Routing/WarningCode pointers (two independent
// EvaluateOverall calls on the same input allocate distinct pointers to
// equal values).
func decisionsEqual(a, b PolicyDecision) bool {
	if a.Mode != b.Mode || a.Action != b.Action || a.ReasonCode != b.ReasonCode {
		return false
	}
	if (a.WarningCode == nil) != (b.WarningCode == nil) {
		return false
	}
	if a.WarningCode != nil && *a.WarningCode != *b.WarningCode {
		return false
	}
	if (a.Routing == nil) != (b.Routing == nil) {
		return false
	}
	if a.Routing != nil && *a.Routing != *b.Routing {
		return false
	}
	return true
}

func establishedSignalFixture() FeedbackSignal {
	overall := dimensionAt(20)
	return usableSignal(overall, map[string]Dimension{"reasoning": dimensionAt(12)})
}

// TestGenerateWithFeedback_OrderAndSuccessDecision proves the pinned order
// provider -> policy -> generator on a success path: provider is called
// before generator (trace), and the decision generator receives is exactly
// what EvaluateOverall independently computes from the same signal — proof
// that policy genuinely ran between the two, not merely that both ran.
func TestGenerateWithFeedback_OrderAndSuccessDecision(t *testing.T) {
	var trace []string
	var gotDecision PolicyDecision
	signal := establishedSignalFixture()
	provider := &spyProvider{signal: signal, trace: &trace}
	generator := spyGenerator(&trace, &gotDecision)

	out, err := GenerateWithFeedback(context.Background(), provider, "acme/model-1", "hello", generator, nil)
	if err != nil {
		t.Fatalf("GenerateWithFeedback: %v", err)
	}
	if out != "generated:hello" {
		t.Errorf("output = %q, want %q", out, "generated:hello")
	}

	if len(trace) != 2 || trace[0] != "provider" || trace[1] != "generator" {
		t.Fatalf("call order = %v, want [provider generator]", trace)
	}

	want := EvaluateOverall(signal, nil)
	if !decisionsEqual(gotDecision, want) {
		t.Errorf("decision handed to generator = %+v, want %+v (EvaluateOverall's own output)", gotDecision, want)
	}
	if gotDecision.Mode != ModeFeedback || gotDecision.Action != ActionRouting {
		t.Fatalf("sanity check: established overall should produce feedback/routing, got %+v", gotDecision)
	}
}

// TestGenerateWithFeedback_BaselineOnEveryErrorType proves that whatever
// the provider returns as an error — each of the four typed adapter
// errors, plus a generic unrecognized error — generation still proceeds
// (provider and generator both run, in order) and the generator always
// receives PolicyDecision{Mode: baseline, Action: baseline, ...}, never a
// decision derived from the (deliberately established-looking) signal the
// provider also returned alongside the error.
func TestGenerateWithFeedback_BaselineOnEveryErrorType(t *testing.T) {
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var trace []string
			var gotDecision PolicyDecision
			// A signal that, if the flow buggily ignored the error, would
			// look established/routable — so this test would fail loudly
			// instead of accidentally passing.
			provider := &spyProvider{signal: establishedSignalFixture(), err: tc.err, trace: &trace}
			generator := spyGenerator(&trace, &gotDecision)

			out, err := GenerateWithFeedback(context.Background(), provider, "acme/model-1", "hello", generator, nil)
			if err != nil {
				t.Fatalf("GenerateWithFeedback must still succeed on a feedback error (plan 4.6: \"ошибка feedback не ломает генерацию\"): %v", err)
			}
			if out != "generated:hello" {
				t.Errorf("output = %q, want %q", out, "generated:hello")
			}
			if len(trace) != 2 || trace[0] != "provider" || trace[1] != "generator" {
				t.Fatalf("call order = %v, want [provider generator] even on a provider error", trace)
			}
			if gotDecision.Mode != ModeBaseline || gotDecision.Action != ActionBaseline || gotDecision.ReasonCode != tc.wantReason {
				t.Errorf("decision = %+v, want baseline/baseline/%s", gotDecision, tc.wantReason)
			}
			if gotDecision.Routing != nil || gotDecision.WarningCode != nil {
				t.Errorf("decision on error must never carry Routing/WarningCode, got %+v", gotDecision)
			}
		})
	}
}

// TestGenerateWithFeedback_LogsSafeFieldsOnly proves the audit log (plan
// 4.6, point 3: "Логировать только signal version и технические IDs, не
// raw feedback и не prompt/context") carries the model key, schema/policy
// version, and the decision's own fixed fields — and never the prompt, a
// review, or any other free text.
func TestGenerateWithFeedback_LogsSafeFieldsOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	var trace []string
	var gotDecision PolicyDecision
	signal := establishedSignalFixture()
	provider := &spyProvider{signal: signal, trace: &trace}
	generator := spyGenerator(&trace, &gotDecision)

	const sensitivePrompt = "super-secret-prompt-content-should-never-be-logged"
	if _, err := GenerateWithFeedback(context.Background(), provider, "acme/model-1", sensitivePrompt, generator, logger); err != nil {
		t.Fatalf("GenerateWithFeedback: %v", err)
	}

	logged := buf.String()
	for _, want := range []string{"acme/model-1", signal.SchemaVersion, signal.PolicyVersion, string(ModeFeedback), string(ActionRouting), string(ReasonEstablished)} {
		if !strings.Contains(logged, want) {
			t.Errorf("log output missing expected safe field %q; log:\n%s", want, logged)
		}
	}
	if strings.Contains(logged, sensitivePrompt) {
		t.Errorf("log output leaked the prompt: %s", logged)
	}
}

// TestGenerateWithFeedback_NilLoggerDefaultsSafely proves a nil logger
// never panics — it defaults to a discard handler.
func TestGenerateWithFeedback_NilLoggerDefaultsSafely(t *testing.T) {
	var trace []string
	var gotDecision PolicyDecision
	provider := &spyProvider{signal: establishedSignalFixture(), trace: &trace}
	generator := spyGenerator(&trace, &gotDecision)

	if _, err := GenerateWithFeedback(context.Background(), provider, "acme/model-1", "hi", generator, nil); err != nil {
		t.Fatalf("GenerateWithFeedback with nil logger: %v", err)
	}
}

// TestFakeGenerator documents FakeGenerator's own trivial, deterministic
// output shape.
func TestFakeGenerator(t *testing.T) {
	got, err := FakeGenerator(context.Background(), "hello", PolicyDecision{Mode: ModeBaseline, Action: ActionBaseline, ReasonCode: ReasonUnavailable})
	if err != nil {
		t.Fatalf("FakeGenerator: %v", err)
	}
	want := "fake-response[mode=baseline action=baseline reason=unavailable]: hello"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGenerateWithFeedback_UsingFakeGenerator is a small end-to-end
// smoke test wiring GenerateWithFeedback to the package's own FakeGenerator
// (rather than a spy), matching how a real caller would use this reference
// flow.
func TestGenerateWithFeedback_UsingFakeGenerator(t *testing.T) {
	provider := &spyProvider{signal: establishedSignalFixture(), trace: &[]string{}}
	out, err := GenerateWithFeedback(context.Background(), provider, "acme/model-1", "hello", FakeGenerator, nil)
	if err != nil {
		t.Fatalf("GenerateWithFeedback: %v", err)
	}
	want := "fake-response[mode=feedback action=routing reason=established]: hello"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

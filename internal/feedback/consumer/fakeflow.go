package consumer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// Generator is the "produce a response" boundary a real inference runtime
// owns (plan 3.4: "Реальный adapter и точка вызова генерации принадлежат
// внешнему inference runtime"). It receives only prompt and the
// already-decided PolicyDecision — never the FeedbackSignal, never raw
// review text or identity — enforcing the "policy decides, generator obeys"
// boundary plan 3.4 requires: feedback can only ever reach generation
// through a PolicyDecision's fixed fields, never as free text or a raw
// payload.
//
// Nothing in this repository — cmd/openrouter included — implements this
// signature as a genuine inference engine; see doc.go.
type Generator func(ctx context.Context, prompt string, decision PolicyDecision) (string, error)

// FakeGenerator is a trivial, deterministic Generator used by this
// package's own tests and as documentation of the contract. It is not a
// real inference engine: it does nothing but echo prompt alongside the
// decision it was handed, so a test can assert exactly what the flow below
// passed it.
func FakeGenerator(_ context.Context, prompt string, decision PolicyDecision) (string, error) {
	return fmt.Sprintf("fake-response[mode=%s action=%s reason=%s]: %s", decision.Mode, decision.Action, decision.ReasonCode, prompt), nil
}

// GenerateWithFeedback is the reference fake generation flow (plan
// 3.4/4.6's integration sequence): provider -> policy -> generator, in that
// fixed order, every single call. It is reference/test material pinning the
// required order and the baseline-on-error contract — not the real
// generation entrypoint. A future runtime integration replaces generate
// with a real inference call; the order and the graceful-fallback guarantee
// this function enforces are exactly what that integration must preserve
// (plan 3.4, point 5: "Assertions фиксируют порядок, baseline при
// provider/transport/schema/policy errors").
//
// Step 1 always calls provider.GetSignal first. Step 2 always evaluates
// EvaluateOverall next — on any error from step 1 (whatever its type),
// EvaluateOverall's own baselineForError guarantees a
// PolicyDecision{Mode: ModeBaseline, Action: ActionBaseline, ...}, so
// generation is never blocked or altered by a feedback outage (plan 4.6:
// "ошибка feedback не ломает генерацию"). Step 3 logs only signal/policy
// versions, the model key, and the decision's own fixed fields — never a
// review, a prompt, or any other free text (plan 4.6, point 3). Step 4
// calls generate last, passing only the decision — never the signal or the
// error.
func GenerateWithFeedback(ctx context.Context, provider SignalProvider, modelKey, prompt string, generate Generator, logger *slog.Logger) (string, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	// Step 1: provider.
	signal, err := provider.GetSignal(ctx, modelKey)

	// Step 2: policy.
	decision := EvaluateOverall(signal, err)

	// Step 3: audit log — technical IDs and versions only, never raw
	// feedback or prompt/context.
	logger.Info("feedback signal evaluated",
		"model_key", modelKey,
		"schema_version", signal.SchemaVersion,
		"policy_version", signal.PolicyVersion,
		"mode", string(decision.Mode),
		"action", string(decision.Action),
		"reason_code", string(decision.ReasonCode),
	)

	// Step 4: generator, decision only.
	return generate(ctx, prompt, decision)
}

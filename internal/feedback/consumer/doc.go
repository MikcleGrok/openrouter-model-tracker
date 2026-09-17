// Package consumer is the CLIENT/adapter side of the trusted-consumer
// signal endpoint GET /v1/models/{model_key}/feedback/signal (Task 7 of
// .task/model-feedback-plan/plan.md, brief in
// .superpowers/sdd/plan/task-7-brief.md; the SERVER side of that same
// endpoint is internal/feedback/httpapi's handleGetSignal, built in Task 4).
//
// This package is for a future external assistant/runtime consumer, not for
// this repository's own TUI or CLI. cmd/openrouter does not perform
// inference, does not own a response_id, and this package never treats it
// as a generator (plan 3.3/3.4). Wiring a real inference engine into the
// order this package pins is a separate, later project: see fakeflow.go's
// GenerateWithFeedback for the reference/test-only stand-in.
//
// # Deliberately decoupled from internal/feedback and internal/feedback/client
//
// Like internal/feedback/client (Task 5's TUI-side client), this package
// imports neither internal/feedback nor internal/feedback/httpapi in its
// production code (only its tests do, to exercise the real server). It is
// also architecturally separate from internal/feedback/client itself and
// does not import it: the two are different consumers of the same server
// (plan 3.3 draws this line explicitly — a user-facing TUI client versus a
// trusted assistant/runtime consumer), with different auth scopes,
// different wire contracts, and no shared code between them by design. So
// every wire-shape type this package needs to decode (http_provider.go) is
// defined here too, deliberately duplicating the handful of details
// httpapi/dto.go and httpapi/handlers_signal.go already define — kept in
// sync by this task's own instruction to read those files as the actual,
// shipped contract, not by a shared import.
//
// # Community-only signal, by construction
//
// The signal endpoint this package calls only ever returns
// signal_scope=community: no "mine", no "others", no personal_position, no
// per-identity data of any kind (plan 4.6). FeedbackSignal (contract.go)
// has no field for a personal variant because there is none to build yet —
// a future personal signal would need its own contract, its own scope
// (feedback:personal-signal:read), and its own server-side identity
// binding, none of which exist today. This package must never be extended
// to guess at that shape ahead of time.
//
// # Typed errors, never a fabricated signal
//
// HTTPReferenceSignalProvider.GetSignal maps every failure mode to exactly
// one of four sentinel errors (contract.go): ErrUnavailable (transport
// failure, timeout, or a 5xx/unexpected HTTP status), ErrUnauthorized (401
// or 403), ErrIncompatibleSchema (a response that fails to decode, fails
// required-field validation, or carries an unsupported schema_version/
// signal_scope/status), and ErrPolicyRejected (the server's own top-level
// status is "policy_rejected"). On any of these, GetSignal returns the zero
// FeedbackSignal — it never returns a partially-decoded signal alongside an
// error, and it never invents a value for a field it could not verify.
//
// # Policy is deterministic and always has a safe fallback
//
// policy.go's EvaluateOverall/EvaluateSkill read only the already-computed
// status fields the server attaches to a signal (insufficient/provisional/
// established/stale/...) — they never recompute sample-count thresholds or
// freshness TTLs locally, since plan 4.6 states those are server-side
// policy, not client config. Every provider/transport/schema/policy error,
// and every non-"established"/non-"provisional" dimension status, resolves
// to PolicyDecision{Mode: ModeBaseline, Action: ActionBaseline, ...} — a
// caller can never accidentally read a fabricated feedback-driven decision
// out of an error path.
package consumer

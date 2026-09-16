// Package httpapi exposes internal/feedback.Service over HTTP (Task 4 of
// .task/model-feedback-plan/plan.md, brief in
// .superpowers/sdd/plan/task-4-brief.md). It is the only network-facing
// layer in this feature: the domain package (internal/feedback) has no
// HTTP dependency, and internal/feedback/sqlite has no HTTP dependency
// either — this package wires a *sqlite.Store into a *feedback.Service and
// answers requests, and cmd/feedback-server (a separate package) turns that
// into a runnable process.
//
// # Two auth scopes, never conflated
//
// Every route belongs to exactly one of two auth scopes, and which one is
// never decided by anything the client sends:
//
//   - User scope: PUT/GET/DELETE routes under /v1/models/.../feedback* and
//     /v1/me/feedback. Requires the shared user Bearer token (plan 6.1) plus
//     a syntactically valid X-Identity-Id; the identity named in that header
//     becomes the request's IdentityID.
//   - Trusted-consumer scope: GET /v1/models/{model_key}/feedback/signal
//     only. Requires the separate consumer Bearer token (plan 6.3), bound
//     server-side at startup to a fixed audience/scope. A client can never
//     claim this scope by sending an X-Scope/X-Audience header or any other
//     client-supplied claim (auth.go never reads such headers at all) — the
//     route itself, plus which of the two known secrets was presented, is
//     the only input to that decision.
//
// # Transport DTOs, never domain types on the wire
//
// dto.go defines a distinct Go type for every JSON shape this package reads
// or writes, each with its own field-by-field mapping to/from
// internal/feedback's domain types (FeedbackInput, Feedback, Aggregate,
// ModelPositions, ...). A domain type's own json tags (some of which happen
// to already match the wire contract in .task/model-feedback-plan/contract.md
// §5) are never marshaled directly: a domain type's zero value and a wire
// "absent"/"null" value do not always coincide (Aggregate.Average is 0.0
// whether count is 0 or a real average happens to be exactly zero-adjacent;
// the wire contract needs null specifically for the former, plan 4.4), and a
// dedicated DTO layer is what lets the two evolve independently instead of a
// domain-type change silently reshaping the wire contract two more tasks
// depend on byte-for-byte.
package httpapi

// Package client is the TUI-side HTTP client for internal/feedback/httpapi
// (Task 5 of .task/model-feedback-plan/plan.md, brief in
// .superpowers/sdd/plan/task-5-brief.md). It talks to a separately-running
// feedback-server process (cmd/feedback-server) over the four user-scope
// routes: PUT a model's feedback, read the caller's own feedback, read a
// model's summary, and delete the caller's identity.
//
// # Deliberately decoupled from the server packages
//
// This package imports neither internal/feedback nor
// internal/feedback/httpapi. Importing httpapi would pull
// internal/feedback/sqlite — and its SQLite driver — into every binary that
// links this client (the TUI, cmd/openrouter), which is exactly the
// server/TUI separation the plan calls for (plan 3.1: cmd/feedback-server
// does not import cmd/openrouter, and the reverse holds just as much). So
// every wire-shape type this package needs (dto.go) and the X-Identity-Id
// format rule (identity.go) are defined here too, deliberately duplicating
// the handful of details httpapi/dto.go and httpapi/auth.go already define
// — kept in sync by the brief's own instruction to read those files as the
// actual contract, not by a shared import.
//
// # Typed errors, not bare status codes
//
// Every non-2xx response the server can send maps to one of a small set of
// typed errors (errors.go): *APIError for a well-formed 4xx/5xx JSON error
// body (with helpers like IsUnauthorized/IsRateLimited/IsServiceUnavailable
// to classify it), *NetworkError/*TimeoutError for a request that never got
// a response at all, and *CredentialError for a problem reading the local
// token_file/identity_file. DeleteMe additionally returns a typed
// DeleteResult distinguishing "done" (204) from "committed, cleanup still
// finishing" (202 cleanup_pending) from a 503 failure — see delete.go.
//
// # Credentials never logged, read fresh per request
//
// The identity and the bearer token are read from identity_file/token_file
// on every request (transport.go), not cached at construction time, so a
// rotated token (plan 6.1: "перезапустить сервер и переоткрыть token-file в
// клиенте") takes effect on this client without a restart. Neither value is
// ever written to a log or included in an error's Error() string beyond the
// file path that held it.
package client

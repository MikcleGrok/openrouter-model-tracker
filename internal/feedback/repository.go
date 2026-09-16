package feedback

import (
	"context"
	"time"
)

// IdentityID is a caller's stable pseudonymous identity — the identity_id
// half of the (identity_id, model_key) upsert key (plan 4.1) and the value
// the SQLite schema's identities table stores as its BLOB primary key
// (plan 5.1). This package neither parses nor generates one: the canonical
// wire format, length, and provisioning belong to whichever later layer
// authenticates a request (plan 6.1's Bearer token + X-Identity-Id), not to
// the domain service.
//
// An IdentityID is never the empty string once it reaches a Service method
// — every method guards against it and returns ErrEmptyIdentity — exactly
// as ModelKey's zero value is never a valid key once normalized (model.go).
type IdentityID string

// Repository is the narrow persistence boundary Service is built on. It is
// the single interface this package defines for storage, deliberately
// giving every read/write path its own explicit shape rather than a
// generic CRUD surface (plan 3.2: "не создавать общего интерфейса на
// каждый тип... одного узкого repository-интерфейса на границе доменного
// сервиса достаточно"), so Service's unit tests run against a small
// in-memory fake and never need a real SQLite database (Task 3's job).
//
// Every method is safe to call for a modelKey/identity that has no rows at
// all — that is a normal empty result, not an error (mirroring
// OwnFeedback's own "absence is not an error" contract, plan 4.3).
type Repository interface {
	// UpsertFeedback creates or updates identity's own feedback for
	// input.ModelKey and returns the stored record. now is the server
	// timestamp (plan 4.1: client time is never the source of truth): it
	// becomes both CreatedAt and UpdatedAt on first insert, and only
	// UpdatedAt on every later call for the same (identity, model_key) —
	// CreatedAt never changes once set. A repeated call from the same
	// identity for the same model updates that one row; it never creates a
	// second row or a vote (plan 1.4).
	//
	// An implementation must provision the identity (and refresh its
	// last-seen marker) in the same atomic unit as the feedback upsert —
	// the SQLite implementation's transactional detail (plan 5.2's
	// "первый PUT не может упасть на FK из-за отсутствующей identity") —
	// but that provisioning is invisible at this interface: callers only
	// ever see the resulting Feedback.
	UpsertFeedback(ctx context.Context, identity IdentityID, input FeedbackInput, now time.Time) (Feedback, error)

	// OwnFeedback returns identity's feedback for modelKey. found is false,
	// with a zero Feedback and a nil error, when identity has never rated
	// that model — never an error (plan 4.3). It must never return another
	// identity's row under any circumstances.
	OwnFeedback(ctx context.Context, identity IdentityID, modelKey ModelKey) (fb Feedback, found bool, err error)

	// AllOwnFeedback returns every model identity has rated, keyed by
	// ModelKey. It exists for personal-position ordering (Service's
	// GetModelPositions), which needs identity's ratings across the whole
	// model set, not just one model — OwnFeedback alone cannot answer that
	// without an unbounded per-model call. It must never include another
	// identity's rows.
	AllOwnFeedback(ctx context.Context, identity IdentityID) (map[ModelKey]Feedback, error)

	// Aggregate computes the community aggregate for modelKey from every
	// currently-stored valid rating (plan 5.2: "Aggregate считается по
	// актуальным строкам, а не по отдельной mutable cache-таблице"), as of
	// now. exclude nil computes the plain "community" scope — every valid
	// identity, including the caller if they voted. A non-nil exclude
	// computes the "others" scope: the identical aggregate with exactly
	// that one identity's own contribution removed (plan 1.4) — the two
	// scopes are the same computation with one parameter differing, not
	// two separate concepts. A modelKey with zero ratings (before or after
	// exclusion) returns Aggregate{Count: 0, ComputedAt: now} with a zero
	// Average, an all-zero Distribution, and no Skills entries.
	Aggregate(ctx context.Context, modelKey ModelKey, exclude *IdentityID, now time.Time) (Aggregate, error)

	// AllAggregates returns the community Aggregate (never excluding
	// anyone — community_position is always community-wide, plan 4.1) for
	// every model that has at least one valid rating, keyed by ModelKey,
	// all computed as of the same now. It exists for community-position
	// ordering (Service's GetModelPositions), which needs every model's
	// aggregate to rank modelKey among them — Aggregate alone cannot
	// answer that without one call per model in the catalogue. A model
	// with zero ratings is simply absent from the returned map, exactly as
	// Aggregate.Skills omits a skill with zero ratings (model.go).
	AllAggregates(ctx context.Context, now time.Time) (map[ModelKey]Aggregate, error)
}

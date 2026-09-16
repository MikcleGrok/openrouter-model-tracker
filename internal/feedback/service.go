package feedback

import (
	"context"
	"errors"
	"time"
)

// ErrEmptyIdentity is returned by every Service method that takes an
// IdentityID when that identity is the empty string. It is never a 400 to
// report to a client: establishing IdentityID's canonical wire format and
// authenticating a request belong to a later layer (plan 6.1's Bearer
// token + X-Identity-Id), not to this service. It exists purely as a
// defensive guard against a caller (a test, a future in-process consumer)
// invoking the service with an unset identity by mistake.
var ErrEmptyIdentity = errors.New("feedback: identity must not be empty")

// ErrEmptyModelKey is returned by every Service method that takes a
// ModelKey when that key is the empty string. NormalizeModelKey already
// rejects an empty raw key on the way in (validate.go), so reaching this
// service with one unset means a caller built a ModelKey by hand instead
// of going through normalization first.
var ErrEmptyModelKey = errors.New("feedback: model key must not be empty")

// Service implements the feedback domain's use cases — SaveFeedback,
// GetOwnFeedback, GetAggregate, GetModelPositions (plan 3.2) — against a
// Repository. It has no dependency on how that Repository is implemented
// (sqlite, an in-memory fake, or anything else) and no dependency on how
// it is called (httpapi, a future TUI client, or a test); neither exists
// yet in this repo.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService constructs a Service backed by repo, using the real wall
// clock for every timestamp it writes or reports. Every such timestamp —
// Feedback.CreatedAt/UpdatedAt, Aggregate.ComputedAt — comes from this
// clock, never from caller input (plan 4.1: "время серверное, client
// timestamp не использовать как источник истины" applies to every use
// case built on Repository, not just SaveFeedback).
//
// Tests within this package may override the unexported now field
// directly after construction for a deterministic clock; that is
// intentionally not exposed as public API, since nothing outside this
// package's own tests needs it.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// SaveFeedback upserts identity's feedback for input.ModelKey (plan 4.1:
// upsert keyed by (identity_id, model_key) — a repeated submission from
// the same identity updates that one row rather than creating a vote) and
// returns the stored record as this identity's own feedback.
//
// input must already be the result of NewFeedbackInput. SaveFeedback does
// not re-run field validation itself: FeedbackInput's own doc comment
// (model.go) already makes NewFeedbackInput the single required entry
// point ("do not build one by hand from unvalidated data and skip that
// call"), and duplicating those rules here would give this package two
// copies to keep in sync instead of one.
func (s *Service) SaveFeedback(ctx context.Context, identity IdentityID, input FeedbackInput) (OwnFeedback, error) {
	if identity == "" {
		return OwnFeedback{}, ErrEmptyIdentity
	}
	if input.ModelKey == "" {
		return OwnFeedback{}, ErrEmptyModelKey
	}
	fb, err := s.repo.UpsertFeedback(ctx, identity, input, s.now())
	if err != nil {
		return OwnFeedback{}, err
	}
	return OwnFeedback(fb), nil
}

// GetOwnFeedback returns identity's own feedback for modelKey, or
// (nil, nil) if identity has never rated that model. Absence of a rating
// is a normal, non-error result (plan 4.3: "200 возвращает собственную
// оценку или own_feedback: null... 404 не использовать для отсутствующей
// оценки"), matching Repository.OwnFeedback's own found=false contract.
func (s *Service) GetOwnFeedback(ctx context.Context, identity IdentityID, modelKey ModelKey) (*OwnFeedback, error) {
	if identity == "" {
		return nil, ErrEmptyIdentity
	}
	if modelKey == "" {
		return nil, ErrEmptyModelKey
	}
	fb, found, err := s.repo.OwnFeedback(ctx, identity, modelKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	own := OwnFeedback(fb)
	return &own, nil
}

// GetAggregate returns the community aggregate for modelKey. exclude nil
// computes the plain "community" scope (every valid identity's rating,
// plan 1.4); a non-nil exclude computes the "others" scope — the identical
// aggregate with exactly that one identity's own contribution removed.
// The two are the same primitive with one parameter differing rather than
// two separate use cases, so a caller building a full FeedbackSummary
// calls this once for "community" (exclude nil) and, only when "others"
// was explicitly requested, a second time with exclude set to the
// caller's own identity.
func (s *Service) GetAggregate(ctx context.Context, modelKey ModelKey, exclude *IdentityID) (Aggregate, error) {
	if modelKey == "" {
		return Aggregate{}, ErrEmptyModelKey
	}
	if exclude != nil && *exclude == "" {
		return Aggregate{}, ErrEmptyIdentity
	}
	return s.repo.Aggregate(ctx, modelKey, exclude, s.now())
}

// GetModelPositions computes modelKey's three independent position signals
// (plan 1.4/4.1): BasePosition from baseRanking alone, PersonalPosition
// from identity's own ratings across the whole model set, and
// CommunityPosition from the community aggregate across the whole model
// set. baseRanking is the existing benchmark/ranking order (index 0 =
// rank 1, the best-ranked model) — this package never computes that order
// itself; it lives wherever the current ranking already lives
// (internal/ranking, outside this package) and the caller supplies it.
//
// identity is required even though BasePosition and CommunityPosition do
// not depend on it: ModelPositions always reports a PersonalPosition, and
// "my personal position" is inherently scoped to one identity.
func (s *Service) GetModelPositions(ctx context.Context, identity IdentityID, modelKey ModelKey, baseRanking []ModelKey) (ModelPositions, error) {
	if identity == "" {
		return ModelPositions{}, ErrEmptyIdentity
	}
	if modelKey == "" {
		return ModelPositions{}, ErrEmptyModelKey
	}
	ownRatings, err := s.repo.AllOwnFeedback(ctx, identity)
	if err != nil {
		return ModelPositions{}, err
	}
	aggregates, err := s.repo.AllAggregates(ctx, s.now())
	if err != nil {
		return ModelPositions{}, err
	}
	return ModelPositions{
		ModelKey:          modelKey,
		BasePosition:      basePosition(modelKey, baseRanking),
		PersonalPosition:  personalPosition(modelKey, ownRatings, baseRanking),
		CommunityPosition: communityPosition(modelKey, aggregates, baseRanking),
	}, nil
}

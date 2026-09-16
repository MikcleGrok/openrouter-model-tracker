package feedback

import (
	"context"
	"errors"
	"time"
)

// fakeRepository is an in-memory Repository used only by this package's own
// tests (Task 2). It holds exactly one source of truth — rows, identity ->
// model_key -> Feedback — and computes every aggregate from those rows on
// each call, mirroring the invariant a real repository must also uphold
// (plan 5.2: "Aggregate считается по актуальным строкам, а не по отдельной
// mutable cache-таблице"). It is not a stand-in for SQLite's own
// constraints (FK, CHECK, transactions) — those are Task 3's integration
// tests, against the real database.
type fakeRepository struct {
	rows map[IdentityID]map[ModelKey]Feedback
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: make(map[IdentityID]map[ModelKey]Feedback)}
}

// Compile-time check that fakeRepository stays in sync with Repository.
var _ Repository = (*fakeRepository)(nil)

func (f *fakeRepository) UpsertFeedback(_ context.Context, identity IdentityID, input FeedbackInput, now time.Time) (Feedback, error) {
	if identity == "" {
		return Feedback{}, errors.New("fakeRepository: empty identity")
	}
	byModel := f.rows[identity]
	if byModel == nil {
		byModel = make(map[ModelKey]Feedback)
		f.rows[identity] = byModel
	}
	createdAt := now
	if existing, ok := byModel[input.ModelKey]; ok {
		createdAt = existing.CreatedAt
	}
	fb := Feedback{
		FeedbackInput: FeedbackInput{
			ModelKey: input.ModelKey,
			Overall:  input.Overall,
			Skills:   append([]SkillRating(nil), input.Skills...),
			Review:   input.Review,
		},
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	byModel[input.ModelKey] = fb
	return fb, nil
}

func (f *fakeRepository) OwnFeedback(_ context.Context, identity IdentityID, modelKey ModelKey) (Feedback, bool, error) {
	byModel, ok := f.rows[identity]
	if !ok {
		return Feedback{}, false, nil
	}
	fb, ok := byModel[modelKey]
	return fb, ok, nil
}

func (f *fakeRepository) AllOwnFeedback(_ context.Context, identity IdentityID) (map[ModelKey]Feedback, error) {
	out := make(map[ModelKey]Feedback, len(f.rows[identity]))
	for mk, fb := range f.rows[identity] {
		out[mk] = fb
	}
	return out, nil
}

func (f *fakeRepository) Aggregate(_ context.Context, modelKey ModelKey, exclude *IdentityID, now time.Time) (Aggregate, error) {
	var (
		count      int
		sum        int
		dist       RatingDistribution
		skillSum   = make(map[string]int)
		skillCount = make(map[string]int)
	)
	for identity, byModel := range f.rows {
		if exclude != nil && identity == *exclude {
			continue
		}
		fb, ok := byModel[modelKey]
		if !ok {
			continue
		}
		count++
		sum += fb.Overall
		dist.Set(fb.Overall, dist.Count(fb.Overall)+1)
		for _, sr := range fb.Skills {
			skillSum[sr.Key] += sr.Rating
			skillCount[sr.Key]++
		}
	}
	agg := Aggregate{Count: count, Distribution: dist, ComputedAt: now}
	if count > 0 {
		agg.Average = float64(sum) / float64(count)
	}
	// Fixed AllowedSkills order, matching model.go's documented Skills
	// ordering, so callers never see a nondeterministic slice order.
	for _, key := range AllowedSkills() {
		if c := skillCount[key]; c > 0 {
			agg.Skills = append(agg.Skills, SkillAggregate{Key: key, Count: c, Average: float64(skillSum[key]) / float64(c)})
		}
	}
	return agg, nil
}

func (f *fakeRepository) AllAggregates(ctx context.Context, now time.Time) (map[ModelKey]Aggregate, error) {
	models := make(map[ModelKey]bool)
	for _, byModel := range f.rows {
		for mk := range byModel {
			models[mk] = true
		}
	}
	out := make(map[ModelKey]Aggregate, len(models))
	for mk := range models {
		agg, err := f.Aggregate(ctx, mk, nil, now)
		if err != nil {
			return nil, err
		}
		out[mk] = agg
	}
	return out, nil
}

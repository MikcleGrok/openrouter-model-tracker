package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// Compile-time check that Store satisfies Task 2's Repository interface.
var _ feedback.Repository = (*Store)(nil)

// UpsertFeedback implements feedback.Repository.UpsertFeedback. In one
// transaction it: refuses if a privacy cleanup job is currently active
// (ErrMaintenanceLocked — plan 5.2, "активная job блокирует feedback
// writes"); upserts the identities row so last_seen_at always advances and a
// first-ever PUT can never fail on the model_feedback -> identities foreign
// key (plan 5.2's "первый PUT не может упасть на FK из-за отсутствующей
// identity"); upserts the one model_feedback row for (identity, model_key),
// preserving created_at from any existing row; and replaces that row's
// skill_ratings wholesale with input.Skills, which is the simplest correct
// way to "синхронизировать skill rows" against an input that may add,
// change, remove, or entirely omit skills between calls.
func (s *Store) UpsertFeedback(ctx context.Context, identity feedback.IdentityID, input feedback.FeedbackInput, now time.Time) (feedback.Feedback, error) {
	if identity == "" {
		return feedback.Feedback{}, fmt.Errorf("sqlite: UpsertFeedback: empty identity")
	}
	if input.ModelKey == "" {
		return feedback.Feedback{}, fmt.Errorf("sqlite: UpsertFeedback: empty model key")
	}

	idBytes := identityBytes(identity)
	modelKey := string(input.ModelKey)
	nowText := formatTime(now)

	return withTx(ctx, s.db, func(tx *sql.Tx) (feedback.Feedback, error) {
		_, active, err := activeJobTx(ctx, tx)
		if err != nil {
			return feedback.Feedback{}, err
		}
		if active {
			return feedback.Feedback{}, ErrMaintenanceLocked
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO identities (id, created_at, last_seen_at) VALUES (?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET last_seen_at = excluded.last_seen_at
		`, idBytes, nowText, nowText); err != nil {
			return feedback.Feedback{}, fmt.Errorf("sqlite: upsert identity: %w", err)
		}

		createdAtText := nowText
		var existingCreatedAt string
		err = tx.QueryRowContext(ctx, `
			SELECT created_at FROM model_feedback WHERE identity_id = ? AND model_key = ?
		`, idBytes, modelKey).Scan(&existingCreatedAt)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// First feedback for this (identity, model_key): created_at = now.
		case err != nil:
			return feedback.Feedback{}, fmt.Errorf("sqlite: read existing feedback: %w", err)
		default:
			createdAtText = existingCreatedAt
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model_feedback (identity_id, model_key, overall, review, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(identity_id, model_key) DO UPDATE SET
				overall = excluded.overall,
				review = excluded.review,
				updated_at = excluded.updated_at
		`, idBytes, modelKey, input.Overall, input.Review, createdAtText, nowText); err != nil {
			return feedback.Feedback{}, fmt.Errorf("sqlite: upsert feedback: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			DELETE FROM skill_ratings WHERE identity_id = ? AND model_key = ?
		`, idBytes, modelKey); err != nil {
			return feedback.Feedback{}, fmt.Errorf("sqlite: clear skill ratings: %w", err)
		}
		for _, sr := range input.Skills {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO skill_ratings (identity_id, model_key, skill_key, rating) VALUES (?, ?, ?, ?)
			`, idBytes, modelKey, sr.Key, sr.Rating); err != nil {
				return feedback.Feedback{}, fmt.Errorf("sqlite: insert skill rating %q: %w", sr.Key, err)
			}
		}

		createdAt, err := parseTime(createdAtText)
		if err != nil {
			return feedback.Feedback{}, err
		}

		return feedback.Feedback{
			FeedbackInput: feedback.FeedbackInput{
				ModelKey: input.ModelKey,
				Overall:  input.Overall,
				Skills:   append([]feedback.SkillRating(nil), input.Skills...),
				Review:   input.Review,
			},
			CreatedAt: createdAt,
			UpdatedAt: now,
		}, nil
	})
}

// OwnFeedback implements feedback.Repository.OwnFeedback.
func (s *Store) OwnFeedback(ctx context.Context, identity feedback.IdentityID, modelKey feedback.ModelKey) (feedback.Feedback, bool, error) {
	idBytes := identityBytes(identity)

	var overall int
	var review, createdAtText, updatedAtText string
	err := s.db.QueryRowContext(ctx, `
		SELECT overall, review, created_at, updated_at
		FROM model_feedback WHERE identity_id = ? AND model_key = ?
	`, idBytes, string(modelKey)).Scan(&overall, &review, &createdAtText, &updatedAtText)
	if errors.Is(err, sql.ErrNoRows) {
		return feedback.Feedback{}, false, nil
	}
	if err != nil {
		return feedback.Feedback{}, false, fmt.Errorf("sqlite: read own feedback: %w", err)
	}

	skills, err := querySkills(ctx, s.db, `
		SELECT skill_key, rating FROM skill_ratings WHERE identity_id = ? AND model_key = ?
	`, idBytes, string(modelKey))
	if err != nil {
		return feedback.Feedback{}, false, err
	}

	createdAt, err := parseTime(createdAtText)
	if err != nil {
		return feedback.Feedback{}, false, err
	}
	updatedAt, err := parseTime(updatedAtText)
	if err != nil {
		return feedback.Feedback{}, false, err
	}

	return feedback.Feedback{
		FeedbackInput: feedback.FeedbackInput{
			ModelKey: modelKey,
			Overall:  overall,
			Skills:   skills,
			Review:   review,
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, true, nil
}

// AllOwnFeedback implements feedback.Repository.AllOwnFeedback. It issues
// exactly two queries total (feedback rows, then every skill row for this
// identity) regardless of how many models identity has rated, rather than
// one skills query per model.
func (s *Store) AllOwnFeedback(ctx context.Context, identity feedback.IdentityID) (map[feedback.ModelKey]feedback.Feedback, error) {
	idBytes := identityBytes(identity)

	rows, err := s.db.QueryContext(ctx, `
		SELECT model_key, overall, review, created_at, updated_at
		FROM model_feedback WHERE identity_id = ?
	`, idBytes)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read all own feedback: %w", err)
	}
	defer rows.Close()

	out := make(map[feedback.ModelKey]feedback.Feedback)
	for rows.Next() {
		var modelKey, review, createdAtText, updatedAtText string
		var overall int
		if err := rows.Scan(&modelKey, &overall, &review, &createdAtText, &updatedAtText); err != nil {
			return nil, fmt.Errorf("sqlite: scan own feedback: %w", err)
		}
		createdAt, err := parseTime(createdAtText)
		if err != nil {
			return nil, err
		}
		updatedAt, err := parseTime(updatedAtText)
		if err != nil {
			return nil, err
		}
		out[feedback.ModelKey(modelKey)] = feedback.Feedback{
			FeedbackInput: feedback.FeedbackInput{
				ModelKey: feedback.ModelKey(modelKey),
				Overall:  overall,
				Review:   review,
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read all own feedback: %w", err)
	}

	skillRows, err := s.db.QueryContext(ctx, `
		SELECT model_key, skill_key, rating FROM skill_ratings WHERE identity_id = ?
	`, idBytes)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read all own skill ratings: %w", err)
	}
	defer skillRows.Close()

	skillsByModel := make(map[feedback.ModelKey][]feedback.SkillRating)
	for skillRows.Next() {
		var modelKey, skillKey string
		var rating int
		if err := skillRows.Scan(&modelKey, &skillKey, &rating); err != nil {
			return nil, fmt.Errorf("sqlite: scan own skill rating: %w", err)
		}
		mk := feedback.ModelKey(modelKey)
		skillsByModel[mk] = append(skillsByModel[mk], feedback.SkillRating{Key: skillKey, Rating: rating})
	}
	if err := skillRows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read all own skill ratings: %w", err)
	}

	for mk, fb := range out {
		skills := skillsByModel[mk]
		sortSkillsCanonical(skills)
		fb.Skills = skills
		out[mk] = fb
	}

	return out, nil
}

// Aggregate implements feedback.Repository.Aggregate.
func (s *Store) Aggregate(ctx context.Context, modelKey feedback.ModelKey, exclude *feedback.IdentityID, now time.Time) (feedback.Aggregate, error) {
	return aggregateFor(ctx, s.db, string(modelKey), exclude, now)
}

// aggregateFor computes one model's community aggregate (optionally
// excluding one identity) via three small, model-scoped queries: count+sum,
// the rating distribution, and per-skill count+average.
func aggregateFor(ctx context.Context, q queryer, modelKey string, exclude *feedback.IdentityID, now time.Time) (feedback.Aggregate, error) {
	agg := feedback.Aggregate{ComputedAt: now}

	var excludeBytes []byte
	if exclude != nil {
		excludeBytes = identityBytes(*exclude)
	}

	countQuery := `SELECT COUNT(*), COALESCE(SUM(overall), 0) FROM model_feedback WHERE model_key = ?`
	distQuery := `SELECT overall, COUNT(*) FROM model_feedback WHERE model_key = ? GROUP BY overall`
	skillQuery := `SELECT skill_key, COUNT(*), AVG(rating) FROM skill_ratings WHERE model_key = ? GROUP BY skill_key`
	args := []any{modelKey}
	if excludeBytes != nil {
		countQuery += ` AND identity_id <> ?`
		distQuery += ` AND identity_id <> ?`
		skillQuery += ` AND identity_id <> ?`
		args = append(args, excludeBytes)
	}

	var count, sum int
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&count, &sum); err != nil {
		return feedback.Aggregate{}, fmt.Errorf("sqlite: aggregate count: %w", err)
	}
	agg.Count = count
	if count > 0 {
		agg.Average = float64(sum) / float64(count)
	}

	distRows, err := q.QueryContext(ctx, distQuery, args...)
	if err != nil {
		return feedback.Aggregate{}, fmt.Errorf("sqlite: aggregate distribution: %w", err)
	}
	defer distRows.Close()
	for distRows.Next() {
		var overall, n int
		if err := distRows.Scan(&overall, &n); err != nil {
			return feedback.Aggregate{}, fmt.Errorf("sqlite: scan aggregate distribution: %w", err)
		}
		agg.Distribution.Set(overall, n)
	}
	if err := distRows.Err(); err != nil {
		return feedback.Aggregate{}, fmt.Errorf("sqlite: aggregate distribution: %w", err)
	}

	skillRows, err := q.QueryContext(ctx, skillQuery, args...)
	if err != nil {
		return feedback.Aggregate{}, fmt.Errorf("sqlite: aggregate skills: %w", err)
	}
	defer skillRows.Close()
	var skills []feedback.SkillAggregate
	for skillRows.Next() {
		var key string
		var n int
		var avg float64
		if err := skillRows.Scan(&key, &n, &avg); err != nil {
			return feedback.Aggregate{}, fmt.Errorf("sqlite: scan aggregate skills: %w", err)
		}
		skills = append(skills, feedback.SkillAggregate{Key: key, Count: n, Average: avg})
	}
	if err := skillRows.Err(); err != nil {
		return feedback.Aggregate{}, fmt.Errorf("sqlite: aggregate skills: %w", err)
	}
	sortSkillAggregatesCanonical(skills)
	agg.Skills = skills

	return agg, nil
}

// AllAggregates implements feedback.Repository.AllAggregates. It computes
// every rated model's community aggregate with exactly two queries total
// (one grouped by model_key+overall, one grouped by model_key+skill_key) —
// never one query per model — so Service.GetModelPositions' community-
// position ranking over the whole model set stays O(1) repository round
// trips regardless of catalogue size.
func (s *Store) AllAggregates(ctx context.Context, now time.Time) (map[feedback.ModelKey]feedback.Aggregate, error) {
	type counts struct {
		count int
		sum   int
		dist  feedback.RatingDistribution
	}
	byModel := make(map[string]*counts)

	rows, err := s.db.QueryContext(ctx, `
		SELECT model_key, overall, COUNT(*) FROM model_feedback GROUP BY model_key, overall
	`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: all aggregates counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var modelKey string
		var overall, n int
		if err := rows.Scan(&modelKey, &overall, &n); err != nil {
			return nil, fmt.Errorf("sqlite: scan all aggregates counts: %w", err)
		}
		c, ok := byModel[modelKey]
		if !ok {
			c = &counts{}
			byModel[modelKey] = c
		}
		c.count += n
		c.sum += overall * n
		c.dist.Set(overall, c.dist.Count(overall)+n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: all aggregates counts: %w", err)
	}

	skillsByModel := make(map[string][]feedback.SkillAggregate)
	skillRows, err := s.db.QueryContext(ctx, `
		SELECT model_key, skill_key, COUNT(*), AVG(rating) FROM skill_ratings GROUP BY model_key, skill_key
	`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: all aggregates skills: %w", err)
	}
	defer skillRows.Close()
	for skillRows.Next() {
		var modelKey, skillKey string
		var n int
		var avg float64
		if err := skillRows.Scan(&modelKey, &skillKey, &n, &avg); err != nil {
			return nil, fmt.Errorf("sqlite: scan all aggregates skills: %w", err)
		}
		skillsByModel[modelKey] = append(skillsByModel[modelKey], feedback.SkillAggregate{Key: skillKey, Count: n, Average: avg})
	}
	if err := skillRows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: all aggregates skills: %w", err)
	}

	out := make(map[feedback.ModelKey]feedback.Aggregate, len(byModel))
	for modelKey, c := range byModel {
		skills := skillsByModel[modelKey]
		sortSkillAggregatesCanonical(skills)
		average := 0.0
		if c.count > 0 {
			average = float64(c.sum) / float64(c.count)
		}
		out[feedback.ModelKey(modelKey)] = feedback.Aggregate{
			Count:        c.count,
			Average:      average,
			Distribution: c.dist,
			Skills:       skills,
			ComputedAt:   now,
		}
	}
	return out, nil
}

// LastUpdatedAt returns the most recent model_feedback.updated_at among
// modelKey's currently-stored rows, and whether any exist at all. It is not
// part of feedback.Repository: Aggregate.ComputedAt is the time OF the
// aggregate computation (the caller-supplied now), never a property of the
// underlying rows, so it cannot answer "when was this model's community
// data last actually written" — exactly the source Task 4's consumer-signal
// freshness policy needs for freshness.as_of (plan 4.6: "as_of вычисляется
// server-side как MAX(updated_at) последней фактически сохранённой оценки,
// вошедшей в signal, а не как время формирования GET"; "при отсутствии
// сохранённых оценок as_of=null"). Kept on Store directly, alongside
// DeleteIdentity/ActiveCleanupJob/RunCleanup, rather than added to the
// narrow feedback.Repository interface Task 2 deliberately kept CRUD-free:
// internal/feedback/httpapi already holds a concrete *Store for the DELETE
// flow, so it can depend on this the same way.
func (s *Store) LastUpdatedAt(ctx context.Context, modelKey feedback.ModelKey) (time.Time, bool, error) {
	var text sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(updated_at) FROM model_feedback WHERE model_key = ?
	`, string(modelKey)).Scan(&text)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("sqlite: last updated at: %w", err)
	}
	if !text.Valid {
		return time.Time{}, false, nil
	}
	t, err := parseTime(text.String)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}

// queryer is the subset of *sql.DB/*sql.Tx that aggregateFor needs, so the
// same aggregate-computation code can run against either.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// querySkills runs a skill_ratings query expected to return (skill_key,
// rating) rows and returns them in AllowedSkills() canonical order.
func querySkills(ctx context.Context, q queryer, query string, args ...any) ([]feedback.SkillRating, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read skill ratings: %w", err)
	}
	defer rows.Close()

	var skills []feedback.SkillRating
	for rows.Next() {
		var sr feedback.SkillRating
		if err := rows.Scan(&sr.Key, &sr.Rating); err != nil {
			return nil, fmt.Errorf("sqlite: scan skill rating: %w", err)
		}
		skills = append(skills, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read skill ratings: %w", err)
	}
	sortSkillsCanonical(skills)
	return skills, nil
}

// sortSkillAggregatesCanonical sorts skill aggregates into AllowedSkills()
// canonical order, mirroring sortSkillsCanonical for SkillAggregate instead
// of SkillRating.
func sortSkillAggregatesCanonical(skills []feedback.SkillAggregate) {
	order := make(map[string]int, len(feedback.AllowedSkills()))
	for i, key := range feedback.AllowedSkills() {
		order[key] = i
	}
	rank := func(key string) int {
		if r, ok := order[key]; ok {
			return r
		}
		return len(order)
	}
	sort.SliceStable(skills, func(i, j int) bool {
		return rank(skills[i].Key) < rank(skills[j].Key)
	})
}

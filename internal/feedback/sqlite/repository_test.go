package sqlite

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

func mustInput(t *testing.T, modelKey string, overall int, skills []feedback.SkillRating, review string) feedback.FeedbackInput {
	t.Helper()
	in, err := feedback.NewFeedbackInput(modelKey, overall, skills, review)
	if err != nil {
		t.Fatalf("NewFeedbackInput(%q): %v", modelKey, err)
	}
	return in
}

func TestUpsertFeedback_CreatesIdentityThenFeedbackInOneTransaction(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)

	identity := feedback.IdentityID("user-1")
	input := mustInput(t, "anthropic/claude", 5, []feedback.SkillRating{{Key: "coding", Rating: 5}}, "great")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Before the call, neither identities nor model_feedback has any row --
	// proving the identity did not need to already exist (plan 5.2: "первый
	// PUT не может упасть на FK из-за отсутствующей identity").
	var before int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM identities`).Scan(&before); err != nil {
		t.Fatalf("count identities before: %v", err)
	}
	if before != 0 {
		t.Fatalf("identities count before = %d, want 0", before)
	}

	fb, err := store.UpsertFeedback(ctx, identity, input, now)
	if err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if fb.ModelKey != input.ModelKey || fb.Overall != 5 || fb.Review != "great" {
		t.Fatalf("unexpected feedback: %+v", fb)
	}
	if !fb.CreatedAt.Equal(now) || !fb.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected timestamps: created=%v updated=%v want %v", fb.CreatedAt, fb.UpdatedAt, now)
	}

	var identityCount, feedbackCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM identities`).Scan(&identityCount); err != nil {
		t.Fatalf("count identities after: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback`).Scan(&feedbackCount); err != nil {
		t.Fatalf("count model_feedback after: %v", err)
	}
	if identityCount != 1 || feedbackCount != 1 {
		t.Fatalf("identityCount=%d feedbackCount=%d, want 1 and 1", identityCount, feedbackCount)
	}
}

func TestUpsertFeedback_RepeatedCallUpdatesSameRow(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-1")
	modelKey := "anthropic/claude"
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	first, err := store.UpsertFeedback(ctx, identity, mustInput(t, modelKey, 3, nil, "meh"), t1)
	if err != nil {
		t.Fatalf("UpsertFeedback (first): %v", err)
	}
	second, err := store.UpsertFeedback(ctx, identity, mustInput(t, modelKey, 5, nil, "actually great"), t2)
	if err != nil {
		t.Fatalf("UpsertFeedback (second): %v", err)
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("CreatedAt changed on update: first=%v second=%v", first.CreatedAt, second.CreatedAt)
	}
	if !second.UpdatedAt.Equal(t2) {
		t.Fatalf("UpdatedAt = %v, want %v", second.UpdatedAt, t2)
	}
	if second.Overall != 5 || second.Review != "actually great" {
		t.Fatalf("unexpected second feedback: %+v", second)
	}

	var rowCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback WHERE identity_id = ?`, identityBytes(identity)).Scan(&rowCount); err != nil {
		t.Fatalf("count model_feedback: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("model_feedback row count = %d, want 1 (update, not a second vote)", rowCount)
	}
}

func TestUpsertFeedback_SyncsSkillRatings(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-1")
	modelKey := "anthropic/claude"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := store.UpsertFeedback(ctx, identity, mustInput(t, modelKey, 4, []feedback.SkillRating{
		{Key: "coding", Rating: 5},
		{Key: "reasoning", Rating: 4},
	}, ""), now)
	if err != nil {
		t.Fatalf("UpsertFeedback (first): %v", err)
	}

	// Second call: change one skill, drop one, add a new one.
	fb, err := store.UpsertFeedback(ctx, identity, mustInput(t, modelKey, 4, []feedback.SkillRating{
		{Key: "coding", Rating: 3},
		{Key: "long_context", Rating: 2},
	}, ""), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("UpsertFeedback (second): %v", err)
	}

	want := map[string]int{"coding": 3, "long_context": 2}
	if len(fb.Skills) != len(want) {
		t.Fatalf("Skills = %+v, want 2 entries matching %v", fb.Skills, want)
	}
	for _, sr := range fb.Skills {
		if want[sr.Key] != sr.Rating {
			t.Errorf("skill %s = %d, want %d", sr.Key, sr.Rating, want[sr.Key])
		}
	}

	own, found, err := store.OwnFeedback(ctx, identity, feedback.ModelKey(modelKey))
	if err != nil || !found {
		t.Fatalf("OwnFeedback: found=%v err=%v", found, err)
	}
	if len(own.Skills) != 2 {
		t.Fatalf("OwnFeedback skills = %+v, want 2 entries (reasoning dropped)", own.Skills)
	}
}

func TestUpsertFeedback_EmptySkillsClearsExisting(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-1")
	modelKey := "anthropic/claude"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := store.UpsertFeedback(ctx, identity, mustInput(t, modelKey, 4, []feedback.SkillRating{{Key: "coding", Rating: 5}}, ""), now)
	if err != nil {
		t.Fatalf("UpsertFeedback (with skills): %v", err)
	}
	fb, err := store.UpsertFeedback(ctx, identity, mustInput(t, modelKey, 4, nil, ""), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("UpsertFeedback (empty skills): %v", err)
	}
	if len(fb.Skills) != 0 {
		t.Fatalf("Skills = %+v, want empty after resubmitting with no skills", fb.Skills)
	}
}

func TestOwnFeedback_NotFound(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	_, found, err := store.OwnFeedback(ctx, feedback.IdentityID("nobody"), feedback.ModelKey("anthropic/claude"))
	if err != nil {
		t.Fatalf("OwnFeedback: %v", err)
	}
	if found {
		t.Fatal("OwnFeedback found=true for an identity that never rated anything")
	}
}

func TestOwnFeedback_NeverReturnsAnotherIdentitysRow(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	modelKey := "anthropic/claude"

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, modelKey, 5, nil, "a's review"), now); err != nil {
		t.Fatalf("UpsertFeedback (a): %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, "user-b", mustInput(t, modelKey, 1, nil, "b's review"), now); err != nil {
		t.Fatalf("UpsertFeedback (b): %v", err)
	}

	own, found, err := store.OwnFeedback(ctx, "user-a", feedback.ModelKey(modelKey))
	if err != nil || !found {
		t.Fatalf("OwnFeedback(a): found=%v err=%v", found, err)
	}
	if own.Review != "a's review" || own.Overall != 5 {
		t.Fatalf("OwnFeedback(a) leaked b's data: %+v", own)
	}
}

func TestAllOwnFeedback(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "model/one", 5, []feedback.SkillRating{{Key: "coding", Rating: 5}}, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "model/two", 2, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, "user-b", mustInput(t, "model/one", 1, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	all, err := store.AllOwnFeedback(ctx, "user-a")
	if err != nil {
		t.Fatalf("AllOwnFeedback: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("AllOwnFeedback = %+v, want 2 entries", all)
	}
	if all["model/one"].Overall != 5 || len(all["model/one"].Skills) != 1 {
		t.Fatalf("model/one entry = %+v", all["model/one"])
	}
	if all["model/two"].Overall != 2 {
		t.Fatalf("model/two entry = %+v", all["model/two"])
	}
}

func TestAllOwnFeedback_EmptyForUnknownIdentity(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	all, err := store.AllOwnFeedback(ctx, "nobody")
	if err != nil {
		t.Fatalf("AllOwnFeedback: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("AllOwnFeedback = %+v, want empty", all)
	}
}

func TestAggregate_ZeroRatings(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	agg, err := store.Aggregate(ctx, feedback.ModelKey("nobody/rated-this"), nil, now)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if agg.Count != 0 || agg.Average != 0 || len(agg.Skills) != 0 {
		t.Fatalf("Aggregate for unrated model = %+v, want zero", agg)
	}
	if !agg.ComputedAt.Equal(now) {
		t.Fatalf("ComputedAt = %v, want %v", agg.ComputedAt, now)
	}
}

func TestAggregate_CommunityAndOthers(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	modelKey := feedback.ModelKey("anthropic/claude")

	ratings := []struct {
		identity feedback.IdentityID
		overall  int
		skill    int
	}{
		{"user-a", 5, 5},
		{"user-b", 3, 3},
		{"user-c", 1, 1},
	}
	for _, r := range ratings {
		in := mustInput(t, string(modelKey), r.overall, []feedback.SkillRating{{Key: "coding", Rating: r.skill}}, "")
		if _, err := store.UpsertFeedback(ctx, r.identity, in, now); err != nil {
			t.Fatalf("UpsertFeedback(%s): %v", r.identity, err)
		}
	}

	community, err := store.Aggregate(ctx, modelKey, nil, now)
	if err != nil {
		t.Fatalf("Aggregate (community): %v", err)
	}
	if community.Count != 3 {
		t.Fatalf("community.Count = %d, want 3", community.Count)
	}
	wantAvg := (5.0 + 3.0 + 1.0) / 3.0
	if community.Average != wantAvg {
		t.Fatalf("community.Average = %v, want %v", community.Average, wantAvg)
	}
	if community.Distribution.Count(5) != 1 || community.Distribution.Count(3) != 1 || community.Distribution.Count(1) != 1 {
		t.Fatalf("community.Distribution = %+v", community.Distribution)
	}
	if len(community.Skills) != 1 || community.Skills[0].Key != "coding" || community.Skills[0].Count != 3 {
		t.Fatalf("community.Skills = %+v", community.Skills)
	}

	excludeA := feedback.IdentityID("user-a")
	others, err := store.Aggregate(ctx, modelKey, &excludeA, now)
	if err != nil {
		t.Fatalf("Aggregate (others): %v", err)
	}
	if others.Count != 2 {
		t.Fatalf("others.Count = %d, want 2", others.Count)
	}
	wantOthersAvg := (3.0 + 1.0) / 2.0
	if others.Average != wantOthersAvg {
		t.Fatalf("others.Average = %v, want %v", others.Average, wantOthersAvg)
	}

	// Upserting user-a's own rating again must never change another
	// identity's own view, and never substitute for the community score.
	if community.Count == others.Count {
		t.Fatal("community and others must differ when the excluded identity actually voted")
	}
}

func TestAggregate_SkillsOmittedWhenZeroRatings(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	modelKey := "anthropic/claude"

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, modelKey, 4, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	agg, err := store.Aggregate(ctx, feedback.ModelKey(modelKey), nil, now)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(agg.Skills) != 0 {
		t.Fatalf("Skills = %+v, want empty (no skill ratings submitted)", agg.Skills)
	}
}

func TestAggregate_SkillsInCanonicalOrder(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	modelKey := "anthropic/claude"

	// Submit skills out of canonical order.
	in := mustInput(t, modelKey, 4, []feedback.SkillRating{
		{Key: "long_context", Rating: 4},
		{Key: "reasoning", Rating: 5},
		{Key: "coding", Rating: 3},
	}, "")
	if _, err := store.UpsertFeedback(ctx, "user-a", in, now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	agg, err := store.Aggregate(ctx, feedback.ModelKey(modelKey), nil, now)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	var gotOrder []string
	for _, s := range agg.Skills {
		gotOrder = append(gotOrder, s.Key)
	}
	wantOrder := []string{"reasoning", "coding", "long_context"} // AllowedSkills() order
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("Skills order = %v, want %v", gotOrder, wantOrder)
	}
}

func TestAllAggregates_MatchesPerModelAggregate(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()

	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "model/one", 5, []feedback.SkillRating{{Key: "coding", Rating: 5}}, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, "user-b", mustInput(t, "model/one", 3, []feedback.SkillRating{{Key: "coding", Rating: 3}}, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "model/two", 2, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}

	all, err := store.AllAggregates(ctx, now)
	if err != nil {
		t.Fatalf("AllAggregates: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("AllAggregates returned %d models, want 2", len(all))
	}

	for _, mk := range []feedback.ModelKey{"model/one", "model/two"} {
		single, err := store.Aggregate(ctx, mk, nil, now)
		if err != nil {
			t.Fatalf("Aggregate(%s): %v", mk, err)
		}
		fromAll := all[mk]
		if fromAll.Count != single.Count || fromAll.Average != single.Average {
			t.Errorf("AllAggregates[%s] = %+v, want to match Aggregate() = %+v", mk, fromAll, single)
		}
	}
}

func TestAllAggregates_UnratedModelAbsent(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := time.Now()
	if _, err := store.UpsertFeedback(ctx, "user-a", mustInput(t, "model/one", 5, nil, ""), now); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
	all, err := store.AllAggregates(ctx, now)
	if err != nil {
		t.Fatalf("AllAggregates: %v", err)
	}
	if _, ok := all["model/never-rated"]; ok {
		t.Fatal("AllAggregates included an unrated model")
	}
}

// TestForeignKeys_SkillRatingRequiresParentFeedback proves foreign_keys=ON
// is really enforced: a skill_ratings row inserted directly for a
// (identity_id, model_key) pair with no model_feedback row must fail.
func TestForeignKeys_SkillRatingRequiresParentFeedback(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO skill_ratings (identity_id, model_key, skill_key, rating) VALUES (?, ?, ?, ?)
	`, identityBytes("ghost"), "no/such-feedback", "coding", 5)
	if err == nil {
		t.Fatal("insert into skill_ratings with no parent model_feedback row succeeded, want FK violation")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
		t.Fatalf("error = %v, want a FOREIGN KEY constraint violation", err)
	}
}

// TestForeignKeys_ModelFeedbackRequiresIdentity proves the same for
// model_feedback -> identities.
func TestForeignKeys_ModelFeedbackRequiresIdentity(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO model_feedback (identity_id, model_key, overall, review, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, identityBytes("ghost"), "some/model", 5, "", formatTime(time.Now()), formatTime(time.Now()))
	if err == nil {
		t.Fatal("insert into model_feedback with no identities row succeeded, want FK violation")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
		t.Fatalf("error = %v, want a FOREIGN KEY constraint violation", err)
	}
}

// TestCheckConstraints_RatingRange proves the CHECK constraints from the
// contract's schema are really enforced at the SQL level, independent of
// domain-layer validation.
func TestCheckConstraints_RatingRange(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	now := formatTime(time.Now())

	if _, err := store.db.ExecContext(ctx, `INSERT INTO identities (id, created_at, last_seen_at) VALUES (?, ?, ?)`, identityBytes("user-a"), now, now); err != nil {
		t.Fatalf("insert identity: %v", err)
	}

	for _, overall := range []int{0, 6, -1} {
		_, err := store.db.ExecContext(ctx, `
			INSERT INTO model_feedback (identity_id, model_key, overall, review, created_at, updated_at)
			VALUES (?, ?, ?, '', ?, ?)
		`, identityBytes("user-a"), "some/model", overall, now, now)
		if err == nil {
			t.Fatalf("insert with overall=%d succeeded, want CHECK violation", overall)
		}
		if !strings.Contains(strings.ToUpper(err.Error()), "CHECK") {
			t.Fatalf("overall=%d error = %v, want a CHECK constraint violation", overall, err)
		}
	}
}

// TestUpsertFeedback_RollsBackOnSkillInsertError proves the whole
// transaction is atomic: forcing a skill_ratings insert to fail partway
// through (a duplicate skill_key within the same call, bypassing domain
// validation by calling the repository directly with a hand-built
// FeedbackInput) must leave no trace of the attempted overall/review update
// either.
func TestUpsertFeedback_RollsBackOnSkillInsertError(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-a")
	modelKey := feedback.ModelKey("anthropic/claude")
	now := time.Now()

	// Establish an initial, valid row.
	initial := mustInput(t, string(modelKey), 3, nil, "initial")
	if _, err := store.UpsertFeedback(ctx, identity, initial, now); err != nil {
		t.Fatalf("UpsertFeedback (initial): %v", err)
	}

	// Hand-build an input with a duplicate skill key -- domain validation
	// (ValidateSkillRatings) would reject this before it ever reaches the
	// repository; constructing it directly exercises the repository's own
	// transactional atomicity independent of that upstream guard.
	badInput := feedback.FeedbackInput{
		ModelKey: modelKey,
		Overall:  5,
		Skills: []feedback.SkillRating{
			{Key: "coding", Rating: 5},
			{Key: "coding", Rating: 1}, // duplicate primary key -> insert fails
		},
		Review: "should not stick",
	}
	_, err := store.UpsertFeedback(ctx, identity, badInput, now.Add(time.Hour))
	if err == nil {
		t.Fatal("UpsertFeedback with a duplicate skill key succeeded, want an error")
	}

	// The original row must be untouched: same overall/review, same
	// updated_at as after the first call.
	own, found, err := store.OwnFeedback(ctx, identity, modelKey)
	if err != nil || !found {
		t.Fatalf("OwnFeedback after failed upsert: found=%v err=%v", found, err)
	}
	if own.Overall != 3 || own.Review != "initial" {
		t.Fatalf("row changed despite rollback: %+v", own)
	}
	if !own.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want unchanged %v", own.UpdatedAt, now)
	}
	if len(own.Skills) != 0 {
		t.Fatalf("Skills = %+v, want none (the failed call's skills must not have partially applied)", own.Skills)
	}
}

// TestUpsertFeedback_ConcurrentSameIdentityNoDoubleVote fires two
// concurrent UpsertFeedback calls for the same identity/model and confirms
// exactly one model_feedback row exists afterward with one of the two
// submitted values -- never two rows, never a lost update silently
// producing a third value.
func TestUpsertFeedback_ConcurrentSameIdentityNoDoubleVote(t *testing.T) {
	ctx := context.Background()
	store := openMigratedStore(t)
	identity := feedback.IdentityID("user-a")
	modelKey := "anthropic/claude"

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	overalls := []int{2, 4}
	inputs := make([]feedback.FeedbackInput, len(overalls))
	for i, overall := range overalls {
		inputs[i] = mustInput(t, modelKey, overall, nil, "")
	}
	for i := range overalls {
		wg.Add(1)
		go func(input feedback.FeedbackInput, when time.Time) {
			defer wg.Done()
			_, err := store.UpsertFeedback(ctx, identity, input, when)
			errs <- err
		}(inputs[i], time.Now().Add(time.Duration(i)*time.Millisecond))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent UpsertFeedback: %v", err)
		}
	}

	var rowCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_feedback WHERE identity_id = ? AND model_key = ?`, identityBytes(identity), modelKey).Scan(&rowCount); err != nil {
		t.Fatalf("count model_feedback: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("model_feedback row count = %d, want 1 (no double vote)", rowCount)
	}

	own, found, err := store.OwnFeedback(ctx, identity, feedback.ModelKey(modelKey))
	if err != nil || !found {
		t.Fatalf("OwnFeedback: found=%v err=%v", found, err)
	}
	if own.Overall != 2 && own.Overall != 4 {
		t.Fatalf("final overall = %d, want one of the two submitted values", own.Overall)
	}
}

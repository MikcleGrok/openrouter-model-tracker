package feedback

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// newTestService returns a Service over a fresh fakeRepository, with its
// clock pinned to fixedNow so CreatedAt/UpdatedAt/ComputedAt assertions
// never depend on wall-clock timing. Tests that need the clock to advance
// mutate svc.now directly (an in-package test may reach the unexported
// field).
func newTestService(fixedNow time.Time) (*Service, *fakeRepository) {
	repo := newFakeRepository()
	svc := NewService(repo)
	svc.now = func() time.Time { return fixedNow }
	return svc, repo
}

func mustFeedbackInput(t *testing.T, modelKey string, overall int, skills []SkillRating, review string) FeedbackInput {
	t.Helper()
	input, err := NewFeedbackInput(modelKey, overall, skills, review)
	if err != nil {
		t.Fatalf("NewFeedbackInput(%q, %d, %v, %q) unexpected error: %v", modelKey, overall, skills, review, err)
	}
	return input
}

func TestSaveFeedbackCreatesNewRecord(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()

	input := mustFeedbackInput(t, "acme/model-1", 4, []SkillRating{{Key: "reasoning", Rating: 5}}, "solid model")
	got, err := svc.SaveFeedback(ctx, "identity-a", input)
	if err != nil {
		t.Fatalf("SaveFeedback: unexpected error: %v", err)
	}
	if got.ModelKey != "acme/model-1" || got.Overall != 4 || got.Review != "solid model" {
		t.Errorf("SaveFeedback result = %+v, want model=acme/model-1 overall=4 review=%q", got, "solid model")
	}
	if len(got.Skills) != 1 || got.Skills[0] != (SkillRating{Key: "reasoning", Rating: 5}) {
		t.Errorf("SaveFeedback result Skills = %v, want [{reasoning 5}]", got.Skills)
	}
	if !got.CreatedAt.Equal(t1) || !got.UpdatedAt.Equal(t1) {
		t.Errorf("SaveFeedback result timestamps = created=%v updated=%v, want both %v", got.CreatedAt, got.UpdatedAt, t1)
	}
}

func TestSaveFeedbackRejectsEmptyIdentity(t *testing.T) {
	svc, _ := newTestService(time.Now())
	input := mustFeedbackInput(t, "acme/model-1", 3, nil, "")
	if _, err := svc.SaveFeedback(context.Background(), "", input); err != ErrEmptyIdentity {
		t.Errorf("SaveFeedback with empty identity: err = %v, want ErrEmptyIdentity", err)
	}
}

func TestSaveFeedbackIsIdempotentUpdate(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(24 * time.Hour)
	svc, repo := newTestService(t1)
	ctx := context.Background()
	const identity IdentityID = "identity-a"

	first := mustFeedbackInput(t, "acme/model-1", 2, []SkillRating{{Key: "coding", Rating: 2}}, "meh")
	created, err := svc.SaveFeedback(ctx, identity, first)
	if err != nil {
		t.Fatalf("first SaveFeedback: unexpected error: %v", err)
	}

	svc.now = func() time.Time { return t2 }
	second := mustFeedbackInput(t, "acme/model-1", 5, []SkillRating{{Key: "coding", Rating: 5}}, "actually great")
	updated, err := svc.SaveFeedback(ctx, identity, second)
	if err != nil {
		t.Fatalf("second SaveFeedback: unexpected error: %v", err)
	}

	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("updated.CreatedAt = %v, want unchanged %v (repeated submission updates, never re-creates)", updated.CreatedAt, created.CreatedAt)
	}
	if !updated.UpdatedAt.Equal(t2) {
		t.Errorf("updated.UpdatedAt = %v, want %v", updated.UpdatedAt, t2)
	}
	if updated.Overall != 5 || updated.Review != "actually great" {
		t.Errorf("updated = %+v, want the second submission's values, not a blend with the first", updated)
	}

	// Exactly one row must exist: an idempotent update never becomes a
	// second vote. Community aggregate over this one identity must show
	// count 1 with today's (second) value, never 2.
	agg, err := svc.GetAggregate(ctx, "acme/model-1", nil)
	if err != nil {
		t.Fatalf("GetAggregate: unexpected error: %v", err)
	}
	if agg.Count != 1 {
		t.Errorf("GetAggregate.Count = %d, want 1 (idempotent update must not create a second vote)", agg.Count)
	}
	if agg.Average != 5 {
		t.Errorf("GetAggregate.Average = %v, want 5 (the latest submitted value)", agg.Average)
	}
	if got := len(repo.rows[identity]); got != 1 {
		t.Errorf("fakeRepository rows for identity = %d, want 1", got)
	}
}

func TestSaveFeedbackUpdateReplacesSkillsWholesale(t *testing.T) {
	// An update is a full replace of one row, not a merge (plan 5.2's
	// upsert-and-sync-skill-rows invariant, service-visible as: whatever
	// Skills the latest submission carries is exactly what is stored
	// afterward). A skill rated before but omitted now must disappear, and
	// submitting an empty skill set must be accepted and stored as empty.
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	const identity IdentityID = "identity-a"

	first := mustFeedbackInput(t, "acme/model-1", 4, []SkillRating{{Key: "reasoning", Rating: 5}, {Key: "coding", Rating: 3}}, "")
	if _, err := svc.SaveFeedback(ctx, identity, first); err != nil {
		t.Fatalf("first SaveFeedback: unexpected error: %v", err)
	}

	second := mustFeedbackInput(t, "acme/model-1", 4, nil, "")
	updated, err := svc.SaveFeedback(ctx, identity, second)
	if err != nil {
		t.Fatalf("second SaveFeedback (empty skills): unexpected error: %v", err)
	}
	if len(updated.Skills) != 0 {
		t.Errorf("updated.Skills = %v, want empty: a previously-rated skill omitted from the latest submission must not survive", updated.Skills)
	}

	own, err := svc.GetOwnFeedback(ctx, identity, "acme/model-1")
	if err != nil || own == nil {
		t.Fatalf("GetOwnFeedback after update: got %+v, err %v", own, err)
	}
	if len(own.Skills) != 0 {
		t.Errorf("GetOwnFeedback.Skills = %v, want empty (a stored empty skill set must round-trip as empty)", own.Skills)
	}
}

func TestGetOwnFeedbackAbsentReturnsNilWithoutError(t *testing.T) {
	svc, _ := newTestService(time.Now())
	got, err := svc.GetOwnFeedback(context.Background(), "identity-a", "acme/model-1")
	if err != nil {
		t.Fatalf("GetOwnFeedback: unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("GetOwnFeedback for a model never rated = %+v, want nil", got)
	}
}

func TestGetOwnFeedbackNeverLeaksAnotherIdentitysRow(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()

	if _, err := svc.SaveFeedback(ctx, "identity-a", mustFeedbackInput(t, "acme/model-1", 5, nil, "")); err != nil {
		t.Fatalf("SaveFeedback(A): unexpected error: %v", err)
	}
	if _, err := svc.SaveFeedback(ctx, "identity-b", mustFeedbackInput(t, "acme/model-1", 1, nil, "")); err != nil {
		t.Fatalf("SaveFeedback(B): unexpected error: %v", err)
	}

	ownA, err := svc.GetOwnFeedback(ctx, "identity-a", "acme/model-1")
	if err != nil || ownA == nil {
		t.Fatalf("GetOwnFeedback(A): got %+v, err %v", ownA, err)
	}
	if ownA.Overall != 5 {
		t.Errorf("GetOwnFeedback(A).Overall = %d, want 5 (A's own rating)", ownA.Overall)
	}

	ownB, err := svc.GetOwnFeedback(ctx, "identity-b", "acme/model-1")
	if err != nil || ownB == nil {
		t.Fatalf("GetOwnFeedback(B): got %+v, err %v", ownB, err)
	}
	if ownB.Overall != 1 {
		t.Errorf("GetOwnFeedback(B).Overall = %d, want 1 (B's own rating, not A's)", ownB.Overall)
	}
}

func TestCommunityAggregateCountsEveryValidVoteEqually(t *testing.T) {
	// A=5, B=3, C=1: if any implementation accidentally double-counted or
	// over-weighted one identity's own vote, the average below would drift
	// away from the plain arithmetic mean of all three.
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	votes := map[IdentityID]int{"identity-a": 5, "identity-b": 3, "identity-c": 1}
	for identity, overall := range votes {
		if _, err := svc.SaveFeedback(ctx, identity, mustFeedbackInput(t, "acme/model-1", overall, nil, "")); err != nil {
			t.Fatalf("SaveFeedback(%s): unexpected error: %v", identity, err)
		}
	}

	agg, err := svc.GetAggregate(ctx, "acme/model-1", nil)
	if err != nil {
		t.Fatalf("GetAggregate: unexpected error: %v", err)
	}
	if agg.Count != 3 {
		t.Errorf("GetAggregate.Count = %d, want 3", agg.Count)
	}
	if want := (5.0 + 3.0 + 1.0) / 3.0; agg.Average != want {
		t.Errorf("GetAggregate.Average = %v, want %v (plain mean, not weighted toward any one caller)", agg.Average, want)
	}
	if !agg.ComputedAt.Equal(t1) {
		t.Errorf("GetAggregate.ComputedAt = %v, want %v", agg.ComputedAt, t1)
	}
}

func TestGetAggregateOthersExcludesOnlyTheRequestedIdentity(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	votes := map[IdentityID]int{"identity-a": 5, "identity-b": 3, "identity-c": 1}
	for identity, overall := range votes {
		if _, err := svc.SaveFeedback(ctx, identity, mustFeedbackInput(t, "acme/model-1", overall, nil, "")); err != nil {
			t.Fatalf("SaveFeedback(%s): unexpected error: %v", identity, err)
		}
	}

	tests := []struct {
		exclude IdentityID
		want    Aggregate
	}{
		{"identity-c", Aggregate{Count: 2, Average: (5.0 + 3.0) / 2}},
		{"identity-a", Aggregate{Count: 2, Average: (3.0 + 1.0) / 2}},
		{"identity-b", Aggregate{Count: 2, Average: (5.0 + 1.0) / 2}},
	}
	for _, tt := range tests {
		exclude := tt.exclude
		others, err := svc.GetAggregate(ctx, "acme/model-1", &exclude)
		if err != nil {
			t.Fatalf("GetAggregate(others excluding %s): unexpected error: %v", exclude, err)
		}
		if others.Count != tt.want.Count || others.Average != tt.want.Average {
			t.Errorf("GetAggregate(others excluding %s) = count=%d avg=%v, want count=%d avg=%v",
				exclude, others.Count, others.Average, tt.want.Count, tt.want.Average)
		}
	}

	// The plain community scope (exclude nil) must still include everyone.
	community, err := svc.GetAggregate(ctx, "acme/model-1", nil)
	if err != nil {
		t.Fatalf("GetAggregate(community): unexpected error: %v", err)
	}
	if community.Count != 3 {
		t.Errorf("GetAggregate(community).Count = %d, want 3 (others must never replace the full community scope)", community.Count)
	}
}

func TestGetAggregateRejectsEmptyExcludeIdentity(t *testing.T) {
	svc, _ := newTestService(time.Now())
	empty := IdentityID("")
	if _, err := svc.GetAggregate(context.Background(), "acme/model-1", &empty); err != ErrEmptyIdentity {
		t.Errorf("GetAggregate with empty exclude: err = %v, want ErrEmptyIdentity", err)
	}
}

func TestPersonalPositionOrdersRatedModelsAndLeavesUnratedOut(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	const identity IdentityID = "identity-a"

	if _, err := svc.SaveFeedback(ctx, identity, mustFeedbackInput(t, "model-1", 5, nil, "")); err != nil {
		t.Fatalf("SaveFeedback(model-1): unexpected error: %v", err)
	}
	if _, err := svc.SaveFeedback(ctx, identity, mustFeedbackInput(t, "model-2", 3, nil, "")); err != nil {
		t.Fatalf("SaveFeedback(model-2): unexpected error: %v", err)
	}
	baseRanking := []ModelKey{"model-3", "model-2", "model-1"}

	pos1, err := svc.GetModelPositions(ctx, identity, "model-1", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(model-1): unexpected error: %v", err)
	}
	if pos1.PersonalPosition != (Position{Value: 1, Status: PositionStatusRanked}) {
		t.Errorf("model-1 (highest rated) PersonalPosition = %+v, want {1 ranked}", pos1.PersonalPosition)
	}

	pos2, err := svc.GetModelPositions(ctx, identity, "model-2", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(model-2): unexpected error: %v", err)
	}
	if pos2.PersonalPosition != (Position{Value: 2, Status: PositionStatusRanked}) {
		t.Errorf("model-2 (second highest rated) PersonalPosition = %+v, want {2 ranked}", pos2.PersonalPosition)
	}

	pos3, err := svc.GetModelPositions(ctx, identity, "model-3", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(model-3): unexpected error: %v", err)
	}
	if pos3.PersonalPosition != (Position{Status: PositionStatusUnranked}) {
		t.Errorf("model-3 (never rated by identity) PersonalPosition = %+v, want unranked with no value", pos3.PersonalPosition)
	}
	// An unrated model is never given a benchmark/ranking-derived stand-in
	// personal position either.
	if pos3.PersonalPosition.Value != 0 {
		t.Errorf("model-3 PersonalPosition.Value = %d, want 0 (never fabricated)", pos3.PersonalPosition.Value)
	}
}

func TestUpdateByIdentityADoesNotChangeIdentityBRowsOrPersonalPosition(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	baseRanking := []ModelKey{"model-1", "model-2"}

	// B rates two models, establishing a real personal ordering to protect.
	if _, err := svc.SaveFeedback(ctx, "identity-b", mustFeedbackInput(t, "model-1", 2, nil, "b's first take")); err != nil {
		t.Fatalf("SaveFeedback(B, model-1): unexpected error: %v", err)
	}
	if _, err := svc.SaveFeedback(ctx, "identity-b", mustFeedbackInput(t, "model-2", 4, nil, "")); err != nil {
		t.Fatalf("SaveFeedback(B, model-2): unexpected error: %v", err)
	}
	// A also rates model-1, so it has a community sample worth changing.
	if _, err := svc.SaveFeedback(ctx, "identity-a", mustFeedbackInput(t, "model-1", 5, nil, "a's first take")); err != nil {
		t.Fatalf("SaveFeedback(A, model-1): unexpected error: %v", err)
	}

	beforeOwnB, err := svc.GetOwnFeedback(ctx, "identity-b", "model-1")
	if err != nil || beforeOwnB == nil {
		t.Fatalf("GetOwnFeedback(B) before A's update: got %+v, err %v", beforeOwnB, err)
	}
	beforePosB1, err := svc.GetModelPositions(ctx, "identity-b", "model-1", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(B, model-1) before: unexpected error: %v", err)
	}
	beforePosB2, err := svc.GetModelPositions(ctx, "identity-b", "model-2", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(B, model-2) before: unexpected error: %v", err)
	}
	beforeCommunity, err := svc.GetAggregate(ctx, "model-1", nil)
	if err != nil {
		t.Fatalf("GetAggregate(model-1) before: unexpected error: %v", err)
	}

	// A updates their own feedback on the shared model.
	svc.now = func() time.Time { return t2 }
	if _, err := svc.SaveFeedback(ctx, "identity-a", mustFeedbackInput(t, "model-1", 1, nil, "a changed their mind")); err != nil {
		t.Fatalf("SaveFeedback(A, update): unexpected error: %v", err)
	}

	afterOwnB, err := svc.GetOwnFeedback(ctx, "identity-b", "model-1")
	if err != nil || afterOwnB == nil {
		t.Fatalf("GetOwnFeedback(B) after A's update: got %+v, err %v", afterOwnB, err)
	}
	if !reflect.DeepEqual(*afterOwnB, *beforeOwnB) {
		t.Errorf("GetOwnFeedback(B) changed after A's update: before=%+v after=%+v", *beforeOwnB, *afterOwnB)
	}

	afterPosB1, err := svc.GetModelPositions(ctx, "identity-b", "model-1", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(B, model-1) after: unexpected error: %v", err)
	}
	if afterPosB1.PersonalPosition != beforePosB1.PersonalPosition {
		t.Errorf("B's PersonalPosition(model-1) changed after A's update: before=%+v after=%+v", beforePosB1.PersonalPosition, afterPosB1.PersonalPosition)
	}
	afterPosB2, err := svc.GetModelPositions(ctx, "identity-b", "model-2", baseRanking)
	if err != nil {
		t.Fatalf("GetModelPositions(B, model-2) after: unexpected error: %v", err)
	}
	if afterPosB2.PersonalPosition != beforePosB2.PersonalPosition {
		t.Errorf("B's PersonalPosition(model-2) changed after A's update: before=%+v after=%+v", beforePosB2.PersonalPosition, afterPosB2.PersonalPosition)
	}

	// The community aggregate, in contrast, must reflect A's change.
	afterCommunity, err := svc.GetAggregate(ctx, "model-1", nil)
	if err != nil {
		t.Fatalf("GetAggregate(model-1) after: unexpected error: %v", err)
	}
	if afterCommunity.Average == beforeCommunity.Average {
		t.Errorf("community Average did not change after A's update: still %v", afterCommunity.Average)
	}
	if afterCommunity.Count != beforeCommunity.Count {
		t.Errorf("community Count changed from %d to %d; A's update should update her one row, not add a vote", beforeCommunity.Count, afterCommunity.Count)
	}
}

// TestUpdateByIdentityAAcrossCountMatrixDoesNotAffectIdentityB combines the
// two properties the brief's section 15 step 2 requires together, at every
// required count level, rather than each in isolation: A acting on a shared
// model M must never move B's own stored row or B's personal position on a
// completely unrelated model, whether M currently has 0, 1, 4, 5, 19, or 20
// other voters. The n=4 case is the interesting one: background voters
// alone leave M at count 4 (ineligible), and it is specifically identity
// A's own vote that tips M to count 5 (eligible) — the boundary crossing
// itself is exercised, not just the two sides of it independently.
func TestUpdateByIdentityAAcrossCountMatrixDoesNotAffectIdentityB(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	const identityA IdentityID = "matrix-ab-identity-a"
	const identityB IdentityID = "matrix-ab-identity-b"

	// B's own ratings, wholly unrelated to anything the loop below does:
	// neither model is ever touched by A or by any background identity.
	if _, err := svc.SaveFeedback(ctx, identityB, mustFeedbackInput(t, "matrix-ab/b-primary", 4, []SkillRating{{Key: "reasoning", Rating: 5}}, "b's own review")); err != nil {
		t.Fatalf("SaveFeedback(B, b-primary): unexpected error: %v", err)
	}
	if _, err := svc.SaveFeedback(ctx, identityB, mustFeedbackInput(t, "matrix-ab/b-secondary", 2, nil, "")); err != nil {
		t.Fatalf("SaveFeedback(B, b-secondary): unexpected error: %v", err)
	}
	bBaseRanking := []ModelKey{"matrix-ab/b-primary", "matrix-ab/b-secondary"}

	snapshotB := func(t *testing.T) (row1, row2 *OwnFeedback, pos1, pos2 Position) {
		t.Helper()
		row1, err := svc.GetOwnFeedback(ctx, identityB, "matrix-ab/b-primary")
		if err != nil || row1 == nil {
			t.Fatalf("GetOwnFeedback(B, b-primary): got %+v, err %v", row1, err)
		}
		row2, err = svc.GetOwnFeedback(ctx, identityB, "matrix-ab/b-secondary")
		if err != nil || row2 == nil {
			t.Fatalf("GetOwnFeedback(B, b-secondary): got %+v, err %v", row2, err)
		}
		posResult1, err := svc.GetModelPositions(ctx, identityB, "matrix-ab/b-primary", bBaseRanking)
		if err != nil {
			t.Fatalf("GetModelPositions(B, b-primary): unexpected error: %v", err)
		}
		posResult2, err := svc.GetModelPositions(ctx, identityB, "matrix-ab/b-secondary", bBaseRanking)
		if err != nil {
			t.Fatalf("GetModelPositions(B, b-secondary): unexpected error: %v", err)
		}
		return row1, row2, posResult1.PersonalPosition, posResult2.PersonalPosition
	}

	beforeRow1, beforeRow2, beforePos1, beforePos2 := snapshotB(t)
	if beforePos1 != (Position{Value: 1, Status: PositionStatusRanked}) {
		t.Fatalf("sanity check: B's b-primary PersonalPosition = %+v, want rank 1 (higher rating)", beforePos1)
	}
	if beforePos2 != (Position{Value: 2, Status: PositionStatusRanked}) {
		t.Fatalf("sanity check: B's b-secondary PersonalPosition = %+v, want rank 2", beforePos2)
	}

	for _, n := range []int{0, 1, 4, 5, 19, 20} {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			model := ModelKey(fmt.Sprintf("matrix-ab/model-n%d", n))
			for i := 0; i < n; i++ {
				background := IdentityID(fmt.Sprintf("matrix-ab-background-n%d-%d", n, i))
				if _, err := svc.SaveFeedback(ctx, background, mustFeedbackInput(t, string(model), 4, nil, "")); err != nil {
					t.Fatalf("SaveFeedback(background %s, %s): unexpected error: %v", background, model, err)
				}
			}
			before, err := svc.GetAggregate(ctx, model, nil)
			if err != nil {
				t.Fatalf("GetAggregate(%s) before A's vote: unexpected error: %v", model, err)
			}
			if before.Count != n {
				t.Fatalf("GetAggregate(%s) before A's vote: Count = %d, want %d", model, before.Count, n)
			}

			// Identity A votes on the shared model, becoming its (n+1)th
			// voter — the only action this subtest takes on shared state.
			if _, err := svc.SaveFeedback(ctx, identityA, mustFeedbackInput(t, string(model), 4, nil, "")); err != nil {
				t.Fatalf("SaveFeedback(A, %s): unexpected error: %v", model, err)
			}

			after, err := svc.GetAggregate(ctx, model, nil)
			if err != nil {
				t.Fatalf("GetAggregate(%s) after A's vote: unexpected error: %v", model, err)
			}
			if after.Count != n+1 {
				t.Errorf("GetAggregate(%s) after A's vote: Count = %d, want %d", model, after.Count, n+1)
			}
			wantEligible := CommunityPositionEligible(n + 1)
			positions, err := svc.GetModelPositions(ctx, identityA, model, nil)
			if err != nil {
				t.Fatalf("GetModelPositions(A, %s): unexpected error: %v", model, err)
			}
			if gotEligible := positions.CommunityPosition.Status == PositionStatusRanked; gotEligible != wantEligible {
				t.Errorf("n=%d (count after A votes=%d): CommunityPosition = %+v, want eligible=%v", n, n+1, positions.CommunityPosition, wantEligible)
			}

			// Whatever just happened to the shared model M, B's own row
			// and personal position on B's own, unrelated models must be
			// byte-identical to the snapshot taken before this test's loop
			// ever started.
			gotRow1, gotRow2, gotPos1, gotPos2 := snapshotB(t)
			if !reflect.DeepEqual(*gotRow1, *beforeRow1) {
				t.Errorf("n=%d: B's b-primary row changed: before=%+v after=%+v", n, *beforeRow1, *gotRow1)
			}
			if !reflect.DeepEqual(*gotRow2, *beforeRow2) {
				t.Errorf("n=%d: B's b-secondary row changed: before=%+v after=%+v", n, *beforeRow2, *gotRow2)
			}
			if gotPos1 != beforePos1 {
				t.Errorf("n=%d: B's b-primary PersonalPosition changed: before=%+v after=%+v", n, beforePos1, gotPos1)
			}
			if gotPos2 != beforePos2 {
				t.Errorf("n=%d: B's b-secondary PersonalPosition changed: before=%+v after=%+v", n, beforePos2, gotPos2)
			}
		})
	}
}

// TestCommunityPositionCountMatrix covers the identity/count matrix the
// task brief requires: community-position eligibility and ranking across
// vote counts 0, 1, 4, 5, 19, 20, cross-checked against exclusion by two
// different identities (A, a voter, and B, a non-voter/"outsider") at each
// count where they exist.
func TestCommunityPositionCountMatrix(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t1)
	ctx := context.Background()
	const outsider IdentityID = "outsider-never-votes"
	counts := []int{0, 1, 4, 5, 19, 20}

	modelFor := func(n int) ModelKey { return ModelKey(fmt.Sprintf("matrix/model-n%d", n)) }
	firstVoter := func(n int) IdentityID { return IdentityID(fmt.Sprintf("matrix-identity-n%d-0", n)) }

	// Every identity votes the same overall (4), so each model's community
	// score differs only by sample size n, isolating the eligibility/rank
	// behavior the matrix exists to prove.
	for _, n := range counts {
		model := modelFor(n)
		for i := 0; i < n; i++ {
			identity := IdentityID(fmt.Sprintf("matrix-identity-n%d-%d", n, i))
			if _, err := svc.SaveFeedback(ctx, identity, mustFeedbackInput(t, string(model), 4, nil, "")); err != nil {
				t.Fatalf("SaveFeedback(%s, %s): unexpected error: %v", identity, model, err)
			}
		}
	}

	for _, n := range counts {
		model := modelFor(n)

		community, err := svc.GetAggregate(ctx, model, nil)
		if err != nil {
			t.Fatalf("GetAggregate(%s): unexpected error: %v", model, err)
		}
		if community.Count != n {
			t.Errorf("n=%d: community Count = %d, want %d", n, community.Count, n)
		}
		if n > 0 && community.Average != 4 {
			t.Errorf("n=%d: community Average = %v, want 4", n, community.Average)
		}

		// Excluding a non-voter must never change the count.
		exclOutsider := outsider
		othersExclOutsider, err := svc.GetAggregate(ctx, model, &exclOutsider)
		if err != nil {
			t.Fatalf("GetAggregate(%s, exclude outsider): unexpected error: %v", model, err)
		}
		if othersExclOutsider.Count != n {
			t.Errorf("n=%d: excluding a non-voter changed Count to %d, want %d", n, othersExclOutsider.Count, n)
		}

		if n > 0 {
			// Excluding an actual voter (identity A for this group) must
			// drop the count by exactly one.
			exclA := firstVoter(n)
			othersExclA, err := svc.GetAggregate(ctx, model, &exclA)
			if err != nil {
				t.Fatalf("GetAggregate(%s, exclude A): unexpected error: %v", model, err)
			}
			if othersExclA.Count != n-1 {
				t.Errorf("n=%d: excluding voter A gave Count = %d, want %d", n, othersExclA.Count, n-1)
			}
			if othersExclA.Count > 0 && othersExclA.Average != 4 {
				t.Errorf("n=%d: excluding voter A gave Average = %v, want 4", n, othersExclA.Average)
			}
		}

		positions, err := svc.GetModelPositions(ctx, outsider, model, nil)
		if err != nil {
			t.Fatalf("GetModelPositions(%s): unexpected error: %v", model, err)
		}
		wantEligible := CommunityPositionEligible(n)
		gotEligible := positions.CommunityPosition.Status == PositionStatusRanked
		if gotEligible != wantEligible {
			t.Errorf("n=%d: CommunityPosition = %+v, want eligible=%v", n, positions.CommunityPosition, wantEligible)
		}
		if !wantEligible && positions.CommunityPosition.Value != 0 {
			t.Errorf("n=%d: ineligible CommunityPosition carries a Value (%d), want none", n, positions.CommunityPosition.Value)
		}
	}

	// Among the eligible models (n=5,19,20), all sharing the same average,
	// higher n must rank better: the shrinkage score climbs toward the raw
	// average as n grows, so n=20 > n=19 > n=5.
	rank := func(n int) int {
		pos, err := svc.GetModelPositions(ctx, outsider, modelFor(n), nil)
		if err != nil {
			t.Fatalf("GetModelPositions(n=%d) for ranking: unexpected error: %v", n, err)
		}
		if pos.CommunityPosition.Status != PositionStatusRanked {
			t.Fatalf("GetModelPositions(n=%d): expected eligible, got %+v", n, pos.CommunityPosition)
		}
		return pos.CommunityPosition.Value
	}
	r5, r19, r20 := rank(5), rank(19), rank(20)
	if !(r20 < r19 && r19 < r5) {
		t.Errorf("community-position ranks by n = {5:%d, 19:%d, 20:%d}, want strictly improving with larger n (20 < 19 < 5)", r20, r19, r5)
	}
}

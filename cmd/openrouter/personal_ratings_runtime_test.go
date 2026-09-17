package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

// personalRatingsTestModel builds a minimal model.Model for these tests —
// only Slug/DisplayName matter, since personal ratings ordering depends on
// the GetOwnFeedback fixture, not on price/quality fields.
func personalRatingsTestModel(slug, name string) model.Model {
	return model.Model{Slug: slug, DisplayName: name}
}

// personalRatingsRuntimeModel builds a tuiModel ready to drive through the
// real key() path, mirroring feedback_runtime_test.go's feedbackRuntimeModel
// — sized so the whole rated subset fits on screen without scrolling.
func personalRatingsRuntimeModel(rows []model.Model, client *feedbackclient.Client) tuiModel {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.visible, m.cursor, m.width, m.height = m.models, 0, 100, 30
	m.feedbackClient = client
	return m
}

// personalRatingsModelKeyFromPath extracts the model key from a
// GetOwnFeedback request path ("/v1/models/<key>/feedback/me"), the inverse
// of internal/feedback/client/transport.go's modelFeedbackPath.
func personalRatingsModelKeyFromPath(t *testing.T, path string) string {
	t.Helper()
	const prefix, suffix = "/v1/models/", "/feedback/me"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		t.Fatalf("unexpected GetOwnFeedback path: %s", path)
	}
	return strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
}

// personalRatingsOwnFeedbackJSON builds a GetOwnFeedback 200 response body
// (client.OwnFeedbackResponse): own_feedback is nil (never rated) when
// overall is 0, otherwise a full OwnFeedback with that Overall value.
func personalRatingsOwnFeedbackJSON(t *testing.T, modelKey string, overall int) string {
	t.Helper()
	body := map[string]any{"model_key": modelKey, "own_feedback": nil}
	if overall > 0 {
		body["own_feedback"] = map[string]any{
			"overall":    overall,
			"skills":     []any{},
			"review":     "",
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(encoded)
}

// TestPersonalRatingsToggleFetchesOrdersAndExcludesUnrated drives the whole
// feature through the real "M" key(), the real personalRatingsCmd batch
// fetch (one GetOwnFeedback request per catalog model, against a real HTTP
// fixture), and the real Update() application: only rated models must show
// up, ordered by Overall descending, and toggling off must restore the
// normal table with no leftover state.
func TestPersonalRatingsToggleFetchesOrdersAndExcludesUnrated(t *testing.T) {
	overalls := map[string]int{
		"vendor/a": 3,
		"vendor/b": 0, // never rated
		"vendor/c": 5,
		"vendor/d": 4,
	}
	var requests int32
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		key := personalRatingsModelKeyFromPath(t, r.URL.Path)
		overall, ok := overalls[key]
		if !ok {
			t.Fatalf("unexpected model key requested: %q", key)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(personalRatingsOwnFeedbackJSON(t, key, overall)))
	})
	rows := []model.Model{
		personalRatingsTestModel("vendor/a", "A"),
		personalRatingsTestModel("vendor/b", "B"),
		personalRatingsTestModel("vendor/c", "C"),
		personalRatingsTestModel("vendor/d", "D"),
	}
	m := personalRatingsRuntimeModel(rows, client)

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if !m.personalRatings.active || !m.personalRatings.loading {
		t.Fatalf("pressing M did not start loading the personal ratings view: %+v", m.personalRatings)
	}
	if view := m.View(); !strings.Contains(view, "Loading your ratings") {
		t.Fatalf("loading state not shown in view:\n%s", view)
	}

	cmd := m.personalRatingsCmd(m.models, m.personalRatingsBaseRanking(), m.personalRatings.generation)
	msg := runFeedbackCmd(t, cmd)
	m = runtimeTUIUpdate(t, m, msg)

	if got := atomic.LoadInt32(&requests); int(got) != len(rows) {
		t.Fatalf("expected exactly one GetOwnFeedback request per model (%d), got %d", len(rows), got)
	}
	if m.personalRatings.loading {
		t.Fatalf("still loading after the batch response landed")
	}
	var gotSlugs []string
	for _, row := range m.personalRatings.rows {
		gotSlugs = append(gotSlugs, row.model.Slug)
	}
	want := []string{"vendor/c", "vendor/d", "vendor/a"} // 5, 4, 3 descending; vendor/b (unrated) excluded
	if strings.Join(gotSlugs, ",") != strings.Join(want, ",") {
		t.Fatalf("personal ratings order = %v, want %v", gotSlugs, want)
	}
	if len(m.visible) != len(want) {
		t.Fatalf("m.visible length = %d, want %d (unrated model must not appear)", len(m.visible), len(want))
	}
	view := m.View()
	if !strings.Contains(view, "My ratings") {
		t.Fatalf("view does not show the personal ratings heading:\n%s", view)
	}
	for _, wantText := range []string{"5/5", "4/5", "3/5"} {
		if !strings.Contains(view, wantText) {
			t.Fatalf("view is missing rating %q:\n%s", wantText, view)
		}
	}

	// Toggling M again must return to the normal table, with no leaked
	// personal-ratings state into it.
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if m.personalRatings.active {
		t.Fatalf("second M press did not turn the view off")
	}
	if len(m.visible) != len(rows) {
		t.Fatalf("normal table not restored after toggling off: %d models visible, want %d", len(m.visible), len(rows))
	}
	if strings.Contains(m.View(), "My ratings") {
		t.Fatalf("normal table view still shows the personal ratings heading:\n%s", m.View())
	}
}

// TestSortPersonalRatingRowsOrdersByOverallThenBaseRank is a focused unit
// test of the tie-break rule alone (personal_ratings.go's
// lessByPersonalBaseRank), which mirrors
// internal/feedback/ranking.go's lessByBaseRank: Overall rating wins first;
// among equal ratings, the lower (better) base_position wins; a model
// absent from the base ranking always loses that tie.
func TestSortPersonalRatingRowsOrdersByOverallThenBaseRank(t *testing.T) {
	rows := []personalRatingRow{
		{model: model.Model{Slug: "vendor/low-rank-tie"}, overall: 4, basePosition: 5, hasBasePosition: true},
		{model: model.Model{Slug: "vendor/best"}, overall: 5, basePosition: 3, hasBasePosition: true},
		{model: model.Model{Slug: "vendor/high-rank-tie"}, overall: 4, basePosition: 1, hasBasePosition: true},
		{model: model.Model{Slug: "vendor/unranked-tie"}, overall: 4, hasBasePosition: false},
	}
	sortPersonalRatingRows(rows)
	var got []string
	for _, row := range rows {
		got = append(got, row.model.Slug)
	}
	want := []string{"vendor/best", "vendor/high-rank-tie", "vendor/low-rank-tie", "vendor/unranked-tie"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sortPersonalRatingRows order = %v, want %v", got, want)
	}
}

// TestPersonalRatingsStaleBatchAfterToggleOffThenOnDoesNotCorruptView is the
// staleness regression test: toggle on (generation 1, a fetch dispatched),
// toggle off, toggle on again (generation 2, a fresh fetch) — the FIRST
// batch's response, arriving late, must be dropped outright, and the
// current (generation 2) batch's own response must still apply normally.
func TestPersonalRatingsStaleBatchAfterToggleOffThenOnDoesNotCorruptView(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
	})
	rows := []model.Model{personalRatingsTestModel("vendor/a", "A"), personalRatingsTestModel("vendor/b", "B")}
	m := personalRatingsRuntimeModel(rows, client)

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	staleGeneration := m.personalRatings.generation
	if !m.personalRatings.active {
		t.Fatalf("first M press did not activate the view")
	}

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if m.personalRatings.active {
		t.Fatalf("second M press did not deactivate the view")
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	currentGeneration := m.personalRatings.generation
	if currentGeneration == staleGeneration {
		t.Fatalf("re-toggling on reused the stale generation: %d", currentGeneration)
	}
	if !m.personalRatings.active {
		t.Fatalf("third M press did not reactivate the view")
	}

	// The FIRST (now stale) batch's response finally arrives.
	stale := tuiPersonalRatingsMsg{generation: staleGeneration, rows: []personalRatingRow{{model: rows[0], overall: 1}}, total: len(rows)}
	before := m.personalRatings
	m = runtimeTUIUpdate(t, m, stale)
	if !reflect.DeepEqual(m.personalRatings, before) {
		t.Fatalf("a stale batch (generation %d, current %d) mutated state:\nbefore=%+v\nafter=%+v", staleGeneration, currentGeneration, before, m.personalRatings)
	}
	if strings.Contains(m.View(), "1/5") {
		t.Fatalf("stale rating leaked into the view:\n%s", m.View())
	}

	// The CURRENT batch's own response still applies normally.
	current := tuiPersonalRatingsMsg{generation: currentGeneration, rows: []personalRatingRow{{model: rows[1], overall: 5}}, total: len(rows)}
	m = runtimeTUIUpdate(t, m, current)
	if len(m.personalRatings.rows) != 1 || m.personalRatings.rows[0].model.Slug != "vendor/b" {
		t.Fatalf("current batch response was not applied: %+v", m.personalRatings)
	}
	if !strings.Contains(m.View(), "5/5") {
		t.Fatalf("current batch's rating not shown:\n%s", m.View())
	}
}

// TestPersonalRatingsToggleIsNoOpWhenFeedbackDisabled confirms the app stays
// fully usable, with the feature simply unavailable, when feedback.enabled
// is false (feedbackClient is nil) — mirroring
// TestFeedbackTabDisabledShowsExplanationAndIgnoresEdit's shape for the
// Feedback tab itself.
func TestPersonalRatingsToggleIsNoOpWhenFeedbackDisabled(t *testing.T) {
	rows := []model.Model{personalRatingsTestModel("vendor/a", "A")}
	m := personalRatingsRuntimeModel(rows, nil)

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if m.personalRatings.active {
		t.Fatalf("M activated the view while feedback is disabled")
	}
	view := m.View()
	if !strings.Contains(view, "Feedback is disabled") {
		t.Fatalf("disabled explanation not shown:\n%s", view)
	}
}

// TestPersonalRatingsFetchBoundsConcurrency confirms the batch fetch never
// runs more than personalRatingsConcurrency GetOwnFeedback requests at
// once, over a catalog large enough that an unbounded fetch would visibly
// exceed it.
func TestPersonalRatingsFetchBoundsConcurrency(t *testing.T) {
	var inFlight, maxInFlight int32
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			observed := atomic.LoadInt32(&maxInFlight)
			if current <= observed {
				break
			}
			if atomic.CompareAndSwapInt32(&maxInFlight, observed, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		key := personalRatingsModelKeyFromPath(t, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(personalRatingsOwnFeedbackJSON(t, key, 0)))
	})
	rows := make([]model.Model, 0, personalRatingsConcurrency*3)
	for i := 0; i < personalRatingsConcurrency*3; i++ {
		slug := fmt.Sprintf("vendor/model-%d", i)
		rows = append(rows, personalRatingsTestModel(slug, slug))
	}
	m := personalRatingsRuntimeModel(rows, client)

	cmd := m.personalRatingsCmd(rows, map[string]int{}, 1)
	runFeedbackCmd(t, cmd)

	if got := atomic.LoadInt32(&maxInFlight); got > int32(personalRatingsConcurrency) {
		t.Fatalf("observed %d concurrent GetOwnFeedback requests, want at most %d", got, personalRatingsConcurrency)
	}
	if got := atomic.LoadInt32(&maxInFlight); got < 2 {
		t.Fatalf("test did not actually exercise concurrency: max observed in flight = %d", got)
	}
}

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
// of internal/feedback/client/transport.go's modelFeedbackPath. It reports
// ok=false instead of failing the test itself: every call site here runs
// inside an httptest handler goroutine, and *testing.T.Fatal(f) must only
// ever be called from the goroutine running the test function — calling it
// elsewhere does not fail the test cleanly and can hang it instead.
func personalRatingsModelKeyFromPath(path string) (string, bool) {
	const prefix, suffix = "/v1/models/", "/feedback/me"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix), true
}

// personalRatingsOwnFeedbackJSON builds a GetOwnFeedback 200 response body
// (client.OwnFeedbackResponse): own_feedback is nil (never rated) when
// overall is 0, otherwise a full OwnFeedback with that Overall value. It
// returns the marshal error instead of calling t.Fatal itself, for the same
// goroutine-safety reason as personalRatingsModelKeyFromPath above — every
// call site is inside an httptest handler.
func personalRatingsOwnFeedbackJSON(modelKey string, overall int) (string, error) {
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
	return string(encoded), err
}

// writePersonalRatingsOwnFeedback resolves the model key from r's path and
// writes its GetOwnFeedback fixture, reporting any problem via t.Errorf
// (safe from a handler goroutine) rather than t.Fatalf.
func writePersonalRatingsOwnFeedback(t *testing.T, w http.ResponseWriter, r *http.Request, overallByKey map[string]int) {
	t.Helper()
	key, ok := personalRatingsModelKeyFromPath(r.URL.Path)
	if !ok {
		t.Errorf("unexpected GetOwnFeedback path: %s", r.URL.Path)
		return
	}
	overall, known := overallByKey[key]
	if !known {
		t.Errorf("unexpected model key requested: %q", key)
		return
	}
	body, err := personalRatingsOwnFeedbackJSON(key, overall)
	if err != nil {
		t.Errorf("marshal fixture: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(body))
}

// TestPersonalRatingsToggleFetchesOrdersAndExcludesUnrated drives the whole
// feature through the real "M" key() dispatch — using the actual tea.Cmd it
// returns, not a hand-reconstructed one, so a bug that made
// togglePersonalRatings return nil or the wrong generation/models/baseRank
// would fail this test instead of passing it unnoticed — the real
// personalRatingsCmd batch fetch (one GetOwnFeedback request per catalog
// model, against a real HTTP fixture), and the real Update() application:
// only rated models must show up, ordered by Overall descending, and
// toggling off must restore the normal table with the exact cursor/
// selection the user had before entering the mode (not wherever the
// freshly loaded batch's own top row happens to be).
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
		writePersonalRatingsOwnFeedback(t, w, r, overalls)
	})
	rows := []model.Model{
		personalRatingsTestModel("vendor/a", "A"),
		personalRatingsTestModel("vendor/b", "B"),
		personalRatingsTestModel("vendor/c", "C"),
		personalRatingsTestModel("vendor/d", "D"),
	}
	m := personalRatingsRuntimeModel(rows, client)

	// Move off the initial cursor position first, so restoring "wherever
	// the batch happens to land" (its own top-rated row) would be visibly
	// distinguishable from actually restoring the pre-toggle position.
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	wantCursor := m.cursor
	wantSlug := m.visible[m.cursor].Slug

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	m = next.(tuiModel)
	if !m.personalRatings.active || !m.personalRatings.loading {
		t.Fatalf("pressing M did not start loading the personal ratings view: %+v", m.personalRatings)
	}
	if cmd == nil {
		t.Fatalf("M keypress did not return a fetch command")
	}
	if m.personalRatings.cancel == nil {
		t.Fatalf("toggling on did not record a cancel func")
	}
	if view := m.View(); !strings.Contains(view, "Loading your ratings") {
		t.Fatalf("loading state not shown in view:\n%s", view)
	}

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

	// Toggling M again must return to the normal table, restoring the
	// EXACT cursor/selection captured before entering the mode — not the
	// freshly loaded batch's own top row (vendor/c, cursor 0).
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if m.personalRatings.active {
		t.Fatalf("second M press did not turn the view off")
	}
	if m.personalRatings.cancel != nil {
		t.Fatalf("toggling off did not clear the cancel func")
	}
	if len(m.visible) != len(rows) {
		t.Fatalf("normal table not restored after toggling off: %d models visible, want %d", len(m.visible), len(rows))
	}
	if strings.Contains(m.View(), "My ratings") {
		t.Fatalf("normal table view still shows the personal ratings heading:\n%s", m.View())
	}
	if m.cursor != wantCursor {
		t.Fatalf("cursor not restored after toggling off: got %d, want %d", m.cursor, wantCursor)
	}
	if m.selectedSlug != wantSlug || m.visible[m.cursor].Slug != wantSlug {
		t.Fatalf("selection not restored after toggling off: selectedSlug=%q cursor row=%q, want %q", m.selectedSlug, m.visible[m.cursor].Slug, wantSlug)
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
		t.Errorf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
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
	got := m.personalRatings
	// cancel is a func value: reflect.DeepEqual never considers two
	// non-nil funcs equal, even the identical closure, so it is excluded
	// from this comparison on both sides — every other field still is
	// compared, which is what actually matters here.
	before.cancel, got.cancel = nil, nil
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("a stale batch (generation %d, current %d) mutated state:\nbefore=%+v\nafter=%+v", staleGeneration, currentGeneration, before, got)
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
		key, ok := personalRatingsModelKeyFromPath(r.URL.Path)
		if !ok {
			t.Errorf("unexpected GetOwnFeedback path: %s", r.URL.Path)
			return
		}
		body, err := personalRatingsOwnFeedbackJSON(key, 0)
		if err != nil {
			t.Errorf("marshal fixture: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	})
	rows := make([]model.Model, 0, personalRatingsConcurrency*3)
	for i := 0; i < personalRatingsConcurrency*3; i++ {
		slug := fmt.Sprintf("vendor/model-%d", i)
		rows = append(rows, personalRatingsTestModel(slug, slug))
	}
	m := personalRatingsRuntimeModel(rows, client)

	cmd := m.personalRatingsCmd(context.Background(), rows, map[string]int{}, 1)
	runFeedbackCmd(t, cmd)

	if got := atomic.LoadInt32(&maxInFlight); got > int32(personalRatingsConcurrency) {
		t.Fatalf("observed %d concurrent GetOwnFeedback requests, want at most %d", got, personalRatingsConcurrency)
	}
	if got := atomic.LoadInt32(&maxInFlight); got < 2 {
		t.Fatalf("test did not actually exercise concurrency: max observed in flight = %d", got)
	}
}

// TestPersonalRatingsToggleOffCancelsInFlightFetch confirms the context
// togglePersonalRatings hands to the batch fetch actually stops it: once
// cancelled (exactly what toggling off does — see
// TestPersonalRatingsToggleFetchesOrdersAndExcludesUnrated's own assertion
// that toggling off clears m.personalRatings.cancel), every GetOwnFeedback
// call fails immediately without ever reaching the server, instead of
// mashing M on a large catalog opening N overlapping batches of real HTTP
// traffic.
func TestPersonalRatingsToggleOffCancelsInFlightFetch(t *testing.T) {
	var served int32
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&served, 1)
		writePersonalRatingsOwnFeedback(t, w, r, map[string]int{})
	})
	rows := make([]model.Model, 0, 20)
	for i := 0; i < 20; i++ {
		slug := fmt.Sprintf("vendor/model-%d", i)
		rows = append(rows, personalRatingsTestModel(slug, slug))
	}
	m := personalRatingsRuntimeModel(rows, client)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	m = next.(tuiModel)
	if cmd == nil {
		t.Fatalf("M keypress did not return a fetch command")
	}
	cancel := m.personalRatings.cancel
	if cancel == nil {
		t.Fatalf("toggling on did not record a cancel func")
	}

	// Simulate the user toggling off before the batch gets a chance to run
	// at all — the real key() path for toggling off calls exactly this
	// cancel func (asserted separately in the main toggle test above).
	cancel()

	msg := runFeedbackCmd(t, cmd)
	got, ok := msg.(tuiPersonalRatingsMsg)
	if !ok {
		t.Fatalf("unexpected message type: %#v", msg)
	}
	if got.errCount != len(rows) {
		t.Fatalf("expected every request to fail once the context was cancelled before the fetch ran: errCount=%d, want %d", got.errCount, len(rows))
	}
	if served := atomic.LoadInt32(&served); served > 0 {
		t.Fatalf("a fetch cancelled before it ran still reached the server %d time(s)", served)
	}
}

// TestPersonalRatingsViewTracksLiveCatalogAcrossRefresh confirms the app's
// own periodic background refresh (which replaces m.models and calls
// rebuild — see tui.go's tuiRefreshMsg handling) never leaves this view
// showing a model that was removed from the live catalog, and shows live
// catalog data (not a frozen snapshot from fetch time) for a model that is
// still present. The rating value/order itself is deliberately left as the
// last fetch computed it — see personalRatingsEffectiveRows's own doc
// comment for why that matches the Feedback tab's existing precedent.
func TestPersonalRatingsViewTracksLiveCatalogAcrossRefresh(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
	})
	rows := []model.Model{personalRatingsTestModel("vendor/a", "A"), personalRatingsTestModel("vendor/b", "B")}
	m := personalRatingsRuntimeModel(rows, client)

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	msg := tuiPersonalRatingsMsg{
		generation: m.personalRatings.generation,
		rows: []personalRatingRow{
			{model: rows[0], overall: 5},
			{model: rows[1], overall: 4},
		},
		total: 2,
	}
	m = runtimeTUIUpdate(t, m, msg)
	if len(m.visible) != 2 {
		t.Fatalf("expected both rated models visible before refresh, got %d", len(m.visible))
	}

	// Simulate the app's own periodic refresh: vendor/b is removed from the
	// live catalog entirely, and vendor/a's display name changes — exactly
	// what tuiRefreshMsg's own handler does before calling m.rebuild().
	m.models = []model.Model{{Slug: "vendor/a", DisplayName: "A Renamed"}}
	m.rebuild()

	if len(m.visible) != 1 || m.visible[0].Slug != "vendor/a" {
		t.Fatalf("removed model still visible after refresh: %+v", m.visible)
	}
	if m.visible[0].DisplayName != "A Renamed" {
		t.Fatalf("m.visible did not pick up the refreshed catalog data: %+v", m.visible[0])
	}
	if !strings.Contains(m.View(), "A Renamed") {
		t.Fatalf("view does not reflect the refreshed catalog data:\n%s", m.View())
	}
}

// TestPersonalRatingsSyncsAfterFeedbackTabSave is the repro from the review:
// open My-ratings, Enter into a model's detail, change its rating from
// inside the Feedback tab, Esc back — the still-open My-ratings list must
// reflect the new rating (and re-sort) without the user manually toggling M
// off and back on.
func TestPersonalRatingsSyncsAfterFeedbackTabSave(t *testing.T) {
	initialOveralls := map[string]int{"vendor/a": 3, "vendor/b": 5}
	var lastPutOverall int
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/feedback/me"):
			writePersonalRatingsOwnFeedback(t, w, r, initialOveralls)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/feedback/summary"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(feedbackSummaryJSON(t, "vendor/a", initialOveralls["vendor/a"], 0, 0)))
		case r.Method == http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode PUT body: %v", err)
				return
			}
			overall, ok := body["overall"].(float64)
			if !ok {
				t.Errorf("PUT body missing numeric overall: %#v", body)
				return
			}
			lastPutOverall = int(overall)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(feedbackSummaryJSON(t, "vendor/a", lastPutOverall, 0, 0)))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	rows := []model.Model{personalRatingsTestModel("vendor/a", "A"), personalRatingsTestModel("vendor/b", "B")}
	m := personalRatingsRuntimeModel(rows, client)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	m = next.(tuiModel)
	m = runtimeTUIUpdate(t, m, runFeedbackCmd(t, cmd))
	if len(m.personalRatings.rows) != 2 || m.personalRatings.rows[0].model.Slug != "vendor/b" {
		t.Fatalf("unexpected initial My-ratings order: %+v", m.personalRatings.rows)
	}

	// Move the cursor onto vendor/a and drill into its detail's Feedback tab.
	for i, row := range m.visible {
		if row.Slug == "vendor/a" {
			m.cursor = i
		}
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")}) // open Feedback tab
	m = runtimeTUIUpdate(t, m, runFeedbackCmd(t, m.feedbackSummaryCmd("vendor/a", m.feedback.reqSeq, false)))
	if !m.feedback.loaded {
		t.Fatalf("Feedback tab summary did not load")
	}

	// Re-rate vendor/a from 3 to 5 (tying vendor/b) and save.
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	next, saveCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(tuiModel)
	if saveCmd == nil {
		t.Fatalf("Ctrl+S did not return a save command")
	}
	m = runtimeTUIUpdate(t, m, runFeedbackCmd(t, saveCmd))
	if lastPutOverall != 5 {
		t.Fatalf("PUT did not carry Overall=5: got %d", lastPutOverall)
	}
	if m.feedback.saveErr != "" || m.feedback.editing {
		t.Fatalf("save did not complete cleanly: saveErr=%q editing=%v", m.feedback.saveErr, m.feedback.editing)
	}

	gotOverall := map[string]int{}
	for _, row := range m.personalRatings.rows {
		gotOverall[row.model.Slug] = row.overall
	}
	if gotOverall["vendor/a"] != 5 {
		t.Fatalf("the open My-ratings list did not pick up the new rating: %+v", gotOverall)
	}

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // back to My-ratings
	if !m.personalRatings.active {
		t.Fatalf("Esc from detail left the My-ratings mode inactive")
	}
	if !strings.Contains(m.View(), "5/5") {
		t.Fatalf("My-ratings view does not show the updated rating:\n%s", m.View())
	}
}

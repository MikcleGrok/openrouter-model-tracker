package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

// newFeedbackRuntimeClient builds a real *feedbackclient.Client against an
// httptest.Server running handler, writing the token_file/identity_file it
// needs to disk — the same shape internal/feedback/client's own tests use
// (testEnv in client_test.go), reused here at the TUI layer so these tests
// exercise the real HTTP client, not a hand-rolled fake.
func newFeedbackRuntimeClient(t *testing.T, handler http.HandlerFunc) *feedbackclient.Client {
	t.Helper()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("test-shared-secret\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if _, _, err := feedbackclient.EnsureIdentityFile(identityFile); err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := feedbackclient.New(feedbackclient.Config{
		Endpoint:       server.URL,
		TokenFile:      tokenFile,
		IdentityFile:   identityFile,
		RequestTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("feedbackclient.New: %v", err)
	}
	return c
}

// feedbackRuntimeModel builds a tuiModel with the detail overlay already
// open on row, ready to switch into the Feedback tab.
func feedbackRuntimeModel(row model.Model, client *feedbackclient.Client) tuiModel {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 100, 30
	m.overlay, m.detailTabsActive, m.detailOffset = "detail", true, 0
	m.feedbackClient = client
	return m
}

func feedbackSummaryJSON(t *testing.T, modelKey string, mineOverall int, communityAvg float64, communityCount int) string {
	t.Helper()
	body := map[string]any{
		"model_key": modelKey,
		"mine": map[string]any{
			"overall":    mineOverall,
			"skills":     []any{},
			"review":     "",
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		},
		"community": map[string]any{
			"count":        communityCount,
			"average":      communityAvg,
			"distribution": [5]int{0, 0, 0, communityCount, 0},
			"skills":       []any{},
			"computed_at":  time.Now().UTC().Format(time.RFC3339),
		},
		"base_position":      map[string]any{"status": "unranked"},
		"personal_position":  map[string]any{"status": "unranked"},
		"community_position": map[string]any{"status": "unranked"},
	}
	if mineOverall == 0 {
		body["mine"] = nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(encoded)
}

// runFeedbackCmd runs a tea.Cmd synchronously and returns its tea.Msg — the
// tests below drive Update() with real messages this way instead of poking
// state directly, following tui_runtime_test.go's runtimeTUIUpdate pattern
// of exercising the actual Update/key path.
func runFeedbackCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected a non-nil tea.Cmd")
	}
	return cmd()
}

func feedbackTestRow() model.Model {
	return model.Model{Slug: "vendor/model", DisplayName: "Vendor Model"}
}

// TestFeedbackTabLoadingThenSuccess covers brief 11.3's "loading" and
// "success" cases in one pass: opening the tab shows the loading skeleton
// and issues exactly one GetSummary call; applying its result replaces the
// skeleton with the separate My rating / Community rating blocks.
func TestFeedbackTabLoadingThenSuccess(t *testing.T) {
	var requests int
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.HasSuffix(r.URL.Path, "/feedback/summary") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(feedbackSummaryJSON(t, "vendor/model", 4, 4.2, 10)))
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if m.detailTab != detailTabFeedback || !m.feedback.loading {
		t.Fatalf("opening tab 5 should start loading: tab=%d loading=%v", m.detailTab, m.feedback.loading)
	}
	if view := m.View(); !strings.Contains(view, "Loading feedback") {
		t.Fatalf("loading skeleton not shown in view:\n%s", view)
	}

	cmd := m.feedbackSummaryCmd(row.Slug, m.feedback.reqSeq, false)
	msg := runFeedbackCmd(t, cmd)
	m = runtimeTUIUpdate(t, m, msg)

	if requests != 1 {
		t.Fatalf("expected exactly one GetSummary request, got %d", requests)
	}
	if m.feedback.loading || !m.feedback.loaded || m.feedback.loadErr != "" {
		t.Fatalf("state after success: loading=%v loaded=%v err=%q", m.feedback.loading, m.feedback.loaded, m.feedback.loadErr)
	}
	view := m.View()
	if !strings.Contains(view, "My rating: 4/5") {
		t.Fatalf("view missing my rating:\n%s", view)
	}
	if !strings.Contains(view, "Community rating: 4.2 (10 votes)") {
		t.Fatalf("view missing community rating:\n%s", view)
	}
	// My rating and Community rating must be visually distinct lines, never
	// blended into one value.
	if strings.Contains(view, "4/5 (4.2") || strings.Contains(view, "4.2 (10 votes)4/5") {
		t.Fatalf("my rating and community rating appear blended:\n%s", view)
	}
}

// TestFeedbackStaleSummaryResponseForDifferentModelDoesNotRepaintScreen is
// brief 11.3's actual "stale response" scenario — "поздний ответ старой
// модели не должен перерисовать новую" — a late GetSummary for a DIFFERENT
// model the user has since left, arriving while a DIFFERENT model is now on
// screen. This is deliberately not the same as a same-slug retry
// superseding an earlier same-slug request (covered by
// TestFeedbackTabLoadingThenSuccess's single-request assertion and by the
// save-path test below): a real model switch closes the detail overlay,
// moves the cursor, and reopens it on a different row entirely.
func TestFeedbackStaleSummaryResponseForDifferentModelDoesNotRepaintScreen(t *testing.T) {
	rowA := model.Model{Slug: "vendor/model-a", DisplayName: "Model A"}
	rowB := model.Model{Slug: "vendor/model-b", DisplayName: "Model B"}
	// The real HTTP round trip is never exercised here: runtimeTUIUpdate
	// only runs Update(), never the tea.Cmd it returns (matching this whole
	// file's convention), so every GetSummary result below is fed in
	// directly as a message. Only a non-nil client is needed, to let
	// ensureFeedbackSummaryLoaded actually dispatch a (never-run) load.
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
	})
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{rowA, rowB})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 100, 30
	m.feedbackClient = client

	// Open A's Feedback tab: a GetSummary for A is now in flight.
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	staleSlug, staleSeq := m.feedback.slug, m.feedback.reqSeq
	if staleSlug != rowA.Slug {
		t.Fatalf("opened tab on the wrong model: %q", staleSlug)
	}

	// The user leaves A without waiting for the response: close detail, move
	// to B, reopen detail, and open B's Feedback tab — a second GetSummary,
	// for a completely different model, is now the current request.
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if m.feedback.slug != rowB.Slug {
		t.Fatalf("Feedback tab did not switch to model B: %+v", m.feedback)
	}
	if m.feedback.reqSeq == staleSeq {
		t.Fatalf("opening B's tab reused A's request sequence number")
	}

	// A's slow response for the OLD slug/seq now finally arrives.
	stale := tuiFeedbackSummaryMsg{slug: staleSlug, reqSeq: staleSeq, summary: feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 1}}}
	before := m.feedback
	m = runtimeTUIUpdate(t, m, stale)
	if m.feedback != before {
		t.Fatalf("A's stale response (slug %q seq %d) mutated state while B (slug %q seq %d) is on screen:\nbefore=%+v\nafter=%+v",
			staleSlug, staleSeq, m.feedback.slug, m.feedback.reqSeq, before, m.feedback)
	}
	if strings.Contains(m.View(), "My rating: 1/5") {
		t.Fatalf("stale model A rating leaked into the view while B is displayed:\n%s", m.View())
	}

	// B's own (current) response still applies normally.
	current := tuiFeedbackSummaryMsg{slug: rowB.Slug, reqSeq: m.feedback.reqSeq, summary: feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 5}}}
	m = runtimeTUIUpdate(t, m, current)
	if !strings.Contains(m.View(), "My rating: 5/5") {
		t.Fatalf("B's current response was not applied:\n%s", m.View())
	}
}

// TestFeedbackStaleSaveResponseAfterModelSwitchAndBackDoesNotOverwriteRetry
// is the regression test for the review's "save-path stale-response guard
// isn't actually monotonic" finding: startFeedbackLoad used to reset the
// whole tuiFeedbackState struct, silently zeroing a separate saveSeq field
// back to 0 every time — so a save started, then abandoned by switching to
// another model and back, then retried, could make the RETRY's sequence
// number collide with the ORIGINAL, still-in-flight save's number, letting
// the stale first response win over the real retry's outcome. reqSeq is now
// the one counter both loads and saves advance, so this can no longer
// happen: concretely, save A (slow) -> switch away and back to A (a load) ->
// save A again (the retry) -> the first save's late response must be
// dropped, and the retry's response must be the one that applies.
func TestFeedbackStaleSaveResponseAfterModelSwitchAndBackDoesNotOverwriteRetry(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)

	// 1. Load A, then start a first save (Overall=2) — the "slow" PUT whose
	// response arrives much later, after the sequence below.
	m, _ = m.startFeedbackLoad(row, false)
	m = runtimeTUIUpdate(t, m, tuiFeedbackSummaryMsg{slug: row.Slug, reqSeq: m.feedback.reqSeq, summary: feedbackclient.Summary{}})
	m.feedback.editing, m.feedback.draftOverall = true, 2
	m, _ = m.startFeedbackSave()
	staleSaveSeq := m.feedback.reqSeq
	if !m.feedback.saving {
		t.Fatalf("first save did not start")
	}

	// 2. The user switches away and back to A before that slow PUT ever
	// resolves. reqSeq must land strictly ahead of the in-flight save.
	m, _ = m.startFeedbackLoad(row, false)
	if m.feedback.reqSeq <= staleSaveSeq {
		t.Fatalf("reqSeq did not advance past the in-flight save: stale=%d after-reload=%d", staleSaveSeq, m.feedback.reqSeq)
	}
	m = runtimeTUIUpdate(t, m, tuiFeedbackSummaryMsg{slug: row.Slug, reqSeq: m.feedback.reqSeq, summary: feedbackclient.Summary{}})

	// 3. The user edits and saves again (Overall=5) — the retry the stale
	// response must never be allowed to clobber.
	m.feedback.editing, m.feedback.draftOverall = true, 5
	m, _ = m.startFeedbackSave()
	currentSaveSeq := m.feedback.reqSeq
	if currentSaveSeq == staleSaveSeq {
		t.Fatalf("the retry reused the first save's sequence number: %d", currentSaveSeq)
	}

	// 4. The FIRST save's response finally arrives, tagged with the now-
	// stale sequence number. It must be dropped, not applied.
	staleSave := tuiFeedbackSaveMsg{slug: row.Slug, reqSeq: staleSaveSeq, summary: feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 2}}}
	before := m.feedback
	m = runtimeTUIUpdate(t, m, staleSave)
	if m.feedback != before {
		t.Fatalf("a stale save response (seq %d, current %d) mutated state:\nbefore=%+v\nafter=%+v", staleSaveSeq, m.feedback.reqSeq, before, m.feedback)
	}

	// 5. The retry's own (current) response arrives and must apply normally.
	currentSave := tuiFeedbackSaveMsg{slug: row.Slug, reqSeq: currentSaveSeq, summary: feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 5}}}
	m = runtimeTUIUpdate(t, m, currentSave)
	if m.feedback.saving || m.feedback.summary.Mine == nil || m.feedback.summary.Mine.Overall != 5 {
		t.Fatalf("the retry's own response was not applied: %+v", m.feedback)
	}
}

// TestFeedbackSaveWhileLoadInFlightDoesNotStrandTheLoadingSpinner is the
// regression test for round 2's own finding: round 1 fixed the stale-save
// race by unifying two independently-resettable counters into one shared
// reqSeq, but checking BOTH load and save responses against that SAME shared
// field meant a save dispatched while a load was still in flight bumped the
// very field the load's own freshness check depended on — so the load's
// eventual response was discarded as stale, and since "loading = false" only
// ever happens inside that discarded branch, the tab was stuck showing
// "Loading feedback..." forever (the only way out was leaving the tab and
// coming back, since retry/others/ensureFeedbackSummaryLoaded are themselves
// gated on !loading). loadReqSeq and saveReqSeq must be independent markers,
// both still drawn from the one shared, never-decreasing reqSeq, so a save
// can never invalidate a load's own tracking or vice versa.
func TestFeedbackSaveWhileLoadInFlightDoesNotStrandTheLoadingSpinner(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)

	// 1. Open the tab: a GetSummary is dispatched and is still "in flight" —
	// its response is not delivered yet.
	m, _ = m.startFeedbackLoad(row, false)
	loadReqSeq := m.feedback.reqSeq
	if !m.feedback.loading {
		t.Fatalf("load did not start")
	}

	// 2. Before that response arrives, the user rates and saves. Dispatched
	// directly at the state-machine level: startFeedbackSave has no loading
	// guard of its own (only feedbackViewKey's "edit" action gates entry on
	// !loading, a separate, additional defense not exercised by this test),
	// so this reaches exactly the interleaving the finding describes
	// regardless of how the UI arrives at it.
	m.feedback.editing, m.feedback.draftOverall = true, 4
	m, _ = m.startFeedbackSave()
	if !m.feedback.saving {
		t.Fatalf("save did not start")
	}
	if m.feedback.reqSeq == loadReqSeq {
		t.Fatalf("save did not advance the shared reqSeq counter")
	}
	if !m.feedback.loading {
		t.Fatalf("starting the save incorrectly cleared the still-in-flight load's loading flag")
	}

	// 3. The load's response finally arrives, tagged with its own original
	// sequence number — which by now differs from the save's.
	loadResp := tuiFeedbackSummaryMsg{slug: row.Slug, reqSeq: loadReqSeq, summary: feedbackclient.Summary{}}
	m = runtimeTUIUpdate(t, m, loadResp)

	if m.feedback.loading {
		t.Fatalf("loading spinner is permanently stuck: the load's response was discarded as stale by the save's own counter bump")
	}
	if !m.feedback.saving {
		t.Fatalf("the still-in-flight save's own state was disturbed by the load response landing")
	}
	if strings.Contains(m.View(), "Loading feedback") {
		t.Fatalf("view still shows the loading skeleton after the load's response landed:\n%s", m.View())
	}

	// 4. The save's own response then arrives too, and must still apply
	// normally — independence must hold both ways.
	saveResp := tuiFeedbackSaveMsg{slug: row.Slug, reqSeq: m.feedback.reqSeq, summary: feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 4}}}
	m = runtimeTUIUpdate(t, m, saveResp)
	if m.feedback.saving || m.feedback.summary.Mine == nil || m.feedback.summary.Mine.Overall != 4 {
		t.Fatalf("the save's own response was not applied: %+v", m.feedback)
	}
}

// TestFeedbackEditActionIgnoredWhileLoadingAvoidsBlankDraft is the regression
// test for the round 2 finding's paired fix: feedbackViewKey's "edit" action
// had no loading guard, so entering edit mode before the initial GetSummary
// resolved started the draft from a blank/zero state instead of the model's
// actual own-rating history, and was also the precondition that let the
// stranding bug above happen at all through the real UI. The tab must stay
// read-only (though still visible, showing the loading skeleton) until the
// load resolves.
func TestFeedbackEditActionIgnoredWhileLoadingAvoidsBlankDraft(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected real HTTP request: %s %s", r.Method, r.URL.Path)
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m.detailTab = detailTabFeedback // feedbackViewKey only fires on this tab

	m, _ = m.startFeedbackLoad(row, false)
	if !m.feedback.loading {
		t.Fatalf("load did not start")
	}

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.feedback.editing {
		t.Fatalf("edit action must be ignored while a load is in flight")
	}
}

// TestFeedbackOfflineSaveKeepsDraftForRetry is brief 11.3's "offline" case:
// a network error on save must never be treated as success, and the user's
// entered draft must survive so they can retry rather than retype it.
func TestFeedbackOfflineSaveKeepsDraftForRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // closed immediately: every request now fails to connect
	dir := t.TempDir()
	tokenFile, identityFile := filepath.Join(dir, "token"), filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := feedbackclient.EnsureIdentityFile(identityFile); err != nil {
		t.Fatal(err)
	}
	client, err := feedbackclient.New(feedbackclient.Config{Endpoint: server.URL, TokenFile: tokenFile, IdentityFile: identityFile, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m.feedback = tuiFeedbackState{slug: row.Slug, loaded: true, draftOverall: 5, draftReview: "great model"}

	next, cmd := m.startFeedbackSave()
	if cmd == nil {
		t.Fatalf("expected a save command to be dispatched")
	}
	m = next
	if !m.feedback.saving {
		t.Fatalf("saving flag not set while the PUT is in flight")
	}
	msg := runFeedbackCmd(t, cmd)
	saveMsg, ok := msg.(tuiFeedbackSaveMsg)
	if !ok || saveMsg.err == nil {
		t.Fatalf("expected a failed save message, got %#v", msg)
	}
	m.feedback.editing = true // the draft is only edited while editing==true
	m = runtimeTUIUpdate(t, m, saveMsg)

	if m.feedback.saving {
		t.Fatalf("saving flag still set after the response landed")
	}
	if m.feedback.saveErr == "" {
		t.Fatalf("expected a non-empty offline save error")
	}
	if !m.feedback.editing {
		t.Fatalf("a failed save must not silently close the edit form")
	}
	if m.feedback.draftOverall != 5 || m.feedback.draftReview != "great model" {
		t.Fatalf("draft was not preserved after a failed save: overall=%d review=%q", m.feedback.draftOverall, m.feedback.draftReview)
	}
}

// TestFeedbackValidationErrorBlocksSaveWithoutNetworkCall is brief 11.3's
// "validation error" case: rating nothing (Overall left at 0) must be caught
// locally, before any PUT is ever sent.
func TestFeedbackValidationErrorBlocksSaveWithoutNetworkCall(t *testing.T) {
	var requests int
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) { requests++ })
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m.feedback = tuiFeedbackState{slug: row.Slug, loaded: true, editing: true}

	next, cmd := m.startFeedbackSave()
	if cmd != nil {
		t.Fatalf("expected no command when Overall is unrated")
	}
	if next.feedback.validationErr == "" {
		t.Fatalf("expected a validation error for an unrated Overall")
	}
	if requests != 0 {
		t.Fatalf("validation failure must not reach the network, got %d requests", requests)
	}
}

// TestFeedbackCancelDiscardsDraftAndClosesForm is brief 11.3's "cancel" case:
// Esc while editing leaves editing mode and reverts the draft to the last
// known-good summary, never persisting the abandoned edit.
func TestFeedbackCancelDiscardsDraftAndClosesForm(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m.feedback = tuiFeedbackState{
		slug: row.Slug, loaded: true, editing: true,
		summary:      feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 2}},
		draftOverall: 5, draftReview: "an edit I changed my mind about",
	}

	next, cmd := m.feedbackEditKey("esc", "esc", nil)
	if cmd != nil {
		t.Fatalf("cancel should not dispatch a command")
	}
	if next.feedback.editing {
		t.Fatalf("cancel did not close the edit form")
	}
	if next.feedback.draftOverall != 2 {
		t.Fatalf("cancel did not revert the draft to the last known-good summary: draftOverall=%d", next.feedback.draftOverall)
	}
	if next.feedback.draftReview != "" {
		t.Fatalf("cancel did not clear the abandoned review draft: %q", next.feedback.draftReview)
	}
}

// TestFeedbackEditFlowThroughRealKeypresses drives the whole edit form
// through the real key() dispatch path (not by calling the edit-form
// helpers directly), to catch a keymap wiring mistake — e.g. an "edit" or
// "save" binding that fails to match a real tea.KeyMsg's canonical string —
// that a lower-level unit test calling feedbackEditKey directly would not.
func TestFeedbackEditFlowThroughRealKeypresses(t *testing.T) {
	var lastRequestBody map[string]any
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/feedback/summary") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(feedbackSummaryJSON(t, "vendor/model", 0, 0, 0)))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&lastRequestBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(feedbackSummaryJSON(t, "vendor/model", 5, 5, 1)))
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")}) // open Feedback tab
	m = runtimeTUIUpdate(t, m, runFeedbackCmd(t, m.feedbackSummaryCmd(row.Slug, m.feedback.reqSeq, false)))
	if !m.feedback.loaded {
		t.Fatalf("summary did not load")
	}

	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}) // enter edit mode
	if !m.feedback.editing || m.feedback.focus != feedbackFocusOverall {
		t.Fatalf("e did not open the edit form on Overall: editing=%v focus=%d", m.feedback.editing, m.feedback.focus)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")}) // rate Overall 5/5
	if m.feedback.draftOverall != 5 {
		t.Fatalf("digit key did not set Overall: %d", m.feedback.draftOverall)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}) // save
	if !m.feedback.saving {
		t.Fatalf("Ctrl+S did not start saving")
	}
	saveMsg := runFeedbackCmd(t, m.feedbackSaveCmd(row.Slug, m.feedback.reqSeq, feedbackclient.FeedbackRequest{Overall: 5}))
	m = runtimeTUIUpdate(t, m, saveMsg)

	if m.feedback.editing || m.feedback.saveErr != "" {
		t.Fatalf("save did not complete cleanly: editing=%v err=%q", m.feedback.editing, m.feedback.saveErr)
	}
	if lastRequestBody == nil || int(lastRequestBody["overall"].(float64)) != 5 {
		t.Fatalf("server did not receive Overall=5: %+v", lastRequestBody)
	}
	if !strings.Contains(m.View(), "My rating: 5/5") {
		t.Fatalf("view does not reflect the saved rating:\n%s", m.View())
	}
}

// TestFeedbackReviewFieldAcceptsGlobalBindingLettersAsLiteralInput is the
// regression test for the review's Critical finding: j, k, l and x are all
// pre-existing global/context key bindings (language_toggle, the "detail"
// context's own navigate_up/navigate_down defaults, and the universal
// overlay-close key) that used to intercept these runes in key()'s prologue
// before the review field's own reviewKey ever saw them — silently eating
// ordinary English words and, for x, destroying the whole in-progress edit
// by closing the detail overlay outright. This must be driven through the
// real Update/key path, not by calling reviewKey directly, since that
// bypass is exactly the layer the bug lived in — every other test in this
// file assigns draftReview as a struct literal, which is why it slipped
// through before.
func TestFeedbackReviewFieldAcceptsGlobalBindingLettersAsLiteralInput(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m.feedback = tuiFeedbackState{slug: row.Slug, loaded: true, editing: true, focus: feedbackFocusReview}

	for _, r := range "jklx" {
		beforeLang := m.lang
		m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if m.overlay != "detail" {
			t.Fatalf("typing %q closed the detail overlay (overlay=%q) instead of inserting it", r, m.overlay)
		}
		if !m.feedback.editing {
			t.Fatalf("typing %q left the edit form instead of inserting it", r)
		}
		if m.lang != beforeLang {
			t.Fatalf("typing %q changed the display language to %q instead of inserting it", r, m.lang)
		}
	}
	if m.feedback.draftReview != "jklx" {
		t.Fatalf("draftReview = %q, want %q — j/k/l/x did not all land as literal characters", m.feedback.draftReview, "jklx")
	}
}

// TestFeedbackErrorMessageSanitizesServerControlledText is the regression
// test for the review's "server-controlled error text bypasses the
// sanitizer" finding: *APIError.Error() embeds the server's own message
// string (errorResponseDTO's "error" field) verbatim, so a compromised or
// malicious feedback-server response could smuggle a C1 CSI/bidi-override
// payload through a load/save error message. feedbackErrorMessage must run
// that text through the same sanitizeFeedbackReviewText review text goes
// through — not rely on the shared Detail pipeline's older, narrower
// normalizePlainLine, which does not strip C1 controls or bidi overrides.
func TestFeedbackErrorMessageSanitizesServerControlledText(t *testing.T) {
	malicious := "clean" + feedbackTestC1CSI + "2Jinjected" + feedbackTestRLO + "reversed" + feedbackTestPDF
	for name, err := range map[string]error{
		"validation error (400, IsValidationError branch)": &feedbackclient.APIError{StatusCode: http.StatusBadRequest, Message: malicious},
		"unexpected error (500, default branch)":           &feedbackclient.APIError{StatusCode: http.StatusInternalServerError, Message: malicious},
	} {
		t.Run(name, func(t *testing.T) {
			for _, lang := range []string{"", "ru"} {
				got := feedbackErrorMessage(err, lang)
				if strings.Contains(got, feedbackTestC1CSI) || strings.Contains(got, feedbackTestRLO) || strings.Contains(got, feedbackTestPDF) {
					t.Fatalf("lang %q: sanitized error message still carries a raw C1/bidi payload: %q", lang, got)
				}
				if !strings.Contains(got, "clean") || !strings.Contains(got, "injected") || !strings.Contains(got, "reversed") {
					t.Fatalf("lang %q: sanitization dropped the safe text alongside the payload: %q", lang, got)
				}
			}
		})
	}
}

// TestFeedbackSuccessfulSaveReplacesLocalStateWithServerResponse confirms
// brief 8.2 step 5: on success the local state is replaced with exactly what
// the server returned, not merely with what was sent.
func TestFeedbackSuccessfulSaveReplacesLocalStateWithServerResponse(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(feedbackSummaryJSON(t, "vendor/model", 5, 4.9, 42)))
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m.feedback = tuiFeedbackState{slug: row.Slug, loaded: true, editing: true, draftOverall: 5}

	next, cmd := m.startFeedbackSave()
	msg := runFeedbackCmd(t, cmd)
	m = runtimeTUIUpdate(t, next, msg)

	if m.feedback.editing {
		t.Fatalf("a successful save should close the edit form")
	}
	if m.feedback.summary.Mine == nil || m.feedback.summary.Mine.Overall != 5 {
		t.Fatalf("local summary was not replaced with the server response: %+v", m.feedback.summary)
	}
	if !m.feedback.savedFlash {
		t.Fatalf("expected the saved-state flag to be set")
	}
}

// TestFeedbackTabDisabledShowsExplanationAndIgnoresEdit confirms the app
// stays fully usable, with the feature simply off, when feedback.enabled is
// false (feedbackClient is nil): the tab still renders (a short, static
// explanation) and its edit key is a no-op rather than a crash.
func TestFeedbackTabDisabledShowsExplanationAndIgnoresEdit(t *testing.T) {
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, nil)
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if m.feedback.loading || m.feedback.loaded {
		t.Fatalf("disabled feedback must never start a load: %+v", m.feedback)
	}
	view := m.View()
	if !strings.Contains(view, "Feedback is disabled") {
		t.Fatalf("disabled explanation not shown:\n%s", view)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.feedback.editing {
		t.Fatalf("edit action must be a no-op when feedback is disabled")
	}
}

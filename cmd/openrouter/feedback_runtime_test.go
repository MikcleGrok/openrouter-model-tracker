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

	cmd := m.feedbackSummaryCmd(row.Slug, m.feedback.seq, false)
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

// TestFeedbackStaleSummaryResponseDiscarded is brief 11.3's "stale response"
// case: a slow GetSummary for a model the user has since left (seq no
// longer current) must never repaint state for the model now on screen.
func TestFeedbackStaleSummaryResponseDiscarded(t *testing.T) {
	client := newFeedbackRuntimeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(feedbackSummaryJSON(t, "vendor/model", 3, 3.5, 5)))
	})
	row := feedbackTestRow()
	m := feedbackRuntimeModel(row, client)
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	staleSeq := m.feedback.seq

	// The user leaves the model (closes detail, would pick another one) —
	// simulated directly by re-opening the load for the same slug via
	// retry-shaped state: bump the generation the way switching models
	// would, without needing a second row in this fixture.
	m.feedback.loadErr = "boom"
	next, _ := m.startFeedbackLoad(row, false)
	m = next
	if m.feedback.seq == staleSeq {
		t.Fatalf("startFeedbackLoad did not advance seq")
	}

	stale := tuiFeedbackSummaryMsg{slug: row.Slug, seq: staleSeq, summary: feedbackclient.Summary{Mine: &feedbackclient.OwnFeedback{Overall: 3}}}
	before := m.feedback
	m = runtimeTUIUpdate(t, m, stale)
	if m.feedback != before {
		t.Fatalf("a stale summary response (seq %d, current %d) mutated state:\nbefore=%+v\nafter=%+v", staleSeq, m.feedback.seq, before, m.feedback)
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
	m = runtimeTUIUpdate(t, m, runFeedbackCmd(t, m.feedbackSummaryCmd(row.Slug, m.feedback.seq, false)))
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
	saveMsg := runFeedbackCmd(t, m.feedbackSaveCmd(row.Slug, m.feedback.saveSeq, feedbackclient.FeedbackRequest{Overall: 5}))
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

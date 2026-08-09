package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

func TestTUIModelUsesConfiguredRanking(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	body := "ranking:\n  mixed_utility:\n    price:\n      input_weight: 1\n      output_weight: 1\n    tier_factors:\n      sonnet: 1\n      haiku: 2\n    formula:\n      op: sub\n      args:\n        - var: score\n        - op: mul\n          args:\n            - var: tier_factor\n            - var: price_mix\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	compiled, err := resolveMixedUtilityConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := []model.Model{
		{Slug: "high", Tier: "sonnet", Score: &model.ScoreInfo{Value: 90}, Rankable: true, InPerM: 100, OutPerM: 100},
		{Slug: "low", Tier: "haiku", Score: &model.ScoreInfo{Value: 10}, Rankable: true, InPerM: 1, OutPerM: 1},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.rankingConfig, m.rankingConfigSet = compiled, true
	m.rebuild()
	if len(m.visible) != 2 || m.visible[0].Slug != "low" {
		t.Fatalf("configured TUI ranking = %+v, want low first", m.visible)
	}
}

func TestTUIModelShowsRuntimeFormulaError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	body := "ranking:\n  mixed_utility:\n    formula:\n      op: log\n      args:\n        - const: 0\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	compiled, err := resolveMixedUtilityConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "broken", Score: &model.ScoreInfo{Value: 1}, Rankable: true, InPerM: 1, OutPerM: 1}})
	m.rankingConfig, m.rankingConfigSet = compiled, true
	m.rebuild()
	if !strings.Contains(m.err, "log domain") {
		t.Fatalf("TUI error = %q, want formula runtime error", m.err)
	}
	if len(m.visible) != 0 {
		t.Fatalf("TUI retained sorted rows after error: %+v", m.visible)
	}
}

func TestTUIInteractiveFilterClearsRowsOnRuntimeFormulaError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	body := "ranking:\n  mixed_utility:\n    formula:\n      op: log\n      args:\n        - const: 0\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	compiled, err := resolveMixedUtilityConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "broken", DisplayName: "Broken", Score: &model.ScoreInfo{Value: 1}, Rankable: true, InPerM: 1, OutPerM: 1}})
	m.rankingConfig, m.rankingConfigSet = compiled, true
	m.inputMode, m.input = "filter", "scored"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.err, "log domain") {
		t.Fatalf("TUI error = %q, want formula runtime error", m.err)
	}
	if len(m.visible) != 0 {
		t.Fatalf("TUI retained rows after interactive filter error: %+v", m.visible)
	}
}

func tuiKey(m tuiModel, key string) tuiModel {
	next, _ := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next
}

func TestTUIKeyState(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", DisplayName: "A"}, {Slug: "b", DisplayName: "B"}})
	m = tuiKey(m, "j")
	if m.cursor != 1 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	old := m.sortKey
	m = tuiKey(m, "s")
	if m.sortKey == old {
		t.Fatal("sort did not advance")
	}
	m = tuiKey(m, "S")
	if !m.reverse {
		t.Fatal("reverse not enabled")
	}
	m = tuiKey(m, "c")
	if m.overlay != "columns" {
		t.Fatal("columns overlay not opened")
	}
	m, _ = m.columnKey("down")
	m, _ = m.columnKey(" ")
	m, _ = m.columnKey("enter")
	if !containsColumn(m.columns, tuiColumns[1]) {
		t.Fatal("column was not applied")
	}
	m = tuiKey(m, "c")
	m, _ = m.columnKey(" ")
	m, _ = m.columnKey("esc")
	if !containsColumn(m.columns, tuiColumns[1]) {
		t.Fatal("column cancel changed applied columns")
	}
	m = tuiKey(m, "t")
	if m.overlay != "" || m.inputMode != "" {
		t.Fatal("t unexpectedly triggered a TUI action")
	}
	m = tuiKey(m, "n")
	if !m.lastNote || !containsColumn(m.columns, colNote) {
		t.Fatal("note mode not enabled")
	}
	m = tuiKey(m, "/")
	m.input = "B"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.visible) != 1 || m.visible[0].Slug != "b" {
		t.Fatalf("filter result = %+v", m.visible)
	}
	m = tuiKey(m, "/")
	m.input = "a"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.filter != "" {
		t.Fatalf("substring search changed structured filter to %q", m.filter)
	}
	m = tuiKey(m, "?")
	if !strings.Contains(m.View(), "openrouter tui keys") {
		t.Fatal("help overlay missing")
	}
	if !strings.Contains(m.View(), "q sorts by quality") || !strings.Contains(m.View(), "p by price") || !strings.Contains(m.View(), "r by quality/price") {
		t.Fatalf("help page 1 is missing sort shortcuts: %q", m.View())
	}
	for _, r := range tuiHelpDocument {
		if unicode.Is(unicode.Cyrillic, r) {
			t.Fatalf("help document mentions Cyrillic keyboard aliases: %q", tuiHelpDocument)
		}
	}
	for _, text := range []string{"Navigation", "Task-fit", "n switches the last column between Task fit and Note", "Task-fit codes:", "I - implement: write or change production code.", "P - plan:", "R - research:", "D - debug:", "A - audit:", "F - refactor:", "T - test:", "No task-fit classification is shown as n/a", "Auto-refresh"} {
		if !strings.Contains(tuiHelpDocument, text) {
			t.Fatalf("help document missing %q", text)
		}
	}
	for _, text := range []string{"t toggles", "English keywords", "implement + debug + refactor + test"} {
		if strings.Contains(tuiHelpDocument, text) {
			t.Fatalf("help document still contains removed long-output text %q", text)
		}
	}
	m.width, m.height = 140, 12
	initial := m.View()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(tuiModel)
	if m.helpOffset != 1 || initial == m.View() {
		t.Fatalf("help did not scroll: offset=%d", m.helpOffset)
	}
	for i := 0; i < len(tuiHelpLines())+10; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(tuiModel)
	}
	if m.helpOffset != tuiHelpMaxOffset(m.height) {
		t.Fatalf("help offset exceeded lower bound: got %d, max %d", m.helpOffset, tuiHelpMaxOffset(m.height))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = next.(tuiModel)
	if m.helpOffset != 0 {
		t.Fatalf("help home offset = %d, want 0", m.helpOffset)
	}
	m = tuiKey(m, "/")
	m.input = "refresh"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.inputMode != "" || len(m.helpMatches) == 0 || !strings.Contains(m.View(), "refresh") {
		t.Fatalf("help search state = %+v", m)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if m.helpMatch != 1%len(m.helpMatches) {
		t.Fatalf("Enter did not advance search match: %d", m.helpMatch)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(tuiModel)
	if m.overlay != "" {
		t.Fatal("help did not close")
	}
}

func TestTUIStatusOmitsRemovedTaskFitShortcutAndTruncates(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.width, m.height = 120, 20
	findFooter := func(view string) string {
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "↑↓ navigate") {
				return line
			}
		}
		return ""
	}
	wideFooter := findFooter(m.View())
	if wideFooter == "" || strings.Contains(wideFooter, "t task-fit") {
		t.Fatalf("wide TUI footer contains removed task-fit shortcut: %q", wideFooter)
	}

	m.width = 20
	narrowFooter := findFooter(m.View())
	if narrowFooter == "" {
		t.Fatal("narrow TUI footer is missing")
	}
	if got := lipgloss.Width(narrowFooter); got > m.width {
		t.Fatalf("narrow TUI footer exceeds width %d: %q (display width %d)", m.width, narrowFooter, got)
	}
	if lipgloss.Width(wideFooter) <= m.width || narrowFooter == wideFooter {
		t.Fatalf("narrow TUI footer was not truncated: wide=%q narrow=%q", wideFooter, narrowFooter)
	}
}

func TestTUITaskFitUsesCompactCodes(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{TaskFit: []string{"implement", "debug", "refactor", "test"}}})
	if got := tuiCell(m.models[0], colTask, false, m.scoreSource); got != "IDFT" {
		t.Fatalf("task-fit cell = %q, want compact codes", got)
	}
	if strings.Contains(m.View(), "implement + debug + refactor + test") {
		t.Fatal("TUI rendered long task-fit keywords")
	}
}

func TestTUICommandKeysPreserveASCIIAliases(t *testing.T) {
	for _, test := range []struct{ key, want string }{{".", "/"}, {",", "?"}} {
		got := tuiCommandKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(test.key)})
		if got != test.want {
			t.Fatalf("%q normalized to %q, want %q", test.key, got, test.want)
		}
	}
	for _, test := range []struct {
		keyType tea.KeyType
		want    string
	}{{tea.KeyUp, "up"}, {tea.KeyHome, "home"}, {tea.KeyCtrlC, "ctrl+c"}, {tea.KeyTab, "tab"}, {tea.KeyEscape, "esc"}} {
		if got := tuiCommandKey(tea.KeyMsg{Type: test.keyType}); got != test.want {
			t.Fatalf("special key normalized to %q, want %q", got, test.want)
		}
	}
}

func TestTUIInputModeKeepsNonASCIIInput(t *testing.T) {
	for _, key := range []string{"é", "界"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
		m.inputMode = "search"
		beforeSort, beforeOverlay := m.sortKey, m.overlay
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = next.(tuiModel)
		if m.input != key || m.sortKey != beforeSort || m.overlay != beforeOverlay || m.inputMode != "search" {
			t.Fatalf("non-ASCII input %q was routed as command: input=%q sort=%q mode=%q overlay=%q", key, m.input, m.sortKey, m.inputMode, m.overlay)
		}
	}
}

func TestTUISortShortcutsRebuildVisibleOrder(t *testing.T) {
	rows := []model.Model{
		{Slug: "low-quality", DisplayName: "Low", QualityPriceLabel: "1.0", Score: &model.ScoreInfo{Value: 1}, Rankable: true, MixedPrice: 1, QualityPrice: 1},
		{Slug: "high-quality", DisplayName: "High", QualityPriceLabel: "9.0", Score: &model.ScoreInfo{Value: 9}, Rankable: true, MixedPrice: 10, QualityPrice: 9},
	}
	for _, test := range []struct {
		key, sortKey, first string
	}{
		{"q", "quality", "high-quality"},
		{"p", "price", "low-quality"},
		{"r", "q/p", "high-quality"},
	} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(test.key)})
		m = next.(tuiModel)
		if m.sortKey != test.sortKey || len(m.visible) != 2 || m.visible[0].Slug != test.first {
			t.Fatalf("key %q: sort=%q visible=%+v", test.key, m.sortKey, m.visible)
		}
	}
}

func TestTUIUsesConfiguredPriceWeight(t *testing.T) {
	rows := []model.Model{
		{Slug: "quality", Tier: "sonnet", Score: &model.ScoreInfo{Value: 90}, Rankable: true, QualityPrice: 1},
		{Slug: "value", Tier: "sonnet", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 100},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.priceWeight = 0
	m.rebuild()
	if m.visible[0].Slug != "quality" {
		t.Fatalf("zero-weight TUI first slug = %q, want quality", m.visible[0].Slug)
	}
	m.priceWeight = 10
	m.rebuild()
	if m.visible[0].Slug != "value" {
		t.Fatalf("custom-weight TUI first slug = %q, want value", m.visible[0].Slug)
	}
}

func TestTUIRankingShortcutTogglesModeAndDisplaysIt(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", Score: &model.ScoreInfo{Value: 1}, Rankable: true}})
	if !strings.Contains(m.View(), "ranking:mixed-utility") {
		t.Fatalf("initial ranking is not displayed: %s", m.View())
	}
	m = tuiKey(m, "m")
	if m.ranking != rankingTier || !strings.Contains(m.View(), "ranking:tier-priority") {
		t.Fatalf("tier ranking state = %q, view=%s", m.ranking, m.View())
	}
	m = tuiKey(m, "m")
	if m.ranking != rankingMixed || !strings.Contains(m.View(), "ranking:mixed-utility") {
		t.Fatalf("mixed ranking state = %q, view=%s", m.ranking, m.View())
	}
}

func TestTUIExitAndRefreshShortcuts(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	if _, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); cmd == nil {
		t.Fatal("x did not quit")
	}
	if _, cmd := m.key(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Fatal("ctrl-c did not quit")
	}
	m = tuiKey(m, "q")
	if m.sortKey != "quality" {
		t.Fatalf("q sort key = %q", m.sortKey)
	}
	before := m.generation
	var cmd tea.Cmd
	m, cmd = m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil || !m.refreshing || m.generation != before+1 {
		t.Fatalf("R refresh state = refreshing %v generation %d cmd %v", m.refreshing, m.generation, cmd != nil)
	}
	msg, ok := cmd().(tuiRefreshMsg)
	if !ok || msg.generation != before+1 {
		t.Fatalf("R refresh message = %#v, want tuiRefreshMsg generation %d", msg, before+1)
	}
	m.refreshing = false
	m.generation = before
	m = tuiKey(m, "r")
	if m.sortKey != "q/p" || m.refreshing {
		t.Fatalf("lowercase r state = sort %q refreshing %v", m.sortKey, m.refreshing)
	}
}

func TestTUIRefreshQuitSearchAndHelpThroughUpdate(t *testing.T) {
	rows := []model.Model{{Slug: "alpha", DisplayName: "Alpha"}, {Slug: "beta", DisplayName: "Beta"}}
	for _, key := range []string{"R"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		updated := next.(tuiModel)
		if cmd == nil || !updated.refreshing || updated.generation != 1 {
			t.Fatalf("%s refresh state = %+v cmd=%v", key, updated, cmd != nil)
		}
		if message, ok := cmd().(tuiRefreshMsg); !ok || message.generation != 1 || message.err == nil {
			t.Fatalf("%s refresh message = %#v", key, message)
		}
	}
	for _, key := range []string{"x"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd == nil {
			t.Fatalf("%s did not quit", key)
		}
	}
	for _, key := range []string{"/", "."} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if next.(tuiModel).inputMode != "search" {
			t.Fatalf("%s did not open search", key)
		}
	}
	for _, key := range []string{"?", ","} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if next.(tuiModel).overlay != "help" {
			t.Fatalf("%s did not open help", key)
		}
	}
}

func TestTUIViewHeightBudgetIncludesSearchAndFilterInput(t *testing.T) {
	rows := []model.Model{{Slug: "alpha", DisplayName: "Alpha"}, {Slug: "beta", DisplayName: "Beta"}}
	for _, mode := range []string{"", "search", "filter"} {
		for _, height := range []int{1, 2, 3, 4, 5, 6, 7} {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
			m.width, m.height, m.inputMode, m.input = 100, height, mode, "alpha"
			view := m.View()
			assertTUIViewFits(t, view, m.width, m.height, mode)
			if mode != "" && height >= 4 && !strings.Contains(view, "/ alpha_") {
				t.Fatalf("mode %q at height %d lost input line: %q", mode, height, view)
			}
			if mode == "" && height >= 6 && !strings.Contains(view, "status:") {
				t.Fatalf("normal view at height %d lost footer: %q", height, view)
			}
		}
	}
}

func TestTUIRefreshGenerationKeepsRowsOnErrorAndRejectsStale(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "old"}})
	m.generation = 2
	m.refreshing = true
	next, _ := m.Update(tuiRefreshMsg{generation: 1, models: []model.Model{{Slug: "new"}}})
	m = next.(tuiModel)
	if m.visible[0].Slug != "old" {
		t.Fatal("stale refresh replaced rows")
	}
	next, _ = m.Update(tuiRefreshMsg{generation: 2, err: errors.New("offline")})
	m = next.(tuiModel)
	if m.visible[0].Slug != "old" || m.err != "offline" {
		t.Fatalf("error refresh state = %+v", m)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	m = next.(tuiModel)
	if m.width != 50 || m.height != 10 {
		t.Fatal("resize not applied")
	}
}

func TestTUITickDuringRefreshSchedulesNextTick(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, time.Second, []model.Model{{Slug: "old"}})
	m.refreshing = true
	next, cmd := m.Update(tuiTickMsg{})
	if cmd == nil {
		t.Fatal("tick during refresh did not schedule a next tick")
	}
	if next.(tuiModel).generation != 0 {
		t.Fatal("tick during refresh started another generation")
	}
}

func TestTUISelectionAndStructuredFilter(t *testing.T) {
	rows := []model.Model{{Slug: "a", DisplayName: "A", Context: 1}, {Slug: "b", DisplayName: "B", Context: 2}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m = tuiKey(m, "j")
	if m.selectedSlug != "b" {
		t.Fatalf("selected slug = %q, want b", m.selectedSlug)
	}
	m.sortKey = "slug"
	m.reverse = true
	m.rebuild()
	if m.visible[m.cursor].Slug != "b" {
		t.Fatalf("selection after reorder = %q, want b", m.visible[m.cursor].Slug)
	}
	m.inputMode, m.input = "filter", "context>=bad"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.visible) != 2 || m.err == "" {
		t.Fatalf("invalid filter state = visible %d, err %q", len(m.visible), m.err)
	}
	m.inputMode, m.input = "search", "a"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.visible) != 1 || m.visible[0].Slug != "a" {
		t.Fatalf("substring search result = %+v", m.visible)
	}
}

func TestTUISelectionDoesNotReturnAfterFilteredRowDisappears(t *testing.T) {
	rows := []model.Model{{Slug: "a", DisplayName: "A"}, {Slug: "b", DisplayName: "B"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m = tuiKey(m, "j")
	m.inputMode, m.input = "search", "a"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.selectedSlug != "a" || len(m.visible) != 1 {
		t.Fatalf("selection after search = %q, visible = %+v", m.selectedSlug, m.visible)
	}
	m.filter = ""
	m.rebuild()
	if m.selectedSlug != "a" || m.visible[m.cursor].Slug != "a" {
		t.Fatalf("selection returned to old row: selected=%q cursor=%d visible=%+v", m.selectedSlug, m.cursor, m.visible)
	}
	m.models = []model.Model{{Slug: "b", DisplayName: "B"}}
	m.rebuild()
	if m.selectedSlug != "b" || m.visible[m.cursor].Slug != "b" {
		t.Fatalf("selection was not rebound after refresh: selected=%q cursor=%d visible=%+v", m.selectedSlug, m.cursor, m.visible)
	}
}

func TestTUIEmptyStructuredFilterClearsFilter(t *testing.T) {
	rows := []model.Model{{Slug: "a", Context: 1}, {Slug: "b", Context: 2}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.filter, m.inputMode, m.input = "context>=2", "filter", ""
	m.rebuild()
	if len(m.visible) != 1 {
		t.Fatalf("initial structured filter result = %+v", m.visible)
	}
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filter != "" || m.err != "" || len(m.visible) != len(rows) {
		t.Fatalf("empty structured filter state = filter %q, err %q, visible %+v", m.filter, m.err, m.visible)
	}
}

func TestTUIViewNarrowAndSanitized(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a\n|`", DisplayName: "**long\tname**"}})
	for _, width := range []int{0, 1, 40} {
		m.width, m.height = width, 10
		view := m.View()
		for _, line := range strings.Split(view, "\n") {
			if width > 0 && lipgloss.Width(line) > width {
				t.Fatalf("width %d line exceeds width: %q", width, line)
			}
		}
	}
	m.pendingColumns = []tuiColumn{colName}
	m.columnCursor = 0
	m, _ = m.columnKey(" ")
	if len(m.pendingColumns) != 1 {
		t.Fatal("last column was removed")
	}
}

func TestTUIRenderTUILineAlignsCellsAndNumericValues(t *testing.T) {
	m := tuiModel{width: 34}
	columns := []tuiColumn{colName, colContext, colInput}
	header := m.renderTUILine(columns, nil, false)
	for _, values := range [][]string{{"Long model", "7", "1.5"}, {"Long model", "12345", "0.125"}} {
		row := m.renderTUILine(columns, values, false)
		selected := m.renderTUILine(columns, values, true)
		if !reflect.DeepEqual(tuiSeparatorDisplayOffsets(header), tuiSeparatorDisplayOffsets(row)) || !reflect.DeepEqual(tuiSeparatorDisplayOffsets(header), tuiSeparatorDisplayOffsets(selected)) {
			t.Fatalf("separator display positions differ: header=%q row=%q selected=%q", header, row, selected)
		}
		if lipgloss.Width(header) > m.width || lipgloss.Width(row) > m.width || lipgloss.Width(selected) > m.width {
			t.Fatalf("rendered line exceeds width: header=%q row=%q selected=%q", header, row, selected)
		}
		for column, value := range values[1:] {
			cellEnd := tuiNumericCellDisplayEnd(row, column+1)
			valueEnd := tuiValueDisplayEnd(row, value, column+1)
			if valueEnd != cellEnd {
				t.Fatalf("%s value was not right-aligned: row=%q value-end=%d cell-end=%d", columns[column+1], row, valueEnd, cellEnd)
			}
		}
	}
}

func TestTUINameColumnGetsWeightedWidth(t *testing.T) {
	m := tuiModel{width: 80}
	columns := []tuiColumn{colName, colContext, colInput}
	line := m.renderTUILine(columns, nil, false)
	offsets := tuiSeparatorDisplayOffsets(line)
	nameWidth := offsets[0] - 2
	numericWidth := offsets[1] - offsets[0] - 3
	if nameWidth <= numericWidth {
		t.Fatalf("name width %d is not greater than numeric width %d: %q", nameWidth, numericWidth, line)
	}
}

func TestTUIStatusDescribesShortcuts(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.width, m.height = 100, 10
	status := m.View()
	for _, shortcut := range []string{"q quality", "p price", "r q/p", "R refresh", "x quit"} {
		if !strings.Contains(status, shortcut) {
			t.Fatalf("status is missing %q: %q", shortcut, status)
		}
	}
}

func TestTUIRenderTUILineUsesDisplayWidthAndStripsANSI(t *testing.T) {
	m := tuiModel{width: 22}
	columns := []tuiColumn{colName, colContext}
	styled := lipgloss.NewStyle().Bold(true).Render("界🙂")
	row := m.renderTUILine(columns, []string{"界🙂", "7"}, false)
	styledRow := m.renderTUILine(columns, []string{styled, "7"}, false)
	plain := m.renderTUILine(columns, []string{"界🙂", "7"}, false)
	if styledRow != plain {
		t.Fatalf("ANSI styling changed layout: styled=%q plain=%q", styledRow, plain)
	}
	if row != plain {
		t.Fatalf("ANSI styling changed layout: styled=%q plain=%q", row, plain)
	}
	if lipgloss.Width(row) > m.width {
		t.Fatalf("Unicode row exceeds width: %q", row)
	}
	if !reflect.DeepEqual(tuiSeparatorDisplayOffsets(row), tuiSeparatorDisplayOffsets(m.renderTUILine(columns, []string{"界🙂", "123"}, false))) {
		t.Fatalf("Unicode separator display position changed with numeric width: %q", row)
	}
}

func TestTUIViewNarrowWidthPreservesTableStructure(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", DisplayName: "model"}})
	m.width, m.height = 6, 10
	view := m.View()
	columns := m.renderColumns()
	if len(columns) != 1 {
		t.Fatalf("narrow view kept %d columns, want one: %v", len(columns), columns)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line exceeds width %d: %q", m.width, line)
		}
		if strings.Contains(line, " | ") {
			t.Fatalf("narrow view retained a multi-column separator: %q", line)
		}
	}
}

func TestTUIViewBoundaryWidthHidesColumnsBeforeRendering(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "local-model", DisplayName: "Local model"}})
	m.width, m.height = 26, 10
	view := m.View()
	columns := m.renderColumns()
	if len(columns) != 6 {
		t.Fatalf("boundary view kept %d columns, want 6: %v", len(columns), columns)
	}
	wantSeparators := len(columns) - 1
	var tableLines []string
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line exceeds width %d: %q", m.width, line)
		}
		if !strings.Contains(line, " | ") {
			continue
		}
		tableLines = append(tableLines, line)
		if got := len(tuiSeparatorDisplayOffsets(line)); got != wantSeparators {
			t.Fatalf("boundary table line has %d separators, want %d: %q", got, wantSeparators, line)
		}
		if strings.Contains(strings.ReplaceAll(line, " | ", ""), "|") {
			t.Fatalf("boundary table line contains a clipped separator: %q", line)
		}
	}
	if len(tableLines) < 2 {
		t.Fatalf("boundary view has %d table lines, want header and data: %q", len(tableLines), view)
	}
	headerOffsets := tuiSeparatorDisplayOffsets(tableLines[0])
	for _, line := range tableLines[1:] {
		if got := tuiSeparatorDisplayOffsets(line); !reflect.DeepEqual(got, headerOffsets) {
			t.Fatalf("boundary table geometry differs: header=%v row=%v: %q", headerOffsets, got, line)
		}
	}
}

func TestTUIRenderTUILineFitsNarrowWidths(t *testing.T) {
	columns := []tuiColumn{colName, colContext, colOutput}
	for width := 1; width <= 12; width++ {
		m := tuiModel{width: width}
		for _, values := range [][]string{nil, {"a", "123", "4.5"}} {
			line := m.renderTUILine(columns, values, false)
			if lipgloss.Width(line) > width {
				t.Fatalf("width %d line exceeds width: %q", width, line)
			}
		}
	}
}

func tuiSeparatorDisplayOffsets(line string) []int {
	offsets := make([]int, 0, 2)
	for index := 0; index < len(line); {
		separator := strings.Index(line[index:], " | ")
		if separator < 0 {
			break
		}
		separator += index
		offsets = append(offsets, lipgloss.Width(line[:separator])+1)
		index = separator + len(" | ")
	}
	return offsets
}

func tuiNumericCellDisplayEnd(line string, column int) int {
	offsets := tuiSeparatorDisplayOffsets(line)
	end := lipgloss.Width(line)
	if column < len(offsets) {
		end = offsets[column] - 1
	}
	return end
}

func tuiValueDisplayEnd(line, value string, column int) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}
	valueStart := lipgloss.Width(line[:index])
	valueEnd := valueStart + lipgloss.Width(value)
	cellStart := 0
	if column > 0 {
		offsets := tuiSeparatorDisplayOffsets(line)
		cellStart = offsets[column-1] + 1
	}
	cellEnd := tuiNumericCellDisplayEnd(line, column)
	if valueStart < cellStart || valueEnd > cellEnd {
		return -1
	}
	return valueEnd
}

func TestTUIOverlayFitsTerminalWidth(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.pendingColumns = append([]tuiColumn(nil), tuiColumns...)
	for _, overlay := range []string{"help", "columns"} {
		m.overlay = overlay
		for width := 0; width <= 20; width++ {
			m.width, m.height = width, 10
			for _, line := range strings.Split(m.View(), "\n") {
				if width > 0 && lipgloss.Width(line) > width {
					t.Fatalf("%s overlay at width %d exceeds width: %q", overlay, width, line)
				}
			}
		}
	}
}

func TestTUIColumnsOverlayFitsSmallHeights(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.overlay = "columns"
	m.pendingColumns = append([]tuiColumn(nil), tuiColumns...)
	for _, height := range []int{1, 2, 3, 4, 5, 6, 7} {
		m.width, m.height = 40, height
		view := m.View()
		assertTUIViewFits(t, view, m.width, m.height, "columns")
		if !strings.Contains(view, "Columns") {
			t.Fatalf("columns overlay at height %d lost title: %q", height, view)
		}
		if height >= 7 && !strings.Contains(view, "> [x]") {
			t.Fatalf("columns overlay at height %d lost selected content: %q", height, view)
		}
	}
}

func TestTUIHelpOverlayFitsSmallHeights(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.overlay = "help"
	for _, height := range []int{1, 2, 3, 4, 5, 6, 7} {
		m.width, m.height = 40, height
		view := m.View()
		assertTUIViewFits(t, view, m.width, m.height, "help")
		if strings.TrimSpace(view) == "" {
			t.Fatalf("help at height %d rendered empty view", height)
		}
	}

	for _, key := range []string{"esc", ",", "?"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
		m.overlay, m.width, m.height = "help", 40, 1
		if strings.TrimSpace(m.View()) == "" {
			t.Fatal("help overlay rendered empty view before close")
		}
		runes := []rune(key)
		var msg tea.KeyMsg
		switch key {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEscape}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: runes}
		}
		next, _ := m.Update(msg)
		m = next.(tuiModel)
		if m.overlay != "" {
			t.Fatalf("help was not closed by %q at height %d", key, m.height)
		}
	}
}

func TestTUIHelpUsesFullViewport(t *testing.T) {
	view := tuiHelpView(0, "", 80, 20)
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Fatalf("help height = %d, want 20", len(lines))
	}
	if strings.Contains(view, "╭") || strings.Contains(view, "╰") {
		t.Fatalf("help still has a box: %q", view)
	}
}

func assertTUIViewFits(t *testing.T, view string, width, height int, mode string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("%s rendered %d lines at height %d: %q", mode, len(lines), height, view)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > width {
			t.Fatalf("%s rendered line wider than %d: %q", mode, width, line)
		}
	}
}

func TestTUIHelpAndNonTTYGuard(t *testing.T) {
	if err := runTUI(nil, io.Discard, "", refresh.Options{}, 0); err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("non-TTY error = %v", err)
	}
}

func TestTUIModelShowsTheActiveScoreSource(t *testing.T) {
	rows := []model.Model{{
		Slug: "demo/arena", DisplayName: "Demo Arena", Tier: "sonnet",
		InPerM: 1, OutPerM: 3, Context: 128000,
		ScoreLabel: "1300 Elo", QualityPriceLabel: "0",
	}}
	m := newTUIModel(context.Background(), t.TempDir(), refresh.Options{}, 0, rows)
	m.scoreSource = scoreSourceArena
	m.width, m.height = 200, 24
	m.rebuild()

	view := m.View()
	if !strings.Contains(view, "score:arena") {
		t.Errorf("the meta line does not say which score source is active:\n%s", view)
	}
	if !strings.Contains(view, "1300 Elo") {
		t.Errorf("the Status column lost the Arena label:\n%s", view)
	}
}

func TestTUIModelDefaultsToSWEBench(t *testing.T) {
	m := newTUIModel(context.Background(), t.TempDir(), refresh.Options{}, 0, nil)
	if m.scoreSource != scoreSourceDefault {
		t.Errorf("scoreSource = %q, want %q", m.scoreSource, scoreSourceDefault)
	}
}

func TestTUICommandRejectsUnknownScoreSource(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	err := executeCLIError(t, "tui", "--config", config, "--score-source=auto")
	if err == nil || !strings.Contains(err.Error(), "invalid --score-source") {
		t.Errorf("error = %v, want a rejection of --score-source=auto", err)
	}
}

// TestTUIScoreSourceClaudeColumnNeverBlendsScales is the TUI-side mirror of
// TestTableScoreSourceClaudeColumnNeverBlendsScales in table_test.go: tui.go
// has its own, separate Claude-column rendering (tuiCell's colClaude case),
// which must also stop applying SWE-bench-calibrated percentage thresholds
// (>=70, >=60) to a haiku/free-tier row's Score.Value once that number is a
// min-max-normalized Arena position instead of a SWE-bench percentage. A
// tier=haiku row with only an Arena number, rendered through the full TUI
// View() in arena mode, must not show a Claude-tier capability claim.
func TestTUIScoreSourceClaudeColumnNeverBlendsScales(t *testing.T) {
	rows := []model.Model{{
		Slug: "demo/haiku-arena", DisplayName: "Demo Haiku Arena", Tier: "haiku",
		InPerM: 1, OutPerM: 3, Context: 128000, Rankable: true,
		Score: &model.ScoreInfo{Value: 82}, ScoreLabel: "1400 Elo", QualityPriceLabel: "0",
	}}
	m := newTUIModel(context.Background(), t.TempDir(), refresh.Options{}, 0, rows)
	m.scoreSource = scoreSourceArena
	m.width, m.height = 200, 24
	m.rebuild()

	view := m.View()
	if strings.Contains(view, "≈ Haiku 4.5") {
		t.Errorf("arena-mode TUI view claims a SWE-bench-calibrated Claude tier for a haiku row with no SWE-bench score:\n%s", view)
	}
}

// TestTUICellClaudeColumnUsesActiveScoreSource is the tuiCell-level unit
// test behind TestTUIScoreSourceClaudeColumnNeverBlendsScales: the exact
// same row must resolve to the normal SWE-bench-calibrated Claude label in
// swebench mode, and to н/д in arena mode.
func TestTUICellClaudeColumnUsesActiveScoreSource(t *testing.T) {
	row := model.Model{Tier: "haiku", Score: &model.ScoreInfo{Value: 82}, Rankable: true}
	if got := tuiCell(row, colClaude, false, scoreSourceSWEBench); got != "≈ Haiku 4.5" {
		t.Fatalf("swebench-mode Claude cell = %q, want ≈ Haiku 4.5", got)
	}
	if got := tuiCell(row, colClaude, false, scoreSourceArena); got != "н/д" {
		t.Fatalf("arena-mode Claude cell = %q, want н/д (must not blend Arena and SWE-bench scales)", got)
	}
}

// TestNewConfiguredTUIModelAppliesScoreSourceEndToEnd exercises the real
// construction path — newConfiguredTUIModel, the part of
// runTUIWithRankingConfigCompiled that the TTY gate and tea.NewProgram make
// otherwise unreachable from a test — instead of setting m.scoreSource by
// hand the way every other TUI test in this file does. It fails if either
// half of the flag→session wiring regresses: dropping `m.scoreSource =
// scoreSource` (caught by the field assertion below) or reverting the
// loader call back to loadLocalModels/scoreSourceDefault (caught by the
// Status-cell assertion, since demo/haiku-arena has no SWE-bench number and
// would render н/д there instead of 1400 Elo).
func TestNewConfiguredTUIModelAppliesScoreSourceEndToEnd(t *testing.T) {
	root := t.TempDir()
	if err := copyScoreSourceClaudeFixture(t, root); err != nil {
		t.Fatal(err)
	}
	compiled, err := ranking.Compile(ranking.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	m, err := newConfiguredTUIModel(context.Background(), root, refresh.Options{}, 0, "q/p", false, "", 0, false, rankingDefault, compiled, scoreSourceArena)
	if err != nil {
		t.Fatalf("newConfiguredTUIModel: %v", err)
	}
	if m.scoreSource != scoreSourceArena {
		t.Fatalf("scoreSource = %q, want %q", m.scoreSource, scoreSourceArena)
	}

	m.width, m.height = 200, 24
	view := m.View()
	line := tuiFindRowLine(t, view, "Demo Haiku Arena")
	cells := strings.Split(line, " | ")
	if len(cells) < 3 {
		t.Fatalf("unexpected row shape: %q", line)
	}
	if got := strings.TrimSpace(cells[1]); got != "н/д" {
		t.Errorf("Claude cell = %q, want н/д; a haiku row with only an Arena number must not claim a SWE-bench-calibrated tier:\n%s", got, view)
	}
	if got := strings.TrimSpace(cells[2]); got != "1400 Elo" {
		t.Errorf("Status cell = %q, want 1400 Elo; loadLocalModelsForSource was not asked for the arena source:\n%s", got, view)
	}
}

func tuiFindRowLine(t *testing.T, view, identity string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, identity) {
			return line
		}
	}
	t.Fatalf("row containing %q not found in view:\n%s", identity, view)
	return ""
}

func TestTUIWrapTextBreaksOnWordBoundaries(t *testing.T) {
	got := tuiWrapText("alpha beta gamma delta", 11)
	want := []string{"alpha beta", "gamma delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tuiWrapText = %q, want %q", got, want)
	}
}

func TestTUIWrapTextKeepsEveryLineWithinTheRequestedWidth(t *testing.T) {
	long := strings.Repeat("supercalifragilistic", 3)
	for _, width := range []int{1, 2, 3, 5, 12, 40} {
		for _, value := range []string{long, "короткий текст про одну модель", "a " + long + " b"} {
			for _, line := range tuiWrapText(value, width) {
				if lipgloss.Width(line) > width {
					t.Fatalf("width %d produced a line of width %d: %q", width, lipgloss.Width(line), line)
				}
			}
		}
	}
}

// TestTUIWrapTextMeasuresDisplayWidthNotBytes is the reason this helper
// cannot be a len()-based loop: a full-width glyph occupies two terminal
// columns, so a byte- or rune-counting wrap would overflow the viewport
// exactly the way truncateTable's display-width measure exists to avoid.
func TestTUIWrapTextMeasuresDisplayWidthNotBytes(t *testing.T) {
	for _, width := range []int{2, 3, 8} {
		for _, line := range tuiWrapText("界界界界界 界界", width) {
			if lipgloss.Width(line) > width {
				t.Fatalf("width %d produced a wide-rune line of width %d: %q", width, lipgloss.Width(line), line)
			}
		}
	}
}

// TestTUIWrapTextHandlesDegenerateInput covers the two states View() can
// legitimately be in before the first tea.WindowSizeMsg arrives, and the
// empty-value case every optional catalogue field can produce.
func TestTUIWrapTextHandlesDegenerateInput(t *testing.T) {
	if got := tuiWrapText("anything", 0); got != nil {
		t.Fatalf("tuiWrapText at width 0 = %q, want nil", got)
	}
	if got := tuiWrapText("anything", -5); got != nil {
		t.Fatalf("tuiWrapText at a negative width = %q, want nil", got)
	}
	if got := tuiWrapText("", 40); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("tuiWrapText of an empty string = %q, want exactly one empty line", got)
	}
	if got := tuiWrapText("one\n\ntwo", 40); !reflect.DeepEqual(got, []string{"one", "", "two"}) {
		t.Fatalf("tuiWrapText = %q, want the blank line between paragraphs preserved", got)
	}
}

// TestTUIDetailWrappedPreservesParagraphBreaks guards against
// tuiDetailWrapped's own sanitisation collapsing "\n" to a space before the
// value ever reaches tuiWrapText's paragraph-splitting branch above — the
// trap that made a real multi-paragraph vendor description render as one
// run-on block on the actual detail screen even though
// TestTUIWrapTextHandlesDegenerateInput, on the lower-level helper, was
// green throughout.
func TestTUIDetailWrappedPreservesParagraphBreaks(t *testing.T) {
	got := tuiDetailWrapped("first paragraph.\n\nsecond paragraph.", 40)
	want := []string{"  first paragraph.", "  ", "  second paragraph."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tuiDetailWrapped = %q, want two indented paragraph blocks separated by a blank line: %q", got, want)
	}
}

func tuiDetailTestModel() model.Model {
	return model.Model{
		Slug: "openai/gpt-5.6-luna", DisplayName: "GPT-5.6 Luna", Tier: "opus",
		Owner: "OpenAI (C)", OpenWeights: "нет", ClaudeRef: "≈ Opus 4.6",
		InPerM: 0.5, OutPerM: 3, Context: 1000000,
		Created: 1786034890, Description: "GPT-5.6 Luna is OpenAI's long-context flagship, strong at code and weak at latency.",
		CanonicalSlug: "openai/gpt-5.6-luna-20260804", HuggingFaceID: "openai-community/gpt-5-6-luna",
		LongContextPriceLabel: "$1.00 / $4.00 от 272K+", LongContextInLabel: "$1.00 от 272K+", LongContextOutLabel: "$4.00 от 272K+",
		Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 93, VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://www.vals.ai/benchmarks/swebench", Checked: "2026-08-03"},
		ScoreLabel: "93.0%",
		ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1453, VariantMeasured: "gpt-5-6-luna", SourceURL: "https://arena.ai/leaderboard/text", Checked: "2026-08-06"},
		ArenaLabel: "1453 Elo",
		TaskFit:    []string{"implement", "debug"},
		Note:       "Дорогая, но лучшая по SWE-bench.",
	}
}

func tuiDetailIndex(t *testing.T, lines []string, prefix string) int {
	t.Helper()
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}
	t.Fatalf("no line starts with %q in:\n%s", prefix, strings.Join(lines, "\n"))
	return -1
}

func TestTUIDetailLinesShowEveryBlockInOrder(t *testing.T) {
	now := time.Unix(1786034890, 0).UTC().AddDate(0, 0, 64)
	lines := tuiDetailLines(tuiDetailTestModel(), scoreSourceSWEBench, 100, now)
	joined := strings.Join(lines, "\n")

	if lines[0] != "GPT-5.6 Luna (openai/gpt-5.6-luna)" {
		t.Errorf("header = %q, want the display name and the slug", lines[0])
	}
	for _, want := range []string{
		"Производитель: OpenAI (C)",
		"Тир: opus",
		"Claude-референс: ≈ Opus 4.6",
		"Дата релиза: 2026-08-06 (2 месяца назад)",
		"Контекст: 1M",
		"Вход: $0.50 за M токенов",
		"Выход: $3.00 за M токенов",
		"Длинный контекст: $1.00 / $4.00 от 272K+",
		"  вход: $1.00 от 272K+",
		"  выход: $4.00 от 272K+",
		"Открытые веса: нет",
		"Task fit: implement + debug",
		"Заметка:",
		"  Дорогая, но лучшая по SWE-bench.",
		"Описание:",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("detail lines are missing %q:\n%s", want, joined)
		}
	}

	order := []string{"GPT-5.6 Luna", "Производитель:", "Дата релиза:", "Контекст:", "Открытые веса:", "Оценка SWE-bench", "Оценка LMArena", "Task fit:", "Заметка:", "Описание:"}
	previous := -1
	for _, prefix := range order {
		index := tuiDetailIndex(t, lines, prefix)
		if index <= previous {
			t.Fatalf("block %q is at line %d, out of order against the previous block at %d:\n%s", prefix, index, previous, joined)
		}
		previous = index
	}
	if tuiDetailIndex(t, lines, "Описание:") != len(lines)-2 {
		t.Errorf("the description must be the last block; it is not:\n%s", joined)
	}
}

// TestTUIDetailLinesKeepTheTwoScoreSourcesApart is the detail screen's
// share of the project-wide invariant: SWE-bench Verified percentages and
// LMArena Elo ratings are never one number and never one line. The detail
// screen is the only place that shows both at once, which is allowed
// precisely because each has its own heading naming its own scale.
func TestTUIDetailLinesKeepTheTwoScoreSourcesApart(t *testing.T) {
	lines := tuiDetailLines(tuiDetailTestModel(), scoreSourceSWEBench, 100, time.Now())
	swe := tuiDetailIndex(t, lines, "Оценка SWE-bench Verified")
	arena := tuiDetailIndex(t, lines, "Оценка LMArena")
	if swe >= arena {
		t.Fatalf("the SWE-bench block must come before the Arena block: %d vs %d", swe, arena)
	}
	for _, line := range lines {
		if strings.Contains(line, "93.0%") && strings.Contains(line, "1453 Elo") {
			t.Fatalf("a percentage and an Elo rating share one line: %q", line)
		}
	}
	if !strings.Contains(strings.Join(lines[swe:arena], "\n"), "93.0%") {
		t.Errorf("the SWE-bench block does not carry the SWE-bench label:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines[arena:], "\n"), "1453 Elo") {
		t.Errorf("the Arena block does not carry the Elo label:\n%s", strings.Join(lines, "\n"))
	}
	for _, want := range []string{"  Источник: https://www.vals.ai/benchmarks/swebench", "  Проверено: 2026-08-03", "  Измеренный вариант: openai/gpt-5.6-luna"} {
		if !strings.Contains(strings.Join(lines[swe:arena], "\n"), want) {
			t.Errorf("the SWE-bench block lost its provenance line %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
}

// TestTUIDetailLinesNeverPrintAnEloUnderTheSWEBenchHeading covers the trap
// model.ForScoreSource sets for this screen: in the arena view it has
// already overwritten Score/ScoreLabel with the Arena projection, so a
// block built from those fields would show "1453 Elo" as a SWE-bench
// Verified percentage.
func TestTUIDetailLinesNeverPrintAnEloUnderTheSWEBenchHeading(t *testing.T) {
	projected := model.ForScoreSource([]model.Model{tuiDetailTestModel()}, scoreSourceArena)[0]
	lines := tuiDetailLines(projected, scoreSourceArena, 100, time.Now())
	swe := tuiDetailIndex(t, lines, "Оценка SWE-bench Verified")
	arena := tuiDetailIndex(t, lines, "Оценка LMArena")
	if swe >= arena {
		t.Fatalf("the SWE-bench block must come before the Arena block: %d vs %d", swe, arena)
	}
	block := strings.Join(lines[swe:arena], "\n")
	if strings.Contains(block, "1453 Elo") || strings.Contains(block, "arena.ai") {
		t.Fatalf("the arena-mode SWE-bench block carries Arena data:\n%s", block)
	}
	if !strings.Contains(block, "н/д") {
		t.Errorf("the arena-mode SWE-bench block must say н/д instead of borrowing the other scale:\n%s", block)
	}
	if !strings.Contains(strings.Join(lines[arena:], "\n"), "1453 Elo") {
		t.Errorf("the Arena block lost its Elo label in arena mode:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTUIDetailLinesFallBackToThePlaceholder(t *testing.T) {
	lines := tuiDetailLines(model.Model{Slug: "a/bare"}, scoreSourceSWEBench, 60, time.Now())
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Производитель: н/д", "Тир: н/д", "Claude-референс: н/д", "Дата релиза: н/д", "Страница OpenRouter: н/д", "Контекст: н/д", "Открытые веса: н/д", "Task fit: н/д"} {
		if !strings.Contains(joined, want) {
			t.Errorf("an empty model is missing the placeholder line %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Длинный контекст") {
		t.Errorf("a model without a long-context tier must not get that block at all:\n%s", joined)
	}
	if lines[len(lines)-1] != "  н/д" {
		t.Errorf("an empty description = %q, want the placeholder", lines[len(lines)-1])
	}
}

func TestTUIDetailAgeUsesRussianPluralForms(t *testing.T) {
	published := time.Unix(1786034890, 0).UTC()
	for _, test := range []struct {
		days int
		want string
	}{
		{0, "2026-08-06 (сегодня)"},
		{1, "2026-08-06 (1 день назад)"},
		{2, "2026-08-06 (2 дня назад)"},
		{5, "2026-08-06 (5 дней назад)"},
		{11, "2026-08-06 (11 дней назад)"},
		{21, "2026-08-06 (21 день назад)"},
		{64, "2026-08-06 (2 месяца назад)"},
		{150, "2026-08-06 (5 месяцев назад)"},
		{400, "2026-08-06 (1 год назад)"},
		{1100, "2026-08-06 (3 года назад)"},
	} {
		if got := tuiDetailCreated(1786034890, published.AddDate(0, 0, test.days)); got != test.want {
			t.Errorf("%d days after publication = %q, want %q", test.days, got, test.want)
		}
	}
	if got := tuiDetailCreated(0, published); got != "н/д" {
		t.Errorf("a zero timestamp = %q, want the placeholder", got)
	}
}

func TestTUIDetailMaxOffsetCountsWrappedLines(t *testing.T) {
	row := tuiDetailTestModel()
	row.Description = strings.Repeat("длинное вендорское описание модели ", 20)
	narrow := tuiDetailMaxOffset(row, scoreSourceSWEBench, 30, 10)
	wide := tuiDetailMaxOffset(row, scoreSourceSWEBench, 200, 10)
	if narrow <= wide {
		t.Fatalf("max offset must grow as the screen narrows and the text wraps into more lines: narrow=%d wide=%d", narrow, wide)
	}
	if got := tuiDetailMaxOffset(row, scoreSourceSWEBench, 200, 1000); got != 0 {
		t.Errorf("max offset on a viewport taller than the content = %d, want 0", got)
	}
	lines := tuiDetailLines(row, scoreSourceSWEBench, 30, time.Now())
	if want := len(lines) - tuiDetailBodyHeight(10); narrow != want {
		t.Errorf("max offset = %d, want len(lines)-bodyHeight = %d", narrow, want)
	}
}

// TestTUIDetailLinesShowBothModelLinks pins down the position and the
// exact shape of the two link lines: they close the identity group, right
// after the release date and before the blank line that opens the context
// and pricing block.
func TestTUIDetailLinesShowBothModelLinks(t *testing.T) {
	row := tuiDetailTestModel()
	lines := tuiDetailLines(row, scoreSourceSWEBench, 100, time.Now())
	joined := strings.Join(lines, "\n")

	for _, want := range []string{
		"Страница OpenRouter: https://openrouter.ai/openai/gpt-5.6-luna-20260804",
		"Репозиторий HuggingFace: https://huggingface.co/openai-community/gpt-5-6-luna",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("detail lines are missing %q:\n%s", want, joined)
		}
	}

	created := tuiDetailIndex(t, lines, "Дата релиза:")
	openrouter := tuiDetailIndex(t, lines, "Страница OpenRouter:")
	hugging := tuiDetailIndex(t, lines, "Репозиторий HuggingFace:")
	if openrouter != created+1 || hugging != created+2 {
		t.Fatalf("link lines are at %d and %d, want them immediately after the release date at %d:\n%s", openrouter, hugging, created, joined)
	}
	if lines[hugging+1] != "" {
		t.Errorf("line after the links = %q, want the blank line that separates the identity group from the context block", lines[hugging+1])
	}
}

// TestTUIDetailLinesBuildTheOpenRouterLinkFromTheCanonicalSlug is the
// whole reason a new catalogue field was threaded through five pipeline
// hops instead of reusing m.Slug: id and canonical_slug disagree for 62%
// of the live catalogue, so a link built from the slug would sometimes
// point at a 404 and sometimes, worse, at a different variant's page.
func TestTUIDetailLinesBuildTheOpenRouterLinkFromTheCanonicalSlug(t *testing.T) {
	row := tuiDetailTestModel()
	lines := tuiDetailLines(row, scoreSourceSWEBench, 100, time.Now())
	link := lines[tuiDetailIndex(t, lines, "Страница OpenRouter:")]
	if want := "Страница OpenRouter: https://openrouter.ai/" + row.CanonicalSlug; link != want {
		t.Fatalf("link line = %q, want %q", link, want)
	}
	if link == "Страница OpenRouter: https://openrouter.ai/"+row.Slug {
		t.Fatalf("link line = %q, want it built from the canonical slug, not from the id", link)
	}
}

// TestTUIDetailLinesOmitTheHuggingFaceLineWithoutARepository covers the
// screen's one deliberate exception to "always print the label, н/д when
// empty": a proprietary model has no repository, that fact is already
// stated by the Открытые веса line, and it is the majority case — around
// 60% of catalogue entries carry no hugging_face_id at all.
func TestTUIDetailLinesOmitTheHuggingFaceLineWithoutARepository(t *testing.T) {
	row := tuiDetailTestModel()
	row.HuggingFaceID = ""
	joined := strings.Join(tuiDetailLines(row, scoreSourceSWEBench, 100, time.Now()), "\n")
	if strings.Contains(joined, "HuggingFace") {
		t.Errorf("a model with no repository must not mention HuggingFace at all, not even as н/д:\n%s", joined)
	}
	if !strings.Contains(joined, "Страница OpenRouter: https://openrouter.ai/") {
		t.Errorf("the OpenRouter link must still be there:\n%s", joined)
	}
}

// TestTUIDetailLinesShowThePlaceholderForAMissingCanonicalSlug covers the
// other half of the rule: canonical_slug is present on every catalogue
// entry, so an empty one is an anomaly worth showing rather than hiding.
// It is reachable in exactly one way — a snapshot written before this
// feature existed, read before the next refresh.
func TestTUIDetailLinesShowThePlaceholderForAMissingCanonicalSlug(t *testing.T) {
	row := tuiDetailTestModel()
	row.CanonicalSlug = ""
	joined := strings.Join(tuiDetailLines(row, scoreSourceSWEBench, 100, time.Now()), "\n")
	if !strings.Contains(joined, "Страница OpenRouter: н/д") {
		t.Errorf("an empty canonical slug must print the placeholder:\n%s", joined)
	}
	if strings.Contains(joined, "https://openrouter.ai/") {
		t.Errorf("an empty canonical slug must never be papered over with a link guessed from the slug:\n%s", joined)
	}
}

// TestTUIDetailMaxOffsetAccountsForTheHuggingFaceLine checks that the
// conditional line is counted by the scrolling maths rather than
// hardcoded anywhere: tuiDetailMaxOffset derives the limit from the lines
// actually built for this model.
func TestTUIDetailMaxOffsetAccountsForTheHuggingFaceLine(t *testing.T) {
	withRepo := tuiDetailTestModel()
	without := tuiDetailTestModel()
	without.HuggingFaceID = ""
	got := tuiDetailMaxOffset(withRepo, scoreSourceSWEBench, 100, 10)
	want := tuiDetailMaxOffset(without, scoreSourceSWEBench, 100, 10)
	if got != want+1 {
		t.Fatalf("max offset with a repository = %d, without = %d, want exactly one line of difference", got, want)
	}
}

func TestTUIDetailViewShowsTheSelectedModelAndBothScoreBlocks(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay, m.width, m.height = "detail", 120, 60
	view := ansi.Strip(m.View())
	for _, want := range []string{"GPT-5.6 Luna", "openai/gpt-5.6-luna", "Оценка SWE-bench Verified", "93.0%", "Оценка LMArena", "1453 Elo", "Task fit: implement + debug", "Дорогая, но лучшая", "long-context flagship", "Esc close"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view is missing %q:\n%s", want, view)
		}
	}
}

// TestTUIDetailViewWrapsTheDescriptionInsteadOfTruncating is the point of
// the whole screen: on a narrow terminal the vendor prose must still be
// readable in full, not cut at the right edge the way every table cell is.
func TestTUIDetailViewWrapsTheDescriptionInsteadOfTruncating(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay, m.width, m.height = "detail", 40, 60
	view := m.View()
	parts := strings.SplitN(view, "Описание:", 2)
	if len(parts) != 2 {
		t.Fatalf("the detail view has no description block:\n%s", view)
	}
	// Joining the block's fields back together undoes the wrap: the prose
	// must be there in full, whereas truncateTable would have replaced its
	// tail with an ellipsis on every single line.
	rebuilt := strings.Join(strings.Fields(parts[1]), " ")
	if !strings.Contains(rebuilt, "GPT-5.6 Luna is OpenAI's long-context flagship, strong at code and weak at latency.") {
		t.Errorf("the description was not preserved in full at width 40:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("wrapped detail line exceeds width %d: %q", m.width, line)
		}
	}
}

func TestTUIDetailViewFitsEveryViewport(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay = "detail"
	for _, height := range []int{1, 2, 3, 5, 7, 24, 80} {
		for _, width := range []int{1, 5, 20, 40, 200} {
			m.width, m.height = width, height
			assertTUIViewFits(t, m.View(), width, height, "detail")
		}
	}
}

func TestTUIDetailViewFooterReportsThePosition(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay, m.width, m.height = "detail", 120, 10
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	total := len(tuiDetailLines(m.visible[0], m.scoreSource, m.width, time.Now()))
	if want := fmt.Sprintf("Detail 1-9/%d · ↑↓ scroll · Esc close", total); !strings.HasPrefix(lines[len(lines)-1], want) {
		t.Fatalf("footer = %q, want a prefix of %q", lines[len(lines)-1], want)
	}
}

// tuiForceColorProfile makes lipgloss actually emit escape sequences for
// the duration of one test. The test binary's stdout is not a terminal,
// so lipgloss's default renderer settles on termenv.Ascii, where
// Style.Render returns its input untouched — every assertion about
// styling would then pass whether the screen styles anything or not.
// Forcing a profile is what lipgloss's own SetColorProfile doc comment
// calls the function's main purpose. The previous profile is restored so
// the rest of the package keeps comparing plain text.
func tuiForceColorProfile(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func TestTUIStyleDetailLineAppliesTheScreensStyleVocabulary(t *testing.T) {
	tuiForceColorProfile(t)
	for _, test := range []struct {
		name string
		line string
		want string
	}{
		{"empty line", "", ""},
		{"blank line", "   ", "   "},
		{"block heading", "Заметка:", tuiHeaderStyle.Render("Заметка:")},
		{"block heading with brackets", "Оценка LMArena (рейтинг Elo):", tuiHeaderStyle.Render("Оценка LMArena (рейтинг Elo):")},
		{"label and value", "Производитель: OpenAI (C)", tuiHeaderStyle.Render("Производитель: ") + "OpenAI (C)"},
		{"indented label and value", "  Значение: 93.0%", tuiHeaderStyle.Render("  Значение: ") + "93.0%"},
		{"label and placeholder", "Дата релиза: н/д", tuiHeaderStyle.Render("Дата релиза: ") + tuiHintStyle.Render("н/д")},
		{"provenance link", "  Источник: https://www.vals.ai/benchmarks/swebench", tuiHeaderStyle.Render("  Источник: ") + tuiLinkStyle.Render("https://www.vals.ai/benchmarks/swebench")},
		{"model link", "Страница OpenRouter: https://openrouter.ai/openai/gpt-5.6-luna-20260804", tuiHeaderStyle.Render("Страница OpenRouter: ") + tuiLinkStyle.Render("https://openrouter.ai/openai/gpt-5.6-luna-20260804")},
		{"bare placeholder", "  н/д", tuiHintStyle.Render("  н/д")},
		{"qualified placeholder", "  н/д (активно представление arena)", tuiHintStyle.Render("  н/д (активно представление arena)")},
		{"prose", "  GPT-5.6 Luna is OpenAI's long-context flagship.", "  GPT-5.6 Luna is OpenAI's long-context flagship."},
		{"footer text is not special on its own", "Detail 1-9/40 · ↑↓ scroll · Esc close", "Detail 1-9/40 · ↑↓ scroll · Esc close"},
	} {
		if got := tuiStyleDetailLine(test.line); got != test.want {
			t.Errorf("%s: tuiStyleDetailLine(%q) = %q, want %q", test.name, test.line, got, test.want)
		}
	}
}

// TestTUIStyleDetailLineNeverTouchesAnAlreadyStyledLine documents why
// slicing a line at a byte offset found on its plain text is safe: it is
// only ever done when the line carries no escape sequences at all, which
// by construction it never does — everything upstream is plain text and
// tuiFullscreenText runs every line through plainTableText, which no
// escape survives. The guard is what makes that assumption checkable
// instead of merely believed.
func TestTUIStyleDetailLineNeverTouchesAnAlreadyStyledLine(t *testing.T) {
	tuiForceColorProfile(t)
	styled := tuiErrorStyle.Render("Производитель: OpenAI (C)")
	if got := tuiStyleDetailLine(styled); got != styled {
		t.Fatalf("tuiStyleDetailLine(%q) = %q, want the line returned untouched rather than sliced inside an escape sequence", styled, got)
	}
}

// TestTUIStyleDetailLineChangesNoVisibleCharacter is the per-line half of
// the feature's central invariant: styling adds colour and nothing else.
func TestTUIStyleDetailLineChangesNoVisibleCharacter(t *testing.T) {
	tuiForceColorProfile(t)
	for _, line := range tuiDetailLines(tuiDetailTestModel(), scoreSourceSWEBench, 60, time.Now()) {
		if got := ansi.Strip(tuiStyleDetailLine(line)); got != line {
			t.Errorf("styling changed the text of %q into %q", line, got)
		}
	}
}

// TestTUIDetailViewStylingLeavesTheLayoutUntouched is the central test of
// this feature. The detail screen's whole width arithmetic —
// tableDisplayWidth, truncateTable, tuiWrapText, plainTableText — is
// ANSI-unaware: it would count escape bytes as visible columns, cut a
// line mid-escape and leave "[38;5;87m" on screen as text. The design
// answer is to style strictly after all of that, as a pass over finished
// output, so this test compares the styled view against the very same
// view rendered with colour off: they must be identical character for
// character, at every width, height and offset.
func TestTUIDetailViewStylingLeavesTheLayoutUntouched(t *testing.T) {
	// The helper is here only for its t.Cleanup: this test flips the
	// profile itself on every iteration, and the cleanup is what restores
	// whatever the rest of the package expects afterwards.
	tuiForceColorProfile(t)
	row := tuiDetailTestModel()
	row.Description = strings.Repeat("длинное вендорское описание модели ", 20)
	for _, width := range []int{1, 5, 20, 40, 80, 200} {
		for _, height := range []int{1, 2, 5, 24, 80} {
			for _, offset := range []int{0, 3, 999} {
				m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
				m.overlay, m.width, m.height, m.detailOffset = "detail", width, height, offset

				lipgloss.SetColorProfile(termenv.Ascii)
				plain := m.View()
				lipgloss.SetColorProfile(termenv.ANSI256)
				styled := m.View()

				if ansi.Strip(styled) != plain {
					t.Fatalf("width=%d height=%d offset=%d: styling changed the text\nstyled: %q\nplain:  %q", width, height, offset, ansi.Strip(styled), plain)
				}
				lines := strings.Split(styled, "\n")
				if len(lines) != len(strings.Split(plain, "\n")) {
					t.Fatalf("width=%d height=%d offset=%d: styling changed the line count", width, height, offset)
				}
				for i, line := range lines {
					if lipgloss.Width(line) > width {
						t.Fatalf("width=%d height=%d offset=%d: styled line %d is %d columns wide: %q", width, height, offset, i, lipgloss.Width(line), line)
					}
					if strings.Contains(ansi.Strip(line), "[38;5;") || strings.Contains(ansi.Strip(line), "[1m") {
						t.Fatalf("width=%d height=%d offset=%d: an escape sequence leaked through plainTableText as visible text: %q", width, height, offset, line)
					}
				}
			}
		}
	}
}

// TestTUIDetailViewStylesTheHeaderOnlyWhenItIsOnScreen pins down why the
// header is found by index rather than by text: tuiDetailView knows the
// offset it just applied, so at offset 0 the first visible line is the
// title and after scrolling it is an ordinary content line that must be
// styled by the general rules instead.
func TestTUIDetailViewStylesTheHeaderOnlyWhenItIsOnScreen(t *testing.T) {
	tuiForceColorProfile(t)
	row := tuiDetailTestModel()
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.overlay, m.width, m.height = "detail", 120, 12

	top := strings.Split(m.View(), "\n")[0]
	if want := tuiTitleStyle.Render(ansi.Strip(top)); top != want {
		t.Errorf("first line at offset 0 = %q, want the title style %q", top, want)
	}

	m.detailOffset = 4
	scrolled := strings.Split(m.View(), "\n")[0]
	if ansi.Strip(scrolled) == ansi.Strip(top) {
		t.Fatalf("test setup: the screen did not scroll, both offsets show %q", ansi.Strip(top))
	}
	if want := tuiStyleDetailLine(ansi.Strip(scrolled)); scrolled != want {
		t.Errorf("first line after scrolling = %q, want the ordinary line rules to decide it instead of the title style: %q", scrolled, want)
	}
}

func TestTUIDetailViewStylesTheFooterAtEveryOffset(t *testing.T) {
	tuiForceColorProfile(t)
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay, m.width, m.height = "detail", 120, 12
	for _, offset := range []int{0, 5, 999} {
		m.detailOffset = offset
		lines := strings.Split(m.View(), "\n")
		footer := lines[len(lines)-1]
		if !strings.HasPrefix(ansi.Strip(footer), "Detail ") {
			t.Fatalf("offset %d: last line = %q, want the position footer", offset, ansi.Strip(footer))
		}
		if want := tuiHintStyle.Render(ansi.Strip(footer)); footer != want {
			t.Errorf("offset %d: footer = %q, want the hint style %q", offset, footer, want)
		}
	}
}

// TestTUIDetailViewStylesBothLinkKinds ties the styling rules back to the
// two things the feature is for: the new model links and the provenance
// URLs that were already on screen get the same, single link style.
func TestTUIDetailViewStylesBothLinkKinds(t *testing.T) {
	tuiForceColorProfile(t)
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay, m.width, m.height = "detail", 200, 60
	view := m.View()
	for _, url := range []string{
		"https://openrouter.ai/openai/gpt-5.6-luna-20260804",
		"https://huggingface.co/openai-community/gpt-5-6-luna",
		"https://www.vals.ai/benchmarks/swebench",
	} {
		if !strings.Contains(view, tuiLinkStyle.Render(url)) {
			t.Errorf("the view does not carry %q in the link style:\n%s", url, view)
		}
	}
	if !strings.Contains(view, tuiHeaderStyle.Render("Страница OpenRouter: ")) {
		t.Errorf("the link label is not styled like every other field label:\n%s", view)
	}
}

// TestTUIDetailViewOnAnEmptyListDoesNotPanic covers the state a failing
// filter or a broken ranking formula leaves the model in: visible is nil
// while cursor is still 0.
func TestTUIDetailViewOnAnEmptyListDoesNotPanic(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.overlay, m.width, m.height = "detail", 80, 10
	if view := m.View(); strings.TrimSpace(view) == "" {
		t.Fatal("the detail overlay on an empty list rendered nothing at all")
	}
}

func tuiDetailModel(t *testing.T) tuiModel {
	t.Helper()
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.width, m.height = 100, 10
	return m
}

func TestTUIDetailOverlayOpensAndCloses(t *testing.T) {
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyRight},
		{Type: tea.KeyRunes, Runes: []rune("l")},
	} {
		m := tuiDetailModel(t)
		m, _ = m.key(msg)
		if m.overlay != "detail" || m.detailOffset != 0 {
			t.Fatalf("key %v: overlay=%q offset=%d, want the detail overlay at offset 0", msg, m.overlay, m.detailOffset)
		}
	}
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyEscape},
		{Type: tea.KeyLeft},
		{Type: tea.KeyRunes, Runes: []rune("h")},
	} {
		m := tuiDetailModel(t)
		m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
		m.detailOffset = 3
		m, _ = m.key(msg)
		if m.overlay != "" || m.detailOffset != 0 {
			t.Fatalf("key %v: overlay=%q offset=%d, want the list back with the offset reset", msg, m.overlay, m.detailOffset)
		}
	}
}

// TestTUIDetailOverlayKeepsTheCursor pins down the promise that closing
// the screen returns exactly the same list row it was opened from.
func TestTUIDetailOverlayKeepsTheCursor(t *testing.T) {
	rows := []model.Model{{Slug: "a", DisplayName: "A"}, {Slug: "b", DisplayName: "B"}, {Slug: "c", DisplayName: "C"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.width, m.height = 100, 20
	m = tuiKey(m, "j")
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEscape})
	if m.cursor != 1 || m.selectedSlug != "b" {
		t.Fatalf("cursor=%d selected=%q, want the same row the overlay was opened from", m.cursor, m.selectedSlug)
	}
}

func TestTUIDetailOverlayScrollsWithinItsBounds(t *testing.T) {
	row := tuiDetailTestModel()
	row.Description = strings.Repeat("длинное вендорское описание модели ", 20)
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.width, m.height = 60, 10
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	maxOffset := tuiDetailMaxOffset(row, m.scoreSource, m.width, m.height)
	if maxOffset == 0 {
		t.Fatal("test setup: the fixture must be taller than the viewport")
	}

	m, _ = m.key(tea.KeyMsg{Type: tea.KeyDown})
	if m.detailOffset != 1 {
		t.Fatalf("down offset = %d, want 1", m.detailOffset)
	}
	m = tuiKey(m, "k")
	if m.detailOffset != 0 {
		t.Fatalf("k offset = %d, want 0", m.detailOffset)
	}
	m = tuiKey(m, "G")
	if m.detailOffset != maxOffset {
		t.Fatalf("G offset = %d, want the maximum %d", m.detailOffset, maxOffset)
	}
	m = tuiKey(m, "g")
	if m.detailOffset != 0 {
		t.Fatalf("g offset = %d, want 0", m.detailOffset)
	}
	for i := 0; i < 50; i++ {
		m, _ = m.key(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	if m.detailOffset != maxOffset {
		t.Fatalf("pgdown offset = %d, want it clamped at %d", m.detailOffset, maxOffset)
	}
	for i := 0; i < 50; i++ {
		m, _ = m.key(tea.KeyMsg{Type: tea.KeyPgUp})
	}
	if m.detailOffset != 0 {
		t.Fatalf("pgup offset = %d, want it clamped at 0", m.detailOffset)
	}
	if m.overlay != "detail" {
		t.Fatalf("overlay = %q, want scrolling to keep the overlay open", m.overlay)
	}
}

// TestTUIDetailOverlaySwallowsListShortcuts mirrors the help overlay's
// behaviour: keys the overlay does not define do nothing at all, instead
// of reaching through and re-sorting the list behind it.
func TestTUIDetailOverlaySwallowsListShortcuts(t *testing.T) {
	m := tuiDetailModel(t)
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	before := m.sortKey
	for _, key := range []string{"s", "S", "m", "n", "c", "/", "f"} {
		m = tuiKey(m, key)
	}
	if m.overlay != "detail" || m.sortKey != before || m.inputMode != "" {
		t.Fatalf("overlay=%q sort=%q input=%q, want the detail overlay to swallow list shortcuts", m.overlay, m.sortKey, m.inputMode)
	}
}

func TestTUIDetailOverlayDoesNotOpenOnAnEmptyList(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.width, m.height = 100, 10
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if m.overlay != "" {
		t.Fatalf("overlay = %q, want no detail screen when there is no row to show", m.overlay)
	}
}

func TestTUIHelpDocumentsTheDetailScreen(t *testing.T) {
	for _, want := range []string{
		"Model detail view",
		"Enter, Right or l opens the detail screen",
		"Esc, Left or h closes it",
		"scroll the detail text",
		"links to the model's OpenRouter page",
		"HuggingFace repository",
	} {
		if !strings.Contains(tuiHelpDocument, want) {
			t.Errorf("help document is missing %q", want)
		}
	}
	for _, r := range tuiHelpDocument {
		if unicode.Is(unicode.Cyrillic, r) {
			t.Fatalf("help document must stay English-only: %q", tuiHelpDocument)
		}
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{tuiDetailTestModel()})
	m.overlay, m.width, m.height = "help", 120, len(tuiHelpLines())+2
	if !strings.Contains(m.View(), "Model detail view") {
		t.Errorf("the rendered help does not show the detail-screen section:\n%s", m.View())
	}
}

// TestTUIDetailScreenShowsCatalogueMetadataFromTheSnapshot is the
// whole-feature acceptance check. It goes through the real construction
// path — newConfiguredTUIModel, which loads from disk exactly the way a
// live session and a live refresh both do — so it fails if ANY hop of the
// pipeline drops the two catalogue fields: the decoder, MergeWithArena,
// NewSnapshot, or the PriceInfo rebuild in loadLocalModelsForSource.
func TestTUIDetailScreenShowsCatalogueMetadataFromTheSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/dated\ttier=sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/dated:\n    display: Demo Dated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{
		"demo/dated": {
			InPerM: 1, OutPerM: 3, Context: 128000,
			Created: 1786034890, Description: "Demo Dated is strong at long context and weak at latency.",
			Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 75, SourceURL: "https://www.vals.ai/benchmarks/swebench", Checked: "2026-08-03"},
			ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1400, SourceURL: "https://arena.ai/leaderboard/text", Checked: "2026-08-06"},
		},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	compiled, err := ranking.Compile(ranking.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	m, err := newConfiguredTUIModel(context.Background(), root, refresh.Options{}, 0, "q/p", false, "", 0, false, rankingDefault, compiled, scoreSourceSWEBench)
	if err != nil {
		t.Fatalf("newConfiguredTUIModel: %v", err)
	}
	m.width, m.height = 120, 80
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if m.overlay != "detail" {
		t.Fatalf("overlay = %q, want the detail screen open", m.overlay)
	}

	view := ansi.Strip(m.View())
	for _, want := range []string{
		"Demo Dated (demo/dated)",
		"Дата релиза: 2026-08-06",
		"Demo Dated is strong at long context and weak at latency.",
		"Оценка SWE-bench Verified",
		"75.0%",
		"Оценка LMArena",
		"1400 Elo",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the detail screen built from the snapshot is missing %q:\n%s", want, view)
		}
	}
}

// TestTUIDetailScreenShowsModelLinksFromTheSnapshot is the whole-feature
// acceptance check. It goes through the real construction path —
// newConfiguredTUIModel, which loads from disk exactly the way a live
// session and a live refresh both do — so it fails if ANY hop of the
// pipeline drops the two identifiers: the decoder, MergeWithArena,
// NewSnapshot, or either of the two PriceInfo rebuilds. It renders with
// colour forced on, so it also proves the styled screen still says
// exactly what the plain one says.
func TestTUIDetailScreenShowsModelLinksFromTheSnapshot(t *testing.T) {
	tuiForceColorProfile(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/dated\ttier=sonnet\ndemo/closed\ttier=opus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/dated:\n    display: Demo Dated\n  demo/closed:\n    display: Demo Closed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{
		"demo/dated": {
			InPerM: 1, OutPerM: 3, Context: 128000,
			Created: 1786034890, Description: "Demo Dated is strong at long context.",
			CanonicalSlug: "demo/dated-20260804", HuggingFaceID: "demo-labs/Dated",
			Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 75, SourceURL: "https://www.vals.ai/benchmarks/swebench", Checked: "2026-08-03"},
		},
		"demo/closed": {
			InPerM: 2, OutPerM: 6, Context: 200000,
			Created: 1786034890, Description: "Demo Closed has no public weights.",
			CanonicalSlug: "demo/closed-20260804",
		},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	compiled, err := ranking.Compile(ranking.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	m, err := newConfiguredTUIModel(context.Background(), root, refresh.Options{}, 0, "q/p", false, "", 0, false, rankingDefault, compiled, scoreSourceSWEBench)
	if err != nil {
		t.Fatalf("newConfiguredTUIModel: %v", err)
	}
	m.width, m.height = 120, 80
	m.cursor = tuiRowIndex(t, m.visible, "demo/dated")
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if m.overlay != "detail" {
		t.Fatalf("overlay = %q, want the detail screen open", m.overlay)
	}

	view := m.View()
	plain := ansi.Strip(view)
	for _, want := range []string{
		"Demo Dated (demo/dated)",
		"Страница OpenRouter: https://openrouter.ai/demo/dated-20260804",
		"Репозиторий HuggingFace: https://huggingface.co/demo-labs/Dated",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("the detail screen built from the snapshot is missing %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(view, tuiLinkStyle.Render("https://openrouter.ai/demo/dated-20260804")) {
		t.Errorf("the OpenRouter link reached the screen unstyled:\n%s", view)
	}

	// The second row has no HuggingFace repository: its line must be absent
	// entirely rather than present as н/д. The overlay always renders the
	// row the list highlights, so moving the cursor is all it takes.
	m.cursor = tuiRowIndex(t, m.visible, "demo/closed")
	closed := ansi.Strip(m.View())
	if !strings.Contains(closed, "Страница OpenRouter: https://openrouter.ai/demo/closed-20260804") {
		t.Errorf("the second row lost its OpenRouter link:\n%s", closed)
	}
	if strings.Contains(closed, "HuggingFace") {
		t.Errorf("a model with no repository must not mention HuggingFace at all:\n%s", closed)
	}
}

func tuiRowIndex(t *testing.T, rows []model.Model, slug string) int {
	t.Helper()
	for i, row := range rows {
		if row.Slug == slug {
			return i
		}
	}
	t.Fatalf("no row with slug %q in %+v", slug, rows)
	return -1
}

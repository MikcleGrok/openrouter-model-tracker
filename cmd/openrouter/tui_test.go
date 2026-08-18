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
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/tier"
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

func TestTUIModelUsesFiveCentDefaultPriceSteps(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	if m.filterSteps.InputCents != 5 || m.filterSteps.OutputCents != 5 {
		t.Fatalf("default TUI price steps = %d/%d cents, want 5/5", m.filterSteps.InputCents, m.filterSteps.OutputCents)
	}
}

func TestTUIUsesConfiguredNameWidthAndClipsToViewport(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{DisplayName: "A long model name", Owner: "OpenAI"}})
	m.nameWidth = 28
	m.width = 100
	line := m.renderTUILine([]tuiColumn{colName, colStatus}, []string{"Ⓜ️ Meta A long model name", "90%"}, false)
	if !strings.Contains(line, "90%") || tableDisplayWidth(line) > 100 {
		t.Fatalf("configured TUI name width rendered unsafely: %q", line)
	}
	m.width = 12
	line = m.renderTUILine([]tuiColumn{colName, colStatus}, []string{"Ⓜ️ Meta A long model name", "90%"}, false)
	if tableDisplayWidth(line) > 12 {
		t.Fatalf("narrow TUI line exceeds viewport: %d: %q", tableDisplayWidth(line), line)
	}
}

func TestTUIUsesConfiguredKeymapAndRendersItInHelp(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.keymap = config.DefaultTUIKeymap()
	m.keymap["main"]["open_settings"] = config.TUIBindings{"z"}
	m = tuiKey(m, "z")
	if m.overlay != "settings" {
		t.Fatalf("custom settings binding opened overlay %q", m.overlay)
	}
	m.overlay, m.helpSection = "help", 2 // Hotkeys: where "open settings." lives
	m.height = len(m.helpLines()) + 2
	if !strings.Contains(m.View(), "z") {
		t.Fatalf("configured settings binding is missing from help: %q", m.View())
	}
}

func TestTUIRefreshMessageAppliesReloadedKeymapToKeyEvents(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.generation = 1
	reloaded := config.DefaultTUIKeymap()
	reloaded["main"]["open_settings"] = config.TUIBindings{"z"}
	next, _ := m.Update(tuiRefreshMsg{generation: 1, keymap: reloaded, models: m.models})
	m = next.(tuiModel)
	m = tuiKey(m, "z")
	if m.overlay != "settings" || m.keymap["main"]["open_settings"][0] != "z" {
		t.Fatalf("refresh did not install keymap: overlay=%q keymap=%v", m.overlay, m.keymap["main"]["open_settings"])
	}
}

func TestTUIConfiguredScalarBindingsHandleCanonicalAliases(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.keymap["settings"]["switch_source"] = config.TUIBindings{"enter"}
	m.overlay, m.settingsCursor = "settings", 1
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if !m.scoreSourceLoading || cmd == nil {
		t.Fatalf("scalar enter source binding did not start loading: loading=%v cmd=%v", m.scoreSourceLoading, cmd)
	}

	m = newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.keymap["columns"]["toggle"] = config.TUIBindings{" "}
	m.keymap["columns"]["apply"] = config.TUIBindings{" enter "}
	m.overlay, m.pendingColumns, m.columnCursor = "columns", append([]tuiColumn(nil), m.columns...), 0
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(tuiModel)
	if len(m.pendingColumns) == len(m.columns) {
		t.Fatal("space binding did not toggle a column")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if m.overlay != "" {
		t.Fatalf("scalar enter apply binding did not close columns overlay: %q", m.overlay)
	}

	m = newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.keymap["detail"]["close"] = config.TUIBindings{" esc "}
	m.overlay = "detail"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if next.(tuiModel).overlay != "" {
		t.Fatal("surrounding-space escape binding did not close detail overlay")
	}
}

func TestTUIFilterViewShowsAllowedTierValues(t *testing.T) {
	m := tuiModel{overlay: "filter", width: 100, height: 20}
	view := m.View()
	if !strings.Contains(view, "Tier options: (any), "+tier.ValuesString()) {
		t.Fatalf("filter view = %q, want tier select options", view)
	}
}

func TestTUIFilterOpensEffectiveDefaultFields(t *testing.T) {
	m := tuiModel{filter: config.DefaultFilter, filterFormExplicit: true}
	m.openFilterEditor()
	if m.filterDraft.quality != "75" || !m.filterDraft.hasQP || m.filterDraft.availability != "paid" {
		t.Fatalf("effective default filter draft = %+v, want quality 75, has Q/P and paid", m.filterDraft)
	}
	view := tuiFilterView(tuiModel{width: 100, height: 20, overlay: "filter", filterDraft: m.filterDraft})
	if !strings.Contains(view, "Quality minimum: 75") || !strings.Contains(view, "Has Q/P: [x]") || !strings.Contains(view, "Availability: paid") {
		t.Fatalf("effective default filter view = %q", view)
	}
}

func TestTUIFilterKeepsExplicitDefaultFilterInForm(t *testing.T) {
	m := tuiModel{filter: config.DefaultFilter, filterFormExplicit: true}
	m.openFilterEditor()
	if m.filterDraft.quality != "75" {
		t.Fatalf("explicit default filter draft = %+v, want quality 75", m.filterDraft)
	}
}

func TestTUIRefreshUpdatesFilterFormExplicitness(t *testing.T) {
	m := tuiModel{filter: config.DefaultFilter, generation: 1}
	next, _ := m.Update(tuiRefreshMsg{generation: 1, filter: config.DefaultFilter, filterFormExplicit: true})
	m = next.(tuiModel)
	m.openFilterEditor()
	if m.filterDraft.quality != "75" {
		t.Fatalf("reloaded explicit default draft = %+v, want quality 75", m.filterDraft)
	}
	next, _ = m.Update(tuiRefreshMsg{generation: 1, filter: config.DefaultFilter, filterFormExplicit: true})
	m = next.(tuiModel)
	m.openFilterEditor()
	if m.filterDraft.quality != "75" {
		t.Fatalf("reloaded effective default draft = %+v, want quality 75", m.filterDraft)
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

func TestTUIInteractiveFilterPersistsAndClears(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "paid", InPerM: 1}, {Slug: "free", Free: true}})
	m.configPath = configPath
	m.inputMode, m.input = "filter", "paid"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUIFilter != "paid" || len(m.visible) != 1 || m.visible[0].Slug != "paid" {
		t.Fatalf("saved filter = %q, visible = %+v", cfg.TUIFilter, m.visible)
	}
	m.inputMode, m.input = "filter", ""
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	cfg, err = config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUIFilterSet || cfg.TUIFilter != "" || len(m.visible) != 2 {
		t.Fatalf("cleared filter = %q, visible = %+v", cfg.TUIFilter, m.visible)
	}
}

func TestTUISearchPersistsAcrossRebuildAndRefreshUntilCleared(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "alpha", DisplayName: "Alpha"}, {Slug: "beta", DisplayName: "Beta"}})
	m.width, m.height = 120, 12
	m.inputMode, m.input = "search", "Beta"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.search != "Beta" || len(m.visible) != 1 || m.visible[0].Slug != "beta" {
		t.Fatalf("search state = %q, visible = %+v", m.search, m.visible)
	}
	if !strings.Contains(m.View(), `search:"Beta" (1 matches)`) {
		t.Fatalf("active search context missing from view: %s", m.View())
	}
	m.generation = 1
	next, _ := m.Update(tuiRefreshMsg{generation: 1, models: m.models, filter: m.filter})
	m = next.(tuiModel)
	if m.search != "Beta" || len(m.visible) != 1 || m.visible[0].Slug != "beta" {
		t.Fatalf("refresh lost search state = %q, visible = %+v", m.search, m.visible)
	}
	m.inputMode, m.input = "search", ""
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.search != "" || len(m.visible) != 2 {
		t.Fatalf("empty search did not clear: search=%q visible=%+v", m.search, m.visible)
	}
}

func TestTUISearchEscapeKeepsAppliedQuery(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "beta", DisplayName: "Beta"}})
	m.search, m.inputMode, m.input = "Beta", "search", "Other"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEscape})
	if m.search != "Beta" || m.inputMode != "" || len(m.visible) != 1 {
		t.Fatalf("search cancel changed applied state: search=%q mode=%q visible=%+v", m.search, m.inputMode, m.visible)
	}
}

func TestTUIDetailPrioritizesFitPricingAndBenchmarks(t *testing.T) {
	lines := tuiDetailLines(model.Model{DisplayName: "Model", Slug: "vendor/model", Description: "long description", TaskFit: []string{"implement"}, InPerM: 1, OutPerM: 2, Score: &model.ScoreInfo{Value: 90}, ScoreLabel: "90%"}, scoreSourceSWEBench, 100, time.Unix(0, 0))
	joined := strings.Join(lines, "\n")
	if strings.Index(joined, "Task fit:") > strings.Index(joined, "-- Pricing --") || strings.Index(joined, "-- Pricing --") > strings.Index(joined, "-- Benchmarks --") {
		t.Fatalf("detail priority order is wrong:\n%s", joined)
	}
	if strings.Index(joined, "Описание:") < strings.Index(joined, "-- Benchmarks --") {
		t.Fatalf("description appears before benchmarks:\n%s", joined)
	}
}

func tuiKeyCmd(m tuiModel, key string) (tuiModel, tea.Cmd) {
	return m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func tuiKey(m tuiModel, key string) tuiModel {
	next, _ := tuiKeyCmd(m, key)
	return next
}

func TestTUIMainCloseBindingsQuitThroughKeyEvents(t *testing.T) {
	for _, key := range []string{"esc", "h"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		if key == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEscape}
		}
		if _, cmd := m.Update(msg); cmd == nil {
			t.Fatalf("default main.close binding %q did not return a quit command", key)
		}
	}

	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.keymap["main"]["close"] = config.TUIBindings{"z"}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")}); cmd == nil {
		t.Fatal("custom main.close binding did not return a quit command")
	}
}

// TestTUILeftArrowDoesNotQuitTheBareMainView is a regression test: Left arrow
// used to share the "esc"/"h" close-overlay-or-quit binding in the main
// context, so pressing it with no overlay open quit the whole TUI instead of
// doing nothing. Only "esc" and "h" should still quit from the bare main
// view; Left arrow must be a no-op there.
func TestTUILeftArrowDoesNotQuitTheBareMainView(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}, {Slug: "b"}})
	cursorBefore, overlayBefore := m.cursor, m.overlay
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd != nil {
		t.Fatal("Left arrow in the bare main view returned a command (expected a no-op)")
	}
	got := next.(tuiModel)
	if got.overlay != overlayBefore {
		t.Fatalf("Left arrow in the bare main view changed overlay: %q -> %q", overlayBefore, got.overlay)
	}
	if got.cursor != cursorBefore {
		t.Fatalf("Left arrow in the bare main view moved the cursor: %d -> %d", cursorBefore, got.cursor)
	}

	// A custom main.close binding may still opt Left back in explicitly.
	m = newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.keymap["main"]["close"] = config.TUIBindings{"left"}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft}); cmd == nil {
		t.Fatal("custom main.close binding on left did not return a quit command")
	}
}

func TestTUIXQuitsMainWindow(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil {
		t.Fatal("x in the main window did not return a quit command")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Fatalf("x in the main window returned %T, want tea.QuitMsg", msg)
	}
}

// TestTUIXDoesNotInterceptActiveTextInput is a regression test: x used to be
// checked before m.inputMode, so it quit the app (search, overlay == "") or
// closed the parent overlay (help-search, overlay == "help") instead of
// inserting the literal character — making it impossible to type an "x" into
// a search string (e.g. "x-ai/grok-4.5", a real model-map.tsv slug). Esc
// already cancels an active input correctly (proven by
// TestTUIEscCancelsActiveTextInput below); x must defer to it, not race it.
func TestTUIXDoesNotInterceptActiveTextInput(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.inputMode, m.input = "search", "draft"
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(tuiModel)
	if cmd != nil {
		t.Fatalf("x while typing a search returned a command %v, want nil (no quit)", cmd)
	}
	if m.inputMode != "search" || m.input != "draftx" {
		t.Fatalf("x while typing a search = mode %q input %q, want mode \"search\" input \"draftx\"", m.inputMode, m.input)
	}

	m = newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m = tuiKey(m, "?") // open help, land on the sectioned overlay
	if m.overlay != "help" {
		t.Fatalf("test setup: ? did not open help, overlay=%q", m.overlay)
	}
	m.inputMode, m.input, m.helpSearch = "help-search", "x-ai", ""
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(tuiModel)
	if cmd != nil {
		t.Fatalf("x while typing a help search returned a command %v, want nil", cmd)
	}
	if m.overlay != "help" || m.inputMode != "help-search" || m.input != "x-aix" {
		t.Fatalf("x while typing a help search = overlay %q mode %q input %q, want overlay \"help\" mode \"help-search\" input \"x-aix\"", m.overlay, m.inputMode, m.input)
	}
}

// TestTUIEscCancelsActiveTextInput documents the cancel path x must not race:
// Esc abandons the in-progress draft (and, for help-search, discards it
// outright) without touching the parent overlay.
func TestTUIEscCancelsActiveTextInput(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.inputMode, m.input, m.search = "search", "x-ai", "old"
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(tuiModel)
	if cmd != nil || m.inputMode != "" || m.search != "old" {
		t.Fatalf("esc while typing a search = mode %q search %q cmd %v, want mode \"\" search \"old\" cmd nil", m.inputMode, m.search, cmd)
	}

	m = tuiKey(m, "?")
	m.inputMode, m.input, m.helpSearch = "help-search", "x-ai", ""
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(tuiModel)
	if cmd != nil || m.overlay != "help" || m.inputMode != "" || m.input != "" {
		t.Fatalf("esc while typing a help search = overlay %q mode %q input %q cmd %v, want overlay \"help\" mode \"\" input \"\" cmd nil", m.overlay, m.inputMode, m.input, cmd)
	}
}

// TestTUIXClosesEveryOverlayType proves x closes every overlay with the same
// state cleanup as that overlay's own dedicated close key — not just a blind
// overlay = "". Detail's own close also zeroes detailOffset (its dedicated
// scroll state); this proves x does too, instead of leaving it stale.
func TestTUIXClosesEveryOverlayType(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T) tuiModel
	}{
		{"detail", tuiShortcutDetailModelScrolled},
		{"help", tuiShortcutHelpModelScrolled},
		{"columns", tuiShortcutColumnsModelScrolled},
		{"settings", func(t *testing.T) tuiModel {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
			return tuiKey(m, "o")
		}},
		{"filter", func(t *testing.T) tuiModel {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
			m.openFilterEditor()
			return m
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := test.setup(t)
			viaDedicatedClose := tuiKey(m, "esc")
			if viaDedicatedClose.overlay != "" {
				t.Fatalf("test setup: esc did not close the %s overlay", test.name)
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
			viaX := next.(tuiModel)
			if cmd != nil {
				t.Fatalf("x on the %s overlay returned a command %v, want nil", test.name, cmd)
			}
			if viaX.overlay != "" {
				t.Fatalf("x on the %s overlay left overlay = %q, want \"\"", test.name, viaX.overlay)
			}
			if viaX.detailOffset != viaDedicatedClose.detailOffset {
				t.Fatalf("x on the %s overlay left detailOffset = %d, want %d (matching the dedicated close key)", test.name, viaX.detailOffset, viaDedicatedClose.detailOffset)
			}
		})
	}
}

func TestTUIKeyState(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", DisplayName: "A"}, {Slug: "b", DisplayName: "B"}})
	m = tuiKey(m, "j")
	if m.cursor != 1 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	m = tuiKey(m, "enter")
	if m.overlay != "detail" {
		t.Fatalf("Enter overlay = %q, want detail", m.overlay)
	}
	m = tuiKey(m, "esc")
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
	m, _ = m.columnKey("down", "down")
	m, _ = m.columnKey(" ", " ")
	m, _ = m.columnKey("enter", "enter")
	if !containsColumn(m.columns, tuiColumns[1]) {
		t.Fatal("column was not applied")
	}
	m = tuiKey(m, "c")
	m, _ = m.columnKey(" ", " ")
	m, _ = m.columnKey("esc", "esc")
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
	m = tuiKey(m, "f1")
	if !strings.Contains(m.View(), "omt tui keys") {
		t.Fatal("help overlay missing")
	}
	if !strings.Contains(tuiHelpDocument, `\tq\tsort\tquality.`) || !strings.Contains(tuiHelpDocument, `\tp\tavailability\tcycle any/free/paid.`) || !strings.Contains(tuiHelpDocument, `\tr\tsort\tquality/price ratio`) {
		t.Fatalf("help document is missing sort shortcuts: %q", tuiHelpDocument)
	}
	for _, r := range tuiHelpDocument {
		if unicode.Is(unicode.Cyrillic, r) {
			t.Fatalf("help document mentions Cyrillic keyboard aliases: %q", tuiHelpDocument)
		}
	}
	for _, text := range []string{"Navigation", "Task-fit codes", "switch the last column between Task fit and Note", "task-fit code", "implement: write or change production code.", "plan: define scope, steps, and decisions.", "research: investigate options, evidence, or behavior.", "find and fix a defect or failure", "No task-fit classification is shown as n/a", "Auto-refresh"} {
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
	if m.helpOffset != m.helpMaxOffset() {
		t.Fatalf("help offset exceeded lower bound: got %d, max %d", m.helpOffset, m.helpMaxOffset())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = next.(tuiModel)
	if m.helpOffset != 0 {
		t.Fatalf("help home offset = %d, want 0", m.helpOffset)
	}
	// "quality" rather than "refresh": search is now section-scoped (see
	// TestTUIHelpSearchIsScopedToTheCurrentSection), and this point in the
	// test is still on section 0 ("Overview") — "refresh" only occurs in the
	// Hotkeys section, but "quality" occurs several times right here.
	m = tuiKey(m, "/")
	m.input = "quality"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.inputMode != "" || len(m.helpMatches) == 0 || !strings.Contains(m.View(), "quality") {
		t.Fatalf("help search state = %+v", m)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(tuiModel)
	if m.helpMatch != 1%len(m.helpMatches) {
		t.Fatalf("n did not advance search match: %d", m.helpMatch)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(tuiModel)
	if m.overlay != "" {
		t.Fatal("help did not close")
	}
}

// TestTUIQuestionMarkOpensFullHelpAtHotkeys confirms the retired-shortcuts
// design: ? is now just a faster entry point into the one sectioned F1 help,
// landing on section 2 (Hotkeys) instead of section 0 (Overview) — fully
// navigable and closing the same way from either entry point, per the
// confirmed design (see the ? key handler in tui.go).
func TestTUIQuestionMarkOpensFullHelpAtHotkeys(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m = tuiKey(m, "?")
	if m.overlay != "help" || m.helpSection != 2 {
		t.Fatalf("question-mark help state = overlay %q, section %d", m.overlay, m.helpSection)
	}
	if strings.Contains(m.View(), "Model detail view") || strings.Contains(m.View(), "Score sources") {
		t.Fatalf("question-mark help shows another section's content: %q", m.View())
	}
	for _, want := range []string{
		`\tF1\thelp\topen full help.`,
		`\tEnter / Right / l\tdetail\topen the model detail screen.`,
		`\tSpace\tswitch\t(in Settings) switch between SWE-bench and Arena.`,
		`\tv\tview\ttoggle all/top-paid-free.`,
	} {
		if !strings.Contains(tuiHelpSectionHotkeysBody, want) {
			t.Fatalf("Hotkeys section does not document %q: %q", want, tuiHelpSectionHotkeysBody)
		}
	}
	// ? closes help exactly like Esc does, from the section it opened on.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(tuiModel)
	if m.overlay != "" {
		t.Fatalf("? did not close help: overlay=%q", m.overlay)
	}
	// F1 is unaffected: it still opens on section 0 (Overview).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = next.(tuiModel)
	m.height = len(tuiHelpLines()) + 2
	if m.helpSection != 0 {
		t.Fatalf("F1 did not open full help at the Overview section: section=%d", m.helpSection)
	}
	if !strings.Contains(m.View(), "version "+version) {
		t.Fatalf("full help does not show runtime version %q: %q", version, m.View())
	}
	// Once open, navigation is identical regardless of entry point: digit 5
	// jumps to the model-detail section just as it would from ?-opened help.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	m = next.(tuiModel)
	if m.helpSection != 4 || !strings.Contains(m.View(), "Model detail view") {
		t.Fatalf("digit 5 did not switch to the model-detail section: section=%d view=%q", m.helpSection, m.View())
	}
	// Digit 6 jumps to the sixth section, Methodology. The check asserts a
	// body-only phrase, not the bare word "Methodology" — that word is also
	// in the tab bar on every section, so it would not catch a broken jump.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	m = next.(tuiModel)
	if m.helpSection != 5 || !strings.Contains(m.View(), "how the table and ranking are built") {
		t.Fatalf("digit 6 did not switch to the methodology section: section=%d view=%q", m.helpSection, m.View())
	}
}

// tuiHelpDocumentLineIndex returns the index of the first line equal to
// want in lines, or -1 if there is none. Used to locate structural
// anchors (the title, the Hotkeys heading) without hard-coding positions.
func tuiHelpDocumentLineIndex(lines []string, want string) int {
	for i, line := range lines {
		if line == want {
			return i
		}
	}
	return -1
}

func TestTUIFullHelpDescribesTheToolBeforeHotkeys(t *testing.T) {
	lines := tuiHelpLines()
	if len(lines) == 0 || lines[0] != "omt tui keys" {
		t.Fatalf("full help title missing or changed: %q", lines)
	}
	hotkeysIndex := tuiHelpDocumentLineIndex(lines, "Hotkeys")
	if hotkeysIndex < 0 {
		t.Fatalf("full help missing Hotkeys heading: %q", lines)
	}
	description := strings.Join(lines[1:hotkeysIndex], " ")
	if strings.TrimSpace(description) == "" {
		t.Fatalf("full help has no description of the tool before Hotkeys: %q", lines)
	}
	for _, want := range []string{"OpenRouter", "quality", "price"} {
		if !strings.Contains(description, want) {
			t.Fatalf("full help description before Hotkeys is missing %q: %q", want, description)
		}
	}
}

func TestTUIHelpSearchNextPreviousWrapsAndDoesNotCaptureInput(t *testing.T) {
	// helpSection 2 (Hotkeys) is where "search" occurs repeatedly
	// ("search Name/Slug", the "Help search" table, ...); search is
	// section-scoped, and the default section 0 ("Overview") does not contain
	// the word at all.
	m := tuiModel{overlay: "help", helpSection: 2, width: 100, height: 10, helpSearch: "search"}
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	m.helpMatch = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if m.helpMatch != 0 {
		t.Fatalf("Enter changed search match in browse mode: %d", m.helpMatch)
	}
	m = tuiKey(m, "n")
	if m.helpMatch != 1%len(m.helpMatches) {
		t.Fatalf("n selected match %d, want %d", m.helpMatch, 1%len(m.helpMatches))
	}
	m = tuiKey(m, "N")
	if m.helpMatch != 0 {
		t.Fatalf("N selected match %d, want 0", m.helpMatch)
	}
	m = tuiKey(m, "N")
	if m.helpMatch != len(m.helpMatches)-1 {
		t.Fatalf("N did not wrap to previous match: %d", m.helpMatch)
	}
	m.helpMatch = len(m.helpMatches) - 1
	m = tuiKey(m, "n")
	if m.helpMatch != 0 {
		t.Fatalf("n did not wrap to first match: %d", m.helpMatch)
	}
	m.inputMode, m.input = "help-search", "search"
	m = tuiKey(m, "n")
	if m.input != "searchn" || m.helpMatch != 0 {
		t.Fatalf("n was captured during search input: input=%q match=%d", m.input, m.helpMatch)
	}
	m = tuiKey(m, "N")
	if m.input != "searchnN" || m.helpMatch != 0 {
		t.Fatalf("N was captured during search input: input=%q match=%d", m.input, m.helpMatch)
	}
}

func TestTUIHelpSearchInputDoesNotNavigateWithUpDown(t *testing.T) {
	// helpSection 2 (Hotkeys) actually contains "search" — Enter's
	// handler recomputes helpMatches against the current section, and an
	// empty result (e.g. from the default section 0) would leave helpMatch
	// at -1 instead of the 0 this test expects.
	m := tuiModel{overlay: "help", helpSection: 2, inputMode: "help-search", input: "search", helpSearch: "search", helpMatches: []int{1, 2}, helpMatch: 0}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyUp}, {Type: tea.KeyDown}} {
		next, _ := m.Update(key)
		m = next.(tuiModel)
		if m.helpMatch != 0 || m.input != "search" {
			t.Fatalf("%s changed search state during input: match=%d input=%q", key.String(), m.helpMatch, m.input)
		}
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if m.inputMode != "" || m.helpSearch != "search" || m.helpMatch != 0 {
		t.Fatalf("Enter did not confirm help search: mode=%q search=%q match=%d", m.inputMode, m.helpSearch, m.helpMatch)
	}
}

func TestTUIHelpFooterShowsMatchPositionAndInputHint(t *testing.T) {
	m := tuiModel{overlay: "help", width: 200, height: len(tuiHelpLines()) + 2, helpSearch: "search", helpMatches: []int{1, 2, 3}, helpMatch: 2}
	view := tuiHelpView(m)
	lines := strings.Split(view, "\n")
	footer := tuiHelpFooterLine(lines)
	if !strings.Contains(ansi.Strip(footer), "3/3 matches") || !strings.Contains(ansi.Strip(footer), "n next match · N previous match") {
		t.Fatalf("completed-search footer = %q", footer)
	}
	m.inputMode, m.input = "help-search", "search"
	lines = strings.Split(tuiHelpView(m), "\n")
	footer = tuiHelpFooterLine(lines)
	if !strings.Contains(ansi.Strip(footer), "Enter confirm search · Esc cancel") || strings.Contains(ansi.Strip(footer), "n next") {
		t.Fatalf("input-mode footer = %q", footer)
	}
	m.helpMatches, m.helpMatch = nil, -1
	m.inputMode = ""
	lines = strings.Split(tuiHelpView(m), "\n")
	footer = tuiHelpFooterLine(lines)
	if !strings.Contains(ansi.Strip(footer), "0 matches") {
		t.Fatalf("zero-match footer = %q", footer)
	}
}

func tuiHelpFooterLine(lines []string) string {
	for _, line := range lines {
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "Help ") && strings.Contains(plain, "/") {
			return line
		}
	}
	return ""
}

func TestTUIHelpSearchCounterZeroAndCurrentStyle(t *testing.T) {
	tuiForceColorProfile(t)
	// helpSection 2 (Hotkeys) has several "column" occurrences —
	// search is section-scoped, and the default section 0 has none.
	m := tuiModel{overlay: "help", helpSection: 2, width: 200, height: len(tuiHelpLines()) + 2, helpSearch: "column"}
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	m.helpMatch = 0
	view := tuiHelpView(m)
	if !strings.Contains(ansi.Strip(view), fmt.Sprintf("%d matches", len(m.helpMatches))) {
		t.Fatalf("help counter missing: %q", view)
	}
	if !strings.Contains(view, tuiMatchStyle.Render("column")) || !strings.Contains(view, tuiCurrentMatchStyle.Render("column")) {
		t.Fatalf("help matches do not use ordinary and current styles: %q", view)
	}
	m.helpSearch = "no-such-help-term"
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	m.helpMatch = -1
	if !strings.Contains(ansi.Strip(tuiHelpView(m)), "0 matches") {
		t.Fatal("help does not report zero matches")
	}
}

func TestTUIMainSpaceSwitchesSourceThroughUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/model\ttier=sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/model:\n    display: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{"demo/model": {Score: &model.ScoreInfo{Value: 80}, ArenaScore: &model.ScoreInfo{Value: 1400}, InPerM: 1, OutPerM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-snapshot.json"), snapshot, 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), root, refresh.Options{}, 0, []model.Model{{Slug: "demo/model"}})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(tuiModel)
	if !m.scoreSourceLoading || m.pendingScoreSource != scoreSourceArena || !strings.Contains(m.status, "loading") || cmd == nil {
		t.Fatalf("main Space pending state = loading %v, pending %q, status %q, cmd %v", m.scoreSourceLoading, m.pendingScoreSource, m.status, cmd)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if next.(tuiModel).scoreSourceGeneration != m.scoreSourceGeneration {
		t.Fatal("main Space started a second switch while loading")
	}
	next, _ = m.Update(cmd())
	m = next.(tuiModel)
	if m.scoreSource != scoreSourceArena || m.scoreSourceLoading || m.pendingScoreSource != "" || m.status != "score source changed" {
		t.Fatalf("main Space success state = source %q, loading %v, pending %q, status %q", m.scoreSource, m.scoreSourceLoading, m.pendingScoreSource, m.status)
	}
	m = newTUIModel(context.Background(), root, refresh.Options{}, 0, []model.Model{{Slug: "demo/model"}})
	m.keymap["main"]["switch_source"] = config.TUIBindings{"z"}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if next.(tuiModel).scoreSourceLoading || cmd != nil {
		t.Fatal("default Space ignored custom main.switch_source")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if !next.(tuiModel).scoreSourceLoading || cmd == nil {
		t.Fatal("custom main.switch_source did not start source switch")
	}
}

func TestTUIHelpFullHelpUsesConfiguredBinding(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.keymap["main"]["full_help"] = config.TUIBindings{"z"}
	m.keymap["help"]["full_help"] = config.TUIBindings{"y"}
	m = tuiKey(m, "z")
	if m.overlay != "help" || m.helpSection != 0 {
		t.Fatalf("custom main full-help binding state = overlay %q, section %d", m.overlay, m.helpSection)
	}
	m = tuiKey(m, "3")
	if m.helpSection != 2 {
		t.Fatalf("test setup: digit 3 did not switch section: %d", m.helpSection)
	}
	m = tuiKey(m, "y")
	if m.helpSection != 0 {
		t.Fatalf("custom help full-help binding did not reset to the Overview section: %d", m.helpSection)
	}
}

func TestTUIHelpRendersConfiguredBindingsByActionGroup(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	for context, actions := range m.keymap {
		for action := range actions {
			m.keymap[context][action] = config.TUIBindings{"custom-" + context + "-" + action}
		}
	}
	// Every binding checked below lives in the Hotkeys section (index 2) of
	// the sectioned full-help document.
	m.helpSection = 2
	view := strings.Join(m.helpLines(), "\n")
	wants := []string{
		"custom-main-navigate_up", "custom-main-navigate_down", "custom-main-open_details", "custom-main-close",
		"custom-settings-navigate_up", "custom-settings-navigate_down", "custom-settings-close", "custom-settings-switch_source",
		"custom-detail-navigate_up", "custom-detail-navigate_down", "custom-detail-close",
		"custom-help-navigate_up", "custom-help-navigate_down", "custom-help-close",
		"custom-columns-navigate_up", "custom-columns-navigate_down", "custom-columns-close", "custom-columns-toggle", "custom-columns-apply",
		"custom-filter-navigate_up", "custom-filter-navigate_down", "custom-filter-close", "custom-filter-toggle", "custom-filter-apply",
		"custom-help-close", "custom-main-help", "custom-main-full_help", "custom-settings-switch_source",
	}
	for _, want := range wants {
		if !strings.Contains(view, want) {
			t.Fatalf("help is missing configured binding %q: %q", want, view)
		}
	}
}

func TestTUICommandKeySupportsRussianLayout(t *testing.T) {
	for russian, english := range map[string]string{"й": "q", "с": "c", "р": "h", "П": "G"} {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(russian)}
		if got := tuiCommandKey(msg); got != english {
			t.Errorf("tuiCommandKey(%q) = %q, want %q", russian, got, english)
		}
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}, {Slug: "b"}})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("й")})
	if next.(tuiModel).sortKey != "quality" {
		t.Fatalf("Russian-layout q shortcut did not work: sort=%q", next.(tuiModel).sortKey)
	}
}

func TestTUISettingsOverlayTransitions(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.filter = "paid"
	m = tuiKey(m, "o")
	if m.overlay != "settings" {
		t.Fatal("settings overlay not opened")
	}
	if view := m.View(); !strings.Contains(view, "Score source: swebench (Space switches SWE-bench/Arena)") || !strings.Contains(view, "Move Down to Score source, then press Space to switch.") || !strings.Contains(view, "Filter: paid") || !strings.Contains(view, "Columns:") {
		t.Fatalf("settings view is missing state: %q", view)
	}
	m = tuiKey(m, "down")
	m, _ = m.settingsKey(" ", " ")
	if m.scoreSource != scoreSourceDefault {
		t.Fatalf("score source changed before local snapshot loaded: %q", m.scoreSource)
	}
	m = tuiKey(m, "down")
	m, _ = m.settingsKey("enter", "enter")
	if m.overlay != "filter" || m.inputMode != "" {
		t.Fatalf("settings filter transition = overlay %q, input mode %q", m.overlay, m.inputMode)
	}
	m.filterDraft = tuiFilterDraft{free: true}
	m, _ = m.filterKey("enter", tea.KeyMsg{Type: tea.KeyEnter})
	if m.filter != "free" || m.overlay != "" || m.inputMode != "" {
		t.Fatalf("settings filter result = filter %q, overlay %q, input mode %q", m.filter, m.overlay, m.inputMode)
	}
	m, _ = m.settingsKey("esc", "esc")
	if m.overlay != "" {
		t.Fatal("settings overlay did not close")
	}
}

func TestTUISettingsAvailabilityRowCyclesAvailabilityInPlace(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.overlay, m.settingsCursor, m.filter = "settings", 3, ""
	m, _ = m.settingsKey(" ", " ")
	if m.overlay == "columns" {
		t.Fatalf("settings cursor 3 (Availability row) opened the columns overlay instead of cycling availability")
	}
	if got := tuiAvailabilityFromFilter(m.filter); got != "free" {
		t.Fatalf("availability after Space on settings cursor 3 = %q, want free", got)
	}
}

func TestTUISettingsAvailabilityRowRightArrowCyclesAvailability(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.overlay, m.settingsCursor, m.filter = "settings", 3, ""
	m, _ = m.settingsKey("right", "right")
	if got := tuiAvailabilityFromFilter(m.filter); got != "free" {
		t.Fatalf("availability after Right on settings cursor 3 = %q, want free", got)
	}
}

func TestTUISettingsAvailabilityRowLeftArrowCyclesBackwardFromPaid(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.overlay, m.settingsCursor, m.filter = "settings", 3, "availability:paid"
	m, _ = m.settingsKey("left", "left")
	if got := tuiAvailabilityFromFilter(m.filter); got != "free" {
		t.Fatalf("availability after Left on settings cursor 3 starting from paid = %q, want free", got)
	}
}

func TestTUISettingsColumnsRowOpensColumnsOverlay(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.overlay, m.settingsCursor, m.topN = "settings", 5, 3
	m, _ = m.settingsKey(" ", " ")
	if m.overlay != "columns" {
		t.Fatalf("settings cursor 5 (Columns row) overlay = %q, want columns", m.overlay)
	}
	if m.topN != 3 {
		t.Fatalf("settings cursor 5 (Columns row) changed topN to %d, want unchanged 3", m.topN)
	}
}

func TestTUIScoreSourceSwitchThroughUpdateShowsPendingSuccessAndError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/model\ttier=sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/model:\n    display: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{"demo/model": {Score: &model.ScoreInfo{Value: 80}, ArenaScore: &model.ScoreInfo{Value: 1400}, InPerM: 1, OutPerM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-snapshot.json"), snapshot, 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), root, refresh.Options{}, 0, []model.Model{{Slug: "demo/model"}})
	m.overlay, m.settingsCursor, m.width, m.height = "settings", 1, 100, 30
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(tuiModel)
	if !m.scoreSourceLoading || m.scoreSource != scoreSourceDefault || !strings.Contains(ansi.Strip(m.View()), "loading arena") {
		t.Fatalf("pending source state = loading %v, source %q, view %q", m.scoreSourceLoading, m.scoreSource, ansi.Strip(m.View()))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if next.(tuiModel).scoreSourceLoading == false {
		t.Fatal("repeated source switch was not blocked while loading")
	}
	next, _ = m.Update(cmd())
	m = next.(tuiModel)
	if m.scoreSource != scoreSourceArena || m.scoreSourceLoading || !strings.Contains(m.status, "changed") {
		t.Fatalf("success source state = source %q, loading %v, status %q", m.scoreSource, m.scoreSourceLoading, m.status)
	}
	m.dataDir = t.TempDir()
	m.scoreSourceGeneration++
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(tuiModel)
	next, _ = m.Update(cmd())
	m = next.(tuiModel)
	if m.scoreSource != scoreSourceArena || m.scoreSourceLoading || m.err == "" || !strings.Contains(ansi.Strip(m.View()), "Error:") {
		t.Fatalf("error source state = source %q, loading %v, err %q, view %q", m.scoreSource, m.scoreSourceLoading, m.err, ansi.Strip(m.View()))
	}
}

func TestTUIFilterFormOpensAppliesAndPersistsStructuredFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []model.Model{
		{Slug: "match", Tier: "sonnet", Context: 128000, InPerM: 1, OutPerM: 2, Score: &model.ScoreInfo{Value: 90}, Rankable: true},
		{Slug: "other", Tier: "opus", Context: 32000, InPerM: 3, OutPerM: 4, Score: &model.ScoreInfo{Value: 70}, Rankable: true},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.configPath = configPath
	m.filter = "paid,tier:sonnet,quality>=90,context>=100000,input<=1,output<=2"
	m = tuiKey(m, "f")
	if m.overlay != "filter" || m.filterDraft.tier != "sonnet" || m.filterDraft.quality != "90" {
		t.Fatalf("filter form open state = overlay %q, draft %+v", m.overlay, m.filterDraft)
	}
	if !m.filterDraft.paid || m.filterDraft.free || m.filterDraft.scored {
		t.Fatalf("filter form boolean state = %+v", m.filterDraft)
	}
	m.filterDraft.free = false
	m.filterDraft.paid = true
	m.filterDraft.scored = true
	m, _ = m.filterKey("enter", tea.KeyMsg{Type: tea.KeyEnter})
	want := "paid,scored,tier:sonnet,quality>=90,context>=100000,input<=1,output<=2"
	want = "paid,scored,tier:sonnet,quality>=90,context>=100000,input<=1.00,output<=2.00"
	if m.overlay != "" || m.filter != want || len(m.visible) != 1 || m.visible[0].Slug != "match" {
		t.Fatalf("applied filter = overlay %q, filter %q, visible %+v", m.overlay, m.filter, m.visible)
	}
	if m.status != "filter: "+want || !strings.Contains(m.View(), "filter: "+want) {
		t.Fatalf("applied filter status = %q, view=%q", m.status, m.View())
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUIFilter != want {
		t.Fatalf("persisted filter = %q, want %q", cfg.TUIFilter, want)
	}
}

func TestTUIFilterTierSelectChangesThroughUpdateAndPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []model.Model{{Slug: "sonnet", Tier: "sonnet"}, {Slug: "haiku", Tier: "haiku"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.configPath = configPath
	m.filter = "tier:sonnet"
	m = tuiKey(m, "f")
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(tuiModel)
	}
	if m.filterCursor != 3 || m.filterDraft.tier != "sonnet" {
		t.Fatalf("tier navigation state = cursor %d, tier %q, want cursor 3 and sonnet", m.filterCursor, m.filterDraft.tier)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(tuiModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	want := "tier:haiku"
	if m.filter != want || len(m.visible) != 1 || m.visible[0].Slug != "haiku" {
		t.Fatalf("applied tier filter = %q, visible %+v, want %q and haiku", m.filter, m.visible, want)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUIFilter != want {
		t.Fatalf("persisted tier filter = %q, want %q", cfg.TUIFilter, want)
	}
}

func TestTUIFilterTierSelectArrowsThroughUpdateAndPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []model.Model{{Slug: "sonnet", Tier: "sonnet"}, {Slug: "haiku", Tier: "haiku"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.configPath = configPath
	m.filter = "tier:sonnet"
	m = tuiKey(m, "f")
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(tuiModel)
	}
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyRight}, {Type: tea.KeyLeft}, {Type: tea.KeyRight}} {
		next, _ := m.Update(msg)
		m = next.(tuiModel)
	}
	if m.filterCursor != 3 || m.filterDraft.tier != "haiku" {
		t.Fatalf("tier arrow state = cursor %d, tier %q, want cursor 3 and haiku", m.filterCursor, m.filterDraft.tier)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if m.filter != "tier:haiku" || len(m.visible) != 1 || m.visible[0].Slug != "haiku" {
		t.Fatalf("applied arrow tier filter = %q, visible %+v", m.filter, m.visible)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUIFilter != "tier:haiku" {
		t.Fatalf("persisted arrow tier filter = %q, want tier:haiku", cfg.TUIFilter)
	}
}

func TestTUIFilterFormCancelAndClear(t *testing.T) {
	rows := []model.Model{{Slug: "paid", InPerM: 1}, {Slug: "free", Free: true}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.filter = "paid"
	m = tuiKey(m, "f")
	m.filterDraft.paid = false
	m, _ = m.filterKey("esc", tea.KeyMsg{Type: tea.KeyEscape})
	if m.filter != "paid" || m.overlay != "" {
		t.Fatalf("cancel changed filter: filter=%q overlay=%q", m.filter, m.overlay)
	}
	m = tuiKey(m, "f")
	m, _ = m.filterKey("c", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m, _ = m.filterKey("enter", tea.KeyMsg{Type: tea.KeyEnter})
	if m.filter != "" || len(m.visible) != 2 {
		t.Fatalf("clear result = filter %q, visible %+v", m.filter, m.visible)
	}
	if m.status != "filter: none (cleared)" || !strings.Contains(m.View(), "filter: none (cleared)") {
		t.Fatalf("cleared filter status = %q, view=%q", m.status, m.View())
	}
}

func TestTUIFilterClearCommandWorksFromCheckboxAndTextFields(t *testing.T) {
	rows := []model.Model{{Slug: "paid", InPerM: 1}, {Slug: "free", Free: true}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.filter = "paid,output<=2"
	m = tuiKey(m, "f")
	m.filterDraft.scored = true
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if got := next.(tuiModel).filterDraft; got != (tuiFilterDraft{}) {
		t.Fatalf("checkbox c did not clear draft: %+v", got)
	}
	m = next.(tuiModel)
	m.filterDraft.output = "12"
	m.filterCursor = 7
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("с")})
	if got := next.(tuiModel).filterDraft; got != (tuiFilterDraft{}) {
		t.Fatalf("Russian-layout c did not clear draft from text field: %+v", got)
	}
}

func TestTUIFilterTextFieldCanBeEditedAfterClear(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.filter = "output<=2"
	m = tuiKey(m, "f")
	m.filterCursor = 7
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(tuiModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(tuiModel)
	if got := m.filterDraft.output; got != "3" {
		t.Fatalf("field edit after clear = %q, want 3", got)
	}
	m, _ = m.filterKey("enter", tea.KeyMsg{Type: tea.KeyEnter})
	if m.filter != "output<=3.00" {
		t.Fatalf("applied filter after clear/edit = %q, want output<=3.00", m.filter)
	}
}

func TestTUIFilterDraftStructuredConversion(t *testing.T) {
	draft := tuiFilterDraftFromString("free,tier:haiku,quality>=85,context>=64000,input<=0.5,output<=1.2")
	if got := draft.string(); got != "free,tier:haiku,quality>=85,context>=64000,input<=0.50,output<=1.20" {
		t.Fatalf("draft conversion = %q", got)
	}
}

func TestTUIFilterDraftCanonicalizesQualityAndPrices(t *testing.T) {
	draft := tuiFilterDraftFromString("quality>=0.8,context>=100000.9,input<=0.88,output<=1")
	if got := draft.string(); got != "quality>=80,context>=100001,input<=0.88,output<=1.00" {
		t.Fatalf("canonical draft = %q", got)
	}
	view := tuiFilterView(tuiModel{width: 100, height: 20, overlay: "filter", filterDraft: draft})
	for _, want := range []string{"Quality minimum: 80", "Context minimum: 100001", "Input max: 0.88", "Output max: 1.00"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestTUIFilterDraftTreatsZeroContextAndPricesAsAny(t *testing.T) {
	draft := tuiFilterDraftFromString("tier:opus,quality>=75,context>=0,input<=0.00,output<=0")
	if draft.tier != "opus" || draft.quality != "75" || draft.context != "" || draft.input != "" || draft.output != "" {
		t.Fatalf("zero predicates draft = %+v, want tier opus, quality 75, other numeric fields unset", draft)
	}
	if got := draft.string(); got != "tier:opus,quality>=75" {
		t.Fatalf("zero predicates serialization = %q, want tier:opus,quality>=75", got)
	}
}

func TestTUIFilterDraftPreservesExplicitQualityZero(t *testing.T) {
	draft := tuiFilterDraftFromString("quality>=0")
	if got := draft.string(); got != "quality>=0" {
		t.Fatalf("quality zero serialization = %q, want quality>=0", got)
	}
}

func TestTUIFilterTierSelectCyclesWhitelistAndClear(t *testing.T) {
	if got, want := tuiFilterTierValues(), append([]string{""}, tier.Values()...); !reflect.DeepEqual(got, want) {
		t.Fatalf("tier select values = %v, want %v", got, want)
	}
	m := tuiModel{overlay: "filter", filterCursor: 3}
	m, _ = m.filterKey(" ", tea.KeyMsg{Type: tea.KeySpace})
	if m.filterDraft.tier != "opus" {
		t.Fatalf("first tier selection = %q, want opus", m.filterDraft.tier)
	}
	for _, want := range []string{"sonnet", "haiku", "free", ""} {
		m, _ = m.filterKey(" ", tea.KeyMsg{Type: tea.KeySpace})
		if m.filterDraft.tier != want {
			t.Fatalf("next tier selection = %q, want %q", m.filterDraft.tier, want)
		}
	}
	m.filterDraft.tier = "sonnet"
	m, _ = m.filterKey("c", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if m.filterDraft.tier != "" {
		t.Fatalf("cleared tier = %q, want empty", m.filterDraft.tier)
	}
}

func TestTUIFilterTierSelectBuildsExistingFilterSyntax(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 3, filterDraft: tuiFilterDraft{quality: "90"}}
	m, _ = m.filterKey(" ", tea.KeyMsg{Type: tea.KeySpace})
	m, _ = m.filterKey("tab", tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.filterKey(" ", tea.KeyMsg{Type: tea.KeySpace})
	if got, want := m.filterDraft.string(), "tier:opus,quality>=90"; got != want {
		t.Fatalf("formed TUI filter = %q, want %q", got, want)
	}
}

func TestTUIFilterAvailabilityRightArrowCyclesForward(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 9, filterDraft: tuiFilterDraft{availability: "paid"}}
	m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
	if m.filterDraft.availability != "" {
		t.Fatalf("availability after Right from paid = %q, want \"\" (wraps around)", m.filterDraft.availability)
	}
}

func TestTUIFilterAvailabilityLeftArrowCyclesBackward(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 9, filterDraft: tuiFilterDraft{availability: "paid"}}
	m, _ = m.filterKey("left", tea.KeyMsg{Type: tea.KeyLeft})
	if m.filterDraft.availability != "free" {
		t.Fatalf("availability after Left from paid = %q, want free", m.filterDraft.availability)
	}
}

func TestTUIFilterNumericArrowsUseDefaultsAndSteps(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 4}
	for _, test := range []struct {
		field int
		want  string
	}{
		{4, "5"}, {5, "8192"}, {6, "0.05"}, {7, "0.05"},
	} {
		m.filterCursor = test.field
		m.filterDraft = tuiFilterDraft{}
		m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
		if got := []string{m.filterDraft.quality, m.filterDraft.context, m.filterDraft.input, m.filterDraft.output}[test.field-4]; got != test.want {
			t.Fatalf("empty field %d first right step = %q, want %q", test.field, got, test.want)
		}
	}
	m.filterDraft = tuiFilterDraft{quality: "95", context: "100000", input: "1", output: "2"}
	for field := 4; field <= 7; field++ {
		m.filterCursor = field
		m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
	}
	if m.filterDraft.quality != "100" || m.filterDraft.context != "108192" || m.filterDraft.input != "1.05" || m.filterDraft.output != "2.05" {
		t.Fatalf("right steps = %+v", m.filterDraft)
	}
}

func TestTUIFilterNumericArrowsMakeProgressAcrossRepeatedSteps(t *testing.T) {
	for _, test := range []struct {
		name      string
		field     int
		start     string
		wantRight string
		wantLeft  string
	}{
		{"context", 5, "5", "24581", ""},
		{"input", 6, "1", "1.15", "0.85"},
		{"output", 7, "1", "1.15", "0.85"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := tuiModel{overlay: "filter", filterCursor: test.field, filterDraft: tuiFilterDraft{context: test.start, input: test.start, output: test.start}}
			for i := 0; i < 3; i++ {
				m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
			}
			got := []string{m.filterDraft.context, m.filterDraft.input, m.filterDraft.output}[test.field-5]
			if got != test.wantRight {
				t.Fatalf("three right steps = %q, want %q", got, test.wantRight)
			}
			m.filterDraft = tuiFilterDraft{context: test.start, input: test.start, output: test.start}
			for i := 0; i < 3; i++ {
				m, _ = m.filterKey("left", tea.KeyMsg{Type: tea.KeyLeft})
			}
			got = []string{m.filterDraft.context, m.filterDraft.input, m.filterDraft.output}[test.field-5]
			if got != test.wantLeft {
				t.Fatalf("three left steps = %q, want %q", got, test.wantLeft)
			}
		})
	}
}

func TestTUIFilterDisplayRoundsPricesWithoutChangingFilterSyntax(t *testing.T) {
	m := tuiModel{width: 100, height: 20, overlay: "filter", filterDraft: tuiFilterDraft{context: "100000.999", input: "1.23456789", output: "2.999999"}}
	view := tuiFilterView(m)
	for _, want := range []string{"Context minimum: 100001", "Input max: 1", "Output max: 3"} {
		if !strings.Contains(view, want) {
			t.Fatalf("filter view does not contain %q: %q", want, view)
		}
	}
	if got := m.filterDraft.string(); got != "context>=100001,input<=1.23,output<=3.00" {
		t.Fatalf("display formatting changed filter syntax: %q", got)
	}
}

func TestTUIFilterNumericStepsUseConfiguredValues(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 6, filterDraft: tuiFilterDraft{input: "0.1"}, filterSteps: config.TUISteps{QualityPoints: 1, ContextTokens: 1, InputCents: 10, OutputCents: 20}}
	m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
	if m.filterDraft.input != "0.20" {
		t.Fatalf("configured input step = %q, want 0.20", m.filterDraft.input)
	}
}

func TestTUIFilterNumericStepsUseConfiguredValuesWithProgress(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 5, filterDraft: tuiFilterDraft{context: "5"}, filterSteps: config.TUISteps{ContextTokens: 1, InputCents: 1, OutputCents: 1}}
	for i := 0; i < 2; i++ {
		m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
	}
	if m.filterDraft.context != "7" {
		t.Fatalf("configured context right steps = %q, want 7", m.filterDraft.context)
	}
	m.filterCursor = 6
	m.filterDraft.input = "5"
	m, _ = m.filterKey("left", tea.KeyMsg{Type: tea.KeyLeft})
	if m.filterDraft.input != "4.99" {
		t.Fatalf("configured input left step = %q, want 4.99", m.filterDraft.input)
	}
}

func TestTUIPriceStepsUseConfiguredCentsAndReachZero(t *testing.T) {
	for _, test := range []struct {
		cents int
		want  int
	}{
		{9, 1}, {99, 1}, {100, 1}, {999, 1}, {1000, 1}, {9999, 1}, {10000, 1},
	} {
		if got := tuiPriceStep(test.cents, 1); got != test.want {
			t.Errorf("tuiPriceStep(%d) = %d, want %d", test.cents, got, test.want)
		}
	}
	m := tuiModel{overlay: "filter", filterCursor: 6, filterDraft: tuiFilterDraft{input: "0.01"}}
	m, _ = m.filterKey("left", tea.KeyMsg{Type: tea.KeyLeft})
	if m.filterDraft.input != "0.00" {
		t.Fatalf("price step crossed zero: %q", m.filterDraft.input)
	}
}

func TestTUIPriceStepsCrossCanonicalBoundaries(t *testing.T) {
	for _, test := range []struct{ start, want string }{{"0.99", "1.00"}, {"9.99", "10.00"}} {
		m := tuiModel{overlay: "filter", filterCursor: 6, filterDraft: tuiFilterDraft{input: test.start}, filterSteps: config.TUISteps{InputCents: 1}}
		m, _ = m.filterKey("right", tea.KeyMsg{Type: tea.KeyRight})
		if m.filterDraft.input != test.want {
			t.Fatalf("price step %s = %q, want %s", test.start, m.filterDraft.input, test.want)
		}
	}
}

func TestTUIRefreshAppliesUpdatedSteps(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.filterSteps = config.DefaultTUISteps()
	next, _ := m.Update(tuiRefreshMsg{generation: 0, scoreSourceGeneration: 0, filterSteps: config.TUISteps{QualityPoints: 2, ContextTokens: 2048, InputCents: 3, OutputCents: 4}})
	if got := next.(tuiModel).filterSteps.InputCents; got != 3 {
		t.Fatalf("refresh did not apply input step: %d", got)
	}
}

func TestTUIFilterNumericArrowsClampAndPersistSyntax(t *testing.T) {
	m := tuiModel{overlay: "filter", filterCursor: 4, filterDraft: tuiFilterDraft{quality: "0", context: "0", input: "0", output: "0"}}
	for field := 4; field <= 7; field++ {
		m.filterCursor = field
		m, _ = m.filterKey("left", tea.KeyMsg{Type: tea.KeyLeft})
	}
	if m.filterDraft != (tuiFilterDraft{}) {
		t.Fatalf("numeric left at zero draft = %+v, want empty fields", m.filterDraft)
	}
	m.filterDraft = tuiFilterDraft{quality: "101", context: "-1", input: "-0.5", output: "-2"}
	m, _ = m.applyFilterDraft()
	if got := m.filter; got != "quality>=100" {
		t.Fatalf("manual negative clamp serialization = %q", got)
	}
}

func TestTUIFilterNumericLeftAtZeroClearsDraftAndRendersAny(t *testing.T) {
	for _, field := range []int{4, 5, 6, 7} {
		m := tuiModel{overlay: "filter", filterCursor: field, filterDraft: tuiFilterDraft{quality: "0", context: "0", input: "0", output: "0"}, width: 100, height: 20}
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = next.(tuiModel)
		if got := []string{m.filterDraft.quality, m.filterDraft.context, m.filterDraft.input, m.filterDraft.output}[field-4]; got != "" {
			t.Fatalf("field %d left at zero draft = %q, want empty", field, got)
		}
		if got := tuiFilterView(m); !strings.Contains(got, []string{"Quality minimum", "Context minimum", "Input max", "Output max"}[field-4]+": (any)") {
			t.Fatalf("field %d left at zero view = %q, want any", field, got)
		}
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = next.(tuiModel)
		if got := []string{m.filterDraft.quality, m.filterDraft.context, m.filterDraft.input, m.filterDraft.output}[field-4]; got != "" {
			t.Fatalf("field %d repeated left draft = %q, want empty", field, got)
		}
	}
}

func TestTUIFilterNumericArrowsDoNotCrossZero(t *testing.T) {
	for _, field := range []int{5, 6, 7} {
		m := tuiModel{overlay: "filter", filterCursor: field, filterDraft: tuiFilterDraft{context: "1", input: "1", output: "1"}}
		for i := 0; i < 200; i++ {
			m, _ = m.filterKey("left", tea.KeyMsg{Type: tea.KeyLeft})
		}
		got := []string{m.filterDraft.context, m.filterDraft.input, m.filterDraft.output}[field-5]
		if got != "" {
			t.Fatalf("field %d repeated left = %q, want empty", field, got)
		}
	}
}

func TestTUIFilterHelpDocumentsExamplesOperatorsAndScoreSource(t *testing.T) {
	for _, want := range []string{
		"omt table --filter 'paid,quality>=80'",
		"press f",
		"Predicates:",
		"Operators:",
		"repeated with CLI --filter",
		"always use AND",
		"active score source",
		"quality>=0.8 means quality>=80",
	} {
		if !strings.Contains(tuiHelpDocument, want) {
			t.Errorf("help does not contain %q", want)
		}
	}
}

func TestTUIScoreSourceMessageRebuildsRows(t *testing.T) {
	rows := []model.Model{{Slug: "a", Score: &model.ScoreInfo{Value: 1}, Rankable: true}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	next, _ := m.Update(tuiScoreSourceMsg{generation: m.scoreSourceGeneration, source: scoreSourceArena, models: rows})
	got := next.(tuiModel)
	if got.scoreSource != scoreSourceArena || got.status != "score source changed" || len(got.visible) != 1 {
		t.Fatalf("score source update = source %q, status %q, visible %d", got.scoreSource, got.status, len(got.visible))
	}
}

func TestTUIScoreSourceMessageIgnoresStaleResult(t *testing.T) {
	oldRows := []model.Model{{Slug: "old"}}
	newRows := []model.Model{{Slug: "new"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, oldRows)
	m.scoreSourceGeneration = 2
	msg := tuiScoreSourceMsg{generation: 1, source: scoreSourceArena, models: newRows}
	next, _ := m.Update(msg)
	got := next.(tuiModel)
	if got.scoreSource != scoreSourceDefault || len(got.models) != 1 || got.models[0].Slug != "old" {
		t.Fatalf("stale score source result was applied: source %q, models %+v", got.scoreSource, got.models)
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
	for _, test := range []struct{ key, want string }{
		{".", "/"}, {",", "?"},
		{"ч", "x"}, // U+0447, физическая позиция латинской x — исходный багрепорт
		{"с", "c"}, // U+0441, омоглиф латинской c
		{"р", "h"}, // U+0440, омоглиф латинской p, но команда — h
		{"к", "r"}, // U+043A, омоглиф латинской k, но команда — r
		{"о", "j"}, // U+043E, омоглиф латинской o, но команда — j
	} {
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
	// Alt и paste обязаны пройти мимо таблицы алиасов и попасть в msg.String()
	// как есть — иначе Alt+ч тихо выполняет "x" (quit), хотя Alt+x на
	// латинской раскладке ничего не делает, и вставка "ч" из буфера обмена
	// тоже выполняла бы quit вместо того, чтобы остаться текстом.
	if got := tuiCommandKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ч"), Alt: true}); got != "alt+ч" {
		t.Fatalf("alt+ч normalized to %q, want %q (must not resolve to the %q alias)", got, "alt+ч", "x")
	}
	if got := tuiCommandKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ч"), Paste: true}); got != "[ч]" {
		t.Fatalf("pasted ч normalized to %q, want %q (must not resolve to the %q alias)", got, "[ч]", "x")
	}
}

// TestTUIInputModeKeepsNonASCIIInput — обязательная регрессия раскладочных
// алиасов: руны ы, ч, й, К, с и р теперь являются командами, и без этой
// проверки нет доказательства, что поиск и фильтр по-прежнему принимают
// русский текст, а не выполняют s, x, q, R, c и h.
func TestTUIInputModeKeepsNonASCIIInput(t *testing.T) {
	for _, key := range []string{"é", "界", "ы", "ч", "й", "К", "с", "р"} {
		for _, mode := range []string{"search", "filter", "help-search"} {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
			m.inputMode = mode
			beforeSort, beforeOverlay, beforeRanking := m.sortKey, m.overlay, m.ranking
			beforeGeneration, beforeRefreshing := m.generation, m.refreshing
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			m = next.(tuiModel)
			if m.input != key || m.inputMode != mode || cmd != nil {
				t.Fatalf("non-ASCII input %q in mode %q was routed as command: input=%q mode=%q cmd=%v", key, mode, m.input, m.inputMode, cmd != nil)
			}
			if m.sortKey != beforeSort || m.overlay != beforeOverlay || m.ranking != beforeRanking || m.generation != beforeGeneration || m.refreshing != beforeRefreshing {
				t.Fatalf("non-ASCII input %q in mode %q changed model state: sort=%q overlay=%q ranking=%q generation=%d refreshing=%v", key, mode, m.sortKey, m.overlay, m.ranking, m.generation, m.refreshing)
			}
		}
	}
}

func TestTUIInputModeKeepsCyrillicChe(t *testing.T) {
	for _, mode := range []string{"search", "filter"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
		m.inputMode = mode
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ч")})
		m = next.(tuiModel)
		if m.input != "ч" || m.inputMode != mode || cmd != nil {
			t.Fatalf("Cyrillic ч in %s input = %q, mode %q, cmd %v; want text input", mode, m.input, m.inputMode, cmd != nil)
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

func TestTUITopPaidFreeLayoutUsesIndependentTopNSections(t *testing.T) {
	rows := []model.Model{{Slug: "paid-1", Free: false, Score: &model.ScoreInfo{Value: 90}, Rankable: true, HasQualityPrice: true}, {Slug: "paid-2", Free: false, Score: &model.ScoreInfo{Value: 80}, Rankable: true, HasQualityPrice: true}, {Slug: "free-1", Free: true, Score: &model.ScoreInfo{Value: 70}, Rankable: true}, {Slug: "free-2", Free: true, Score: &model.ScoreInfo{Value: 60}, Rankable: true}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.layout, m.topN, m.filter = "top-paid-free", 1, ""
	m.rebuild()
	if len(m.visible) != 2 || m.visible[0].Slug != "paid-1" || m.visible[1].Slug != "free-1" || m.topSeparator != 1 {
		t.Fatalf("top layout = %+v, separator=%d", m.visible, m.topSeparator)
	}
}

func TestTUITopPaidFreePipelineSearchAndGlobalLimit(t *testing.T) {
	rows := []model.Model{
		{Slug: "paid-1", DisplayName: "Paid one", Free: false, Score: &model.ScoreInfo{Value: 90}, Rankable: true, HasQualityPrice: true},
		{Slug: "paid-2", DisplayName: "Paid two", Free: false, Score: &model.ScoreInfo{Value: 80}, Rankable: true, HasQualityPrice: true},
		{Slug: "free-1", DisplayName: "Free one", Free: true, Score: &model.ScoreInfo{Value: 70}, Rankable: true},
		{Slug: "free-2", DisplayName: "Free two", Free: true, Score: &model.ScoreInfo{Value: 60}, Rankable: true},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.layout, m.topN, m.limit = "top-paid-free", 2, 3
	m.rebuild()
	if got := []string{m.visible[0].Slug, m.visible[1].Slug, m.visible[2].Slug}; !reflect.DeepEqual(got, []string{"paid-1", "paid-2", "free-1"}) || m.topSeparator != 2 {
		t.Fatalf("limited top layout = %v, separator=%d", got, m.topSeparator)
	}
	m.inputMode, m.input = "search", "free"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.visible) != 2 || m.visible[0].Slug != "free-1" || m.visible[1].Slug != "free-2" || m.topSeparator != -1 {
		t.Fatalf("searched top layout = %+v, separator=%d", m.visible, m.topSeparator)
	}
}

func TestTUITopPaidFreeSearchKeepsSeparatorAcrossBothSections(t *testing.T) {
	rows := []model.Model{
		{Slug: "paid-shared", DisplayName: "Shared paid", Free: false, Score: &model.ScoreInfo{Value: 90}, Rankable: true, HasQualityPrice: true},
		{Slug: "paid-other", DisplayName: "Paid other", Free: false, Score: &model.ScoreInfo{Value: 80}, Rankable: true, HasQualityPrice: true},
		{Slug: "free-shared", DisplayName: "Shared free", Free: true, Score: &model.ScoreInfo{Value: 70}, Rankable: true},
		{Slug: "free-other", DisplayName: "Free other", Free: true, Score: &model.ScoreInfo{Value: 60}, Rankable: true},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.layout, m.topN, m.limit, m.width, m.height = "top-paid-free", 2, 3, 120, 12
	m.inputMode, m.input = "search", "shared"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})

	if got := []string{m.visible[0].Slug, m.visible[1].Slug}; !reflect.DeepEqual(got, []string{"paid-shared", "free-shared"}) || m.topSeparator != 1 {
		t.Fatalf("searched top layout = %v, separator=%d, want both sections separated at 1", got, m.topSeparator)
	}
	lines := strings.Split(m.View(), "\n")
	for i, line := range lines {
		if strings.Contains(line, "Shared free") {
			if i == 0 || lines[i-1] != "" {
				t.Fatalf("missing paid/free separator before free row in view:\n%s", m.View())
			}
			return
		}
	}
	t.Fatalf("free match missing from view:\n%s", m.View())
}

func TestTUITopPaidFreeHonorsExplicitAvailabilityAndHasQP(t *testing.T) {
	rows := []model.Model{
		{Slug: "paid", Free: false, HasQualityPrice: true},
		{Slug: "free", Free: true, HasQualityPrice: false},
	}
	for _, test := range []struct {
		name, filter string
		want         []string
	}{
		{"free availability", "availability:free", []string{"free"}},
		{"paid availability", "availability:paid", []string{"paid"}},
		{"any availability", "availability:any", []string{"paid", "free"}},
		{"legacy free", "free", []string{"free"}},
		{"legacy paid", "paid", []string{"paid"}},
		{"has q/p", "has-q/p", []string{"paid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
			m.layout, m.topN, m.filter, m.filterFormExplicit = "top-paid-free", 3, test.filter, true
			m.rebuild()
			got := make([]string, 0, len(m.visible))
			for _, row := range m.visible {
				got = append(got, row.Slug)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("filter %q produced %v, want %v", test.filter, got, test.want)
			}
		})
	}
}

func TestTUISettingsLayoutTogglePersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tui:\n  layout: all\n  top_n: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.configPath, m.overlay, m.settingsCursor = path, "settings", 4
	m, _ = m.settingsKey("enter", "enter")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TUI.Layout != "top-paid-free" {
		t.Fatalf("saved layout = %q, want top-paid-free", cfg.TUI.Layout)
	}
	restarted := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	restarted.layout, restarted.topN = cfg.TUI.Layout, cfg.TUI.TopN
	if restarted.layout != "top-paid-free" || restarted.topN != 3 {
		t.Fatalf("reloaded layout = %q/%d, want top-paid-free/3", restarted.layout, restarted.topN)
	}
}

func TestNewTUIModelUsesDefaultIconGap(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	if m.iconGap != int(config.DefaultIconGap) {
		t.Fatalf("default TUI icon gap = %d, want %d", m.iconGap, config.DefaultIconGap)
	}
}

func TestTUIRefreshMessageAppliesReloadedLayoutAndTopN(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tui:\n  layout: top-paid-free\n  top_n: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a"}})
	m.configPath = path
	m.generation = 1
	message, ok := m.refreshCmd()().(tuiRefreshMsg)
	if !ok || message.layout != "top-paid-free" || message.topN != 7 {
		t.Fatalf("refresh message = %#v, want reloaded layout/top_n", message)
	}
	next, _ := m.Update(message)
	got := next.(tuiModel)
	if got.layout != "top-paid-free" || got.topN != 7 {
		t.Fatalf("refresh did not apply config: layout=%q topN=%d", got.layout, got.topN)
	}
}

func TestTUIRefreshReloadsIconGapForListAndDetail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("table:\n  icon_gap: 1\n  icon_gaps: {Meta: 0}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	row := model.Model{DisplayName: "OpenAI Model", Owner: "OpenAI"}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.configPath, m.generation, m.iconGap = path, 1, 1
	m.iconGaps = config.IconGaps{"meta": 0}
	if got := tuiCellWithIconsAndGaps(row, colName, false, scoreSourceDefault, m.icons, m.iconGap, m.iconGaps); !strings.Contains(got, "🌀 OpenAI") {
		t.Fatalf("initial list identity = %q, want one gap", got)
	}
	if err := os.WriteFile(path, []byte("table:\n  icon_gap: 3\n  icon_gaps: {Meta: 3}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	message, ok := m.refreshCmd()().(tuiRefreshMsg)
	if !ok || !message.iconGapSet || message.iconGap != 3 {
		t.Fatalf("refresh message icon gap = %#v, want effective gap 3", message)
	}
	message.err = nil
	next, _ := m.Update(message)
	got := next.(tuiModel)
	if got.iconGap != 3 {
		t.Fatalf("refreshed icon gap = %d, want 3", got.iconGap)
	}
	if list := tuiCellWithIconsAndGap(row, colName, false, scoreSourceDefault, got.icons, got.iconGap); !strings.Contains(list, "🌀   OpenAI") {
		t.Fatalf("refreshed list identity = %q, want three gaps", list)
	}
	meta := model.Model{DisplayName: "Meta Model", Owner: "Meta"}
	if list := tuiCellWithIconsAndGaps(meta, colName, false, scoreSourceDefault, got.icons, got.iconGap, got.iconGaps); list != "Ⓜ️    Meta Meta Model" {
		t.Fatalf("refreshed vendor override = %q, want fixed-slot padding plus three gaps", list)
	}
	detail := tuiDetailLinesWithHistoryAndIconsAndGaps(row, scoreSourceDefault, 100, time.Now(), nil, got.icons, got.iconGap, got.iconGaps)
	if !strings.Contains(strings.Join(detail, "\n"), "Производитель: 🌀   OpenAI") {
		t.Fatalf("refreshed detail does not use gap 3: %v", detail)
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
	for _, key := range []string{"R", "К"} {
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
	for _, key := range []string{"x", "ч"} {
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

func TestTUIRefreshReloadsPriceHistoryBeforeClampingDetail(t *testing.T) {
	dataDir := t.TempDir()
	row := tuiDetailTestModel()
	base := pricehistory.Price{Found: true, InPerM: row.InPerM, OutPerM: row.OutPerM, Context: row.Context}
	changed := base
	changed.OutPerM = 4
	history := &pricehistory.History{Observations: []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: base}},
	}}
	if err := history.Save(pricehistory.Path(dataDir)); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel(context.Background(), dataDir, refresh.Options{}, 0, []model.Model{row})
	m.priceHistory = history
	m.overlay, m.width, m.height, m.detailOffset, m.generation, m.refreshing = "detail", 30, 10, 10000, 1, true
	updated := &pricehistory.History{Observations: append(history.Observations, pricehistory.Observation{ObservedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: changed}})}
	if err := updated.Save(pricehistory.Path(dataDir)); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tuiRefreshMsg{generation: 1, models: []model.Model{row}})
	m = next.(tuiModel)
	if len(m.priceHistory.Observations) != 2 {
		t.Fatalf("refresh kept stale price history: %+v", m.priceHistory)
	}
	want := tuiDetailMaxOffsetWithHistory(row, m.scoreSource, m.width, m.height, m.priceHistory)
	if m.detailOffset != want {
		t.Fatalf("detail offset after history refresh = %d, want %d", m.detailOffset, want)
	}
}

func TestTUIRefreshIgnoresResultFromPreviousScoreSource(t *testing.T) {
	oldRows := []model.Model{{Slug: "old"}}
	newRows := []model.Model{{Slug: "new"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, oldRows)
	m.generation = 1
	m.scoreSourceGeneration = 2
	m.refreshing = false
	msg := tuiRefreshMsg{generation: 1, scoreSourceGeneration: 1, models: newRows, err: errors.New("stale source error")}
	next, _ := m.Update(msg)
	got := next.(tuiModel)
	if got.models[0].Slug != "old" || got.err != "" || got.status != "" {
		t.Fatalf("stale refresh changed state: source generation %d, models %+v, err %q, status %q", got.scoreSourceGeneration, got.models, got.err, got.status)
	}
	next, _ = got.Update(tuiRefreshMsg{generation: 1, scoreSourceGeneration: 2, models: newRows})
	got = next.(tuiModel)
	if got.models[0].Slug != "new" || got.status != "refreshed" {
		t.Fatalf("current-source refresh was not applied: models %+v, status %q", got.models, got.status)
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

func TestTUIDetailNormalizesLegacyMissingLabels(t *testing.T) {
	lines := tuiDetailScoreLines(&model.ScoreInfo{Metric: "н/д", Uncertainty: "н/д", SampleSize: "н/д"}, "n/a")
	if strings.Contains(strings.Join(lines, "\n"), "н/д") {
		t.Fatalf("TUI detail contains legacy missing label: %v", lines)
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
	m.search = ""
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
	m, _ = m.columnKey(" ", " ")
	if len(m.pendingColumns) != 1 {
		t.Fatal("last column was removed")
	}
}

func TestTUIRenderTUILineAlignsCellsAndNumericValues(t *testing.T) {
	m := tuiModel{width: 34}
	columns := []tuiColumn{colName, colContext, colInput}
	wantOffsets := []int{12, 26}
	header := m.renderTUILine(columns, nil, false)
	for _, values := range [][]string{{"Long model", "7", "1.5"}, {"Long model", "12345", "0.125"}} {
		row := m.renderTUILine(columns, values, false)
		selected := m.renderTUILine(columns, values, true)
		if !reflect.DeepEqual(tuiSeparatorDisplayOffsets(header), wantOffsets) || !reflect.DeepEqual(tuiSeparatorDisplayOffsets(row), wantOffsets) || !reflect.DeepEqual(tuiSeparatorDisplayOffsets(selected), wantOffsets) {
			t.Fatalf("separator display positions = header=%v row=%v selected=%v, want %v", tuiSeparatorDisplayOffsets(header), tuiSeparatorDisplayOffsets(row), tuiSeparatorDisplayOffsets(selected), wantOffsets)
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
	for _, shortcut := range []string{"q quality", "p availability", "r q/p", "R refresh", "x quit", "o settings", "f filter"} {
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
	if !reflect.DeepEqual(tuiSeparatorDisplayOffsets(row), []int{9}) || !reflect.DeepEqual(tuiSeparatorDisplayOffsets(m.renderTUILine(columns, []string{"界🙂", "123"}, false)), []int{9}) {
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

func TestTUIViewportPreservesDisplayedSourceAwareHeaders(t *testing.T) {
	for _, test := range []struct {
		width  int
		source string
		want   string
	}{
		{80, scoreSourceSWEBench, "SWE %"},
		{96, scoreSourceArena, "Arena Elo"},
		{120, scoreSourceSWEBench, "Q/P score/$M"},
	} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
		m.width, m.scoreSource = test.width, test.source
		columns := m.renderColumns()
		header := m.renderTUILine(columns, nil, false)
		if lipgloss.Width(header) > test.width {
			t.Fatalf("width %d produced a %d-column header: %q", test.width, lipgloss.Width(header), header)
		}
		if !strings.Contains(header, test.want) {
			t.Fatalf("width %d source %s lost displayed header %q: columns=%v header=%q", test.width, test.source, test.want, columns, header)
		}
		for _, column := range columns {
			label := tuiColumnLabel(column, test.source)
			if !strings.Contains(header, label) {
				t.Fatalf("width %d source %s truncated displayed header %q: columns=%v header=%q", test.width, test.source, label, columns, header)
			}
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

func testTUISeparatorColumns(width, columnCount int) []int {
	if columnCount != 8 {
		panic(fmt.Sprintf("missing independent TUI geometry contract for %d columns", columnCount))
	}
	want, ok := map[int][]int{
		120: {43, 53, 62, 78, 93, 103, 114},
		40:  {4, 8, 12, 16, 20, 24, 28},
	}[width]
	if !ok {
		panic(fmt.Sprintf("missing independent TUI geometry contract for width %d", width))
	}
	return want
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
	view := tuiHelpView(tuiModel{width: 80, height: 20})
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Fatalf("help height = %d, want 20", len(lines))
	}
	if strings.Contains(view, "╭") || strings.Contains(view, "╰") {
		t.Fatalf("help still has a box: %q", view)
	}
}

func TestTUIHelpSearchMaxOffsetIncludesInputRow(t *testing.T) {
	// m.helpLines() is what is actually scrolled — not the legacy
	// whole-document tuiHelpLines() — now that the F1 overlay renders one
	// section at a time. Section 2 (Hotkeys) is used because its
	// last line is a short \t-row (tuiFormatHelpLine leaves it exactly as
	// rendered); "Overview"'s own last line is long, plain prose that
	// tuiFullscreenText's width-based truncation would cut but
	// tuiFormatHelpLine (having no \t to format) would not — a mismatch
	// this test does not exist to exercise.
	m := tuiModel{overlay: "help", helpSection: 2, width: 100, height: 5, inputMode: "help-search"}
	helpLines := m.helpLines()
	if got, want := m.helpMaxOffset(), len(helpLines)-4; got != want {
		t.Fatalf("help search max offset = %d, want %d", got, want)
	}
	m.helpOffset = m.helpMaxOffset()
	lines := strings.Split(tuiHelpView(m), "\n")
	if got, want := ansi.Strip(lines[3]), tuiFormatHelpLine(helpLines[len(helpLines)-1], m.width); got != want {
		t.Fatalf("last visible help line = %q, want %q", got, want)
	}
}

func tuiHelpLineIndexContaining(needle string) int {
	for i, line := range tuiHelpLines() {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func TestTUIHelpRowsUseColumnsAndFitNarrowTerminals(t *testing.T) {
	row := `\tq / p / r\tsort\tq sorts by quality; p by price; r by quality/price ratio (q/p).`
	for _, width := range []int{12, 24, 80} {
		formatted := tuiFormatHelpLine(row, width)
		if tableDisplayWidth(formatted) > width {
			t.Fatalf("width %d: formatted help row is %d columns wide: %q", width, tableDisplayWidth(formatted), formatted)
		}
		if width >= 24 && !strings.Contains(formatted, "  ") {
			t.Fatalf("width %d: help row has no column spacing: %q", width, formatted)
		}
	}
	tuiForceColorProfile(t)
	m := tuiModel{overlay: "help", width: 80, height: 20, helpSearch: "quality"}
	view := tuiHelpView(m)
	if !strings.Contains(view, tuiHeaderStyle.Render("omt tui keys (version "+version+")")) {
		t.Fatal("help title is not colour-accented")
	}
	if ansi.Strip(view) == view {
		t.Fatal("help search or headings did not emit ANSI styling")
	}
	for _, line := range strings.Split(view, "\n") {
		if tableDisplayWidth(ansi.Strip(line)) > m.width {
			t.Fatalf("styled help line exceeds width %d: %q", m.width, line)
		}
	}
}

func TestTUIHelpHotkeysKeepActionsGroupedAndTaskFitCodesSeparate(t *testing.T) {
	for _, group := range []string{"Hotkeys", "Navigation", "Data/view", "Filters/settings", "Task-fit codes", "General/help"} {
		if !strings.Contains(tuiHelpDocument, "\n"+group+"\n") {
			t.Fatalf("help document missing group %q: %q", group, tuiHelpDocument)
		}
	}
	if strings.Contains(tuiHelpDocument, "Task-fit\n") {
		t.Fatalf("help document has ambiguous Task-fit heading: %q", tuiHelpDocument)
	}
	for _, action := range []string{"sort", "settings", "filter", "help"} {
		if !strings.Contains(tuiHelpDocument, `\t`+action+`\t`) {
			t.Fatalf("help document has no action cell for %q: %q", action, tuiHelpDocument)
		}
	}
	groupedEquivalentKeys := `\tq / p / r\tsort\tquality, price, or quality/price ratio.`
	if fields := strings.Split(groupedEquivalentKeys, `\t`); len(fields) != 4 {
		t.Fatalf("equivalent hotkeys must share one action row: %q", groupedEquivalentKeys)
	}
}

func TestTUIHelpRowsDoNotMixActions(t *testing.T) {
	for _, row := range []string{
		`\tSpace\tcolumns\ttoggle a column.`,
		`\tSpace\ttier\tcycle Tier.`,
		`\tSpace\tswitch\t(in Settings) switch between SWE-bench and Arena.`,
	} {
		if !strings.Contains(tuiHelpDocument, row) {
			t.Fatalf("help document is missing single-action row %q: %q", row, tuiHelpDocument)
		}
	}

	for _, row := range []string{
		`\tc\tcolumns\topen selection.`,
		`\tSpace\tcolumns\ttoggle a column.`,
		`\tEnter\tcolumns\tapply the column selection.`,
		`\tEsc\tcolumns\tcancel the column selection.`,
		`\tx / Ctrl-C\texit\texit the TUI.`,
		`\tEsc\tclose\tclose help.`,
		`\tEsc\tback\treturn to the list from the current overlay.`,
	} {
		if !strings.Contains(tuiHelpDocument, row) {
			t.Fatalf("full help is missing single-action row %q: %q", row, tuiHelpDocument)
		}
	}

	for _, mixed := range []string{
		`\tSpace\tcolumns/tier\ttoggle a column or cycle Tier.`,
		`\tc / Space / Enter / Esc\tcolumns\topen selection; toggle a column; apply or cancel.`,
		`\tx / Ctrl-C / Esc\tclose\texit, close help, or return to the list.`,
		`open settings; move Down to Score source, then press Space to switch`,
		`open help at Hotkeys or close help.`,
	} {
		if strings.Contains(tuiHelpDocument, mixed) {
			t.Fatalf("help still contains mixed-action description %q", mixed)
		}
	}
}

func TestTUIHelpRowsRemainSingleColumnWhenNarrow(t *testing.T) {
	row := `\to\tsettings\topen settings.`
	for _, width := range []int{1, 10, 14} {
		if got := tableDisplayWidth(tuiFormatHelpLine(row, width)); got > width {
			t.Fatalf("width %d: formatted help row is %d columns wide", width, got)
		}
	}
}

// tuiHelpRowColumns splits a help-document content line the same way
// tuiFormatHelpLine does, and reports whether it parsed as a clean
// \tKey\tAction\tDescription row. It exists so the tests below can assert on
// the exact Key/Action/Description tuiFormatHelpLine would render, without
// duplicating its width-formatting logic.
func tuiHelpRowColumns(line string) (key, action, description string, ok bool) {
	if !strings.Contains(line, `\t`) {
		return "", "", "", false
	}
	parts := strings.Split(line, `\t`)
	if len(parts) == 4 && parts[0] == "" {
		parts = parts[1:]
	}
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// TestTUIHelpDocumentTemplatedRowsHaveCleanTabColumns audits every templated
// row (any line containing a literal `\t`) in the help document for the
// tab-column shape tuiFormatHelpLine actually requires to render a row as
// aligned columns: the line must start with a literal `\t` and contain
// exactly three of them (\tKey\tAction\tDescription). A stray extra leading
// tab character, or an extra literal `\t` anywhere else, defeats
// tuiFormatHelpLine's `len(parts) != 3` guard, and the raw, unsplit line
// (literal backslash-t sequences and all) is rendered verbatim instead of
// being formatted into columns.
func TestTUIHelpDocumentTemplatedRowsHaveCleanTabColumns(t *testing.T) {
	for i, line := range strings.Split(tuiHelpDocument, "\n") {
		if !strings.Contains(line, `\t`) {
			continue // blank line, heading, or plain prose: not a table row.
		}
		if !strings.HasPrefix(line, `\t`) {
			t.Errorf("line %d does not start with a templated column marker (stray leading character before the first \\t): %q", i, line)
		}
		if count := strings.Count(line, `\t`); count != 3 {
			t.Errorf("line %d has %d tab-column markers, want 3 (\\tKey\\tAction\\tDescription): %q", i, count, line)
		}
		if _, _, _, ok := tuiHelpRowColumns(line); !ok {
			t.Errorf("line %d does not parse into exactly 3 columns via tuiFormatHelpLine's own rule: %q", i, line)
		}
	}
}

// tuiConfiguredRowColumns finds the raw document line uniquely identified by
// descriptionSubstring, runs it through tuiConfiguredHelpLines with the
// given keymap (in isolation, as its own single-line document), and returns
// the resulting Key/Action/Description columns.
func tuiConfiguredRowColumns(t *testing.T, rawLines []string, descriptionSubstring string, keymap config.TUIKeymap) (key, action, description string) {
	t.Helper()
	var raw []string
	for _, line := range rawLines {
		if strings.Contains(line, descriptionSubstring) {
			raw = append(raw, line)
		}
	}
	if len(raw) == 0 {
		t.Fatalf("expected at least one raw line containing %q, found none", descriptionSubstring)
	}
	// A document may legitimately document the same row twice (e.g. the full
	// help repeats "open full help." in both "Refresh and finish" and
	// "General/help"). Every occurrence must be the exact same text and must
	// substitute identically, so checking the first is representative.
	for _, line := range raw[1:] {
		if line != raw[0] {
			t.Fatalf("raw lines containing %q are not identical: %q vs %q", descriptionSubstring, raw[0], line)
		}
	}
	configured := tuiConfiguredHelpLines([]string{raw[0]}, keymap)
	key, action, description, ok := tuiHelpRowColumns(configured[0])
	if !ok {
		t.Fatalf("configured line for %q did not parse into 3 columns: %q", descriptionSubstring, configured[0])
	}
	return key, action, description
}

// TestTUIConfiguredHelpLinesKeepKeyColumnSeparateFromAction guards against a
// second, related defect the tab-column audit above cannot see: several
// keymap-substitution markers in tuiConfiguredHelpLines matched starting
// from the Action column instead of the Key column (e.g. `\tswitch\t...`
// instead of `\tSpace\tswitch\t...`). Even on a syntactically clean
// \tKey\tAction\tDescription row, that shape overwrites the Action column
// with the real bound key and leaves the Key column as a static, unbound
// placeholder — rendering as a duplicated-looking row (e.g. "o" / "o", "?" /
// "?") that also silently stops reflecting a customised keybinding. Every
// row here must end up with the real configured key in the Key column and
// the documented static action label untouched in the Action column.
func TestTUIConfiguredHelpLinesKeepKeyColumnSeparateFromAction(t *testing.T) {
	keymap := config.DefaultTUIKeymap()
	cases := []struct {
		description string
		wantKey     string
		wantAction  string
	}{
		{"(main) switch between SWE-bench and Arena.", "space", "switch"},
		{"(in Settings) switch between SWE-bench and Arena.", "space / enter", "switch"},
		{"open settings.", "o", "settings"},
		{"open help at Hotkeys.", "?", "help"},
		{"open full help.", "f1", "help"},
	}
	for _, tc := range cases {
		key, action, _ := tuiConfiguredRowColumns(t, tuiHelpLines(), tc.description, keymap)
		if key != tc.wantKey || action != tc.wantAction {
			t.Errorf("row %q: key=%q action=%q, want key=%q action=%q", tc.description, key, action, tc.wantKey, tc.wantAction)
		}
	}
}

// TestTUIConfiguredHelpLinesDoNotDoubleSubstituteOpenDetails guards against
// a related hazard: the Hotkeys section's "open the model detail screen."
// row used to be matched by two different replacements-map markers at once —
// `\tEnter / Right / l\tdetail\t` (correct: replaces only the Key column)
// and `\tdetail\topen the model detail screen.` (the same Action-column-first
// shape as the switch_source/settings/help markers above). Because Go
// randomises map iteration order, whichever marker happened to run first
// determined whether the row rendered correctly or with the real key
// duplicated into both the Key and Action columns. Running the full
// document (so both markers are actually candidates for the same line, as
// in production) pins the outcome deterministically.
func TestTUIConfiguredHelpLinesDoNotDoubleSubstituteOpenDetails(t *testing.T) {
	keymap := config.DefaultTUIKeymap()
	for i := 0; i < 20; i++ { // map iteration order is randomised per run; repeat to catch either order.
		configured := tuiConfiguredHelpLines(tuiHelpLines(), keymap)
		var raw string
		for _, line := range configured {
			if strings.Contains(line, "open the model detail screen.") {
				raw = line
				break
			}
		}
		key, action, _, ok := tuiHelpRowColumns(raw)
		if !ok {
			t.Fatalf("configured 'open the model detail screen.' line did not parse into 3 columns: %q", raw)
		}
		if wantKey := "enter / right / l"; key != wantKey {
			t.Fatalf("configured 'open the model detail screen.' key = %q, want %q (line: %q)", key, wantKey, raw)
		}
		if action != "detail" {
			t.Fatalf("configured 'open the model detail screen.' action = %q, want \"detail\" (real key must not leak into the Action column): line %q", action, raw)
		}
	}
}

func TestTUIHelpSectionSwitchRebuildsSearchMatches(t *testing.T) {
	m := tuiModel{overlay: "help", width: 100, height: 24, helpSearch: "source"}
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	m.helpMatch = 1
	m = tuiKey(m, "?")
	if m.overlay != "" {
		t.Fatalf("question mark did not close help: overlay=%q", m.overlay)
	}
	m = tuiKey(m, "?")
	// ? lands on section 2 (Hotkeys); matches must be rebuilt against that
	// section, not whatever section/search state was left over before close.
	if m.helpSection != 2 {
		t.Fatalf("? did not land on the Hotkeys section: section=%d", m.helpSection)
	}
	wantHotkeys := tuiHelpSearchInLines(m.helpSearch, tuiHelpSectionLines(2))
	if !reflect.DeepEqual(m.helpMatches, wantHotkeys) || m.helpMatch != -1 {
		t.Fatalf("?-opened help kept stale search state: matches=%v, match=%d, want=%v/-1", m.helpMatches, m.helpMatch, wantHotkeys)
	}
	m = tuiKey(m, "f1")
	// F1 resets to section 0 (Overview); matches must be rebuilt against
	// that section too, not whatever ? last left behind.
	if m.helpSection != 0 {
		t.Fatalf("f1 did not reset to the Overview section: section=%d", m.helpSection)
	}
	wantOverview := tuiHelpSearchInLines(m.helpSearch, tuiHelpSectionLines(0))
	if !reflect.DeepEqual(m.helpMatches, wantOverview) || m.helpMatch != -1 {
		t.Fatalf("f1 kept stale search state: matches=%v, match=%d, want=%v/-1", m.helpMatches, m.helpMatch, wantOverview)
	}
}

// TestTUIHelpDigitKeysJumpToSection covers the F1 overlay's six-section
// navigation: digit keys 1-6 jump straight to the matching tuiHelpSections
// index, reset the scroll offset, and rebuild search matches against the
// newly active section. F1 and ? both open this same sectioned overlay (at
// section 0 and 2 respectively), so there is no separate mode to switch
// between anymore.
func TestTUIHelpDigitKeysJumpToSection(t *testing.T) {
	for digit, wantSection := range map[string]int{"1": 0, "2": 1, "3": 2, "4": 3, "5": 4, "6": 5} {
		m := tuiModel{overlay: "help", helpSection: 0, helpOffset: 7, width: 100, height: 10, helpSearch: "search"}
		m.helpMatches = tuiHelpSearchInLines(m.helpSearch, tuiHelpSectionLines(0))
		m.helpMatch = 0
		m = tuiKey(m, digit)
		if m.helpSection != wantSection {
			t.Fatalf("digit %s: section = %d, want %d", digit, m.helpSection, wantSection)
		}
		if m.helpOffset != 0 {
			t.Fatalf("digit %s: offset = %d, want 0 (section switch resets scroll)", digit, m.helpOffset)
		}
		wantMatches := tuiHelpSearchInLines(m.helpSearch, tuiHelpSectionLines(wantSection))
		if !reflect.DeepEqual(m.helpMatches, wantMatches) || m.helpMatch != -1 {
			t.Fatalf("digit %s: matches = %v (match %d), want %v rebuilt against the new section (match -1)", digit, m.helpMatches, m.helpMatch, wantMatches)
		}
	}
}

// TestTUIHelpLeftRightStepAndClampWithoutWrapping covers the other half of
// the navigation design: Left/Right step to the adjacent section one at a
// time and clamp at both ends rather than wrapping — the same choice every
// other cursor-style movement in this file already makes (m.cursor,
// columnCursor, settingsCursor, filterCursor: see tuiClampHelpSection).
func TestTUIHelpLeftRightStepAndClampWithoutWrapping(t *testing.T) {
	m := tuiModel{overlay: "help", width: 100, height: 10}
	if m.helpSection != 0 {
		t.Fatalf("test setup: helpSection = %d, want 0", m.helpSection)
	}
	m = tuiKey(m, "left")
	if m.helpSection != 0 {
		t.Fatalf("left at the first section = %d, want clamped at 0 (no wraparound)", m.helpSection)
	}
	for want := 1; want <= 5; want++ {
		m = tuiKey(m, "right")
		if m.helpSection != want {
			t.Fatalf("right step %d: section = %d, want %d", want, m.helpSection, want)
		}
	}
	m = tuiKey(m, "right")
	if m.helpSection != 5 {
		t.Fatalf("right past the last section = %d, want clamped at 5 (no wraparound)", m.helpSection)
	}
	for want := 4; want >= 0; want-- {
		m = tuiKey(m, "left")
		if m.helpSection != want {
			t.Fatalf("left step to %d: section = %d, want %d", want, m.helpSection, want)
		}
	}
}

// TestTUIHelpUpDownScrollsWithinSectionOnly guards against Up/Down (or
// j/k) accidentally being reinterpreted as section navigation: they must
// keep doing exactly what they did before sectioning existed — move
// helpOffset within the current section — and never change helpSection.
func TestTUIHelpUpDownScrollsWithinSectionOnly(t *testing.T) {
	m := tuiModel{overlay: "help", helpSection: 2, width: 100, height: 5}
	m = tuiKey(m, "down")
	if m.helpSection != 2 || m.helpOffset != 1 {
		t.Fatalf("down changed section=%d offset=%d, want section=2 offset=1", m.helpSection, m.helpOffset)
	}
	m = tuiKey(m, "j")
	if m.helpSection != 2 || m.helpOffset != 2 {
		t.Fatalf("j changed section=%d offset=%d, want section=2 offset=2", m.helpSection, m.helpOffset)
	}
	m = tuiKey(m, "up")
	if m.helpSection != 2 || m.helpOffset != 1 {
		t.Fatalf("up changed section=%d offset=%d, want section=2 offset=1", m.helpSection, m.helpOffset)
	}
}

// TestTUIHelpSearchIsScopedToTheCurrentSection is the direct behavioural
// check for "search scopes to the current section only": a needle that
// exists in one section (Hotkeys, via the Task-fit codes table)
// must not be reported as a match while a different section (Overview) is
// active, even though both sections are part of the same helpSearch state.
func TestTUIHelpSearchIsScopedToTheCurrentSection(t *testing.T) {
	needle := "task-fit code"
	if strings.Contains(tuiHelpSectionOverviewBody, needle) {
		t.Fatalf("test invalid: %q unexpectedly occurs in Overview", needle)
	}
	if !strings.Contains(tuiHelpSectionHotkeysBody, needle) {
		t.Fatalf("test invalid: %q does not occur in Hotkeys", needle)
	}
	m := tuiModel{overlay: "help", helpSection: 0, width: 100, height: 10, helpSearch: needle}
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	if len(m.helpMatches) != 0 {
		t.Fatalf("Overview reported %d matches for a needle that only occurs in Hotkeys: %v", len(m.helpMatches), m.helpMatches)
	}
	m.helpSection = 2
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	if len(m.helpMatches) == 0 {
		t.Fatal("Hotkeys reported no matches for a needle it does contain")
	}
}

// TestTUIHelpOverviewAndScoreSourcesAreItemizedAndKeepLoadBearingPrecision
// locks two things about the Overview and Score Sources bodies now that
// their prose is broken into "- " bulleted points instead of run-on
// paragraphs: the itemized shape is actually present (guards against a
// future edit silently flattening the bullets back into paragraphs), and
// every precision-critical qualifier from the original prose survived the
// reformat word-for-word. These exact phrases are the ones a sloppy rewrite
// could plausibly soften first (see git history / task notes): swebench.com's
// median-of-scaffolds, one-vote-per-scaffold mechanics, the exact_product /
// variant_mismatch / !variant identity-gate vocabulary, and LMArena Elo's
// Bradley-Terry, roughly-950-1550 crowd-preference caveat.
func TestTUIHelpOverviewAndScoreSourcesAreItemizedAndKeepLoadBearingPrecision(t *testing.T) {
	if !strings.Contains(tuiHelpSectionOverviewBody, "\n- ") {
		t.Fatalf("Overview body is not itemized with \"- \" bullets: %q", tuiHelpSectionOverviewBody)
	}
	if !strings.Contains(tuiHelpSectionScoreSourcesBody, "\n- ") {
		t.Fatalf("Score Sources body is not itemized with \"- \" bullets: %q", tuiHelpSectionScoreSourcesBody)
	}
	// Bullets are hand-wrapped onto multiple physical source lines (see the
	// wrapping note above tuiHelpSectionScoreSourcesBody), so a
	// precision-critical phrase can legitimately straddle a line break.
	// Flatten whitespace before matching so this check tracks wording, not
	// wrap points.
	flatten := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	overviewFlat := flatten(tuiHelpSectionOverviewBody)
	scoreFlat := flatten(tuiHelpSectionScoreSourcesBody)
	for _, want := range []string{
		"exact_product",
		"variant_mismatch",
		"!variant (for example vals!variant=some/other-checkpoint)",
		"model-map.tsv",
	} {
		if !strings.Contains(overviewFlat, want) {
			t.Errorf("Overview body lost load-bearing phrase %q after itemization", want)
		}
	}
	for _, want := range []string{
		`one vote per scaffold; a resubmission of the same scaffold replaces it rather than adding a second vote`,
		`the row's own text says "median of N scaffolds" when that happened`,
		"Bradley-Terry, roughly 950-1550",
		"vals.ai wins whenever it has a usable, identity-checked row for a model; swebench.com is used only as a fallback, when vals.ai has no row at all or its row fails the identity check",
	} {
		if !strings.Contains(scoreFlat, want) {
			t.Errorf("Score Sources body lost load-bearing phrase %q after itemization", want)
		}
	}
}

// TestTUIHelpMethodologySectionCoversRankingPrinciples locks the sixth F1
// help section (Methodology) to the load-bearing facts this project's rating
// honesty rests on: the three independent kinds of data behind every row,
// the model-map.tsv identity gate and its !variant escape hatch, the
// vals.ai/swebench.com/LMArena three-way split and vals.ai's priority, the
// mixed-utility formula's exact constants, and the identity states that
// exclude a row from ranking despite carrying a real number. It is itemized
// with "- " bullets like Overview and Score Sources, and it points back to
// docs/methodology.md for the fuller write-up.
func TestTUIHelpMethodologySectionCoversRankingPrinciples(t *testing.T) {
	if !strings.Contains(tuiHelpSectionMethodologyBody, "\n- ") {
		t.Fatalf("Methodology body is not itemized with \"- \" bullets: %q", tuiHelpSectionMethodologyBody)
	}
	// The body's price_weight=10 claim is checked against the real constant,
	// not just copy-of-the-constant prose, so the two cannot silently drift
	// apart if ranking.DefaultPriceWeight is ever changed.
	if ranking.DefaultPriceWeight != 10 {
		t.Fatalf("ranking.DefaultPriceWeight = %v, want 10 to match the Methodology body's price_weight=10 claim (update the body text too if this is an intentional change)", ranking.DefaultPriceWeight)
	}
	flat := strings.Join(strings.Fields(tuiHelpSectionMethodologyBody), " ")
	for _, want := range []string{
		"docs/methodology.md",
		"Price and context come live from the OpenRouter catalogue.",
		"Tier is a hand-assigned, Claude-relative capability estimate from model-map.tsv, set by a human, not computed from the score.",
		"model-map.tsv is the only path to a score",
		"exact_product",
		"marked !variant on the source name (for example vals!variant=some/other-checkpoint)",
		"variant_mismatch",
		"vals.ai runs every model itself on one fixed, independent harness",
		"swebench.com is a self-submitted leaderboard",
		"it is only a fallback, used when vals.ai has no row at all or its row fails identity",
		"Bradley-Terry, roughly 950-1550",
		"never shown alongside SWE-bench",
		"tier-priority: rankable models first, then Opus, Sonnet, Haiku, score, and Q/P.",
		"score + price_weight*tier_factor*ln(1+quality_price)",
		"price_weight=10, factors Opus=1, Sonnet=1, Haiku=0.5, Free=0",
		"3:1 input:output blend: (3*input+output)/4 per $/M tokens",
		"Task-fit is never a multiplier.",
		"missing_identity",
		"legacy_unknown",
		"observation-only override, which is a vendor claim rather than an independent measurement",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("Methodology body is missing load-bearing phrase %q", want)
		}
	}
}

// TestTUIHelpTabBarShowsAllSixSectionsWithActiveHighlighted is the direct
// check for the confirmed tab-bar format: "[1 Overview] 2 Score Sources
// 3 Hotkeys 4 Filters 5 Model Detail 6 Methodology", the bracketed entry
// matching the active section, and that entry (and only that entry) styled
// distinctly (tuiSelectedStyle, the same style the main table's selected
// row uses).
func TestTUIHelpTabBarShowsAllSixSectionsWithActiveHighlighted(t *testing.T) {
	if len(tuiHelpSections) != 6 {
		t.Fatalf("tuiHelpSections has %d entries, want 6", len(tuiHelpSections))
	}
	wantTitles := []string{"Overview", "Score Sources", "Hotkeys", "Filters", "Model Detail", "Methodology"}
	for i, want := range wantTitles {
		if tuiHelpSections[i].Title != want {
			t.Fatalf("tuiHelpSections[%d].Title = %q, want %q", i, tuiHelpSections[i].Title, want)
		}
	}
	for active := range tuiHelpSections {
		plain := tuiHelpTabBarLine(active)
		for i, section := range tuiHelpSections {
			label := fmt.Sprintf("%d %s", i+1, section.Title)
			if i == active {
				label = "[" + label + "]"
			}
			if !strings.Contains(plain, label) {
				t.Fatalf("active=%d: tab bar %q is missing label %q", active, plain, label)
			}
		}
		tuiForceColorProfile(t)
		m := tuiModel{overlay: "help", helpSection: active, width: 200, height: 10}
		view := tuiHelpView(m)
		wantStyled := tuiSelectedStyle.Render("[" + fmt.Sprintf("%d %s", active+1, tuiHelpSections[active].Title) + "]")
		if !strings.Contains(view, wantStyled) {
			t.Fatalf("active=%d: rendered help does not style the active tab with tuiSelectedStyle:\n%s", active, view)
		}
		for i, section := range tuiHelpSections {
			if i == active {
				continue
			}
			styledOther := tuiSelectedStyle.Render(fmt.Sprintf("%d %s", i+1, section.Title))
			if strings.Contains(view, styledOther) {
				t.Fatalf("active=%d: inactive tab %d (%s) is also styled with tuiSelectedStyle", active, i, section.Title)
			}
		}
	}
}

// tuiHelpSectionLineIndexContaining returns the index of the first line in
// section's full rendered lines (title/tab-bar/body, exactly what
// tuiHelpView renders for it — see tuiHelpSectionLines) containing needle,
// or -1. The sectioned-help counterpart of tuiHelpLineIndexContaining,
// which searches the legacy, unsectioned whole-document view instead.
func tuiHelpSectionLineIndexContaining(section int, needle string) int {
	for i, line := range tuiHelpSectionLines(section) {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func TestTUIHelpSearchHighlightsMatchesWithoutChangingLayout(t *testing.T) {
	tuiForceColorProfile(t)
	// helpSection 2 (Hotkeys) is where both "column"/"columns" and
	// "match"/"matches" occur — search is section-scoped now.
	m := tuiModel{overlay: "help", helpSection: 2, width: 200, height: 60, helpSearch: "column"}
	view := tuiHelpView(m)
	lines := strings.Split(view, "\n")
	columnLine := tuiHelpSectionLineIndexContaining(2, `\tSpace\tcolumns\ttoggle a column.`)
	if columnLine < 0 || lines[columnLine] == ansi.Strip(lines[columnLine]) {
		t.Fatalf("matching line was not styled: %q", lines[columnLine])
	}
	if got := ansi.Strip(lines[columnLine]); got != tuiFormatHelpLine(tuiHelpSectionLines(2)[columnLine], m.width) {
		t.Fatalf("matching line changed: got %q, want %q", got, tuiFormatHelpLine(tuiHelpSectionLines(2)[columnLine], m.width))
	}
	if got := strings.Count(lines[columnLine], tuiMatchStyle.Render("column")); got != 2 {
		t.Fatalf("matching line has %d styled occurrences, want 2: %q", got, lines[columnLine])
	}

	m.helpSearch = "match"
	m.height = len(tuiHelpLines()) + 2
	lines = strings.Split(tuiHelpView(m), "\n")
	matchLine := -1
	for i, line := range lines {
		if strings.Count(line, tuiMatchStyle.Render("match")) == 2 {
			matchLine = i
			break
		}
	}
	if matchLine < 0 {
		t.Fatalf("match line was not highlighted: %q", lines)
	}
	footer := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(ansi.Strip(lines[i]), "Help ") {
			footer = lines[i]
			break
		}
	}
	if !strings.Contains(ansi.Strip(footer), `"match"`) || footer != ansi.Strip(footer) {
		t.Fatalf("footer was not kept plain: %q", footer)
	}

	// "Help search" is a Hotkeys-section heading (still section 2); "Columns,
	// search, and filters" is the Filters section's own heading (index 3) —
	// the two used to share one page and one render, but now live in
	// different sections and need their own render each.
	m.helpSearch = "search"
	lines = strings.Split(tuiHelpView(m), "\n")
	if index := tuiHelpSectionLineIndexContaining(2, "Help search"); index < 0 {
		t.Fatal("Hotkeys section is missing the Help search heading")
	} else if want := tuiHeaderStyle.Render(tuiHelpSectionLines(2)[index]); lines[index] != want {
		t.Fatalf("heading line %d = %q, want %q", index, lines[index], want)
	}
	if index := tuiHelpSectionLineIndexContaining(2, `/\tsearch`); index < 0 || lines[index] == ansi.Strip(lines[index]) {
		t.Fatalf("content line %d was not highlighted: %q", index, lines)
	}

	m.helpSection = 3
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, tuiHelpSectionLines(3))
	lines = strings.Split(tuiHelpView(m), "\n")
	if index := tuiHelpSectionLineIndexContaining(3, "Columns, search, and filters"); index < 0 {
		t.Fatal("Filters section is missing the Columns, search, and filters heading")
	} else if want := tuiHeaderStyle.Render(tuiHelpSectionLines(3)[index]); lines[index] != want {
		t.Fatalf("heading line %d = %q, want %q", index, lines[index], want)
	}

	m.helpSearch, m.inputMode, m.input = "search", "help-search", "search"
	lines = strings.Split(tuiHelpView(m), "\n")
	foundInput := false
	for _, line := range lines {
		if ansi.Strip(line) == "/ search_" {
			foundInput = true
			if line != ansi.Strip(line) {
				t.Fatalf("input line was highlighted: %q", line)
			}
		}
	}
	if !foundInput {
		t.Fatal("input line was not found in rendered help")
	}
}

func TestTUIHelpSearchPreservesDisplayCaseAndNormalizesNeedle(t *testing.T) {
	tuiForceColorProfile(t)
	// "Task-fit codes" lives in the Hotkeys section (index 2).
	m := tuiModel{overlay: "help", helpSection: 2, width: 200, height: 60, helpSearch: "task-fit"}
	lines := strings.Split(tuiHelpView(m), "\n")
	codeLine := tuiHelpSectionLineIndexContaining(2, "Task-fit codes")
	rowLine := tuiHelpSectionLineIndexContaining(2, `\tI\ttask-fit code`)
	if !strings.Contains(lines[rowLine], tuiMatchStyle.Render("task-fit")) {
		t.Fatalf("display case was not preserved: code=%q, row=%q", lines[codeLine+1], lines[rowLine])
	}
	headingLine := tuiHelpSectionLineIndexContaining(2, "Task-fit")
	if want := tuiHeaderStyle.Render("Task-fit codes"); lines[headingLine] != want {
		t.Fatalf("heading was not kept intact: got %q, want %q", lines[headingLine], want)
	}
	for _, needle := range []string{"TASK-FIT", "Task-Fit", "task-fit"} {
		if got := tuiHelpSearch(needle); !reflect.DeepEqual(got, tuiHelpSearch("task-fit")) {
			t.Fatalf("tuiHelpSearch(%q) = %v, want %v", needle, got, tuiHelpSearch("task-fit"))
		}
	}
}

func TestTUIHelpSearchWithEmptyNeedleDoesNotHighlightContent(t *testing.T) {
	tuiForceColorProfile(t)
	for _, needle := range []string{"", "   "} {
		m := tuiModel{overlay: "help", width: 200, height: 60, helpSearch: needle}
		view := tuiHelpView(m)
		assertTUIViewFits(t, view, m.width, m.height, "help without search")
		// The tab bar (e.g. "[1 Overview] 2 Score Sources ...") is always
		// styled too — tuiSelectedStyle marks the active section — the same
		// way the title line always is, regardless of the search needle.
		tabBar := tuiHelpTabBarLine(m.helpSection)
		for index, line := range strings.Split(view, "\n") {
			plain := ansi.Strip(line)
			isHeader := strings.HasPrefix(plain, "omt tui ") || strings.HasSuffix(plain, "keys") || plain == "Hotkeys" || plain == "Navigation" || plain == "Data/view" || plain == "Filters/settings" || plain == "Task-fit codes" || plain == "General/help" || strings.HasSuffix(plain, "view") || strings.HasSuffix(plain, "filters") || strings.HasSuffix(plain, "finish") || strings.HasSuffix(plain, "search") || plain == tabBar
			if !isHeader && line != plain {
				t.Fatalf("empty needle styled content line %d for %q: %q", index, needle, line)
			}
		}
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
	if got := tuiCell(row, colClaude, false, scoreSourceArena); got != "n/a" {
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
	if got := strings.TrimSpace(cells[1]); got != "n/a" {
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
		"Производитель: 🌀 OpenAI (C)",
		"Провайдер: n/a",
		"Лицензия: нет",
		"Описание:",
		"Тир: opus",
		"Claude-референс: ≈ Opus 4.6",
		"Дата релиза: 2026-08-06 (2 месяца назад); дата создания записи каталога, релиз неизвестен",
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

	order := []string{"GPT-5.6 Luna", "Производитель:", "Провайдер:", "Лицензия:", "Тир:", "Task fit:", "-- Pricing --", "Контекст:", "Открытые веса:", "Оценка SWE-bench", "Оценка LMArena", "Дата релиза:", "Описание:", "Заметка:"}
	previous := -1
	for _, prefix := range order {
		index := tuiDetailIndex(t, lines, prefix)
		if index <= previous {
			t.Fatalf("block %q is at line %d, out of order against the previous block at %d:\n%s", prefix, index, previous, joined)
		}
		previous = index
	}
}

func TestTUIDetailPricingShowsOnlyRecordedPriceChanges(t *testing.T) {
	base := pricehistory.Price{Found: true, InPerM: 0.5, OutPerM: 3, Context: 1000000}
	changed := base
	changed.HasOverride = true
	changed.OverrideMinTokens = 272000
	changed.OverrideInPerM = 1
	changed.OverrideOutPerM = 4
	history := &pricehistory.History{Observations: []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{"openai/gpt-5.6-luna": base}},
		{ObservedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{"openai/gpt-5.6-luna": changed}},
	}}
	lines := tuiDetailLinesWithHistory(tuiDetailTestModel(), scoreSourceSWEBench, 100, time.Now(), history)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Динамика цен:") || !strings.Contains(joined, "2026-08-02: $0.5/$3, 1000K -> $0.5/$3, 1000K; long-context $1/$4 от 272K+") {
		t.Fatalf("pricing history is missing the recorded change:\n%s", joined)
	}
	withoutHistory := strings.Join(tuiDetailLines(tuiDetailTestModel(), scoreSourceSWEBench, 100, time.Now()), "\n")
	if strings.Contains(withoutHistory, "Динамика цен:") {
		t.Fatalf("detail invented pricing history without observations:\n%s", withoutHistory)
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

func TestTUIDetailLinesKeepCanonicalTierMetadataWithoutDuplicates(t *testing.T) {
	lines := tuiDetailLines(tuiDetailTestModel(), scoreSourceSWEBench, 100, time.Now())
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Тир: opus", "Claude-референс: ≈ Opus 4.6"} {
		if strings.Count(joined, want) != 1 {
			t.Fatalf("canonical detail metadata %q appears %d times:\n%s", want, strings.Count(joined, want), joined)
		}
	}
	for _, duplicate := range []string{"Capability estimate / Claude tier:", "Tier estimate:"} {
		if strings.Contains(joined, duplicate) {
			t.Fatalf("detail metadata still contains duplicate label %q:\n%s", duplicate, joined)
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
	if !strings.Contains(block, "n/a") {
		t.Errorf("the arena-mode SWE-bench block must say н/д instead of borrowing the other scale:\n%s", block)
	}
	if !strings.Contains(strings.Join(lines[arena:], "\n"), "1453 Elo") {
		t.Errorf("the Arena block lost its Elo label in arena mode:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTUIDetailLinesFallBackToThePlaceholder(t *testing.T) {
	lines := tuiDetailLines(model.Model{Slug: "a/bare"}, scoreSourceSWEBench, 60, time.Now())
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Производитель: ❔", "Провайдер: n/a", "Лицензия: n/a", "Описание:", "Тир: n/a", "Claude-референс: n/a", "Дата релиза: n/a", "Страница OpenRouter: https://openrouter.ai/a/bare", "Контекст: n/a", "Открытые веса: n/a", "Task fit: n/a"} {
		if !strings.Contains(joined, want) {
			t.Errorf("an empty model is missing the placeholder line %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Длинный контекст") {
		t.Errorf("a model without a long-context tier must not get that block at all:\n%s", joined)
	}
	if lines[len(lines)-1] != "  n/a" {
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
	if got := tuiDetailCreated(0, published); got != "n/a" {
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

func TestTUIDetailOffsetClampsAfterRefreshAndResize(t *testing.T) {
	row := tuiDetailTestModel()
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.overlay, m.width, m.height, m.detailOffset = "detail", 30, 10, 10000
	m.generation = 1
	next, _ := m.Update(tuiRefreshMsg{generation: 1, scoreSourceGeneration: m.scoreSourceGeneration, models: []model.Model{row}})
	m = next.(tuiModel)
	if maxOffset := tuiDetailMaxOffset(row, m.scoreSource, m.width, m.height); m.detailOffset != maxOffset {
		t.Fatalf("detail offset after refresh = %d, want %d", m.detailOffset, maxOffset)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 1000})
	m = next.(tuiModel)
	if m.detailOffset != 0 {
		t.Fatalf("detail offset after widening and enlarging viewport = %d, want 0", m.detailOffset)
	}
}

func TestTUIDetailScrollAccountsForPricingHistory(t *testing.T) {
	row := tuiDetailTestModel()
	base := pricehistory.Price{Found: true, InPerM: row.InPerM, OutPerM: row.OutPerM, Context: row.Context}
	changed := base
	changed.OutPerM = 4
	history := &pricehistory.History{Observations: []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: base}},
		{ObservedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: changed}},
	}}
	lines := tuiDetailLinesWithHistory(row, scoreSourceSWEBench, 30, time.Now(), history)
	want := max(0, len(lines)-tuiDetailBodyHeight(10))
	if got := tuiDetailMaxOffsetWithHistory(row, scoreSourceSWEBench, 30, 10, history); got != want {
		t.Fatalf("history-aware max offset = %d, want %d", got, want)
	}
}

func TestTUIDetailGroupsDataAndAlignsWideRows(t *testing.T) {
	lines := tuiDetailLines(tuiDetailTestModel(), scoreSourceSWEBench, 200, time.Now())
	joined := strings.Join(lines, "\n")
	for _, marker := range []string{"-- Identity --", "-- Pricing --", "-- Benchmarks --", "-- Fit and notes --"} {
		if !strings.Contains(joined, marker) {
			t.Fatalf("detail view is missing section marker %q:\n%s", marker, joined)
		}
	}
	positions := make([]int, 0, 3)
	for _, label := range []string{"Производитель", "Дата релиза", "Контекст"} {
		for _, line := range lines {
			if strings.HasPrefix(line, label+": ") {
				valueStart := strings.Index(line, ": ") + 2
				valueStart += len(line[valueStart:]) - len(strings.TrimLeft(line[valueStart:], " "))
				positions = append(positions, tableDisplayWidth(line[:valueStart]))
				break
			}
		}
	}
	if len(positions) != 3 || positions[0] != positions[1] || positions[1] != positions[2] {
		t.Fatalf("wide detail rows are not aligned: positions=%v\n%s", positions, joined)
	}
}

func TestTUIDetailKeepsPlainContentAtNarrowWidths(t *testing.T) {
	row := tuiDetailTestModel()
	row.Description = "Описание с | markdown **маркерами** и управляющим\nсимволом"
	for _, width := range []int{1, 5, 20, 40} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
		m.overlay, m.width, m.height = "detail", width, 24
		view := ansi.Strip(m.View())
		for _, line := range strings.Split(view, "\n") {
			if tableDisplayWidth(line) > width {
				t.Fatalf("width=%d: rendered detail line exceeds width: %q (%d)", width, line, tableDisplayWidth(line))
			}
		}
		if strings.Contains(view, "**") || strings.Contains(view, "|") {
			t.Fatalf("width=%d: unsanitized detail content: %q", width, view)
		}
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
	if lines[hugging+1] != "Описание:" {
		t.Errorf("line after the links = %q, want the deferred description block", lines[hugging+1])
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
	if !strings.Contains(joined, "Страница OpenRouter: https://openrouter.ai/"+row.Slug) {
		t.Errorf("an empty canonical slug must fall back to the stable model id URL:\n%s", joined)
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
		{"label and placeholder", "Дата релиза: n/a", tuiHeaderStyle.Render("Дата релиза: ") + tuiHintStyle.Render("n/a")},
		{"provenance link", "  Источник: https://www.vals.ai/benchmarks/swebench", tuiHeaderStyle.Render("  Источник: ") + tuiLinkStyle.Render("https://www.vals.ai/benchmarks/swebench")},
		{"model link", "Страница OpenRouter: https://openrouter.ai/openai/gpt-5.6-luna-20260804", tuiHeaderStyle.Render("Страница OpenRouter: ") + tuiLinkStyle.Render("https://openrouter.ai/openai/gpt-5.6-luna-20260804")},
		{"bare placeholder", "  n/a", tuiHintStyle.Render("  n/a")},
		{"qualified placeholder", "  n/a (active arena view)", tuiHintStyle.Render("  n/a (active arena view)")},
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
	plainView := ansi.Strip(view)
	for _, url := range []string{
		"https://openrouter.ai/openai/gpt-5.6-luna-20260804",
		"https://huggingface.co/openai-community/gpt-5-6-luna",
		"https://www.vals.ai/benchmarks/swebench",
	} {
		if !strings.Contains(plainView, url) {
			t.Errorf("the view does not carry %q:\n%s", url, plainView)
		}
	}
	if !strings.Contains(view, "38;5;74") || strings.Contains(view, "38;5;81") {
		t.Errorf("the view does not use the calm link palette 74 exclusively:\n%s", view)
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

func TestTUIDetailNavigationUsesHistoryAwareBounds(t *testing.T) {
	row := tuiDetailTestModel()
	base := pricehistory.Price{Found: true, InPerM: row.InPerM, OutPerM: row.OutPerM, Context: row.Context}
	changed := base
	changed.OutPerM = 4
	history := &pricehistory.History{Observations: []pricehistory.Observation{
		{Prices: map[string]pricehistory.Price{row.Slug: base}},
		{Prices: map[string]pricehistory.Price{row.Slug: changed}},
	}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.priceHistory, m.width, m.height, m.overlay = history, 60, 10, "detail"
	maxOffset := tuiDetailMaxOffsetWithHistory(row, m.scoreSource, m.width, m.height, history)
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyPgDown}, {Type: tea.KeyEnd}} {
		m, _ = m.key(msg)
	}
	if m.detailOffset != maxOffset {
		t.Fatalf("detail navigation offset = %d, want history-aware maximum %d", m.detailOffset, maxOffset)
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
	// "Model detail view" lives in its own section now ("Model Detail",
	// index 4) rather than sharing the page with everything else.
	m.overlay, m.helpSection, m.width, m.height = "help", 4, 120, len(tuiHelpLines())+2
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
	if err := os.WriteFile(filepath.Join(root, "model-snapshot.json"), body, 0o644); err != nil {
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

// TestTUILayoutAliasesAreWellFormed сверяет таблицу раскладочных алиасов с
// golden-списком, заданным численными кодовыми точками. Численные литералы
// здесь принципиальны: часть кириллических рун визуально неотличима от
// латинских ('с' U+0441 против 'c', 'р' U+0440 против 'p', 'к' U+043A против
// 'k', 'о' U+043E против 'o', 'а' U+0430 против 'a'), поэтому сверка «руна с
// руной» не поймала бы омоглифную опечатку — она бы её повторила.
func TestTUILayoutAliasesAreWellFormed(t *testing.T) {
	golden := map[rune]string{
		0x0447: "x", // ч
		0x043B: "k", // л
		0x043E: "j", // о
		0x0449: "o", // щ
		0x043F: "g", // п
		0x041F: "G", // П
		0x0440: "h", // р
		0x0434: "l", // д
		0x044B: "s", // ы
		0x042B: "S", // Ы
		0x044C: "m", // ь
		0x0441: "c", // с
		0x0442: "n", // т
		0x0430: "f", // а
		0x0439: "q", // й
		0x0437: "p", // з
		0x043A: "r", // к
		0x041A: "R", // К
		0x002E: "/", // .
		0x002C: "?", // ,
	}
	if len(tuiLayoutAliases) != len(golden) {
		t.Fatalf("alias table has %d entries, want the %d golden pairs", len(tuiLayoutAliases), len(golden))
	}
	for code, want := range golden {
		got, ok := tuiLayoutAliases[code]
		if !ok {
			t.Fatalf("alias table has no entry for U+%04X (%q), want the command %q", code, code, want)
		}
		if got != want {
			t.Fatalf("alias table maps U+%04X (%q) to %q, want %q", code, code, got, want)
		}
	}
	commands := map[string]rune{}
	for alias, command := range tuiLayoutAliases {
		if alias < utf8.RuneSelf && unicode.IsLetter(alias) {
			t.Fatalf("alias %q (U+%04X) is an ASCII letter; a Latin letter must never be an alias key", alias, alias)
		}
		if unicode.IsLetter(alias) && !unicode.Is(unicode.Cyrillic, alias) {
			t.Fatalf("alias %q (U+%04X) is a letter outside the Cyrillic block", alias, alias)
		}
		if len(command) != 1 || command[0] >= utf8.RuneSelf {
			t.Fatalf("alias %q maps to %q, want a single ASCII character", alias, command)
		}
		if previous, ok := commands[command]; ok {
			t.Fatalf("aliases %q and %q both map to the command %q", previous, alias, command)
		}
		commands[command] = alias
		if got := tuiCommandKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(command)}); got != command {
			t.Fatalf("tuiCommandKey is not idempotent on the command %q: it returned %q", command, got)
		}
	}
}

// tuiShortcutState — явный снимок полей модели, которые вообще способны
// измениться от нажатия хоткея. Сравнивать tuiModel целиком через
// reflect.DeepEqual нельзя: она несёт context.Context и ranking.Compiled, где
// сравнение скомпилированной формулы ненадёжно.
//
// Критерий включения поля: сюда попадают только поля, которые меняет хотя бы
// один кейс из tuiShortcutCases() сегодня — например, pendingColumns,
// helpMatches, helpMatch, scoreSource и updatedAt сюда намеренно не входят.
// Новый кейс, единственный эффект которого приходится на поле вне этого
// списка, пройдёт тест впустую (снимки совпадут, потому что поле не
// снимается) — сначала расширь эту структуру и tuiShortcutSnapshot, потом
// добавляй кейс.
type tuiShortcutState struct {
	cursor       int
	selectedSlug string
	sortKey      string
	reverse      bool
	ranking      string
	overlay      string
	inputMode    string
	input        string
	filter       string
	helpSearch   string
	helpOffset   int
	detailOffset int
	columnCursor int
	lastNote     bool
	refreshing   bool
	generation   uint64
	status       string
	err          string
	visible      []string
	columns      []tuiColumn
}

func tuiShortcutSnapshot(m tuiModel) tuiShortcutState {
	slugs := make([]string, 0, len(m.visible))
	for _, row := range m.visible {
		slugs = append(slugs, row.Slug)
	}
	return tuiShortcutState{
		cursor: m.cursor, selectedSlug: m.selectedSlug, sortKey: m.sortKey, reverse: m.reverse,
		ranking: m.ranking, overlay: m.overlay, inputMode: m.inputMode, input: m.input,
		filter: m.filter, helpSearch: m.helpSearch, helpOffset: m.helpOffset,
		detailOffset: m.detailOffset, columnCursor: m.columnCursor, lastNote: m.lastNote,
		refreshing: m.refreshing, generation: m.generation, status: m.status, err: m.err,
		visible: slugs, columns: append([]tuiColumn(nil), m.columns...),
	}
}

func tuiShortcutRows() []model.Model {
	return []model.Model{
		{Slug: "alpha", DisplayName: "Alpha", QualityPriceLabel: "9.0", Score: &model.ScoreInfo{Value: 9}, Rankable: true, MixedPrice: 10, QualityPrice: 9},
		{Slug: "beta", DisplayName: "Beta", QualityPriceLabel: "4.0", Score: &model.ScoreInfo{Value: 4}, Rankable: true, MixedPrice: 4, QualityPrice: 4},
		{Slug: "gamma", DisplayName: "Gamma", QualityPriceLabel: "1.0", Score: &model.ScoreInfo{Value: 1}, Rankable: true, MixedPrice: 1, QualityPrice: 1},
	}
}

func tuiShortcutListModel(t *testing.T) tuiModel {
	t.Helper()
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, tuiShortcutRows())
	m.width, m.height = 120, 20
	return m
}

// tuiShortcutListModelAtBottom нужен клавишам «вверх» и «в начало»: с курсором
// на нулевой строке они не меняют ничего, и проверка стала бы вырожденной.
func tuiShortcutListModelAtBottom(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutListModel(t)
	m.cursor = len(m.visible) - 1
	return m
}

// tuiShortcutListModelSortedByName нужен клавише r: она выставляет сортировку
// "q/p", которая и так стоит по умолчанию.
func tuiShortcutListModelSortedByName(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutListModel(t)
	m.sortKey = "name"
	return m
}

func tuiShortcutDetailModel(t *testing.T) tuiModel {
	t.Helper()
	row := tuiDetailTestModel()
	row.Description = strings.Repeat("длинное вендорское описание модели ", 20)
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.width, m.height = 60, 10
	if tuiDetailMaxOffset(row, m.scoreSource, m.width, m.height) < 2 {
		t.Fatal("test setup: the detail fixture must be at least two lines taller than the viewport")
	}
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEnter})
	if m.overlay != "detail" {
		t.Fatalf("test setup: the detail overlay did not open, overlay=%q", m.overlay)
	}
	return m
}

func tuiShortcutDetailModelScrolled(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutDetailModel(t)
	m.detailOffset = 2
	return m
}

func tuiShortcutHelpModel(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutListModel(t)
	m.overlay, m.helpOffset = "help", 0
	if tuiHelpMaxOffset(m.height) < 5 {
		t.Fatalf("test setup: the help document must be scrollable at height %d", m.height)
	}
	return m
}

func tuiShortcutHelpModelScrolled(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutHelpModel(t)
	m.helpOffset = 5
	return m
}

func tuiShortcutColumnsModel(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutListModel(t)
	m.overlay, m.pendingColumns, m.columnCursor = "columns", append([]tuiColumn(nil), m.columns...), 0
	return m
}

func tuiShortcutColumnsModelScrolled(t *testing.T) tuiModel {
	t.Helper()
	m := tuiShortcutColumnsModel(t)
	m.columnCursor = 2
	return m
}

type tuiShortcutCase struct {
	name    string
	latin   string
	russian string
	setup   func(t *testing.T) tuiModel
}

// tuiShortcutCases покрывает все 20 алиасов таблицы tuiLayoutAliases; клавиши
// навигации, живущие сразу в нескольких switch-блоках, получают строку на
// каждый контекст, потому что именно там ломается «алиас есть, но не во всех
// оверлеях».
func tuiShortcutCases() []tuiShortcutCase {
	return []tuiShortcutCase{
		{name: "list quit", latin: "x", russian: "ч", setup: tuiShortcutListModel},
		{name: "detail close via x", latin: "x", russian: "ч", setup: tuiShortcutDetailModelScrolled},
		{name: "help close via x", latin: "x", russian: "ч", setup: tuiShortcutHelpModelScrolled},
		{name: "columns close via x", latin: "x", russian: "ч", setup: tuiShortcutColumnsModelScrolled},
		{name: "list cursor down", latin: "j", russian: "о", setup: tuiShortcutListModel},
		{name: "list cursor up", latin: "k", russian: "л", setup: tuiShortcutListModelAtBottom},
		{name: "list jump home", latin: "g", russian: "п", setup: tuiShortcutListModelAtBottom},
		{name: "list jump end", latin: "G", russian: "П", setup: tuiShortcutListModel},
		{name: "list open detail", latin: "l", russian: "д", setup: tuiShortcutListModel},
		{name: "list cycle sort key", latin: "s", russian: "ы", setup: tuiShortcutListModel},
		{name: "list reverse order", latin: "S", russian: "Ы", setup: tuiShortcutListModel},
		{name: "list toggle ranking", latin: "m", russian: "ь", setup: tuiShortcutListModel},
		{name: "list open columns", latin: "c", russian: "с", setup: tuiShortcutListModel},
		{name: "list toggle last column", latin: "n", russian: "т", setup: tuiShortcutListModel},
		{name: "list edit filter", latin: "f", russian: "а", setup: tuiShortcutListModel},
		{name: "list sort by quality", latin: "q", russian: "й", setup: tuiShortcutListModelSortedByName},
		{name: "list sort by price", latin: "p", russian: "з", setup: tuiShortcutListModelSortedByName},
		{name: "list sort by quality-price", latin: "r", russian: "к", setup: tuiShortcutListModelSortedByName},
		{name: "list refresh", latin: "R", russian: "К", setup: tuiShortcutListModel},
		{name: "list open search", latin: "/", russian: ".", setup: tuiShortcutListModel},
		{name: "list open help", latin: "?", russian: ",", setup: tuiShortcutListModel},
		{name: "list open settings", latin: "o", russian: "щ", setup: tuiShortcutListModel},
		{name: "detail close", latin: "h", russian: "р", setup: tuiShortcutDetailModel},
		{name: "detail scroll down", latin: "j", russian: "о", setup: tuiShortcutDetailModel},
		{name: "detail scroll up", latin: "k", russian: "л", setup: tuiShortcutDetailModelScrolled},
		{name: "detail scroll home", latin: "g", russian: "п", setup: tuiShortcutDetailModelScrolled},
		{name: "detail scroll end", latin: "G", russian: "П", setup: tuiShortcutDetailModel},
		{name: "help scroll down", latin: "j", russian: "о", setup: tuiShortcutHelpModel},
		{name: "help scroll up", latin: "k", russian: "л", setup: tuiShortcutHelpModelScrolled},
		{name: "help scroll home", latin: "g", russian: "п", setup: tuiShortcutHelpModelScrolled},
		{name: "help scroll end", latin: "G", russian: "П", setup: tuiShortcutHelpModel},
		{name: "help open search", latin: "/", russian: ".", setup: tuiShortcutHelpModel},
		{name: "help close", latin: "?", russian: ",", setup: tuiShortcutHelpModel},
		{name: "columns cursor down", latin: "j", russian: "о", setup: tuiShortcutColumnsModel},
		{name: "columns cursor up", latin: "k", russian: "л", setup: tuiShortcutColumnsModelScrolled},
	}
}

// TestTUIShortcutCasesCoverAllAliases сверяет множество русских рун, которые
// реально задействуют кейсы tuiShortcutCases(), со множеством ключей
// tuiLayoutAliases. TestTUILayoutAliasesAreWellFormed проверяет только
// количество и форму записей таблицы — она не заметит новый алиас, для
// которого забыли завести поведенческий кейс, а эта проверка заметит.
func TestTUIShortcutCasesCoverAllAliases(t *testing.T) {
	tested := map[rune]bool{}
	for _, test := range tuiShortcutCases() {
		runes := []rune(test.russian)
		if len(runes) != 1 {
			t.Fatalf("case %q has a non-single-rune russian key %q", test.name, test.russian)
		}
		tested[runes[0]] = true
	}
	for alias := range tuiLayoutAliases {
		if !tested[alias] {
			t.Errorf("alias %q (U+%04X) has no case in tuiShortcutCases()", alias, alias)
		}
	}
	for alias := range tested {
		if _, ok := tuiLayoutAliases[alias]; !ok {
			t.Errorf("tuiShortcutCases() exercises russian rune %q (U+%04X), which is not a key in tuiLayoutAliases", alias, alias)
		}
	}
}

// TestTUIRussianLayoutShortcutsMatchLatin доказывает не «клавиша что-то
// сделала», а «RU-ветка привела модель ровно в то же состояние, что и
// EN-ветка». Такое утверждение переживает любое будущее изменение смысла
// хоткея: тест не придётся переписывать вслед за поведением.
func TestTUIRussianLayoutShortcutsMatchLatin(t *testing.T) {
	for _, test := range tuiShortcutCases() {
		t.Run(test.name, func(t *testing.T) {
			before := tuiShortcutSnapshot(test.setup(t))
			latinModel, latinCmd := tuiKeyCmd(test.setup(t), test.latin)
			russianModel, russianCmd := tuiKeyCmd(test.setup(t), test.russian)
			latin, russian := tuiShortcutSnapshot(latinModel), tuiShortcutSnapshot(russianModel)
			if reflect.DeepEqual(latin, before) && latinCmd == nil {
				t.Fatalf("the Latin key %q changed neither state nor command in this setup, so the case proves nothing", test.latin)
			}
			if !reflect.DeepEqual(latin, russian) {
				t.Fatalf("keys %q (Latin) and %q (Russian) diverge:\n latin   = %+v\n russian = %+v", test.latin, test.russian, latin, russian)
			}
			if (latinCmd == nil) != (russianCmd == nil) {
				t.Fatalf("key %q returned a command = %v, but key %q returned a command = %v", test.latin, latinCmd != nil, test.russian, russianCmd != nil)
			}
		})
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
	if err := os.WriteFile(filepath.Join(root, "model-snapshot.json"), body, 0o644); err != nil {
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

// tuiViewHasPlainLine сообщает, есть ли в выводе отдельная строка,
// plain-текст которой в точности равен want. Проверять именно строку, а
// не strings.Contains по всему экрану, обязательно: документ справки сам
// содержит и слово refresh — строка "R refreshes the local data now." —
// и строку, начинающуюся с "/ ": "/ or . searches Name/Slug as plain
// substring text.". Поэтому Contains ловил бы текст документа вместо
// строки ввода. Равенство точное, без TrimRight: truncateTable не
// добивает строку пробелами до ширины.
func tuiViewHasPlainLine(view, want string) bool {
	for _, line := range strings.Split(view, "\n") {
		if ansi.Strip(line) == want {
			return true
		}
	}
	return false
}

// TestTUIHelpSearchRendersTheQueryWhileTyping — главный тест задачи и
// прямой страж пропущенного класса. Он утверждает о строках, которые
// вернул View() В МОМЕНТ набора (inputMode == "help-search", до Enter), а
// не о полях tuiModel после Enter, и идёт пользовательским путём:
// клавиша ? открывает справку, / включает поиск, руны едут через Update.
// Существующие проверки в TestTUIKeyState присваивают m.input напрямую и
// сразу жмут Enter, поэтому единственное состояние, в котором жил баг, на
// экран там ни разу не выводится.
func TestTUIHelpSearchRendersTheQueryWhileTyping(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", DisplayName: "A"}})
	m.width, m.height = 100, 24
	m = tuiKey(m, "?")
	if m.overlay != "help" {
		t.Fatalf("test setup: overlay = %q, want %q", m.overlay, "help")
	}
	m = tuiKey(m, "/")
	if m.inputMode != "help-search" {
		t.Fatalf("test setup: inputMode = %q, want %q", m.inputMode, "help-search")
	}
	view := m.View()
	assertTUIViewFits(t, view, m.width, m.height, "help typing")
	if !tuiViewHasPlainLine(view, "/ _") {
		t.Fatalf("сразу после / справка не показывает пустую строку ввода %q:\n%s", "/ _", view)
	}
	for _, step := range []struct{ key, want string }{
		{"r", "/ r_"},
		{"e", "/ re_"},
		{"f", "/ ref_"},
	} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(step.key)})
		m = next.(tuiModel)
		if m.inputMode != "help-search" || m.overlay != "help" {
			t.Fatalf("руна %q вышла из режима ввода: inputMode = %q, overlay = %q", step.key, m.inputMode, m.overlay)
		}
		view := m.View()
		assertTUIViewFits(t, view, m.width, m.height, "help typing")
		if !tuiViewHasPlainLine(view, step.want) {
			t.Fatalf("после руны %q справка не показывает строку ввода %q:\n%s", step.key, step.want, view)
		}
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(tuiModel)
	if m.inputMode != "" || m.helpSearch != "ref" {
		t.Fatalf("Enter не зафиксировал запрос: inputMode = %q, helpSearch = %q", m.inputMode, m.helpSearch)
	}
	view = m.View()
	assertTUIViewFits(t, view, m.width, m.height, "help after enter")
	for _, line := range strings.Split(view, "\n") {
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "/ ") && strings.HasSuffix(plain, "_") {
			t.Fatalf("вне режима help-search вывод содержит строку в форме ввода: %q\n%s", plain, view)
		}
	}
}

// TestTUIHelpSearchInputLineNotStyledWhenEndingInUnderscore — регрессионный
// тест для проверки, что строка ввода не стилизуется даже если её текст
// заканчивается на underscore. Ранее ошибка проявлялась так: когда
// пользователь вводил "search_", строка становилась "/ search__"
// (ввод + курсор), потом plainTableText удалял __ как markdown-bold,
// оставляя "/ search", который попадал под условие HasSuffix(plain, "search")
// и стилизовался заголовком. Тест проверяет, что этого не происходит.
func TestTUIHelpSearchInputLineNotStyledWhenEndingInUnderscore(t *testing.T) {
	tuiForceColorProfile(t)
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", DisplayName: "A"}})
	m.width, m.height = 100, 24
	m = tuiKey(m, "?")
	m = tuiKey(m, "/")
	// Type "search_" — это завершится на underscore, создав "/ search__"
	// после добавления маркера курсора.
	for _, ch := range "search_" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = next.(tuiModel)
	}
	if m.input != "search_" {
		t.Fatalf("test setup: input = %q, want %q", m.input, "search_")
	}
	view := m.View()
	assertTUIViewFits(t, view, m.width, m.height, "help typing")
	// После plainTableText обработки "/ search__" становится "/ search" (двойное
	// подчеркивание удаляется как markdown bold).
	// Проверяем, что эта строка присутствует в выводе.
	if !tuiViewHasPlainLine(view, "/ search") {
		t.Fatalf("input line %q not found in view:\n%s", "/ search", view)
	}
	// Теперь проверяем, что она не стилизована. Ранее bug был в том, что
	// "/ search" совпадает с условием HasSuffix(plain, "search") в цикле стилизации,
	// так что строка становилась tuiHeaderStyle.Render(...), что добавляет ANSI-коды.
	// С нашей фиксацией, строка ввода должна быть пропущена в цикле стилизации.
	styledLines := strings.Split(view, "\n")
	for _, styledLine := range styledLines {
		plain := ansi.Strip(styledLine)
		if plain == "/ search" {
			// Если есть ANSI-коды от стилизации, то styledLine != plain
			if styledLine != plain {
				t.Fatalf("input line was styled as heading (has escape codes):\nplain=%q\nstyled=%q", plain, styledLine)
			}
			return
		}
	}
	t.Fatalf("internal error: tuiViewHasPlainLine found the line but split by newline didn't")
}

// TestTUIHelpInputLineCostsExactlyOneContentRow — проверка off-by-one.
// Она сравнивает вывод справки во время набора с выводом той же справки
// без набора на том же helpOffset: контентные строки первого обязаны быть
// в точности первыми height-1 контентными строками второго, а последней
// строкой экрана обязана быть строка ввода. Ни потерянной строки
// документа, ни лишней пустой. Высоты подобраны так, чтобы на каждой из
// них строка ввода реально была видна — это требует раздела длиннее любой
// проверяемой высоты (59), иначе контент не заполняет экран и после него
// остаётся пустой хвост, а не строка ввода последней. Раздел "Hotkeys"
// (helpSection 2) — 86 строк с заголовком, заведомо длиннее;
// "Overview" (34 строки), напротив, короче большинства проверяемых высот и
// для этой проверки не годится.
func TestTUIHelpInputLineCostsExactlyOneContentRow(t *testing.T) {
	for _, height := range []int{2, 3, 5, 10, 24, 40, 59} {
		idle := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
		idle.overlay, idle.helpSection, idle.width, idle.height = "help", 2, 200, height
		idleLines := strings.Split(idle.View(), "\n")

		typing := idle
		typing.inputMode, typing.input = "help-search", "ref"
		view := typing.View()
		assertTUIViewFits(t, view, typing.width, typing.height, "help typing")
		typingLines := strings.Split(view, "\n")
		if len(typingLines) != height {
			t.Fatalf("height %d: справка во время набора отрисовала %d строк, want %d", height, len(typingLines), height)
		}
		if got := ansi.Strip(typingLines[height-1]); got != "/ ref_" {
			t.Fatalf("height %d: последняя строка экрана = %q, want строку ввода %q", height, got, "/ ref_")
		}
		for i := 0; i < height-1; i++ {
			got, want := ansi.Strip(typingLines[i]), ansi.Strip(idleLines[i])
			if got != want {
				t.Fatalf("height %d: контентная строка %d = %q во время набора, want %q — строка ввода обязана стоить ровно одну строку", height, i, got, want)
			}
		}
	}
}

// TestTUIHelpOverlayFitsSmallHeightsWhileTyping — зеркало
// TestTUIHelpOverlayFitsSmallHeights для режима набора. Оно закрывает два
// вырожденных случая, принятых спекой как есть: height == 1, где на экране
// физически одна строка и обратной связи быть не может, и helpOffset,
// выставленный далеко за конец документа клавишей G или helpNextMatch, —
// клэмп внутри tuiHelpView обязан удержать срез в границах, а не отдать
// пустой экран или запаниковать.
func TestTUIHelpOverlayFitsSmallHeightsWhileTyping(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4, 5, 6, 7} {
		// 0 is the very start, 999 clamps to the document's end; 5 is a
		// representative small non-zero offset that must land on a real
		// content row (not a blank paragraph separator) so the height==1
		// case below can assert the screen is never blank. m.helpSection
		// defaults to 0 (Overview) and is never changed by this test, so
		// the offset indexes into tuiHelpSectionOverviewBody's own lines
		// (see tuiHelpSectionLines), not the Hotkeys section — index 5 is
		// the Overview section's first bullet line ("- Quality comes
		// from..."). This index moves whenever the Overview body is
		// reflowed; pick a fresh non-blank index against the current body
		// if this assertion starts failing again.
		for _, offset := range []int{0, 5, 999} {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
			m.overlay, m.width, m.height = "help", 40, height
			m.inputMode, m.input, m.helpOffset = "help-search", "refresh", offset
			view := m.View()
			assertTUIViewFits(t, view, m.width, m.height, "help typing")
			if lines := strings.Split(view, "\n"); len(lines) != height {
				t.Fatalf("height %d, offset %d: справка отрисовала %d строк, want %d", height, offset, len(lines), height)
			}
			if strings.TrimSpace(view) == "" {
				t.Fatalf("height %d, offset %d: справка во время набора отрисовала пустой экран", height, offset)
			}
			if height >= 2 && !tuiViewHasPlainLine(view, "/ refresh_") {
				t.Fatalf("height %d, offset %d: строка ввода не пережила обрезку:\n%s", height, offset, view)
			}
		}
	}
}

// TestTUIHelpInputLineSitsBetweenTheContentAndTheFooter — единственная
// проверка порядка трёх частей экрана, и она обязана идти на высоте 60.
// Ниже её футер справки не виден и сегодня: документ — 58 строк,
// tuiHelpView дописывает футер уже после нарезки на height строк, а
// tuiFullscreenText режет лишнее, поэтому во время набора футер выживает
// только при height >= 60. Это известный дефект обрезки футера, он вне
// скопа задачи и здесь не чинится — на нём просто нельзя утверждать
// порядок ниже этой высоты.
func TestTUIHelpInputLineSitsBetweenTheContentAndTheFooter(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.overlay, m.width, m.height = "help", 200, 60
	m.height = len(tuiHelpLines()) + 2
	m.inputMode, m.input = "help-search", "ref"
	view := m.View()
	assertTUIViewFits(t, view, m.width, m.height, "help typing")
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("справка отрисовала %d строк, want %d", len(lines), m.height)
	}
	inputIndex := -1
	footerIndex := -1
	for i, line := range lines {
		switch ansi.Strip(line) {
		case "/ ref_":
			inputIndex = i
		default:
			if strings.HasPrefix(ansi.Strip(line), "Help ") && strings.Contains(ansi.Strip(line), "/") {
				footerIndex = i
			}
		}
	}
	if inputIndex < 1 || footerIndex != inputIndex+1 {
		t.Fatalf("строка ввода и футер имеют неверный порядок: input=%d footer=%d", inputIndex, footerIndex)
	}
	if got := ansi.Strip(lines[footerIndex]); !strings.Contains(got, "· / search · Enter confirm search · Esc cancel") {
		t.Fatalf("футер справки = %q, want строку навигации под строкой ввода", got)
	}
}

// TestTUIHelpInputLineIsNeverStyledAsAHeading фиксирует инвариант из
// spec.md: пост-проход подсветки заголовков в tuiHelpView — текстовое
// правило по суффиксу, и любая новая строка вывода обязана быть проверена
// на ложное срабатывание. Строку ввода это правило не спасает по её
// содержимому: plainTableText вычищает markdown-маркер "__" из каждой
// строки, так что запрос, заканчивающийся на "_", вместе с добавленным
// курсором вполне может схлопнуться в текст с суффиксом "search" — ровно
// это и пинит TestTUIHelpSearchInputLineNotStyledWhenEndingInUnderscore
// ("search_" → "/ search__" → "/ search"). Настоящая защита — tuiHelpView
// запоминает индекс строки ввода (inputLineIndex) и пропускает её по
// этому индексу в цикле стилизации, а не полагается на какое-либо
// свойство её текста. Цветовой профиль форсирован: без него lipgloss
// отдаёт термен Ascii, Render возвращает вход как есть, и тест прошёл бы
// при любой стилизации.
func TestTUIHelpInputLineIsNeverStyledAsAHeading(t *testing.T) {
	tuiForceColorProfile(t)
	for _, input := range []string{"", "search", "Task-fit", "keys", "view"} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
		m.overlay, m.width, m.height = "help", 100, 24
		m.inputMode, m.input = "help-search", input
		want := "/ " + input + "_"
		view := m.View()
		assertTUIViewFits(t, view, m.width, m.height, "help typing")
		found := ""
		for _, line := range strings.Split(view, "\n") {
			if ansi.Strip(line) == want {
				found = line
				break
			}
		}
		if found == "" {
			t.Fatalf("ввод %q: в выводе нет строки ввода %q:\n%s", input, want, view)
		}
		if found != want {
			t.Fatalf("ввод %q: строка ввода попала под пост-проход стилей: %q, want неокрашенную %q", input, found, want)
		}
	}
}

// TestTUIHelpSearchSeedsTheInputLineWithThePreviousQuery проверяет, что
// существующее поведение входа по / — m.input = m.helpSearch — теперь
// видно на экране: повторное нажатие / сразу показывает предыдущий
// запрос, а не пустую строку. Поведение не менялось, менялась только его
// видимость, поэтому проверка идёт по выводу View().
func TestTUIHelpSearchSeedsTheInputLineWithThePreviousQuery(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "a", DisplayName: "A"}})
	m.width, m.height = 100, 24
	m = tuiKey(m, "?")
	m = tuiKey(m, "/")
	m.input = "refresh"
	m, _ = m.inputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.inputMode != "" || m.helpSearch != "refresh" {
		t.Fatalf("test setup: inputMode = %q, helpSearch = %q", m.inputMode, m.helpSearch)
	}
	m = tuiKey(m, "/")
	if m.inputMode != "help-search" || m.input != "refresh" {
		t.Fatalf("повторный / не засеял поле ввода: inputMode = %q, input = %q", m.inputMode, m.input)
	}
	view := m.View()
	assertTUIViewFits(t, view, m.width, m.height, "help typing")
	if !tuiViewHasPlainLine(view, "/ refresh_") {
		t.Fatalf("повторный / не показал затравку предыдущим запросом:\n%s", view)
	}
}

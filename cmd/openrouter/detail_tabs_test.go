package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

func TestDetailTabBarLocalizesAndHighlightsActiveTab(t *testing.T) {
	bar := tuiDetailTabBar(2, "ru")
	if !strings.Contains(bar, "[3 Бенчмарки]") || !strings.Contains(bar, "[1 Идентичность]") {
		t.Fatalf("Russian tab bar = %q", bar)
	}
	if plain := strings.Join(strings.Fields(bar), " "); !strings.Contains(plain, "Pricing") {
		english := tuiDetailTabBar(0, "")
		if !strings.Contains(english, "Pricing") {
			t.Fatalf("English tab bar = %q", english)
		}
	}
}

func TestDetailTabsOpenAndNavigateWithResetScroll(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo", Tier: "sonnet", InPerM: 1, OutPerM: 2}})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 60, 20
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.detailTabsActive || m.detailTab != 0 {
		t.Fatalf("detail did not open on visible Identity tab: active=%v tab=%d", m.detailTabsActive, m.detailTab)
	}
	m.detailOffset = 3
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.detailTab != 2 || m.detailOffset != 0 {
		t.Fatalf("digit navigation = tab %d offset %d", m.detailTab, m.detailOffset)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.detailTab != 3 || m.detailOffset != 0 {
		t.Fatalf("Tab navigation = tab %d offset %d", m.detailTab, m.detailOffset)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.detailTab != 2 || m.detailOffset != 0 {
		t.Fatalf("Shift+Tab navigation = tab %d offset %d", m.detailTab, m.detailOffset)
	}
	if bar := tuiDetailTabBar(2, "", 20); !strings.Contains(bar, "[3/5]") {
		t.Fatalf("narrow tab bar lost active accessibility: %q", bar)
	}
}

func TestDetailLeftRightClampAtTabBoundariesThroughRuntime(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo"}})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 80, 20
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.overlay != "detail" || m.detailTab != 0 {
		t.Fatalf("Left at first tab = overlay %q tab %d, want detail tab 0", m.overlay, m.detailTab)
	}
	m.detailTab = detailTabCount - 1
	m.detailOffset = 4
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.overlay != "detail" || m.detailTab != detailTabCount-1 || m.detailOffset != 0 {
		t.Fatalf("Right at last tab = overlay %q tab %d offset %d, want detail tab %d offset 0", m.overlay, m.detailTab, m.detailOffset, detailTabCount-1)
	}
}

func TestDetailProductionPathSeparatesHistoryByTab(t *testing.T) {
	row := model.Model{Slug: "demo/model", DisplayName: "Demo", Tier: "sonnet", InPerM: 1, OutPerM: 2}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 100, 30
	m.priceHistory = &pricehistory.History{Observations: []pricehistory.Observation{{ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 1, OutPerM: 2}}, Scores: map[string]pricehistory.Score{row.Slug + "\x00swebench": {SourceFamily: "swebench", IdentityStatus: model.IdentityExact, Value: 80}}}, {ObservedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 2, OutPerM: 4}}}}}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.detailTab = 1
	pricing := strings.Join(m.detailLinesForTab(row), "\n")
	if !strings.Contains(pricing, "Price series") || strings.Contains(pricing, "SWE score (%)") {
		t.Fatalf("Pricing tab mixed history families: %q", pricing)
	}
	m.detailTab = 2
	benchmarks := strings.Join(m.detailLinesForTab(row), "\n")
	if !strings.Contains(benchmarks, "SWE score (%)") || strings.Contains(benchmarks, "Price series") {
		t.Fatalf("Benchmarks tab mixed history families: %q", benchmarks)
	}
}

func TestDetailScoreSeriesLabelsMissingHistoryAndRawArena(t *testing.T) {
	if got := detailScoreSeries(nil, "demo/model", ""); got != "Efficiency history: unavailable (no observations)" {
		t.Fatalf("empty history = %q", got)
	}
	history := &pricehistory.History{Observations: []pricehistory.Observation{{Scores: map[string]pricehistory.Score{
		"demo/model\x00arena": {SourceFamily: "arena", Value: 1400, Unit: "Elo"},
	}}}}
	if got := detailScoreSeries(history, "demo/model", ""); !strings.Contains(got, "1400 Elo") || strings.Contains(got, "%") {
		t.Fatalf("Arena history = %q", got)
	}
}

func TestDetailScoreHistoryUsesObservedSWESourceLabels(t *testing.T) {
	for _, test := range []struct {
		name, sourceID, provenance, want string
	}{
		{"vals source id", "vals", "https://example.invalid/other", "vals.ai"},
		{"swebench source id", "swebench", "https://swebench.com/verified", "swebench.com"},
		{"unknown source id", "notvals", "other-swebench", "unknown"},
		{"unknown provenance", "", "https://example.invalid/vals", "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &pricehistory.History{Observations: []pricehistory.Observation{{Scores: map[string]pricehistory.Score{
				"demo/model\x00swebench": {SourceID: test.sourceID, Provenance: test.provenance, IdentityStatus: model.IdentityExact, Value: 80},
			}}}}
			got := strings.Join(tuiDetailScoreHistoryLines(history, "demo/model", ""), "\n")
			if !strings.Contains(got, "source: "+test.want+"; unit: %") || !strings.Contains(got, "derived from "+test.want+"; unit:") {
				t.Fatalf("source label = %q, want %q in score and Q/P series", got, test.want)
			}
		})
	}
}

func TestDetailHelpEnglishRussianParity(t *testing.T) {
	english := tuiHelpSectionDetailBody
	russian := tuiHelpSectionDetailBodyRU
	if got := detailHelpKeyActionInventory(t, english); !reflect.DeepEqual(got, detailHelpKeyActionInventory(t, russian)) {
		t.Fatalf("English/Russian detail help key/action inventory differs: EN=%v RU=%v", got, detailHelpKeyActionInventory(t, russian))
	}
	for _, want := range []string{"Identity", "Pricing", "Benchmarks", "Provenance", "Fit & Notes", "1-5", "Left / Right", "Tab / Shift+Tab", "Esc or h", "SWE score", "Q/P", "source", "unit"} {
		if !strings.Contains(english, want) {
			t.Errorf("English detail help missing %q", want)
		}
	}
	for _, want := range []string{"Идентичность", "Цены", "Бенчмарки", "Происхождение", "Соответствие и заметки", "1-5", "Left / Right", "Tab / Shift+Tab", "Esc или h", "SWE", "Q/P", "источник", "единицу"} {
		if !strings.Contains(russian, want) {
			t.Errorf("Russian detail help missing %q", want)
		}
	}
}

func detailHelpKeyActionInventory(t *testing.T, body string) map[string]int {
	t.Helper()
	inventory := make(map[string]int)
	for lineNumber, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "\t") {
			t.Fatalf("detail help line %d contains a real tab byte: %q", lineNumber+1, line)
		}
		if !strings.Contains(line, `\t`) {
			continue
		}
		key, action, _, ok := tuiHelpRowColumns(line)
		if !ok || !strings.HasPrefix(line, `\t`) || strings.Count(line, `\t`) != 3 {
			t.Fatalf("malformed detail help row at line %d: %q", lineNumber+1, line)
		}
		canonicalKey := strings.ToLower(strings.TrimSpace(key))
		if normalized := map[string]string{"enter или right": "enter or right", "esc или h": "esc or h", "up/down или j/k": "up/down or j/k"}[canonicalKey]; normalized != "" {
			canonicalKey = normalized
		}
		canonicalAction := map[string]string{"вкладки": "tabs", "детали": "detail", "прокрутка": "scroll"}[strings.ToLower(strings.TrimSpace(action))]
		if canonicalAction == "" {
			canonicalAction = strings.ToLower(strings.TrimSpace(action))
		}
		canonical := canonicalKey + "\x00" + canonicalAction
		inventory[canonical]++
	}
	return inventory
}

func TestDetailHistoryRendersSeparateMetricSparklinesAndGaps(t *testing.T) {
	history := &pricehistory.History{Observations: []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Scores: map[string]pricehistory.Score{"demo/model\x00swebench": {SourceFamily: "swebench", SourceID: "vals", IdentityStatus: "exact_product", Value: 80, QualityPrice: floatPtr(2)}}},
		{ObservedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Scores: map[string]pricehistory.Score{"other/model\x00arena": {SourceFamily: "arena", SourceID: "arena", Value: 1400, Unit: "Elo"}}},
	}}
	got := strings.Join(tuiDetailScoreHistoryLines(history, "demo/model", ""), "\n")
	for _, want := range []string{"SWE score (%)", "SWE Q/P", "Arena raw Elo", "gaps:", "2026-01-01"} {
		if !strings.Contains(got, want) {
			t.Errorf("history graph missing %q in %q", want, got)
		}
	}
	ru := strings.Join(tuiDetailScoreHistoryLines(history, "demo/model", "ru"), "\n")
	for _, want := range []string{"Оценка SWE (%)", "Качество/цена SWE", "Сырой Elo LMArena", "пропуски:", "source: vals.ai; unit: %", "source: LMArena; unit: Elo"} {
		if !strings.Contains(ru, want) {
			t.Errorf("Russian history graph missing %q in %q", want, ru)
		}
	}
	for _, label := range []string{"SWE score (%)", "SWE Q/P", "Arena raw Elo"} {
		if strings.Contains(ru, label) {
			t.Errorf("Russian history graph contains English metric label %q: %q", label, ru)
		}
	}
	if gotMetrics, ruMetrics := strings.Count(got, " [source:"), strings.Count(ru, " [source:"); gotMetrics != ruMetrics {
		t.Fatalf("EN/RU metric inventory differs: EN=%d RU=%d", gotMetrics, ruMetrics)
	}
	if strings.Contains(got, "1400 Elo") {
		t.Errorf("graph renderer should not turn Arena into SWE detail: %q", got)
	}
}

func floatPtr(value float64) *float64 { return &value }

package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
	tuioutput "github.com/sboborikin/openrouter-model-tracker/internal/tui/screen/output"
)

// detailScreenBudget is the "fits one screen" target this layout is cut
// to: a full-screen terminal on a laptop display, in rows including the
// model header, the tab bar, the footer and their separators. 80 columns
// is the narrow end of the same budget; the full detail view is never
// laid out for anything narrower without scrolling.
const detailScreenBudget = 40

// detailBudgetRow is a deliberately worst-case row for the height budget:
// every task-fit tag the taxonomy has, and a note as long as the longest
// one notes.yaml actually carries, written as the several claims such a
// note is really made of.
func detailBudgetRow() model.Model {
	return model.Model{
		Slug: "vendor/worst-case", DisplayName: "Worst case model", Provider: "Vendor", License: "да (кастомная лицензия, свободно до 100M MAU)", Tier: "sonnet", ClaudeRef: "≈ Sonnet 5 — тот же чекпоинт",
		Context: 1000000, InPerM: 1.25, OutPerM: 8.5, OpenWeights: "да, MIT", CanonicalSlug: "vendor/worst-case-20260715", HuggingFaceID: "vendor/Worst-Case", ModelURL: "https://vendor.example/news/worst-case", MetadataSourceURL: "https://arena.ai/leaderboard/text", Created: 1784160000,
		Description: "Worst case is a 2.8T parameter open-weight multimodal reasoning model. It is suited for complex coding, knowledge work, and long-horizon agentic workflows, and is particularly strong at end-to-end software tasks and code review.",
		TaskFit:     []string{"implement", "plan", "research", "debug", "audit", "refactor", "test"},
		Note: "Основное число изменено в этом обновлении: 93.4% взяты с живого независимого лидерборда vals.ai (ранг #4, обновление 2026-07-22, продукт совпадает точно). " +
			"Прошлая версия документа показывала 76.8%, но источник этого числа не удалось отследить на 2026-07-30 — нашлись только другие метрики (Toolathlon-Verified 76.5%, FrontierSWE 81.2%, DeepSWE 67.5%, SWE Marathon 42.0%, Program Bench 77.8%), ни одна из них не «76.8% SWE-bench Verified». " +
			"По правилу «независимое измерение приоритетнее вендорского» в ранжирование пошло 93.4% (качество/цена 15.6 вместо прежних 12.8); 76.8% сохранено здесь как альтернативная цифра с неподтверждённым источником. " +
			"Расхождение такого размера у этой модели правдоподобно объясняется скаффолдом: на Terminal-Bench 2.1 у неё же 88.3% на харнессе Moonshot против 80.9% на прогоне vals.ai. " +
			"По сырой оценке 93.4% модель уже на уровне тира выше, но оценка спорная, а цена равна полному прайсу без скидки — строка оставлена в этом тире; если 93.4% подтвердится вторым независимым источником, её следует перенести выше.",
	}
}

func detailBudgetModel(t *testing.T, row model.Model, lang string, width, height int) tuiModel {
	t.Helper()
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.visible, m.cursor, m.lang = m.models, 0, lang
	m.width, m.height = width, height
	m.overlay, m.detailTabsActive, m.detailOffset = "detail", true, 0
	return m
}

func detailTabHeight(m tuiModel, row model.Model, tab int) (used, capacity int) {
	m.detailTab = tab
	frame := tuioutput.Detail(tuioutput.DetailData{Width: m.width, Height: m.height, Offset: 0, Lines: m.detailFrameLines(row)})
	return frame.MaxOffset, m.height
}

// TestEveryDetailTabFitsOneScreen is the budget the consolidation was cut
// to, asserted on the screen's own viewport primitive: MaxOffset is zero
// exactly when a tab needs no scrolling at all. It runs on a worst-case
// row with no price or score history — history is a graph that scrolls by
// design and is not what the grouping is sized for — in both languages
// and at both ends of the width budget.
func TestEveryDetailTabFitsOneScreen(t *testing.T) {
	row := detailBudgetRow()
	row.Score = &model.ScoreInfo{Value: 93.4, Metric: "SWE-bench Verified", Unit: "%", VariantMeasured: "vendor/worst-case", SourceURL: "https://www.vals.ai/benchmarks/swebench", Checked: "2026-09-01", IdentityStatus: model.IdentityExact}
	for _, lang := range []string{"", "ru"} {
		for _, width := range []int{100, 80} {
			m := detailBudgetModel(t, row, lang, width, detailScreenBudget)
			for tab := 0; tab < detailTabCount; tab++ {
				if overflow, _ := detailTabHeight(m, row, tab); overflow > 0 {
					m.detailTab = tab
					t.Errorf("lang %q width %d: tab %d (%s) overflows the %d-row budget by %d rows:\n%s", lang, width, tab+1, detailTabTitles[tab][0], detailScreenBudget, overflow, strings.Join(m.detailFrameLines(row), "\n"))
				}
			}
		}
	}
}

// TestFitAndNotesStaysItsOwnTabBecauseItWouldNotFitInsideIdentity records
// why the merge stopped where it did. The user's rule is "one screen, and
// split what does not fit": Identity and provenance together do fit, so
// they share a tab; adding the fit block to them does not, so it keeps
// its own. If the content ever shrinks enough for all three to fit, this
// test is the one that says the grouping may be revisited.
func TestFitAndNotesStaysItsOwnTabBecauseItWouldNotFitInsideIdentity(t *testing.T) {
	row := detailBudgetRow()
	m := detailBudgetModel(t, row, "", 80, detailScreenBudget)
	m.detailTab = detailTabIdentity
	identity := m.detailFrameLines(row)
	m.detailTab = detailTabFitNotes
	fit := m.detailFrameLines(row)
	// The merged tab would carry one header and one tab bar, so the fit
	// block contributes everything past its own three chrome rows.
	merged := append(append([]string(nil), identity...), fit[3:]...)
	if tuioutput.Detail(tuioutput.DetailData{Width: 80, Height: detailScreenBudget, Lines: merged}).MaxOffset == 0 {
		t.Fatalf("Identity, provenance and the fit block now fit one screen together (%d rows at 80 columns); the three-way merge is worth revisiting", len(merged))
	}
	if tuioutput.Detail(tuioutput.DetailData{Width: 80, Height: detailScreenBudget, Lines: identity}).MaxOffset != 0 {
		t.Fatalf("the Identity tab that does carry provenance no longer fits one screen:\n%s", strings.Join(identity, "\n"))
	}
}

// TestFitAndNotesTabIsAListNotAParagraph is the user-visible result of
// the restructuring, taken off the rendered screen rather than off the
// line builder: one item per task-fit tag, one item per note claim, and
// no claim left glued to the next. Each task-fit item also carries its
// one-line gloss ("keyword: gloss."), in both languages — the keyword
// itself stays English in both, only the gloss text and the note heading
// vary with lang (see taskFitGlosses in internal/tui/screen/output).
func TestFitAndNotesTabIsAListNotAParagraph(t *testing.T) {
	row := model.Model{Slug: "vendor/model", DisplayName: "Vendor model", TaskFit: []string{"implement", "plan", "test"}, Note: "Первое утверждение про модель. Второе утверждение про неё же."}
	for _, test := range []struct {
		lang        string
		fitHeading  string
		fitLines    []string
		noteHeading string
	}{
		{"", "Task fit:", []string{"  - implement: write or change production code.", "  - plan: define scope, steps, and decisions.", "  - test: add or improve automated verification."}, "Note:"},
		{"ru", "Task fit:", []string{"  - implement: написать или изменить продакшен-код.", "  - plan: определить объём, шаги и решения.", "  - test: добавить или улучшить автоматизированную проверку."}, "Заметка:"},
	} {
		m := detailBudgetModel(t, row, test.lang, 100, detailScreenBudget)
		m.detailTab = detailTabFitNotes
		view := ansi.Strip(m.View())
		want := append([]string{test.fitHeading}, test.fitLines...)
		want = append(want, test.noteHeading, "  - Первое утверждение про модель.", "  - Второе утверждение про неё же.")
		previous := -1
		for _, line := range want {
			index := strings.Index(view, "\n"+line)
			if index < 0 {
				t.Fatalf("lang %q: the fit tab has no row %q:\n%s", test.lang, line, view)
			}
			if index <= previous {
				t.Fatalf("lang %q: row %q is out of order:\n%s", test.lang, line, view)
			}
			previous = index
		}
		if strings.Contains(view, "implement + plan") || strings.Contains(view, "модель. Второе") {
			t.Fatalf("lang %q: the fit tab still renders joined prose:\n%s", test.lang, view)
		}
	}
}

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

// TestDetailTabBarActiveTabHighlightSurvivesFullRender guards against a
// regression where tuiDetailTabBar's own active-tab styling (verified in
// isolation by TestDetailTabBarLocalizesAndHighlightsActiveTab) never
// reaches the screen: detailFrameLines splices the already-styled bar into
// the logical detail lines, and tuioutput.Detail's line-sanitization step
// used to strip every ANSI escape — including the active tab's highlight —
// before the physical frame was ever composed. tuiStyleDetail only paints
// unstyled content afterwards, so a wipe at that earlier stage is
// invisible to any test that inspects tuiDetailTabBar's return value alone.
func TestDetailTabBarActiveTabHighlightSurvivesFullRender(t *testing.T) {
	tuiForceColorProfile(t)
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo", Tier: "sonnet", InPerM: 1, OutPerM: 2}})
	m.visible, m.cursor = m.models, 0
	m = runtimeTUIUpdate(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	view := m.View()
	activeStyled := tuiSelectedStyle.Render("[3 Benchmarks]")
	if !strings.Contains(view, activeStyled) {
		t.Fatalf("active tab highlight did not survive the full detail render: want %q in view, got:\n%s", activeStyled, view)
	}
	if inactiveStyled := tuiSelectedStyle.Render("[1 Identity]"); strings.Contains(view, inactiveStyled) {
		t.Fatalf("inactive tab is incorrectly highlighted: %s", view)
	}
}

// TestDetailFrameLinesInsertsBlankLineBetweenTitleAndTabBar guards the
// vertical-spacing convention DetailLines itself already applies between the
// title and every section heading (see detail_lines.go, e.g. "lines =
// append(lines, "", l.Identity)"): the tab bar is spliced in by
// detailFrameLines rather than DetailLines, so it needs its own blank-line
// separator from the title, matching every other block in this view.
func TestDetailFrameLinesInsertsBlankLineBetweenTitleAndTabBar(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo", Tier: "sonnet", InPerM: 1, OutPerM: 2}})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 100, 24
	m.detailTabsActive = true
	lines := m.detailFrameLines(m.visible[0])
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (title, blank, tab bar, blank), got %d: %v", len(lines), lines)
	}
	if lines[0] == "" {
		t.Fatalf("expected the model title on the first line, got blank")
	}
	if lines[1] != "" {
		t.Fatalf("expected a blank line separating the title from the tab bar, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "Identity") {
		t.Fatalf("expected the tab bar on the third line, got %q", lines[2])
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
	if bar := tuiDetailTabBar(2, "", 20); !strings.Contains(bar, "[3/4]") {
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
	for _, want := range []string{"Identity", "Pricing", "Benchmarks", "provenance", "Fit & Notes", "1-4", "Left / Right", "Tab / Shift+Tab", "Esc or h", "SWE score", "Q/P", "source", "unit"} {
		if !strings.Contains(english, want) {
			t.Errorf("English detail help missing %q", want)
		}
	}
	for _, want := range []string{"Идентичность", "Цены", "Бенчмарки", "Происхождение", "Соответствие и заметки", "1-4", "Left / Right", "Tab / Shift+Tab", "Esc или h", "SWE", "Q/P", "источник", "единицу"} {
		if !strings.Contains(russian, want) {
			t.Errorf("Russian detail help missing %q", want)
		}
	}
	// The help must name the tabs the build actually has, in the build's
	// own order, in both languages — a merged-away group left in the text
	// is exactly the drift the inventory check above cannot see.
	for tab, titles := range detailTabTitles {
		if !strings.Contains(english, titles[0]) {
			t.Errorf("English detail help does not name tab %d (%q)", tab+1, titles[0])
		}
		if !strings.Contains(russian, titles[1]) {
			t.Errorf("Russian detail help does not name tab %d (%q)", tab+1, titles[1])
		}
	}
	for _, gone := range []string{"1-5", "The five groups", "or Fit & Notes; resets scroll.\n\t"} {
		if strings.Contains(english, gone) {
			t.Errorf("English detail help still documents the removed five-tab layout: %q", gone)
		}
	}
	if strings.Contains(russian, "1-5") || strings.Contains(russian, "Пять групп") {
		t.Errorf("Russian detail help still documents the removed five-tab layout")
	}
}

// TestDetailTabInventoryIsSequentialAndMergesProvenanceIntoIdentity pins
// the consolidated layout itself: four tabs, digit keys 1..4 with no
// gap, and every section heading the document can emit routed to one of
// them — Provenance into Identity, Fit and notes kept on its own.
func TestDetailTabInventoryIsSequentialAndMergesProvenanceIntoIdentity(t *testing.T) {
	if detailTabCount != 4 || len(detailTabTitles) != detailTabCount {
		t.Fatalf("detail tab count = %d with %d titles, want 4", detailTabCount, len(detailTabTitles))
	}
	for _, want := range []struct {
		heading string
		tab     int
	}{
		{"-- Identity --", detailTabIdentity},
		{"-- Идентичность --", detailTabIdentity},
		{"-- Provenance and metadata --", detailTabIdentity},
		{"-- Происхождение и метаданные --", detailTabIdentity},
		{"-- Pricing --", detailTabPricing},
		{"-- Цены --", detailTabPricing},
		{"-- Benchmarks --", detailTabBenchmarks},
		{"-- Бенчмарки --", detailTabBenchmarks},
		{"-- Fit and notes --", detailTabFitNotes},
		{"-- Соответствие и заметки --", detailTabFitNotes},
	} {
		got, ok := detailSectionTab(want.heading)
		if !ok || got != want.tab {
			t.Errorf("detailSectionTab(%q) = %d/%v, want %d", want.heading, got, ok, want.tab)
		}
	}
	if _, ok := detailSectionTab("Provider: Acme"); ok {
		t.Errorf("a plain field row was taken for a section heading")
	}
	for _, titles := range detailTabTitles {
		for _, title := range titles {
			if strings.TrimSpace(title) == "" {
				t.Errorf("tab title %q is blank", title)
			}
		}
	}
}

// TestDetailDigitKeysCoverEveryTabAndStopAtTheLastOne walks the real
// runtime key path: 1..detailTabCount each select their own tab, and the
// first digit past the end changes nothing, so no key points at content
// that was merged away.
func TestDetailDigitKeysCoverEveryTabAndStopAtTheLastOne(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo", Tier: "sonnet", InPerM: 1, OutPerM: 2}})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 80, 24
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	for tab := 0; tab < detailTabCount; tab++ {
		m.detailOffset = 5
		m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune('1' + tab)}})
		if m.detailTab != tab || m.detailOffset != 0 {
			t.Fatalf("key %d selected tab %d offset %d", tab+1, m.detailTab, m.detailOffset)
		}
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune('1' + detailTabCount)}})
	if m.detailTab != detailTabCount-1 || m.overlay != "detail" {
		t.Fatalf("key %d past the last tab = tab %d overlay %q", detailTabCount+1, m.detailTab, m.overlay)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.overlay != "" {
		t.Fatalf("Esc did not close the detail screen: overlay %q", m.overlay)
	}
}

// TestDetailIdentityTabCarriesProvenanceWithItsOwnSeparator is the merge
// itself, asserted on the tab's real lines: one tab holds both blocks, in
// document order, separated by a blank row it does not borrow from the
// section that happens to precede it.
func TestDetailIdentityTabCarriesProvenanceWithItsOwnSeparator(t *testing.T) {
	row := model.Model{Slug: "demo/model", DisplayName: "Demo", Provider: "Acme", License: "MIT", Tier: "sonnet", ClaudeRef: "≈ Sonnet", CanonicalSlug: "demo/model", MetadataSourceURL: "https://meta.example/demo", Description: "vendor description", TaskFit: []string{"implement"}, Note: "A note."}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.visible, m.cursor, m.width, m.height = m.models, 0, 100, 40
	m.overlay, m.detailTabsActive, m.detailTab = "detail", true, detailTabIdentity
	lines := m.detailLinesForTab(row)
	identity, provenance := indexOfLine(lines, "-- Identity --"), indexOfLine(lines, "-- Provenance and metadata --")
	if identity < 0 || provenance < 0 || provenance < identity {
		t.Fatalf("Identity tab = %#v", lines)
	}
	if strings.TrimSpace(lines[provenance-1]) != "" {
		t.Errorf("the provenance block has no separator above it: %q", lines[provenance-1])
	}
	for _, want := range []string{"Provider: Acme", "Release date:", "OpenRouter page: https://openrouter.ai/demo/model", "Description:"} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Errorf("Identity tab is missing %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	for _, gone := range []string{"-- Pricing --", "-- Benchmarks --", "-- Fit and notes --", "vendor note"} {
		if strings.Contains(strings.Join(lines, "\n"), gone) {
			t.Errorf("Identity tab leaked %q from another tab", gone)
		}
	}
}

func indexOfLine(lines []string, want string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) == want {
			return i
		}
	}
	return -1
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

// TestDetailHistoryFrequentSameDayRefreshesDoNotFloodGapsOrSparkline
// reproduces the exact user-reported symptom end to end through the real
// ingestion path (pricehistory.History.AddObservation), not a hand-built
// literal: a live TUI --interval refreshing every 40 minutes, where
// demo/model's Arena identity match succeeds only on the first refresh of
// the day — every later refresh that day still records a valid Arena
// score for some OTHER model (so each observation's Scores map is
// non-empty overall, exactly the real multi-model-refresh shape), but
// keeps missing demo/model specifically. Before pricehistory's same-day
// coalescing this produced 36 observations, a "gaps:" line repeating
// "2026-09-11 Arena" 35 times, and an Arena sparkline of one real point
// followed by 35 unlabeled "?" — verified live against the pre-fix code
// (see decisions.md for the captured before/after render).
func TestDetailHistoryFrequentSameDayRefreshesDoNotFloodGapsOrSparkline(t *testing.T) {
	history := &pricehistory.History{}
	day := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	prices := map[string]sources.PriceInfo{"demo/model": {Found: true, InPerM: 1, OutPerM: 2}, "other/model": {Found: true, InPerM: 1, OutPerM: 2}}
	history.AddObservation(day, prices, nil,
		[]sources.ScoreRow{{Slug: "demo/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1400, IdentityStatus: "exact_product"}})
	for hour := 1; hour < 36; hour++ {
		history.AddObservation(day.Add(time.Duration(hour)*time.Minute*40), prices, nil,
			[]sources.ScoreRow{{Slug: "other/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1300, IdentityStatus: "exact_product"}})
	}
	if len(history.Observations) != 1 {
		t.Fatalf("36 same-day refreshes were not coalesced into one observation: %d", len(history.Observations))
	}
	got := strings.Join(tuiDetailScoreHistoryLines(history, "demo/model", ""), "\n")
	if count := strings.Count(got, "2026-09-11 Arena"); count > 1 {
		t.Fatalf("gaps line still repeats the same date (%d times): %q", count, got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Arena raw Elo") && strings.Count(line, "?") > 0 {
			t.Fatalf("Arena sparkline is a wall of gap characters instead of one point per day: %q", line)
		}
	}
}

func floatPtr(value float64) *float64 { return &value }

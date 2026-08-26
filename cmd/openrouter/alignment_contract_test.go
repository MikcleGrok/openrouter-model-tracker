package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/screen"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/screen/output"
)

func TestAlignmentCLIWidthContractMatrix(t *testing.T) {
	rows := []model.Model{
		{Slug: "zeta/long-model", DisplayName: "A very long display model", Owner: "Acme", Context: 128000, InPerM: 1.25, OutPerM: 9.5, ScoreLabel: "93.0%", QualityPriceLabel: "82.7", TaskFit: []string{"implement", "test"}},
		{Slug: "alpha/short", DisplayName: "S", Owner: "B", Context: 0, InPerM: 0, OutPerM: 0, ScoreLabel: "n/a", QualityPriceLabel: "n/a"},
		{Slug: "middle/numeric", DisplayName: "界🙂", Owner: "CJK", Context: 4096, InPerM: 0.01, OutPerM: 123.45, ScoreLabel: "", QualityPriceLabel: "n/a (price $0)"},
	}
	icons := config.IconConfig{Manufacturers: map[string]string{"acme": "x", "b": "界", "cjk": "🙂"}, Unknown: "?"}
	for _, width := range []int{40, 80, 120, 180} {
		t.Run("width-"+strconv.Itoa(width), func(t *testing.T) {
			output := renderTableModeWithIconsAndNameWidthAndGaps(rows, width, false, "notes", scoreSourceDefault, icons, 40, 1, nil)
			lines := nonEmptyTableLines(output)
			if len(lines) != 7 {
				t.Fatalf("line count = %d, want header, 3 rows, and 3 separators:\n%s", len(lines), output)
			}
			for _, line := range lines {
				if got := tablePipeColumns(line); !equalInts(got, alignmentCLISeparators[width]) {
					t.Fatalf("separator columns = %v, want literal contract %v: %q", got, alignmentCLISeparators[width], line)
				}
				wantLineWidth := alignmentCLISeparators[width][len(alignmentCLISeparators[width])-1] + 1
				if tableDisplayWidth(ansi.Strip(line)) != wantLineWidth {
					t.Fatalf("line width = %d, want literal border width %d: %q", tableDisplayWidth(ansi.Strip(line)), wantLineWidth, line)
				}
			}
			if !strings.Contains(output, "n/a") || width >= 80 && (!strings.Contains(output, "界🙂") || !strings.Contains(output, "123.45")) {
				t.Fatalf("wide/empty/numeric values missing at width %d:\n%s", width, output)
			}
		})
	}
}

var alignmentCLISeparators = map[int][]int{
	40:  {0, 7, 13, 17, 23, 27, 31, 35, 39},
	80:  {0, 33, 42, 51, 59, 69, 81, 94, 101},
	120: {0, 38, 47, 58, 73, 87, 99, 112, 119},
	180: {0, 43, 52, 63, 78, 147, 159, 172, 179},
}

func TestAlignmentCustomIconContractMatrix(t *testing.T) {
	icons := []struct {
		name, icon string
		wantSlot   string
		wantWidth  int
	}{
		{"empty", "", "? ", 0},
		{"ascii", "x", "x ", 1},
		{"cjk", "界", "界", 2},
		{"emoji", "🙂", "🙂", 2},
		{"variation", "☁️", "☁️", 2},
		{"skin-tone", "👍🏽", "👍🏽", 2},
		{"flag", "🇺🇸", "🇺🇸", 2},
		{"zwj", "👩‍💻", "👩‍💻", 2},
		{"xiaomi-default", "🟠", "🟠", 2},
		{"nvidia-default", "🟢", "🟢", 2},
		{"zai-default", "🔷", "🔷", 2},
		{"minimax-default", "🎲", "🎲", 2},
		{"meta-default", "🔵", "🔵", 2},
		{"mistral-default", "💨", "💨", 2},
		{"moonshot-default", "🌙", "🌙", 2},
		{"tencent-default", "🐧", "🐧", 2},
	}
	for _, test := range icons {
		for _, gap := range []int{0, 1, 3} {
			t.Run(test.name+"/gap-"+strconv.Itoa(gap), func(t *testing.T) {
				cfg := config.IconConfig{Manufacturers: map[string]string{"acme": test.icon}, Unknown: "?"}
				row := model.Model{DisplayName: "Acme Model", Owner: "Acme"}
				want := test.wantSlot + strings.Repeat(" ", gap) + "Acme Acme Model"
				if got := modelIdentityWithIconsAndGap(row, cfg, gap); got != want {
					t.Fatalf("identity bytes % x, want % x", []byte(got), []byte(want))
				}
				if got := tableDisplayWidth(modelIdentityWithIconsAndGap(row, cfg, gap)[:strings.Index(want, "Acme")]); got != 2+gap {
					t.Fatalf("manufacturer prefix width = %d, want %d", got, 2+gap)
				}
				if got := tableDisplayWidth(test.icon); got != test.wantWidth {
					t.Fatalf("icon width = %d, want explicit policy width %d", got, test.wantWidth)
				}
			})
		}
	}
	if got := (config.IconConfig{Manufacturers: map[string]string{"acme": ""}, Unknown: "?"}).Icon("Acme"); got != "?" {
		t.Fatalf("empty configured icon = %q, want unknown fallback", got)
	}
	if got := (config.IconConfig{Manufacturers: map[string]string{"acme": "界界"}, Unknown: "?"}).Icon("Acme"); got != "界界" {
		t.Fatalf("wide custom icon was discarded: %q", got)
	}
	wide := model.Model{DisplayName: "Acme Model", Owner: "Acme"}
	if got, want := modelIdentityWithIconsAndGap(wide, config.IconConfig{Manufacturers: map[string]string{"acme": "界界"}, Unknown: "?"}, 1), "界界 Acme Acme Model"; got != want {
		t.Fatalf("wide custom icon policy changed bytes: %q, want %q", got, want)
	}
}

func TestAlignmentIconConfigRejectsMalformedUTF8AtConfigBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, append([]byte("icons:\n  manufacturers:\n    acme: \""), 0xff, '\n', '"', '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("malformed UTF-8 icon config unexpectedly loaded")
	}
}

func TestAlignmentTUIViewContractMatrix(t *testing.T) {
	tuiForceColorProfile(t)
	rows := []model.Model{{Slug: "a", DisplayName: "A1", ScoreLabel: "90%"}, {Slug: "b", DisplayName: "B2", ScoreLabel: "n/a"}}
	for _, width := range []int{40, 80, 120, 180} {
		m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
		m.width, m.height = width, 12
		m.columns = []tuiColumn{colName, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colTask, colNote}
		m.icons = config.IconConfig{Unknown: "x"}
		m.rebuild()
		columns := m.renderColumns()
		plain := ansi.Strip(m.View())
		if !strings.Contains(plain, "Name") || !strings.Contains(plain, "SWE %") || !strings.Contains(plain, "A1") || !strings.Contains(plain, "B2") {
			t.Fatalf("width %d View lost header/row: %q", width, plain)
		}
		viewLines := strings.Split(plain, "\n")
		wantPipes := alignmentTUISeparators[width][len(columns)]
		if len(wantPipes) == 0 {
			t.Fatalf("missing independent TUI geometry contract for width %d and columns %v", width, columns)
		}
		if got := tablePipeColumns(viewLines[2]); !equalInts(got, wantPipes) {
			t.Fatalf("width %d header pipe columns = %v, want literal contract %v: %q", width, got, wantPipes, viewLines[2])
		}
		for _, line := range strings.Split(plain, "\n") {
			if tableDisplayWidth(line) > width {
				t.Fatalf("width %d View line exceeds viewport: %d %q", width, tableDisplayWidth(line), line)
			}
		}
		m = tuiKey(m, "j")
		selected := m.View()
		selectedLines := strings.Split(ansi.Strip(selected), "\n")
		if m.cursor != 1 || !strings.Contains(selectedLines[4], "> ") || !strings.Contains(selectedLines[4], "B2") {
			t.Fatalf("width %d second row was not selected: cursor=%d line=%q", width, m.cursor, selectedLines[4])
		}
		if !strings.Contains(selected, "\x1b[") || ansi.Strip(selected) == selected {
			t.Fatalf("width %d View omitted ANSI styling: %q", width, selected)
		}
	}
}

var alignmentTUISeparators = map[int]map[int][]int{
	40:  {4: {9, 18, 26}},
	80:  {7: {16, 25, 33, 48, 62, 71}},
	120: {9: {38, 47, 55, 70, 84, 93, 103, 114}},
	180: {9: {43, 59, 74, 96, 117, 133, 150, 168}},
}

func TestAlignmentTUIViewRefreshResizeSortFilterSelectionContract(t *testing.T) {
	rows := []model.Model{{Slug: "z", DisplayName: "A", Free: false, ScoreLabel: "90%"}, {Slug: "a", DisplayName: "B", Free: true, ScoreLabel: "80%"}}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.width, m.height, m.generation, m.sortKey = 80, 8, 1, "name"
	m.rebuild()
	if m.visible[0].Slug != "z" {
		t.Fatalf("initial name sort = %q, want z", m.visible[0].Slug)
	}
	m = tuiKey(m, "s")
	if m.sortKey == "name" || m.visible[0].Slug != "a" {
		t.Fatalf("sort did not reorder rows: sort=%q visible=%v", m.sortKey, m.visible)
	}
	m = tuiKey(m, "j")
	if m.cursor != 1 || m.selectedSlug != "z" {
		t.Fatalf("cursor did not select second row: cursor=%d selected=%q", m.cursor, m.selectedSlug)
	}
	m.filter = "availability:paid"
	m.rebuild()
	if len(m.visible) != 1 || m.visible[0].Slug != "z" || m.cursor != 0 || m.selectedSlug != "z" {
		t.Fatalf("filter did not clamp selection: cursor=%d selected=%q visible=%v", m.cursor, m.selectedSlug, m.visible)
	}
	m.generation = 1
	next, _ := m.Update(tuiRefreshMsg{generation: 1, models: rows, filter: "", nameWidth: 20, iconGap: 3, iconGapSet: true, iconGaps: config.IconGaps{"z": 0}, icons: config.IconConfig{Unknown: "x"}, iconsSet: true})
	m = next.(tuiModel)
	if len(m.visible) != 2 || m.iconGap != 3 || m.nameWidth != 20 || m.iconGaps["z"] != 0 || m.icons.Unknown != "x" {
		t.Fatalf("refresh state visible=%v iconGap=%d", m.visible, m.iconGap)
	}
	if m.width != 80 || m.height != 8 {
		t.Fatalf("refresh changed geometry: %dx%d", m.width, m.height)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	m = next.(tuiModel)
	if m.width != 40 || m.height != 8 || tableDisplayWidth(strings.Split(ansi.Strip(m.View()), "\n")[2]) > 40 {
		t.Fatalf("resize contract failed: width=%d view=%q", m.width, m.View())
	}
	if got := tablePipeColumns(strings.Split(ansi.Strip(m.View()), "\n")[2]); !equalInts(got, alignmentTUISeparators[40][len(m.renderColumns())]) {
		t.Fatalf("resize separators = %v, want %v", got, alignmentTUISeparators[40][len(m.renderColumns())])
	}
}

func TestAlignmentDetailGapAndNavigationContract(t *testing.T) {
	row := model.Model{Slug: "acme/model-with-a-deliberately-long-history-identity", DisplayName: "Model with a deliberately long name", Owner: "Acme", Description: strings.Repeat("long text ", 20), Note: "FINAL-NOTE"}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	// height 9, not 8: tuiDetailBodyHeight now reserves two trailing rows
	// (a blank separator plus the position footer, not just the footer), so
	// the first page needs one more row of viewport to still reach
	// "Производитель: ?" without scrolling.
	m.overlay, m.width, m.height = "detail", 28, 9
	m.iconGap, m.iconGaps, m.icons = 1, config.IconGaps{"acme": 8}, config.IconConfig{Unknown: "?"}
	configuredView := ansi.Strip(m.View())
	if !strings.Contains(configuredView, "Manufacturer: ?") {
		t.Fatalf("detail did not render configured gap: %q", ansi.Strip(m.View()))
	}
	m = tuiKey(m, "G")
	wantOffset := detailMaxOffsetForLangForTest(row, m.scoreSource, m.width, m.height, m.priceHistory, m.icons, m.iconGap, m.iconGaps, m.lang)
	if m.detailOffset != wantOffset {
		t.Fatalf("configured detail offset = %d, want rendered max offset %d", m.detailOffset, wantOffset)
	}
	lastView := ansi.Strip(m.View())
	if !strings.Contains(lastView, "FINAL-NOTE") || !strings.Contains(lastView, "Detail ") {
		t.Fatalf("G did not render final detail page: %q", lastView)
	}
	m, _ = m.key(tea.KeyMsg{Type: tea.KeyEscape})
	if m.overlay != "" {
		t.Fatal("detail close did not return to list")
	}
}

func TestTUIViewAlwaysReturnsACompleteFrameAcrossTransitions(t *testing.T) {
	rows := []model.Model{{Slug: "sentinel/model", DisplayName: "Sentinel model", Owner: "Acme", ScoreLabel: "90%"}, {Slug: "second/model", DisplayName: "Second model", Owner: "Beta", ScoreLabel: "80%"}}
	for _, width := range []int{160, 190, 200} {
		t.Run("width-"+strconv.Itoa(width), func(t *testing.T) {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
			m.width, m.height = width, 24
			m.rebuild()
			list := ansi.Strip(m.View())
			assertCompleteTUIFrame(t, list, m.width, m.height)
			if !strings.Contains(list, "Name") || !strings.Contains(list, "Sentinel model") {
				t.Fatalf("list frame lost header or sentinel row:\n%s", list)
			}

			m = tuiKey(m, "enter")
			detail := ansi.Strip(m.View())
			assertCompleteTUIFrame(t, detail, m.width, m.height)
			if strings.Contains(detail, "SWE %") || strings.Contains(detail, "sentinel/model |") {
				t.Fatalf("detail frame retained list header/row:\n%s", detail)
			}
			assertStableDetailValueColumn(t, detail)

			m = tuiKey(m, "esc")
			returned := ansi.Strip(m.View())
			assertCompleteTUIFrame(t, returned, m.width, m.height)
			if !strings.Contains(returned, "SWE %") || !strings.Contains(returned, "Sentinel model") {
				t.Fatalf("return-to-list frame lost list content:\n%s", returned)
			}

			m = tuiKey(m, "f")
			assertCompleteTUIFrame(t, ansi.Strip(m.View()), m.width, m.height)
			m = tuiKey(m, "esc")
			m = tuiKey(m, "c")
			assertCompleteTUIFrame(t, ansi.Strip(m.View()), m.width, m.height)
			m = tuiKey(m, "esc")
			m = tuiKey(m, "o")
			assertCompleteTUIFrame(t, ansi.Strip(m.View()), m.width, m.height)
			m = tuiKey(m, "esc")
			m = tuiKey(m, "?")
			assertCompleteTUIFrame(t, ansi.Strip(m.View()), m.width, m.height)
		})
	}
}

func TestTUIDetailFrameHasExactIndependentLinesAndClearsRightEdge(t *testing.T) {
	row := model.Model{Slug: "openai/test", DisplayName: "Test model", Owner: "OpenAI", Provider: "OpenAI", Tier: "sonnet", Context: 128000, InPerM: 1.2, OutPerM: 4.5, ScoreLabel: "96.2%", Score: &model.ScoreInfo{Value: 96.2}, MetadataSourceURL: "https://example.test/meta", Description: "description"}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.width, m.height, m.overlay = 190, 24, "detail"
	m.rebuild()
	plain := ansi.Strip(m.View())
	lines := strings.Split(plain, "\n")
	want := []string{"Test model (openai/test)", "", "-- Identity --", "Manufacturer:     🌀 OpenAI", "Provider:         OpenAI", "License:          n/a", "Tier:             sonnet", "Claude reference: n/a", "Task fit:         n/a", "", "-- Pricing --", "Context:          128K tokens", "Input:            $1.20 per M tokens", "Output:           $4.50 per M tokens", "Open weights:     n/a", "", "-- Benchmarks --", "SWE-bench Verified score (percent):", "  Value: 96.2%", "  Variant measured: n/a", "  Metric: n/a"}
	for i, expected := range want {
		if lines[i] != expected {
			t.Fatalf("detail line %d = %q, want %q\n%s", i, lines[i], expected, plain)
		}
	}
	if strings.Contains(plain, "SWE %") || strings.Contains(plain, "openai/test |") {
		t.Fatalf("detail frame retained list content:\n%s", plain)
	}
}

func TestTUIDetailAcceptanceFrameKeepsLongBlocksOnOwnedRows(t *testing.T) {
	row := model.Model{
		Slug: "provider/model-cjk-🙂", DisplayName: "中文 model with a deliberately long name", Owner: "Acme", Provider: "provider-with-a-long-name", Tier: "sonnet", Context: 128000, InPerM: 1.2, OutPerM: 4.5,
		Description: strings.Repeat("long description with 中文 and emoji 🙂 ", 12), Note: strings.Repeat("note text ", 8), CanonicalSlug: "provider/model-cjk-🙂", HuggingFaceID: "org/model-with-a-very-long-repository-name", MetadataSourceURL: "https://metadata.example.test/catalogue/provider/model-cjk-🙂",
		Score: &model.ScoreInfo{Value: 96.2}, ScoreLabel: "96.2%", ArenaScore: &model.ScoreInfo{Value: 1234}, ArenaLabel: "1234", TaskFit: []string{"I", "P", "R"},
	}
	history := &pricehistory.History{Observations: []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 1, OutPerM: 4, Context: row.Context}}},
		{ObservedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 2, OutPerM: 8, Context: row.Context}}},
	}}
	for _, tc := range []struct {
		lang          string
		width, height int
	}{{"", 44, 18}, {"ru", 57, 24}, {"", 160, 30}} {
		m := detailModelForTest(row, scoreSourceSWEBench, tc.width, history, config.IconConfig{Unknown: "界"}, 1, nil)
		m.columns = []tuiColumn{colName, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colTask}
		m.lang, m.overlay = tc.lang, "detail"
		m.height = tc.height
		m.visible, m.cursor = []model.Model{row}, 0
		m.screenController = screen.New(nil)
		var rendered strings.Builder
		lines := m.detailLines(row)
		maxOffset := output.Detail(output.DetailData{Width: tc.width, Height: tc.height, Lines: lines, Regions: output.RegionsFromLines(lines)}).MaxOffset
		for offset := 0; offset <= maxOffset; offset++ {
			m.detailOffset = offset
			viewLines := strings.Split(ansi.Strip(m.View()), "\n")
			if len(viewLines) != tc.height {
				t.Fatalf("%s %dx%d offset=%d: rows=%d, want %d", tc.lang, tc.width, tc.height, offset, len(viewLines), tc.height)
			}
			for i, line := range viewLines {
				if tableDisplayWidth(line) > tc.width {
					t.Fatalf("%s %dx%d row %d width=%d: %q", tc.lang, tc.width, tc.height, i, tableDisplayWidth(line), line)
				}
			}
			rendered.WriteString(strings.Join(viewLines, "\n"))
			rendered.WriteByte('\n')
		}
		historyHeading := "Price history:"
		if tc.lang == "ru" {
			historyHeading = "Динамика цен:"
		}
		if !strings.Contains(rendered.String(), historyHeading) || !strings.Contains(rendered.String(), "SWE-bench") {
			t.Fatalf("%s detail omitted acceptance blocks", tc.lang)
		}
		m, _ = m.key(tea.KeyMsg{Type: tea.KeyEscape})
		if got := strings.Split(ansi.Strip(m.View()), "\n"); len(got) != tc.height || m.overlay != "" {
			t.Fatalf("%s detail-to-list transition corrupted frame", tc.lang)
		}
	}
}

func TestTUIViewCompleteFrameCoversEmptyLoadingAndErrorStates(t *testing.T) {
	states := []struct {
		name  string
		setup func(tuiModel) tuiModel
	}{
		{"empty", func(m tuiModel) tuiModel { m.visible = nil; return m }},
		{"loading", func(m tuiModel) tuiModel { m.refreshing = true; return m }},
		{"error", func(m tuiModel) tuiModel { m.err = "fixture error"; return m }},
	}
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "sentinel/model", DisplayName: "Sentinel model"}})
			m.width, m.height = 160, 24
			m = state.setup(m)
			assertCompleteTUIFrame(t, ansi.Strip(m.View()), m.width, m.height)
		})
	}
}

func assertCompleteTUIFrame(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("frame has %d lines, want %d:\n%s", len(lines), height, view)
	}
	for i, line := range lines {
		if tableDisplayWidth(line) > width {
			t.Fatalf("frame line %d is %d columns wide, want <= %d: %q", i, tableDisplayWidth(line), width, line)
		}
	}
}

func assertStableDetailValueColumn(t *testing.T, view string) {
	t.Helper()
	valueColumn := -1
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "-- ") || strings.HasPrefix(line, "Detail ") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			continue
		}
		index := strings.IndexByte(line, ':')
		if index < 0 || strings.HasPrefix(line, "  ") {
			continue
		}
		value := strings.TrimLeft(line[index+1:], " ")
		valueIndex := strings.Index(line[index+1:], value)
		column := tableDisplayWidth(line[:index+1+valueIndex])
		if valueColumn < 0 {
			valueColumn = column
		} else if column != valueColumn {
			t.Fatalf("detail label/value column drifted: got %d after %d in %q", column, valueColumn, line)
		}
		if strings.Contains(line, " | ") {
			t.Fatalf("detail reused list separator: %q", line)
		}
	}
	if valueColumn < 0 {
		t.Fatal("detail frame contains no label/value rows")
	}
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

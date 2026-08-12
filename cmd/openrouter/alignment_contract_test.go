package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
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
	m.overlay, m.width, m.height = "detail", 28, 8
	m.iconGap, m.iconGaps, m.icons = 1, config.IconGaps{"acme": 8}, config.IconConfig{Unknown: "?"}
	configuredView := ansi.Strip(m.View())
	if !strings.Contains(configuredView, "Производитель: ?") {
		t.Fatalf("detail did not render configured gap: %q", ansi.Strip(m.View()))
	}
	legacy := m
	legacy.iconGaps = nil
	legacy, _ = legacy.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if legacy.detailOffset == 0 {
		t.Fatalf("legacy detail navigation did not reach a later page: view=%q", ansi.Strip(legacy.View()))
	}
	m = tuiKey(m, "G")
	if m.detailOffset <= legacy.detailOffset {
		t.Fatalf("configured vendor gap did not increase navigable detail content: configured=%d legacy=%d", m.detailOffset, legacy.detailOffset)
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

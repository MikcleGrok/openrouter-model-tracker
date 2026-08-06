package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

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
	if !m.taskLong {
		t.Fatal("long task fit not enabled")
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
	for _, text := range []string{"Navigation", "IDFT", "English keywords"} {
		if !strings.Contains(m.View(), text) {
			t.Fatalf("help page 1 missing %q", text)
		}
	}
	if !strings.Contains(m.View(), "Page 1/3") {
		t.Fatal("help page indicator missing")
	}
	m = tuiKey(m, "tab")
	if m.overlay != "help" || m.helpPage != 1 || !strings.Contains(m.View(), "quality>=N") || !strings.Contains(m.View(), "Columns") || !strings.Contains(m.View(), "structured filter") {
		t.Fatalf("tab did not advance help: overlay=%q page=%d view=%q", m.overlay, m.helpPage, m.View())
	}
	m = tuiKey(m, "right")
	if m.helpPage != 2 || !strings.Contains(m.View(), "Auto-refresh") || !strings.Contains(m.View(), "refresh") {
		t.Fatalf("right did not advance help: page=%d", m.helpPage)
	}
	if !strings.Contains(m.View(), "R refreshes") || !strings.Contains(m.View(), "x or Ctrl-C exits") || strings.Contains(m.View(), "r refreshes") {
		t.Fatalf("help page 3 has stale shortcuts: %q", m.View())
	}
	m = tuiKey(m, "up")
	if m.overlay != "help" || m.helpPage != 2 {
		t.Fatal("up left help overlay")
	}
	m = tuiKey(m, "left")
	if m.helpPage != 1 {
		t.Fatalf("left did not return to previous help page: %d", m.helpPage)
	}
	m = tuiKey(m, "esc")
	if m.overlay != "" {
		t.Fatal("help did not close")
	}
	m = tuiKey(m, "?")
	m = tuiKey(m, "?")
	if m.overlay != "" {
		t.Fatal("repeated ? did not close help")
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
		m = tuiKey(m, test.key)
		if m.sortKey != test.sortKey || len(m.visible) != 2 || m.visible[0].Slug != test.first {
			t.Fatalf("key %q: sort=%q visible=%+v", test.key, m.sortKey, m.visible)
		}
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
			if width > 0 && tableDisplayWidth(line) > width {
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
		if tableDisplayWidth(header) > m.width || tableDisplayWidth(row) > m.width || tableDisplayWidth(selected) > m.width {
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
	if tableDisplayWidth(row) > m.width {
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
		if tableDisplayWidth(line) > m.width {
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
		if tableDisplayWidth(line) > m.width {
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
			if tableDisplayWidth(line) > width {
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
		offsets = append(offsets, tableDisplayWidth(line[:separator])+1)
		index = separator + len(" | ")
	}
	return offsets
}

func tuiNumericCellDisplayEnd(line string, column int) int {
	offsets := tuiSeparatorDisplayOffsets(line)
	end := tableDisplayWidth(line)
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
	valueStart := tableDisplayWidth(line[:index])
	valueEnd := valueStart + tableDisplayWidth(value)
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
				if width > 0 && tableDisplayWidth(line) > width {
					t.Fatalf("%s overlay at width %d exceeds width: %q", overlay, width, line)
				}
			}
		}
	}
}

func TestTUIHelpAndNonTTYGuard(t *testing.T) {
	if err := runTUI(nil, io.Discard, "", refresh.Options{}, 0); err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("non-TTY error = %v", err)
	}
}

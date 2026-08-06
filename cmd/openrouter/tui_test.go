package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

func TestTUISelectionTextStripsANSIAndPadding(t *testing.T) {
	got := tuiSelectionText([]string{"alpha  ", "\x1b[31mbeta\x1b[0m   "}, tuiSelectionPoint{0, 0}, tuiSelectionPoint{1, 4})
	if got != "alpha\nbeta" {
		t.Fatalf("selection text = %q", got)
	}
}

func TestTUIMouseDragCopiesVisibleTextAsync(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "alpha"}})
	m.width, m.height = 100, 12
	var copied string
	m.clipboardWrite = func(text string) error { copied = text; return nil }
	updated, _ := m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	updated, _ = updated.(tuiModel).Update(tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	updated, cmd := updated.(tuiModel).Update(tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	value := updated.(tuiModel)
	if cmd == nil || copied != "" || !value.selection.active || value.selection.dragging {
		t.Fatalf("drag state: cmd=%v copied=%q selection=%+v", cmd != nil, copied, value.selection)
	}
	result := cmd().(tuiClipboardResultMsg)
	updated, _ = value.Update(result)
	if copied != "OpenR" || updated.(tuiModel).status != "copied selection" {
		t.Fatalf("copied=%q status=%q", copied, updated.(tuiModel).status)
	}
}

func TestTUIMouseReleaseOutsideVisibleTextCopiesClampedRangeWithoutToast(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.width, m.height = 40, 4
	m.clipboardWrite = func(text string) error {
		if text != "OpenRouter models" {
			t.Fatalf("clipboard payload = %q, want the visible range", text)
		}
		return nil
	}
	updated, _ := m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	updated, _ = updated.(tuiModel).Update(tea.MouseMsg{X: 999, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	updated, cmd := updated.(tuiModel).Update(tea.MouseMsg{X: 999, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	value := updated.(tuiModel)
	if cmd == nil || value.status == "selection ended outside visible text" || !value.selection.active {
		t.Fatalf("outside release was not safely clamped: cmd=%v status=%q selection=%+v", cmd != nil, value.status, value.selection)
	}
	result := cmd().(tuiClipboardResultMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
}

func TestTUISelectionCopiesFullVisibleRowAndUnicodeCells(t *testing.T) {
	if got := tuiSelectionText([]string{"界🙂model  "}, tuiSelectionPoint{0, 0}, tuiSelectionPoint{0, ansi.StringWidth("界🙂model")}); got != "界🙂model" {
		t.Fatalf("full visible Unicode row = %q", got)
	}
	if got := tuiSelectionText([]string{"name...  "}, tuiSelectionPoint{0, 0}, tuiSelectionPoint{0, ansi.StringWidth("name...")}); got != "name..." {
		t.Fatalf("visible truncation marker = %q", got)
	}
}

func TestTUIMousePressRejectsShortLineEndAndEmptyLine(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.selection.frame = []string{"abc", ""}
	for _, point := range []struct{ x, y int }{{3, 0}, {4, 0}, {0, 1}} {
		if _, ok := m.selectionStartPoint(m.selection.frame, point.x, point.y); ok {
			t.Fatalf("point %+v was accepted as selectable text", point)
		}
	}
}

func TestTUINewPressInvalidatesOldClipboardResult(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.selection.frame = []string{"old"}
	m.clipboardWrite = func(string) error { return nil }
	updated, _ := m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(tuiModel)
	updated, oldCmd := m.Update(tea.MouseMsg{X: 3, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	m = updated.(tuiModel)
	oldResult := oldCmd
	if oldResult == nil {
		t.Fatal("old selection did not schedule clipboard copy")
	}
	updated, _ = m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(tuiModel)
	m.status = "new selection pending"
	updated, _ = m.Update(oldResult().(tuiClipboardResultMsg))
	if got := updated.(tuiModel); got.status != "new selection pending" || got.clipboardPending {
		t.Fatalf("old result changed new selection: status=%q pending=%v", got.status, got.clipboardPending)
	}
}

func TestTUISynchronizedWriterSerializesWholeWrites(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var writes []string
	underlying := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		writes = append(writes, string(p))
		mu.Unlock()
		if string(p) == "renderer" {
			close(started)
			<-release
		}
		return len(p), nil
	})
	w := &tuiSynchronizedWriter{out: underlying}
	firstDone := make(chan struct{})
	go func() { _, _ = w.Write([]byte("renderer")); close(firstDone) }()
	<-started
	secondDone := make(chan struct{})
	go func() { _, _ = w.Write([]byte("osc52")); close(secondDone) }()
	select {
	case <-secondDone:
		t.Fatal("clipboard write interleaved with renderer write")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	<-firstDone
	<-secondDone
	mu.Lock()
	got := append([]string(nil), writes...)
	mu.Unlock()
	if !bytes.Equal([]byte(strings.Join(got, "|")), []byte("renderer|osc52")) {
		t.Fatalf("writes = %q", got)
	}
}

func TestTUIFrameWriterClearsRightEdgeWithoutChangingViewText(t *testing.T) {
	var output bytes.Buffer
	w := &tuiFrameWriter{out: &output}
	view := "detail line\nsecond line"
	if _, err := w.Write([]byte(view)); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if ansi.Strip(got) != view || !strings.Contains(got, ansi.EraseLineRight+"\n") || !strings.HasSuffix(got, ansi.EraseLineRight) {
		t.Fatalf("frame transport = %q", got)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestTUISelectionSurvivesRefreshAndStaleClipboard(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "alpha"}})
	m.width, m.height = 100, 12
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 5}, context: m.selectionContext(), frame: []string{"OpenRouter models"}}
	m.clipboardToken = 2
	updated, _ := m.Update(tuiClipboardResultMsg{token: 1})
	value := updated.(tuiModel)
	if !value.selection.active || value.status != "" {
		t.Fatal("stale clipboard result changed selection")
	}
	updated, _ = value.Update(tuiRefreshMsg{generation: value.generation, scoreSourceGeneration: value.scoreSourceGeneration, models: value.models})
	if !updated.(tuiModel).selection.active {
		t.Fatal("refresh cleared selection")
	}
	if !strings.Contains(ansi.Strip(updated.(tuiModel).View()), "OpenRouter") {
		t.Fatal("redraw lost selected frame")
	}
}

func TestTUISelectionIsNotAppliedAfterViewTransition(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo model", ScoreLabel: "90%"}})
	m.width, m.height = 190, 24
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 5}, context: m.selectionContext(), frame: m.renderedFrame()}
	m.overlay = "detail"
	got := m.View()
	withoutSelection := m
	withoutSelection.selection = tuiSelection{}
	want := withoutSelection.View()
	if got != want {
		t.Fatalf("stale list selection changed detail frame:\nwith selection:\n%s\nwithout selection:\n%s", got, want)
	}
	plain := ansi.Strip(got)
	if strings.Contains(plain, "SWE %") || !strings.Contains(plain, "SWE-bench Verified") || !strings.Contains(plain, "LMArena") {
		t.Fatalf("detail frame has incorrect list/detail content:\n%s", plain)
	}
	m = newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "demo/model", DisplayName: "Demo model", ScoreLabel: "90%"}})
	m.width, m.height = 190, 24
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 5}, context: m.selectionContext(), frame: m.renderedFrame()}
	m.overlay = "detail"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(tuiModel)
	if m.selection.active {
		t.Fatal("closing detail restored stale list selection")
	}
}

func TestTUISelectionOverlayPreservesCurrentANSIFrame(t *testing.T) {
	selection := tuiSelection{active: true, context: "main\x00", anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 5}, frame: []string{"stale"}}
	rendered := "\x1b[31mcurrent text\x1b[0m"
	got := tuiRenderSelection(rendered, selection)
	if plain := ansi.Strip(got); plain != "current text" {
		t.Fatalf("selection changed current frame text: %q", plain)
	}
	if !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("selection lost current ANSI context: %q", got)
	}
}

func TestTUISelectionCopyPreservesDetailLineBoundaries(t *testing.T) {
	lines := []string{"SWE-bench Verified score (percent):", "  Value: 93.0%", "  Source: https://example.test/score"}
	got := tuiSelectionText(lines, tuiSelectionPoint{0, 0}, tuiSelectionPoint{2, ansi.StringWidth(lines[2])})
	want := strings.Join(lines, "\n")
	if got != want {
		t.Fatalf("copied detail text = %q, want %q", got, want)
	}
}

func TestTUIClipboardSkipsStaleWriteAfterNewCopy(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	writes := []string{}
	m.clipboardWrite = func(text string) error {
		writes = append(writes, text)
		return nil
	}
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 3}, context: m.selectionContext(), frame: []string{"old"}}
	oldCmd := m.copyActiveSelection()
	m.selection.frame = []string{"new"}
	newCmd := m.copyActiveSelection()

	if result := newCmd().(tuiClipboardResultMsg); result.err != nil {
		t.Fatal(result.err)
	}
	if result := oldCmd().(tuiClipboardResultMsg); result.err != nil {
		t.Fatal(result.err)
	}
	if strings.Join(writes, ",") != "new" {
		t.Fatalf("clipboard writes = %q, want only new copy", writes)
	}
}

func TestTUISelectionSurvivesRedrawAnimationRefreshAndToastTimeout(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.width, m.height = 80, 10
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 4}, context: m.selectionContext(), frame: []string{"OpenRouter models"}}
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 80, Height: 10}, tuiTickMsg{}, tuiToastTimeoutMsg{}} {
		updated, _ := m.Update(msg)
		m = updated.(tuiModel)
		if !m.selection.active {
			t.Fatalf("message %T cleared selection", msg)
		}
	}
}

type tuiToastTimeoutMsg struct{}

func TestTUIYCopiesSelectionAndNavigationInvalidatesIt(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{{Slug: "alpha"}, {Slug: "beta"}})
	var copied string
	m.clipboardWrite = func(text string) error { copied = text; return nil }
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 5}, context: m.selectionContext(), frame: []string{"OpenRouter models"}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil || !updated.(tuiModel).selection.active {
		t.Fatal("y did not copy selection")
	}
	if result := cmd().(tuiClipboardResultMsg); result.err != nil {
		t.Fatal(result.err)
	}
	if copied != "OpenR" {
		t.Fatalf("y copied %q", copied)
	}
	updated, _ = updated.(tuiModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if updated.(tuiModel).selection.active {
		t.Fatal("keyboard navigation retained selection")
	}
	value := updated.(tuiModel)
	value.selection = tuiSelection{active: true, context: value.selectionContext(), frame: []string{"OpenRouter models"}}
	updated, _ = value.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if updated.(tuiModel).selection.active {
		t.Fatal("new help screen retained selection")
	}
}

func TestTUISelectionDoesNotStealFilterInput(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.inputMode, m.input = "filter", ""
	m.selection = tuiSelection{active: true, context: m.selectionContext(), frame: []string{"text"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	value := updated.(tuiModel)
	if value.input != "y" || !value.selection.active {
		t.Fatalf("filter input changed selection handling: input=%q active=%v", value.input, value.selection.active)
	}
}

func TestTUISelectionClipboardErrorIsReported(t *testing.T) {
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, nil)
	m.selection = tuiSelection{active: true, anchor: tuiSelectionPoint{0, 0}, end: tuiSelectionPoint{0, 1}, context: m.selectionContext(), frame: []string{"x"}}
	m.clipboardWrite = func(string) error { return errors.New("unavailable") }
	cmd := m.copyActiveSelection()
	if cmd == nil {
		t.Fatal("copy command missing")
	}
	result := cmd().(tuiClipboardResultMsg)
	updated, _ := m.Update(result)
	if status := updated.(tuiModel).status; status != "copy failed: unavailable" {
		t.Fatalf("clipboard error status = %q", status)
	}
}

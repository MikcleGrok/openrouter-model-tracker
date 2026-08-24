package screen

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/input"
)

func event(msg tea.Msg) input.MessageEvent {
	value, ok := input.FromMessage(msg)
	if !ok {
		panic("not a terminal message")
	}
	return value
}

func TestControllerTranslatesKeyAndWindowMessages(t *testing.T) {
	c := New(nil)
	command, _ := c.Handle(event(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'о'}}), nil, "main")
	key, ok := command.(KeyCommand)
	if !ok || key.Value != "j" || key.Original != "о" {
		t.Fatalf("key command = %#v, want normalized j with original rune", command)
	}
	command, _ = c.Handle(event(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'ч'}, Alt: true}), nil, "main")
	key, ok = command.(KeyCommand)
	if !ok || key.Value == "x" {
		t.Fatalf("modified key was normalized as command: %#v", command)
	}
	command, _ = c.Handle(event(tea.WindowSizeMsg{Width: 80, Height: 24}), nil, "main")
	if got := command.(ResizeCommand); got.Width != 80 || got.Height != 24 {
		t.Fatalf("resize command = %#v", got)
	}
}

func TestControllerSelectionUsesInjectedClipboardWriter(t *testing.T) {
	var copied string
	c := New(func(value string) error { copied = value; return nil })
	frame := []string{"alpha", "beta"}
	effect, cmd := c.Selection.Handle(event(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}).Mouse, frame, "main")
	if !effect.Selected || cmd != nil {
		t.Fatalf("press = effect:%+v cmd:%v", effect, cmd)
	}
	_, cmd = c.Selection.Handle(event(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}).Mouse, frame, "main")
	if cmd == nil {
		t.Fatal("release did not schedule copy")
	}
	message := cmd()
	if _, err, ok := c.Selection.Result(message); !ok || err != nil || copied != "lpha\nbe" {
		t.Fatalf("copy result: ok=%v err=%v copied=%q", ok, err, copied)
	}
}

func TestControllerSelectionRejectsBlankAndOutOfRangePoints(t *testing.T) {
	c := New(func(string) error { t.Fatal("blank selection copied"); return nil })
	effect, cmd := c.Selection.Handle(event(tea.MouseMsg{X: 0, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}).Mouse, []string{""}, "main")
	if effect.Selected || cmd != nil {
		t.Fatalf("invalid press = effect:%+v cmd:%v", effect, cmd)
	}
}

func TestControllerInvalidatesOnlyLogicalViewTransitions(t *testing.T) {
	c := New(nil)
	if cmd := c.Transition("list"); cmd == nil {
		t.Fatal("initial logical view was not invalidated")
	}
	if cmd := c.Transition("list"); cmd != nil {
		t.Fatal("unchanged logical view was invalidated")
	}
	cmd := c.Transition("detail")
	if cmd == nil || cmd() != tea.ClearScreen() {
		t.Fatalf("detail transition command = %v", cmd)
	}
}

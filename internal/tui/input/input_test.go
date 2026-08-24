package input

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCommandKeyNormalizesLayoutWithoutTouchingModifiedInput(t *testing.T) {
	for _, test := range []struct {
		name string
		msg  tea.KeyMsg
		want string
	}{
		{"cyrillic physical j", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("о")}, "j"},
		{"cyrillic physical g", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("п")}, "g"},
		{"alt stays opaque", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("о"), Alt: true}, "alt+о"},
		{"paste stays opaque", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("о"), Paste: true}, "[о]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := CommandKey(test.msg); got != test.want {
				t.Fatalf("CommandKey() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFromMessagePreservesKeyBoundaryDetails(t *testing.T) {
	tests := []struct {
		name  string
		msg   tea.KeyMsg
		want  string
		runes []rune
	}{
		{name: "enter", msg: tea.KeyMsg{Type: tea.KeyEnter}, want: "enter"},
		{name: "escape", msg: tea.KeyMsg{Type: tea.KeyEscape}, want: "esc"},
		{name: "space", msg: tea.KeyMsg{Type: tea.KeySpace}, want: " "},
		{name: "navigation", msg: tea.KeyMsg{Type: tea.KeyUp}, want: "up"},
		{name: "ctrl", msg: tea.KeyMsg{Type: tea.KeyCtrlC}, want: "ctrl+c"},
		{name: "alt", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'ч'}, Alt: true}, want: "alt+ч", runes: []rune{'ч'}},
		{name: "paste", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("жx"), Paste: true}, want: "[жx]", runes: []rune("жx")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, ok := FromMessage(test.msg)
			if !ok || event.Kind != Key || event.Key.Key != test.want || event.Key.Original != test.msg.String() {
				t.Fatalf("event = %#v, want key %q/original %q", event, test.want, test.msg.String())
			}
			if string(event.Key.Runes) != string(test.runes) {
				t.Fatalf("runes = %q, want %q", string(event.Key.Runes), string(test.runes))
			}
		})
	}
}

func TestFromMessagePreservesMouseAndWindowDetails(t *testing.T) {
	mouse, ok := FromMessage(tea.MouseMsg{X: 7, Y: 9, Shift: true, Alt: true, Ctrl: true, Action: tea.MouseActionMotion, Button: tea.MouseButtonWheelDown})
	if !ok || mouse.Kind != Mouse || mouse.Mouse.X != 7 || mouse.Mouse.Y != 9 || !mouse.Mouse.Shift || !mouse.Mouse.Alt || !mouse.Mouse.Ctrl || mouse.Mouse.Action != MouseMotion || mouse.Mouse.Button != MouseWheelDown {
		t.Fatalf("mouse event = %#v", mouse)
	}
	window, ok := FromMessage(tea.WindowSizeMsg{Width: 120, Height: 40})
	if !ok || window.Kind != Window || window.Window != (WindowSize{Width: 120, Height: 40}) {
		t.Fatalf("window event = %#v", window)
	}
	if _, ok := FromMessage(struct{}{}); ok {
		t.Fatal("unknown message was accepted")
	}
}

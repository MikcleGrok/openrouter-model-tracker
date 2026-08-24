// Package input translates terminal key messages into stable TUI commands.
package input

import tea "github.com/charmbracelet/bubbletea"

type Event struct {
	Key      string
	Original string
}

type Kind uint8

const (
	Key Kind = iota
	Mouse
	Window
)

type MessageEvent struct {
	Kind   Kind
	Key    KeyEvent
	Mouse  MouseEvent
	Window WindowSize
}

type KeyEvent struct {
	Key, Original string
	Runes         []rune
}

type MouseEvent struct {
	X, Y             int
	Shift, Alt, Ctrl bool
	Action           MouseAction
	Button           MouseButton
}

type WindowSize struct{ Width, Height int }

type MouseAction uint8

const (
	MousePress MouseAction = iota
	MouseRelease
	MouseMotion
)

type MouseButton uint8

const (
	MouseNone MouseButton = iota
	MouseLeft
	MouseMiddle
	MouseRight
	MouseWheelUp
	MouseWheelDown
	MouseWheelLeft
	MouseWheelRight
	MouseBackward
	MouseForward
	Mouse10
	Mouse11
)

// Message converts the Bubble Tea boundary message into a typed input event.
// The application never needs to know how aliases or modified keys are
// represented by Bubble Tea.
func Message(msg tea.KeyMsg) Event { return Event{Key: CommandKey(msg), Original: msg.String()} }

func FromMessage(msg tea.Msg) (MessageEvent, bool) {
	switch value := msg.(type) {
	case tea.KeyMsg:
		event := Message(value)
		return MessageEvent{Kind: Key, Key: KeyEvent{Key: event.Key, Original: event.Original, Runes: append([]rune(nil), value.Runes...)}}, true
	case tea.MouseMsg:
		return MessageEvent{Kind: Mouse, Mouse: MouseEvent{X: value.X, Y: value.Y, Shift: value.Shift, Alt: value.Alt, Ctrl: value.Ctrl, Action: mouseAction(value.Action), Button: mouseButton(value.Button)}}, true
	case tea.WindowSizeMsg:
		return MessageEvent{Kind: Window, Window: WindowSize{Width: value.Width, Height: value.Height}}, true
	default:
		return MessageEvent{}, false
	}
}

func mouseAction(value tea.MouseAction) MouseAction { return MouseAction(value) }
func mouseButton(value tea.MouseButton) MouseButton { return MouseButton(value) }

var layoutAliases = map[rune]string{
	'ч': "x", 'л': "k", 'о': "j", 'щ': "o", 'п': "g", 'П': "G", 'р': "h", 'д': "l",
	'ы': "s", 'Ы': "S", 'ь': "m", 'с': "c", 'т': "n", 'а': "f", 'й': "q", 'з': "p",
	'к': "r", 'К': "R", '.': "/", ',': "?",
}

// CommandKey keeps modified and pasted input opaque while normalizing the
// physical ЙЦУКЕН positions used by the command layer.
func CommandKey(msg tea.KeyMsg) string {
	if len(msg.Runes) == 1 && !msg.Alt && !msg.Paste {
		if key, ok := layoutAliases[msg.Runes[0]]; ok {
			return key
		}
	}
	return msg.String()
}

func LayoutAliases() map[rune]string {
	aliases := make(map[rune]string, len(layoutAliases))
	for key, value := range layoutAliases {
		aliases[key] = value
	}
	return aliases
}

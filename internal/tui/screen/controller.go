// Package screen owns the terminal-facing orchestration of the TUI.
package screen

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sboborikin/openrouter-model-tracker/internal/tui/clipboard"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/input"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/selection"
)

type KeyCommand struct {
	Value    string
	Original string
	Runes    []rune
}

type ResizeCommand struct{ Width, Height int }

type MouseCommand struct{}

// Controller translates Bubble Tea's terminal messages into stable commands.
// It deliberately does not know model data or overlay names.
type Controller struct {
	Selection    *selection.Controller
	previousView string
}

func New(writer clipboard.Writer) *Controller {
	return &Controller{Selection: selection.NewController(writer)}
}

func (c *Controller) SetWriter(writer clipboard.Writer) { c.Selection.SetWriter(writer) }

func (c *Controller) SetFrameToken(token uint64) {
	c.Selection.SetFrameToken(token)
}

// Transition invalidates the terminal when a logical composition changes.
// Bubble Tea then redraws the complete frame returned by Frame.
func (c *Controller) Transition(view string) tea.Cmd {
	if c.previousView == view {
		return nil
	}
	previous := c.previousView
	c.previousView = view
	return InvalidateOnTransition(previous, view)
}

func (c *Controller) Handle(msg input.MessageEvent, frame []string, context string) (any, tea.Cmd) {
	switch msg.Kind {
	case input.Key:
		return KeyCommand{Value: msg.Key.Key, Original: msg.Key.Original, Runes: msg.Key.Runes}, nil
	case input.Window:
		return ResizeCommand{Width: msg.Window.Width, Height: msg.Window.Height}, nil
	case input.Mouse:
		effect, cmd := c.Selection.Handle(msg.Mouse, frame, context)
		if effect.Selected {
			return MouseCommand{}, cmd
		}
		return nil, cmd
	}
	return nil, nil
}

func InvalidateOnTransition(previous, current string) tea.Cmd {
	if previous == current {
		return nil
	}
	return func() tea.Msg { return tea.ClearScreen() }
}

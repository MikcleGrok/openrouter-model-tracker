package selection

import (
	"slices"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sboborikin/openrouter-model-tracker/internal/tui/clipboard"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/input"
)

type Effect struct {
	Status   string
	Selected bool
}

type Result struct {
	Token uint64
	Error error
}

type State struct {
	Active, Dragging bool
	Anchor, End      Point
	Context          string
	Frame            []string
	Generation       uint64
}

type Controller struct {
	mu         sync.Mutex
	writer     clipboard.Writer
	state      State
	frameToken uint64
	token      uint64
	pending    bool
}

func NewController(writer clipboard.Writer) *Controller { return &Controller{writer: writer} }

func (c *Controller) SetWriter(writer clipboard.Writer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writer = writer
}

func (c *Controller) Snapshot() State { c.mu.Lock(); defer c.mu.Unlock(); return c.state }

func (c *Controller) Clear() { c.mu.Lock(); defer c.mu.Unlock(); c.clearLocked() }

func (c *Controller) SetFrameToken(token uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frameToken = token
	if c.state.Generation != token {
		c.clearLocked()
	}
}

func (c *Controller) Handle(msg input.MouseEvent, frame []string, context string) (Effect, tea.Cmd) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg.Action == input.MousePress && (msg.Button == input.MouseWheelUp || msg.Button == input.MouseWheelDown) {
		c.clearLocked()
		return Effect{}, nil
	}
	if msg.Action == input.MousePress && msg.Button == input.MouseLeft {
		point, ok := pointInFrame(frame, msg.X, msg.Y, false)
		if !ok {
			c.clearLocked()
			return Effect{Status: "no selectable text at cursor"}, nil
		}
		c.clearLocked()
		c.state = State{Active: true, Dragging: true, Anchor: point, End: point, Context: context, Frame: append([]string(nil), frame...), Generation: c.frameToken}
		return Effect{Selected: true}, nil
	}
	if msg.Action == input.MouseMotion && c.state.Active && c.state.Dragging && (msg.Button == input.MouseLeft || msg.Button == input.MouseNone) {
		if !slices.Equal(c.state.Frame, frame) {
			c.clearLocked()
			return Effect{Status: "selection frame is stale"}, nil
		}
		if point, ok := pointInFrame(c.state.Frame, msg.X, msg.Y, true); ok {
			c.state.End = point
		}
		return Effect{Selected: true}, nil
	}
	if msg.Action != input.MouseRelease || !c.state.Active || (msg.Button != input.MouseLeft && msg.Button != input.MouseNone) {
		return Effect{}, nil
	}
	if !slices.Equal(c.state.Frame, frame) {
		c.clearLocked()
		return Effect{Status: "selection frame is stale"}, nil
	}
	point, ok := pointInFrame(c.state.Frame, msg.X, msg.Y, true)
	if !ok {
		point = clampedPoint(c.state.Frame, msg.X, msg.Y)
	}
	c.state.End, c.state.Dragging = point, false
	cmd, status := c.copyLocked()
	return Effect{Selected: true, Status: status}, cmd
}

func (c *Controller) Copy(frameToken uint64) tea.Cmd {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Generation != frameToken {
		c.clearLocked()
		return nil
	}
	cmd, _ := c.copyLocked()
	return cmd
}

func (c *Controller) Result(msg tea.Msg) (uint64, error, bool) {
	result, ok := msg.(Result)
	if !ok {
		return 0, nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pending || result.Token != c.token {
		return result.Token, nil, false
	}
	c.pending = false
	return result.Token, result.Error, true
}

func (c *Controller) Paint(rendered string, context string, frameToken uint64) string {
	state := c.Snapshot()
	if state.Generation != frameToken {
		return rendered
	}
	return Paint(rendered, state.Active && state.Context == context, state.Anchor, state.End)
}

func (c *Controller) clearLocked() {
	c.state = State{}
	c.pending = false
	c.token++
	c.state.Generation = c.frameToken
}

func (c *Controller) copyLocked() (tea.Cmd, string) {
	if !c.state.Active {
		return nil, ""
	}
	text := Text(c.state.Frame, c.state.Anchor, c.state.End)
	if text == "" {
		c.clearLocked()
		return nil, "selection is empty"
	}
	c.token++
	token, writer := c.token, c.writer
	c.pending = true
	return func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.pending || c.token != token {
			return Result{Token: token}
		}
		if writer == nil {
			return Result{Token: token}
		}
		return Result{Token: token, Error: writer(text)}
	}, ""
}

func pointInFrame(frame []string, x, y int, allowEnd bool) (Point, bool) {
	if x < 0 || y < 0 || y >= len(frame) {
		return Point{}, false
	}
	width := ansi.StringWidth(strings.TrimRight(frame[y], " "))
	if width == 0 || x > width || !allowEnd && x == width {
		return Point{}, false
	}
	return Point{Line: y, Column: x}, true
}

func clampedPoint(frame []string, x, y int) Point {
	if len(frame) == 0 {
		return Point{}
	}
	y = max(0, min(y, len(frame)-1))
	width := ansi.StringWidth(strings.TrimRight(frame[y], " "))
	return Point{Line: y, Column: max(0, min(x, width))}
}

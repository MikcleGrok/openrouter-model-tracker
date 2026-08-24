package selection

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/input"
)

func mouse(msg tea.MouseMsg) input.MouseEvent {
	event, ok := input.FromMessage(msg)
	if !ok {
		panic("not a mouse message")
	}
	return event.Mouse
}

func TestControllerMouseLifecycleAnchorFocusAndBounds(t *testing.T) {
	c := NewController(func(string) error { return nil })
	frame := []string{"alpha", "beta"}
	effect, _ := c.Handle(mouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), frame, "main")
	if !effect.Selected || !c.Snapshot().Dragging {
		t.Fatalf("press effect/state = %+v/%+v", effect, c.Snapshot())
	}
	effect, _ = c.Handle(mouse(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}), frame, "main")
	if !effect.Selected || c.Snapshot().End != (Point{1, 2}) {
		t.Fatalf("motion state = %+v", c.Snapshot())
	}
	effect, cmd := c.Handle(mouse(tea.MouseMsg{X: 999, Y: 999, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), frame, "main")
	if !effect.Selected || cmd == nil || c.Snapshot().Dragging {
		t.Fatalf("release effect/state/cmd = %+v/%+v/%v", effect, c.Snapshot(), cmd != nil)
	}
}

func TestControllerEmptySelectionAndInvalidBounds(t *testing.T) {
	c := NewController(func(string) error { t.Fatal("empty selection copied"); return nil })
	effect, cmd := c.Handle(mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), []string{"x"}, "main")
	if !effect.Selected || cmd != nil {
		t.Fatalf("press = %+v/%v", effect, cmd)
	}
	effect, cmd = c.Handle(mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), []string{"x"}, "main")
	if effect.Status != "selection is empty" || cmd != nil || c.Snapshot().Active {
		t.Fatalf("empty release = %+v/%v/%+v", effect, cmd, c.Snapshot())
	}
	effect, _ = c.Handle(mouse(tea.MouseMsg{X: 4, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), []string{"x"}, "main")
	if effect.Status != "no selectable text at cursor" || c.Snapshot().Active {
		t.Fatalf("invalid press = %+v/%+v", effect, c.Snapshot())
	}
}

func TestControllerCopySuccessAndWriterError(t *testing.T) {
	var copied string
	c := NewController(func(value string) error { copied = value; return nil })
	_, _ = c.Handle(mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), []string{"alpha"}, "main")
	_, cmd := c.Handle(mouse(tea.MouseMsg{X: 2, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), []string{"alpha"}, "main")
	if _, err, ok := c.Result(cmd()); !ok || err != nil || copied != "al" {
		t.Fatalf("copy result = ok:%v err:%v copied:%q", ok, err, copied)
	}
	c.SetWriter(func(string) error { return errors.New("unavailable") })
	_, cmd = c.Handle(mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), []string{"x"}, "main")
	_, cmd = c.Handle(mouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), []string{"x"}, "main")
	if _, err, ok := c.Result(cmd()); !ok || err == nil || err.Error() != "unavailable" {
		t.Fatalf("writer error result = ok:%v err:%v", ok, err)
	}
}

func TestControllerDropsStaleGeneration(t *testing.T) {
	var writes []string
	c := NewController(func(value string) error { writes = append(writes, value); return nil })
	_, _ = c.Handle(mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), []string{"old"}, "main")
	_, old := c.Handle(mouse(tea.MouseMsg{X: 3, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), []string{"old"}, "main")
	_, _ = c.Handle(mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), []string{"new"}, "main")
	_, fresh := c.Handle(mouse(tea.MouseMsg{X: 3, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), []string{"new"}, "main")
	if _, _, ok := c.Result(old()); ok {
		t.Fatal("stale result accepted")
	}
	if _, err, ok := c.Result(fresh()); !ok || err != nil {
		t.Fatalf("fresh result = ok:%v err:%v", ok, err)
	}
	if !strings.EqualFold(strings.Join(writes, ","), "new") {
		t.Fatalf("writes = %q", writes)
	}
}

func TestControllerInvalidatesPressedFrameBeforePaintAndCopy(t *testing.T) {
	c := NewController(func(string) error { t.Fatal("stale selection copied"); return nil })
	oldFrame := []string{"old content"}
	newFrame := []string{"new content"}
	_, _ = c.Handle(mouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), oldFrame, "list")
	if !c.Snapshot().Active {
		t.Fatal("press did not activate selection")
	}
	effect, cmd := c.Handle(mouse(tea.MouseMsg{X: 3, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}), newFrame, "list")
	if cmd != nil || effect.Status != "selection frame is stale" || c.Snapshot().Active {
		t.Fatalf("stale release = effect:%+v cmd:%v state:%+v", effect, cmd, c.Snapshot())
	}

	_, _ = c.Handle(mouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), oldFrame, "list")
	c.SetFrameToken(c.Snapshot().Generation + 1)
	if state := c.Snapshot(); state.Active || len(state.Frame) != 0 || state.Generation == 0 {
		t.Fatalf("invalidated selection state = %+v", state)
	}
}

func TestControllerRejectsPaintAndCopyAfterExternalFrameTokenChange(t *testing.T) {
	var copied string
	c := NewController(func(value string) error { copied = value; return nil })
	oldFrame := []string{"old text"}
	_, _ = c.Handle(mouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}), oldFrame, "list")
	oldToken := c.Snapshot().Generation
	newToken := oldToken + 1
	c.SetFrameToken(newToken)
	if got := c.Paint("old text", "list", oldToken); got != "old text" {
		t.Fatalf("stale paint = %q, want unpainted text", got)
	}
	if cmd := c.Copy(oldToken); cmd != nil {
		t.Fatal("stale copy command was created")
	}
	if c.Snapshot().Active || copied != "" {
		t.Fatalf("stale selection survived token change: state=%+v copied=%q", c.Snapshot(), copied)
	}
}

func TestTextUsesOrderedCellBoundariesAndStripsTerminalProtocol(t *testing.T) {
	lines := []string{"界🙂model  ", "\x1b[31msecond\x1b[0m"}
	if got := Text(lines, Point{1, ansi.StringWidth("second")}, Point{0, 0}); got != "界🙂model\nsecond" {
		t.Fatalf("Text() = %q", got)
	}
	if got := Text(lines, Point{-1, 0}, Point{0, 1}); got != "" {
		t.Fatalf("out-of-range Text() = %q", got)
	}
}

func TestPaintPreservesCurrentANSIFrameAndOnlyPaintsRange(t *testing.T) {
	frame := "\x1b[31mcurrent text\x1b[0m\nsecond"
	got := Paint(frame, true, Point{0, 0}, Point{0, 7})
	if ansi.Strip(got) != ansi.Strip(frame) {
		t.Fatalf("Paint changed text: %q", got)
	}
	if !contains(got, "\x1b[31m") || !contains(got, "\x1b[7m") {
		t.Fatalf("Paint lost ANSI state: %q", got)
	}
}

func contains(value, needle string) bool {
	return len(value) >= len(needle) && stringIndex(value, needle) >= 0
}
func stringIndex(value, needle string) int {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

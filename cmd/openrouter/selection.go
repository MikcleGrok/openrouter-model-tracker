package main

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var tuiClipboardMu sync.Mutex

type tuiSelectionPoint struct {
	line, column int
}

type tuiSelection struct {
	active, dragging bool
	anchor, end      tuiSelectionPoint
	context          string
	frame            []string
	version          uint64
}

type tuiClipboardResultMsg struct {
	token uint64
	err   error
}

type tuiClipboardState struct {
	token uint64
}

type tuiSynchronizedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

type tuiFrameWriter struct {
	out io.Writer
}

func (w *tuiFrameWriter) Write(p []byte) (int, error) {
	frame := strings.ReplaceAll(string(p), "\n", ansi.EraseLineRight+"\n") + ansi.EraseLineRight
	if _, err := io.WriteString(w.out, frame); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *tuiSynchronizedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}

func (m tuiModel) selectionContext() string { return m.overlay + "\x00" + m.inputMode }

func (m tuiModel) renderedFrame() []string { return strings.Split(ansi.Strip(m.baseView()), "\n") }

func (m tuiModel) selectionPoint(x, y int) (tuiSelectionPoint, bool) {
	return m.selectionPointInFrame(m.selection.frame, x, y, true)
}

func (m tuiModel) clampedSelectionPoint(x, y int) tuiSelectionPoint {
	if len(m.selection.frame) == 0 {
		return tuiSelectionPoint{}
	}
	y = max(0, min(y, len(m.selection.frame)-1))
	width := ansi.StringWidth(strings.TrimRight(m.selection.frame[y], " "))
	return tuiSelectionPoint{line: y, column: max(0, min(x, width))}
}

func (m tuiModel) selectionStartPoint(frame []string, x, y int) (tuiSelectionPoint, bool) {
	return m.selectionPointInFrame(frame, x, y, false)
}

func (m tuiModel) selectionPointInFrame(frame []string, x, y int, allowEnd bool) (tuiSelectionPoint, bool) {
	if x < 0 || y < 0 || y >= len(frame) {
		return tuiSelectionPoint{}, false
	}
	line := frame[y]
	width := ansi.StringWidth(strings.TrimRight(line, " "))
	if width == 0 || x > width || !allowEnd && x == width {
		return tuiSelectionPoint{}, false
	}
	return tuiSelectionPoint{line: y, column: x}, true
}

func (m tuiModel) updateSelection(msg tea.MouseMsg) (tuiModel, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		m.clearSelection()
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		frame := m.renderedFrame()
		if msg.Y < 0 || msg.Y >= len(frame) || msg.X < 0 {
			m.clearSelection()
			m.status = "no selectable text at cursor"
			return m, nil
		}
		if _, ok := m.selectionStartPoint(frame, msg.X, msg.Y); !ok {
			m.clearSelection()
			m.status = "no selectable text at cursor"
			return m, nil
		}
		m.clearSelection()
		m.selection = tuiSelection{active: true, dragging: true, anchor: tuiSelectionPoint{line: msg.Y, column: msg.X}, end: tuiSelectionPoint{line: msg.Y, column: msg.X}, context: m.selectionContext(), frame: append([]string(nil), frame...), version: m.selection.version + 1}
		return m, nil
	}
	if msg.Action == tea.MouseActionMotion && m.selection.active && m.selection.dragging && (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone) {
		if point, ok := m.selectionPoint(msg.X, msg.Y); ok {
			m.selection.end = point
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionRelease || !m.selection.active || (msg.Button != tea.MouseButtonLeft && msg.Button != tea.MouseButtonNone) {
		return m, nil
	}
	point, ok := m.selectionPoint(msg.X, msg.Y)
	if !ok {
		point = m.clampedSelectionPoint(msg.X, msg.Y)
	}
	m.selection.end = point
	m.selection.dragging = false
	return m, m.copyActiveSelection()
}

func (m *tuiModel) copyActiveSelection() tea.Cmd {
	if !m.selection.active {
		return nil
	}
	text := tuiSelectionText(m.selection.frame, m.selection.anchor, m.selection.end)
	if text == "" {
		m.clearSelection()
		m.status = "selection is empty"
		return nil
	}
	m.clipboardToken++
	token, writer := m.clipboardToken, m.clipboardWrite
	state := m.clipboardState
	if state == nil {
		state = &tuiClipboardState{token: token}
		m.clipboardState = state
	} else {
		tuiClipboardMu.Lock()
		state.token = token
		tuiClipboardMu.Unlock()
	}
	if writer == nil {
		output := m.clipboardOutput
		writer = func(value string) error { return tuiWriteClipboard(value, output) }
	}
	m.clipboardPending = true
	return func() tea.Msg {
		tuiClipboardMu.Lock()
		defer tuiClipboardMu.Unlock()
		if state.token != token {
			return tuiClipboardResultMsg{token: token}
		}
		return tuiClipboardResultMsg{token: token, err: writer(text)}
	}
}

func tuiWriteClipboard(text string, output io.Writer) error {
	if err := clipboard.WriteAll(text); err == nil {
		return nil
	}
	if output == nil {
		output = os.Stdout
	}
	sequence := osc52.New(text)
	if os.Getenv("TMUX") != "" {
		sequence = sequence.Tmux()
	}
	_, err := sequence.WriteTo(output)
	return err
}

func (m *tuiModel) clearSelection() {
	m.selection.active = false
	m.selection.dragging = false
	m.clipboardPending = false
	m.selection.version++
	m.clipboardToken++
	if m.clipboardState != nil {
		tuiClipboardMu.Lock()
		m.clipboardState.token = m.clipboardToken
		tuiClipboardMu.Unlock()
	}
}

func tuiSelectionText(lines []string, a, b tuiSelectionPoint) string {
	if a.line > b.line || a.line == b.line && a.column > b.column {
		a, b = b, a
	}
	if a.line < 0 || b.line >= len(lines) || a.line > b.line {
		return ""
	}
	parts := make([]string, 0, b.line-a.line+1)
	for line := a.line; line <= b.line; line++ {
		start, finish := 0, ansi.StringWidth(strings.TrimRight(lines[line], " "))
		if line == a.line {
			start = a.column
		}
		if line == b.line {
			finish = b.column
		}
		if finish < start {
			finish = start
		}
		parts = append(parts, ansi.Cut(lines[line], start, finish))
	}
	return strings.TrimRight(ansi.Strip(strings.Join(parts, "\n")), " \n")
}

func tuiRenderSelection(rendered string, selection tuiSelection) string {
	if !selection.active || selection.context == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	a, b := selection.anchor, selection.end
	if a.line > b.line || a.line == b.line && a.column > b.column {
		a, b = b, a
	}
	for line := a.line; line <= b.line && line < len(lines) && line < len(selection.frame); line++ {
		current := lines[line]
		plain := ansi.Strip(current)
		start, finish := 0, ansi.StringWidth(strings.TrimRight(plain, " "))
		if line == a.line {
			start = a.column
		}
		if line == b.line {
			finish = b.column
		}
		if finish > start {
			lines[line] = ansi.Cut(current, 0, start) + ansi.SelectGraphicRendition(ansi.ReverseAttr) + ansi.Cut(current, start, finish) + ansi.SelectGraphicRendition(ansi.NoReverseAttr) + ansi.Cut(current, finish, ansi.StringWidth(plain))
		}
	}
	return strings.Join(lines, "\n")
}

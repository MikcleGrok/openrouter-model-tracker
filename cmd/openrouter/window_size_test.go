package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// This file proves, through a real tea.Program and the terminal emulator in
// runtime_session_test.go, what a missing WindowSizeMsg actually costs — the
// second half of the chain internal/tui/clipboard's
// TestSynchronizedKeepsTheWrappedTerminalRecognizable guards the first half of.
//
// WindowSizeMsg is the only thing that ever assigns the patched renderer's own
// r.width/r.height (internal/thirdparty/bubbletea-patched/standard_renderer.go:
// handleMessages stages it, applyPendingResize adopts it). No startup option
// reaches those fields, and the model's own width/height are a separate thing
// entirely. flush() appends ansi.EraseLineRight to a repainted line only when
// ansi.StringWidth(line) < r.width — a comparison that is false for every line
// while r.width is 0. So a program that is never told its size never erases the
// tail of a line that got shorter, and the previous frame's leftover glyphs
// stay on screen next to the new content: the reported "blurred / duplicated
// text".
//
// Unlike the resize race renderer_resize_test.go polices, this needs no timing
// at all. One shrinking line is enough, at any point in the session.

const (
	staleTailWidth   = 24
	staleTailHeight  = 3
	staleTailLong    = "hello there!"
	staleTailShort   = "hi"
	staleTailQuiet   = 80 * time.Millisecond
	staleTailTimeout = 5 * time.Second
)

// staleTailSetMsg replaces the model's single rendered line.
type staleTailSetMsg string

// staleTailModel renders exactly one line, whose length the test controls, so
// the only thing the renderer has to get right is erasing what the line
// vacates when it shrinks.
type staleTailModel struct{ text string }

func (m staleTailModel) Init() tea.Cmd { return nil }

func (m staleTailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if text, ok := msg.(staleTailSetMsg); ok {
		m.text = string(text)
	}
	return m, nil
}

func (m staleTailModel) View() string { return m.text }

// TestProgramToldItsWindowSizeErasesAShrinkingLine is the behaviour production
// is supposed to have: the WindowSizeMsg a real terminal delivers gives the
// renderer a width, and the shrinking line's vacated tail is erased.
func TestProgramToldItsWindowSizeErasesAShrinkingLine(t *testing.T) {
	rp := startRuntimeProgramForModel(t, staleTailModel{text: staleTailLong}, staleTailWidth, staleTailHeight)
	sess := newRuntimeSession(staleTailWidth, staleTailHeight)

	if err := feedStaleTailWrites(sess, drainRuntimeWritesUntilQuiet(rp, staleTailQuiet)); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if got := strings.TrimRight(sess.Rows()[0], " "); got != staleTailLong {
		t.Fatalf("startup row 0 = %q, want %q", got, staleTailLong)
	}

	rp.Send(staleTailSetMsg(staleTailShort))
	if err := feedStaleTailWrites(sess, drainRuntimeWritesUntilQuiet(rp, staleTailQuiet)); err != nil {
		t.Fatalf("shrinking the line left content the renderer never erased: %v", err)
	}
	if got := strings.TrimRight(sess.Rows()[0], " "); got != staleTailShort {
		t.Fatalf("row 0 = %q, want %q with the vacated tail erased", got, staleTailShort)
	}

	rp.Quit(t, staleTailTimeout)
}

// TestProgramNeverToldItsWindowSizeLeavesAStaleTail is the same script against
// a program that never received a WindowSizeMsg — which is precisely the state
// an output wrapper that hides its term.File identity puts production in. The
// renderer is stuck at r.width == 0, the shrinking line is painted without
// ansi.EraseLineRight, and the tail of the longer previous frame survives on
// screen.
//
// It asserts the damage rather than the fix, deliberately: it is the executable
// statement of why the WindowSizeMsg has to arrive at all, and it fails the day
// the renderer stops depending on r.width — at which point the invariant the
// clipboard wrapper maintains would need re-justifying rather than silently
// carrying on.
func TestProgramNeverToldItsWindowSizeLeavesAStaleTail(t *testing.T) {
	rp := launchRuntimeProgram(t, staleTailModel{text: staleTailLong})
	sess := newRuntimeSession(staleTailWidth, staleTailHeight)

	if err := feedStaleTailWrites(sess, drainRuntimeWritesUntilQuiet(rp, staleTailQuiet)); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if got := strings.TrimRight(sess.Rows()[0], " "); got != staleTailLong {
		t.Fatalf("startup row 0 = %q, want %q", got, staleTailLong)
	}

	rp.Send(staleTailSetMsg(staleTailShort))
	err := feedStaleTailWrites(sess, drainRuntimeWritesUntilQuiet(rp, staleTailQuiet))
	if err == nil {
		t.Fatalf("expected leftover content on row 0 (%q), got a clean shrink to %q — "+
			"a renderer at width 0 has no way to decide to erase the vacated tail",
			sess.Rows()[0], staleTailShort)
	}
	if !strings.Contains(err.Error(), "stale content") {
		t.Fatalf("expected a stale-content report, got: %v", err)
	}
	if got := sess.Rows()[0]; !strings.HasPrefix(got, staleTailShort) || !strings.Contains(got, staleTailLong[len(staleTailShort):]) {
		t.Fatalf("row 0 = %q, want the new %q with the old line's tail still visible behind it", got, staleTailShort)
	}

	rp.Quit(t, staleTailTimeout)
}

// feedStaleTailWrites replays raw renderer writes into sess, one Frame() per
// Write(), and returns the first frame the session rejected. Unlike
// feedRuntimeWrites it hands the error back instead of failing the test,
// because here a rejected frame is the expected outcome of one of the two
// scripts.
func feedStaleTailWrites(sess *runtimeSession, writes []string) error {
	for _, w := range writes {
		if _, err := sess.Frame(w); err != nil {
			return err
		}
	}
	return nil
}

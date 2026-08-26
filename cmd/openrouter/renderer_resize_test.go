package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// This file polices one property of the patched renderer
// (internal/thirdparty/bubbletea-patched/standard_renderer.go), not of the
// product's TUI model: the renderer must never paint a
// (dimensions, content) pair that was never produced together.
//
// The race it guards, in upstream bubbletea v1.3.10 terms. Per message, the
// event loop (tea.go eventLoop) does three things in a row, on its own
// goroutine:
//
//  1. standardRenderer.handleMessages(msg) — for a WindowSizeMsg this takes
//     the renderer's mutex, assigns r.width/r.height and calls repaint()
//     (which clears lastRender/lastRenderedLines), then releases it.
//  2. model.Update(msg) — arbitrary application work, no lock held. For a
//     resize this is the expensive one: a full re-layout.
//  3. renderer.write(model.View()) — takes the mutex again and replaces
//     r.buf with the content laid out for the NEW size.
//
// Steps 1 and 3 are two separate critical sections with unbounded app work
// between them, while the renderer's own ticker goroutine (listen()) calls
// flush() on a ~16.67ms timer under that same mutex. A tick landing in the
// gap flushes r.buf — still the PRE-resize content — against the already
// updated r.width/r.height: old content, truncated and cropped to the new
// geometry, painted at the wrong screen positions. repaint() in step 1 makes
// this worse rather than rarer: by clearing lastRender it defeats flush()'s
// own "nothing changed" early-out, so the stale buffer is guaranteed to be
// painted rather than skipped.
//
// The artifact is one frame and self-heals on the next flush, which always
// carries a matching content+dimensions pair. It is still a real tear, and
// over a drag-resize (many SIGWINCH events, each a full table re-layout)
// it is hit repeatedly.
const (
	resizeStampInitialWidth  = 80
	resizeStampInitialHeight = 20
	resizeStampFinalWidth    = 40
	resizeStampFinalHeight   = 6
)

const (
	// resizeStampObserveWindow is how long the test watches the renderer
	// while model.Update is deliberately parked mid-resize. The renderer
	// ticks at 60fps (standard_renderer.go defaultFPS), so this window
	// spans ~9 ticks: any one of them is enough to expose the tear, which
	// is what makes an inherently timing-dependent race reproduce
	// deterministically rather than occasionally.
	resizeStampObserveWindow = 150 * time.Millisecond
	// resizeStampQuietWindow is how long the renderer must emit nothing
	// before its output is considered settled. Comfortably more than one
	// tick period, so a settled screen is not mistaken for a slow one.
	resizeStampQuietWindow = 80 * time.Millisecond
	// resizeStampSyncTimeout bounds the waits that are not timing
	// experiments but plain "did the program get this at all" handshakes.
	resizeStampSyncTimeout = 5 * time.Second
)

// sizeStampPokeMsg is a message the model deliberately ignores. Handling it
// still runs the event loop's full per-message path, so it ends in
// renderer.write(model.View()) with content identical to what is already on
// screen — which is exactly how production behaves under the mouse-motion
// traffic tea.WithMouseCellMotion() generates, and which is what leaves
// r.buf non-empty (dirty) going into the resize. See the test body.
type sizeStampPokeMsg struct{}

// sizeStampModel is a minimal tea.Model built to make the renderer-level
// property above observable, and nothing else.
//
// Every line it renders stamps the exact WIDTHxHEIGHT it was laid out for,
// so any painted cell can be attributed to the layout that produced it —
// the tear stops being "text looks wrong" and becomes "content stamped
// [80x20] is on screen after the renderer adopted 40x6". Lines are padded
// to the full width so a frame touches every column, keeping
// runtimeSession's stale-content bookkeeping unambiguous.
//
// Its second job is to widen the race window on demand: on a resize that
// actually changes the size it announces itself on entered and parks on
// release until the test lets it go, standing in for the real cost of a
// full table re-layout without depending on how long that happens to take.
type sizeStampModel struct {
	w, h    int
	entered chan<- struct{}
	release <-chan struct{}
}

func (m sizeStampModel) Init() tea.Cmd { return nil }

func (m sizeStampModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	size, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		return m, nil
	}
	// The startup WindowSizeMsg carries the size the model was constructed
	// at (see startRuntimeProgramForModel), so gating on an actual change
	// parks only on the resize the test is about.
	if size.Width != m.w || size.Height != m.h {
		select {
		case m.entered <- struct{}{}:
		default:
		}
		<-m.release
	}
	m.w, m.h = size.Width, size.Height
	return m, nil
}

func (m sizeStampModel) View() string {
	lines := make([]string, m.h)
	for i := range lines {
		lines[i] = sizeStampLine(m.w, m.h, i+1)
	}
	return strings.Join(lines, "\n")
}

// sizeStampMarker is the prefix every line of a WIDTHxHEIGHT layout carries.
func sizeStampMarker(w, h int) string { return fmt.Sprintf("[%dx%d]", w, h) }

func sizeStampLine(w, h, n int) string {
	label := fmt.Sprintf("%s#%02d", sizeStampMarker(w, h), n)
	if len(label) >= w {
		return label[:w]
	}
	return label + strings.Repeat(".", w-len(label))
}

// TestRendererNeverPaintsPreResizeContentAtNewDimensions drives a real
// tea.Program through a resize whose Update is parked mid-flight, and
// asserts that nothing the renderer paints during that window carries the
// pre-resize layout.
//
// The sequence is built so the race is not a matter of luck:
//
//  1. Let startup settle. The last flush leaves lastRender == the 80x20
//     view and r.buf empty.
//  2. Send a poke the model ignores. The event loop still calls
//     write(model.View()), so r.buf goes dirty again with content byte-identical
//     to lastRender. This is the precondition the tear needs and the one
//     production satisfies continuously: r.buf dirty at the moment the
//     resize arrives. Note that a tick landing here paints nothing — flush()
//     early-outs on buf == lastRender WITHOUT resetting the buffer, so the
//     buffer stays dirty.
//  3. Resize to 40x6. Update parks, holding the loop between handleMessages
//     and write for as long as the test wants.
//  4. Watch. Unpatched, step 1's repaint() has cleared lastRender, so
//     flush()'s early-out no longer fires and the very next tick paints
//     the 80x20 buffer cropped to 40x6. Patched, the resize has not been
//     applied yet, buf still equals lastRender, and every tick in the
//     window is a no-op.
//  5. Release, and confirm the resize does complete — the fix must close
//     the race, not stall the resize.
func TestRendererNeverPaintsPreResizeContentAtNewDimensions(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	rp := startRuntimeProgramForModel(t, sizeStampModel{
		w:       resizeStampInitialWidth,
		h:       resizeStampInitialHeight,
		entered: entered,
		release: release,
	}, resizeStampInitialWidth, resizeStampInitialHeight)

	var releaseOnce sync.Once
	releaseGate := func() { releaseOnce.Do(func() { close(release) }) }
	// Registered after the program's own stop cleanup so that, on a
	// mid-test t.Fatalf, LIFO cleanup order unparks the model first and
	// the program can then actually quit instead of timing out.
	t.Cleanup(releaseGate)

	oldMarker := sizeStampMarker(resizeStampInitialWidth, resizeStampInitialHeight)
	newMarker := sizeStampMarker(resizeStampFinalWidth, resizeStampFinalHeight)

	// Step 1: settle at the initial size.
	sess := newRuntimeSession(resizeStampInitialWidth, resizeStampInitialHeight)
	rows := feedRuntimeWrites(t, sess, drainRuntimeWritesUntilQuiet(rp, resizeStampQuietWindow), "startup")
	if !runtimeRowsContain(rows, oldMarker) {
		t.Fatalf("startup: expected the %s layout on screen, got:\n%s", oldMarker, strings.Join(rows, "\n"))
	}

	// Step 2: re-dirty r.buf with content identical to what was just painted.
	rp.Send(sizeStampPokeMsg{})

	// Step 3: resize, and wait until the model is parked inside Update.
	rp.Send(tea.WindowSizeMsg{Width: resizeStampFinalWidth, Height: resizeStampFinalHeight})
	select {
	case <-entered:
	case <-time.After(resizeStampSyncTimeout):
		t.Fatalf("model never received the resize within %s", resizeStampSyncTimeout)
	}

	// Step 4: watch the renderer while the resize is still being processed.
	// The physical terminal has already resized — that is what delivered the
	// event — so anything painted now lands in a 40x6 screen.
	parked := collectRuntimeWritesFor(rp, resizeStampObserveWindow)
	sess.Resize(resizeStampFinalWidth, resizeStampFinalHeight)
	rows = feedRuntimeWrites(t, sess, parked, "mid-resize")
	if runtimeRowsContain(rows, oldMarker) {
		t.Fatalf("render tear: the renderer painted %s content after adopting %dx%d, "+
			"i.e. the pre-resize buffer cropped to the post-resize geometry.\n"+
			"screen (%dx%d):\n%s",
			oldMarker, resizeStampFinalWidth, resizeStampFinalHeight,
			resizeStampFinalWidth, resizeStampFinalHeight, strings.Join(rows, "\n"))
	}

	// Step 5: the resize must still complete once the model is unparked.
	releaseGate()
	rows = feedRuntimeWrites(t, sess, drainRuntimeWritesUntilQuiet(rp, resizeStampQuietWindow), "post-resize")
	if !runtimeRowsContain(rows, newMarker) {
		t.Fatalf("post-resize: expected the %s layout on screen, got:\n%s", newMarker, strings.Join(rows, "\n"))
	}
	for i, row := range rows {
		if want := sizeStampLine(resizeStampFinalWidth, resizeStampFinalHeight, i+1); row != want {
			t.Fatalf("post-resize row %d: got %q, want %q", i, row, want)
		}
	}

	rp.Quit(t, resizeStampSyncTimeout)
}

// drainRuntimeWritesUntilQuiet collects raw renderer writes until none has
// arrived for quiet, i.e. until the renderer has settled.
func drainRuntimeWritesUntilQuiet(rp *runtimeProgram, quiet time.Duration) []string {
	var out []string
	for {
		s, ok := rp.NextWrite(quiet)
		if !ok {
			return out
		}
		out = append(out, s)
	}
}

// collectRuntimeWritesFor collects every raw renderer write arriving within
// d, always waiting the full window — unlike drainRuntimeWritesUntilQuiet it
// must not stop early, because "nothing was emitted" is the expected result
// and only elapsed time can establish it.
func collectRuntimeWritesFor(rp *runtimeProgram, d time.Duration) []string {
	deadline := time.Now().Add(d)
	var out []string
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return out
		}
		s, ok := rp.NextWrite(remaining)
		if !ok {
			return out
		}
		out = append(out, s)
	}
}

// feedRuntimeWrites replays writes into sess one Frame() per Write(), which
// is the contract runtime_program_test.go's frameWriter and
// runtime_session_test.go's per-frame bookkeeping both depend on, and
// returns the resulting screen.
func feedRuntimeWrites(t *testing.T, sess *runtimeSession, writes []string, stage string) []string {
	t.Helper()
	var rows []string
	for i, w := range writes {
		var err error
		rows, err = sess.Frame(w)
		if err != nil {
			t.Fatalf("%s: renderer write %d of %d could not be applied to the terminal: %v\nraw: %q", stage, i+1, len(writes), err, w)
		}
	}
	if rows == nil {
		rows = sess.Rows()
	}
	return rows
}

func runtimeRowsContain(rows []string, marker string) bool {
	for _, row := range rows {
		if strings.Contains(row, marker) {
			return true
		}
	}
	return false
}

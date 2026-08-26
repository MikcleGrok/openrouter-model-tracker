package main

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// runtimeStopTimeout bounds how long the t.Cleanup safety net waits for a
// program abandoned mid-script (t.Fatalf → runtime.Goexit) to shut down.
const runtimeStopTimeout = 2 * time.Second

// frameWriter forwards every Write() call the renderer makes into a
// channel, one channel item per Write() call, exactly as the bytes arrived.
//
// It is tempting to assume "one Write() = one rendered frame", since
// standard_renderer.go's flush() does issue exactly one r.out.Write() per
// paint. But the same renderer also writes bare control sequences through
// execute(), which shares that same io.Writer: entering the alt screen,
// hiding/showing the cursor, toggling bracketed paste and mouse modes —
// and, crucially, the ESC[2J/ESC[H pair that the product's own
// tea.ClearScreen() (internal/tui/screen/controller.go) emits on every
// overlay toggle.
//
// Those control writes are NOT noise to be filtered out: a real terminal
// receives them interleaved with the frames, and ESC[2J in particular
// blanks every cell. Dropping them would let the test's model of the screen
// drift away from a real one — stale content surviving where a terminal
// would have cleared it — which is the exact "harness diverges from
// production" failure class this file exists to avoid. So everything the
// renderer writes goes into the channel, in order, and the consumer replays
// all of it into the terminal emulator.
type frameWriter struct {
	mu     sync.Mutex
	writes chan string
}

func newFrameWriter() *frameWriter {
	return &frameWriter{writes: make(chan string, 64)}
}

func (w *frameWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	b := make([]byte, len(p))
	copy(b, p)
	select {
	case w.writes <- string(b):
	default:
		// The test isn't keeping up with NextWrite/DrainRemaining calls.
		// Fail loudly instead of deadlocking the renderer goroutine.
		panic("frameWriter: write channel full, test is not draining frames")
	}
	return len(p), nil
}

// runtimeProgram drives a real tea.Program against an in-memory writer
// instead of a real terminal, with production's exact option set (see
// startRuntimeProgram), and exposes the renderer's raw write boundaries in
// place of a sleep-then-drain capture.
type runtimeProgram struct {
	program *tea.Program
	fw      *frameWriter
	done    chan error

	stopOnce sync.Once
	stopped  bool
	runErr   error
}

// startRuntimeProgram starts m with the same tea.Program options production
// uses (tui.go's runTUIWithRankingConfigCompiled: WithContext, WithAltScreen,
// WithMouseCellMotion, WithOutput — and notably no WithANSICompressor, which
// teatest hardcodes and which fragments every paint into many small writes),
// plus the two deviations a test with no terminal needs: WithInput(nil),
// since the script drives everything through Send(), and WithoutSignals().
//
// The size is applied twice, deliberately, because a terminal supplies it
// twice:
//
//   - on the model, before tea.NewProgram — exactly what
//     runTUIWithRankingConfigCompiled does with tuiTerminalSize(out).
//     Without it the first paint would render newTUIModel's hardcoded
//     100x24 default, because
//     tea.Program.Run() calls renderer.start() — which starts the 60fps
//     ticker — before renderer.write(model.View()), and both of those run
//     before the event loop can process any message. A tick landing in that
//     window flushes whatever the model rendered at construction size.
//   - as a WindowSizeMsg, which is the only way to set the renderer's own
//     internal r.width/r.height (standard_renderer.go handleMessages) —
//     no startup option reaches them, and they drive truncation and
//     erase-line-right decisions.
//
// Seeding the model does not make the ticker race impossible: the renderer
// may still flush once before the WindowSizeMsg is processed, and then
// again after the repaint() that message forces. What it does is make both
// flushes carry the same visible content, so the frame count stops
// mattering. Callers must therefore never assume "one action, one frame" —
// see the checkpoint loop in runtime_capture_test.go, which replays every
// write into a terminal emulator and asserts on the resulting screen.
//
// The omission of WithANSICompressor above is load-bearing beyond the
// fragmentation it avoids for callers: detectStaleContent (runtime_session_test.go)
// depends on exactly one Write() call reaching exactly one sess.Frame() call,
// in order, unfiltered, to attribute a frame's touched columns correctly.
// Reintroducing WithANSICompressor (or anything else that merges/splits
// writes before they reach Frame()) would silently break that bookkeeping.
func startRuntimeProgram(t *testing.T, m tuiModel, width, height int) *runtimeProgram {
	t.Helper()
	m.width, m.height = width, height
	return startRuntimeProgramForModel(t, m, width, height)
}

// startRuntimeProgramForModel is startRuntimeProgram's model-agnostic core:
// the same production option set and the same startup sequence, for any
// tea.Model rather than the product's own tuiModel.
//
// It exists so that tests aimed at the renderer/tea.Program layer itself
// (renderer_resize_test.go) can drive a minimal synthetic model — one built
// to make a renderer-level property observable — without either duplicating
// the option set (which would let the two drift apart, defeating the whole
// point of the doc comment above) or dragging the product's TUI model into
// a test that is not about it.
//
// Unlike startRuntimeProgram it does not seed m's own size fields: a
// synthetic model owns its size representation. Callers must construct m
// already sized width x height, for exactly the reason the comment above
// gives — a tick can flush the construction-size view before the first
// WindowSizeMsg is processed.
func startRuntimeProgramForModel(t *testing.T, m tea.Model, width, height int) *runtimeProgram {
	t.Helper()
	fw := newFrameWriter()
	p := tea.NewProgram(m,
		tea.WithContext(context.Background()),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithOutput(fw),
		tea.WithInput(nil), // driven entirely via Send(); no real input source
		tea.WithoutSignals(),
	)
	rp := &runtimeProgram{program: p, fw: fw, done: make(chan error, 1)}
	go func() {
		_, err := p.Run()
		rp.done <- err
	}()
	// Safety net: a mid-script t.Fatalf aborts the test goroutine via
	// runtime.Goexit, which skips the Quit at the end of the script and
	// would leak the program and renderer-ticker goroutines for the rest
	// of the test binary. stop() runs at most once, so on the happy path
	// this is a no-op and Quit remains the one that asserts.
	t.Cleanup(func() { rp.stop(runtimeStopTimeout) })
	rp.Send(tea.WindowSizeMsg{Width: width, Height: height})
	return rp
}

// Send injects a message into the running program, exactly like real input
// or a resize event would.
func (rp *runtimeProgram) Send(msg tea.Msg) {
	rp.program.Send(msg)
}

// NextWrite returns the renderer's next raw write — a rendered frame or a
// bare control sequence, exactly as it was written — or reports false if
// none arrived within timeout. It deliberately filters nothing; see
// frameWriter's doc comment for why dropping control writes would corrupt
// the caller's model of the screen.
func (rp *runtimeProgram) NextWrite(timeout time.Duration) (string, bool) {
	select {
	case s := <-rp.fw.writes:
		return s, true
	case <-time.After(timeout):
		return "", false
	}
}

// DrainRemaining returns whatever raw writes are immediately queued, in
// order, without blocking. Only call this once the program is known to have
// stopped producing output — e.g. right after Quit has confirmed Run()
// returned, since shutdown() runs fully synchronously before Run() returns,
// so nothing more can arrive afterward.
func (rp *runtimeProgram) DrainRemaining() []string {
	var out []string
	for {
		select {
		case s := <-rp.fw.writes:
			out = append(out, s)
		default:
			return out
		}
	}
}

// stop sends a QuitMsg and waits for Run() to return, reporting whether it
// returned in time and with what error. Sending is safe at any point: once
// the program's context is cancelled — which Run() defers — Program.Send
// returns immediately instead of blocking, so a second QuitMsg after the
// program already exited (or after a Ctrl+C already triggered tea.Quit) is
// a no-op. The body runs at most once and later calls reuse its outcome,
// which is what lets Quit and the t.Cleanup safety net both call it.
func (rp *runtimeProgram) stop(timeout time.Duration) (bool, error) {
	rp.stopOnce.Do(func() {
		rp.program.Send(tea.Quit())
		select {
		case rp.runErr = <-rp.done:
			rp.stopped = true
		case <-time.After(timeout):
		}
	})
	return rp.stopped, rp.runErr
}

// Quit shuts the program down and asserts it did so cleanly — this is what
// confirms Run() actually returned without error.
func (rp *runtimeProgram) Quit(t *testing.T, timeout time.Duration) {
	t.Helper()
	stopped, err := rp.stop(timeout)
	if !stopped {
		t.Fatalf("runtimeProgram: timed out after %s waiting for program to finish", timeout)
	}
	if err != nil {
		t.Fatalf("runtimeProgram: Run() returned error: %v", err)
	}
}

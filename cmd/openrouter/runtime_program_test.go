package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// frameWriter forwards every Write() call the renderer makes into a
// channel, one channel item per Write() call, exactly as the bytes arrived.
//
// It is tempting to assume "one Write() = one rendered frame", since
// standard_renderer.go's flush() does issue exactly one r.out.Write() per
// paint. But the same renderer also writes bare control sequences (entering
// the alt screen, hiding/showing the cursor, toggling bracketed paste and
// mouse modes) through execute(), which shares that same io.Writer — see
// standard_renderer.go's execute()/flush(). Those control writes fire at
// program start and shutdown independently of any scripted action, so a
// consumer that wants "the frame this action produced" has to look past
// them; that filtering lives in runtimeProgram.NextFrame below, not here.
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
		// The test isn't keeping up with NextFrame/DrainRemaining calls.
		// Fail loudly instead of deadlocking the renderer goroutine.
		panic("frameWriter: write channel full, test is not draining frames")
	}
	return len(p), nil
}

// hasVisibleContent reports whether a raw write carries actual rendered
// content, as opposed to being purely a renderer state-transition control
// sequence (see frameWriter's doc comment for why the two share a Writer).
// ansi.Strip removes ANSI/CSI escape sequences; a control-only write leaves
// nothing but whitespace/control bytes (e.g. "\r") behind, while a real
// frame from flush() always carries the rendered view's visible text.
func hasVisibleContent(s string) bool {
	return strings.TrimSpace(ansi.Strip(s)) != ""
}

// runtimeProgram drives a real tea.Program the same way production does
// (cmd/openrouter/tui.go: tea.WithAltScreen(), no tea.WithANSICompressor())
// against an in-memory writer instead of a real terminal, and exposes a
// deterministic per-frame signal in place of a sleep-then-drain capture.
type runtimeProgram struct {
	program *tea.Program
	fw      *frameWriter
	done    chan error
}

// startRuntimeProgram starts m and sends the initial WindowSizeMsg a real
// terminal would have supplied before the first paint (production reads
// this from the real TTY via term.GetSize in tuiTerminalSize; this harness
// has no real terminal, so it supplies it explicitly instead).
func startRuntimeProgram(m tea.Model, width, height int) *runtimeProgram {
	fw := newFrameWriter()
	p := tea.NewProgram(m,
		tea.WithContext(context.Background()),
		tea.WithAltScreen(),
		tea.WithOutput(fw),
		tea.WithInput(nil), // driven entirely via Send(); no real input source
		tea.WithoutSignals(),
	)
	rp := &runtimeProgram{program: p, fw: fw, done: make(chan error, 1)}
	go func() {
		_, err := p.Run()
		rp.done <- err
	}()
	rp.Send(tea.WindowSizeMsg{Width: width, Height: height})
	return rp
}

// Send injects a message into the running program, exactly like real input
// or a resize event would.
func (rp *runtimeProgram) Send(msg tea.Msg) {
	rp.program.Send(msg)
}

// NextFrame blocks until the renderer produces its next real content frame
// — skipping any bare control-sequence writes along the way — or fails the
// test if none arrives within the given (single, overall) timeout.
func (rp *runtimeProgram) NextFrame(t *testing.T, timeout time.Duration) string {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case s := <-rp.fw.writes:
			if hasVisibleContent(s) {
				return s
			}
		case <-deadline:
			t.Fatalf("runtimeProgram: timed out after %s waiting for a frame", timeout)
			return ""
		}
	}
}

// DrainRemaining returns whatever raw writes (content or bare control
// sequences alike) are immediately queued, without blocking. Only call this
// once the program is known to have stopped producing output — e.g. right
// after Quit has confirmed Run() returned, since shutdown() runs fully
// synchronously before Run() returns, so nothing more can arrive afterward.
func (rp *runtimeProgram) DrainRemaining() string {
	var b strings.Builder
	for {
		select {
		case s := <-rp.fw.writes:
			b.WriteString(s)
		default:
			return b.String()
		}
	}
}

// Quit sends a QuitMsg — a no-op if the program is already shutting down or
// has already exited, e.g. because a prior key (Ctrl+C) already triggered
// tea.Quit — and blocks until Run() actually returns, failing the test on
// error or timeout. This is what confirms the program shut down cleanly.
func (rp *runtimeProgram) Quit(t *testing.T, timeout time.Duration) {
	t.Helper()
	rp.program.Send(tea.Quit())
	select {
	case err := <-rp.done:
		if err != nil {
			t.Fatalf("runtimeProgram: Run() returned error: %v", err)
		}
	case <-time.After(timeout):
		t.Fatalf("runtimeProgram: timed out after %s waiting for program to finish", timeout)
	}
}

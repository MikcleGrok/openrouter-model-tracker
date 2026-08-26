package clipboard

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/charmbracelet/x/term"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("terminal unavailable") }

// terminalFile is the shape github.com/charmbracelet/x/term.File describes —
// io.ReadWriteCloser plus Fd() — recording every call, so a test can prove the
// wrapper forwards to the terminal it was given instead of answering on its
// own behalf.
type terminalFile struct {
	writes []string
	input  string
	fd     uintptr
	closed bool
}

func (f *terminalFile) Write(p []byte) (int, error) {
	f.writes = append(f.writes, string(p))
	return len(p), nil
}

func (f *terminalFile) Read(p []byte) (int, error) {
	if f.input == "" {
		return 0, io.EOF
	}
	n := copy(p, f.input)
	f.input = f.input[n:]
	return n, nil
}

func (f *terminalFile) Close() error { f.closed = true; return nil }

func (f *terminalFile) Fd() uintptr { return f.fd }

func TestSynchronizedWritesWholePayloadAndPropagatesFailure(t *testing.T) {
	var output bytes.Buffer
	w := NewSynchronized(&output)
	if _, err := w.Write([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	if output.String() != "frame" {
		t.Fatalf("output = %q", output.String())
	}
	if _, err := (&Synchronized{out: failWriter{}}).Write([]byte("x")); err == nil {
		t.Fatal("expected writer failure")
	}
}

// TestSynchronizedKeepsTheWrappedTerminalRecognizable pins the property the
// TUI's window size depends on, end to end.
//
// cmd/openrouter/tui.go hands NewSynchronized's result to tea.WithOutput.
// bubbletea decides whether it is talking to a real terminal by type-asserting
// that value to term.File and calling term.IsTerminal on its Fd()
// (internal/thirdparty/bubbletea-patched/tty_unix.go, initInput). A wrapper
// that only implements io.Writer fails that assertion, p.ttyOutput stays nil,
// and handleResize() (tea.go) then skips both checkResize() and
// listenForResize() — so the program receives no WindowSizeMsg at all: not the
// startup one, and none for any later SIGWINCH.
//
// The visible consequence is not just a frozen layout. WindowSizeMsg is also
// the only thing that ever sets the renderer's own r.width/r.height
// (standard_renderer.go), and flush() appends ansi.EraseLineRight to a
// repainted line only when ansi.StringWidth(line) < r.width — never true at
// r.width == 0. Every line that gets shorter between two frames therefore
// keeps the tail of the previous, longer frame on screen, which is exactly the
// "blurred / duplicated text" symptom. See
// cmd/openrouter/window_size_test.go for that half proved through a real
// program.
func TestSynchronizedKeepsTheWrappedTerminalRecognizable(t *testing.T) {
	file := &terminalFile{fd: 42, input: "input"}
	w := NewSynchronized(file)

	wrapped, ok := w.(term.File)
	if !ok {
		t.Fatalf("NewSynchronized(term.File) returned %T, which does not implement term.File; "+
			"bubbletea's initInput() leaves p.ttyOutput nil for such an output, so the program "+
			"never receives a WindowSizeMsg and the renderer stays at width 0", w)
	}
	if _, err := wrapped.Write([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	if len(file.writes) != 1 || file.writes[0] != "frame" {
		t.Fatalf("writes = %q, want exactly one whole %q", file.writes, "frame")
	}
	if got := wrapped.Fd(); got != file.Fd() {
		t.Fatalf("Fd() = %d, want the wrapped terminal's %d — term.GetSize would query the wrong descriptor", got, file.Fd())
	}
	buf := make([]byte, len("input"))
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}
	if string(buf[:n]) != "input" {
		t.Fatalf("Read() = %q, want the wrapped terminal's %q", buf[:n], "input")
	}
	if err := wrapped.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	if !file.closed {
		t.Fatal("Close() did not reach the wrapped terminal")
	}
}

// TestSynchronizedOverStdoutIsATerminalFile checks the concrete argument
// production passes: runTUIWithRankingConfigCompiled only reaches the wrapping
// after tuiIsTTY(out) has already established that out is an *os.File on a
// terminal, so *os.File — not some interface — is what has to survive the
// wrapper.
func TestSynchronizedOverStdoutIsATerminalFile(t *testing.T) {
	if _, ok := NewSynchronized(os.Stdout).(term.File); !ok {
		t.Fatal("NewSynchronized(os.Stdout) must still be a term.File; bubbletea detects the terminal through that interface")
	}
}

// TestSynchronizedOverAPlainWriterIsNotMistakenForATerminal is the other half:
// the wrapper must report the terminal identity of what it wraps, never
// manufacture one. Claiming term.File over a non-file writer would hand
// bubbletea a made-up descriptor to run term.IsTerminal/term.GetSize against.
func TestSynchronizedOverAPlainWriterIsNotMistakenForATerminal(t *testing.T) {
	if _, ok := NewSynchronized(&bytes.Buffer{}).(term.File); ok {
		t.Fatal("NewSynchronized(io.Writer) must not claim to be a term.File")
	}
}

// overlapRecorder reports whether two Write calls were ever in flight at the
// same time.
type overlapRecorder struct {
	mu       sync.Mutex
	inFlight int
	overlap  bool
	count    int
}

func (r *overlapRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	r.inFlight++
	if r.inFlight > 1 {
		r.overlap = true
	}
	r.mu.Unlock()
	// Widen the window a real terminal write would occupy, so an unserialized
	// wrapper overlaps reliably rather than occasionally.
	runtime.Gosched()
	r.mu.Lock()
	r.inFlight--
	r.count++
	r.mu.Unlock()
	return len(p), nil
}

// overlapTerminal is overlapRecorder presented as a term.File, so the same
// assertion can be run against the terminal-aware wrapping path.
type overlapTerminal struct{ *overlapRecorder }

func (overlapTerminal) Read([]byte) (int, error) { return 0, io.EOF }

func (overlapTerminal) Close() error { return nil }

func (overlapTerminal) Fd() uintptr { return 0 }

// TestSynchronizedSerializesWritesOnEveryWrappingPath guards the type's
// original reason to exist — Bubble Tea's renderer goroutine and the OSC-52
// clipboard write (System, dispatched from a tea.Cmd goroutine) share one
// terminal and must never interleave escape sequences — across both shapes
// NewSynchronized can return.
func TestSynchronizedSerializesWritesOnEveryWrappingPath(t *testing.T) {
	const writers = 32
	cases := []struct {
		name string
		out  func(*overlapRecorder) io.Writer
	}{
		{"plain writer", func(r *overlapRecorder) io.Writer { return r }},
		{"terminal file", func(r *overlapRecorder) io.Writer { return overlapTerminal{r} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &overlapRecorder{}
			w := NewSynchronized(tc.out(recorder))
			var wg sync.WaitGroup
			for i := 0; i < writers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = w.Write([]byte("frame"))
				}()
			}
			wg.Wait()
			if recorder.overlap {
				t.Fatal("two writes reached the terminal at once; escape sequences can interleave on the alternate screen")
			}
			if recorder.count != writers {
				t.Fatalf("writes reaching the terminal = %d, want %d", recorder.count, writers)
			}
		})
	}
}

func TestWriteFallsBackToOSC52AndReportsFallbackFailure(t *testing.T) {
	var output bytes.Buffer
	if err := Write(func(string) error { return errors.New("native unavailable") }, "copy", &output); err != nil {
		t.Fatalf("fallback write failed: %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("fallback produced no terminal protocol")
	}
	if err := Write(func(string) error { return errors.New("native unavailable") }, "copy", failWriter{}); err == nil {
		t.Fatal("expected fallback failure")
	}
}

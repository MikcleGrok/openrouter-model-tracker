// Package clipboard defines the clipboard port and terminal adapters.
package clipboard

import (
	"io"
	"os"
	"sync"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/x/term"
)

// Writer is the only dependency selection logic needs for copying text.
type Writer func(string) error

// Synchronized serializes complete terminal writes. Bubble Tea and OSC-52
// must never interleave escape sequences on the alternate screen.
type Synchronized struct {
	mu  sync.Mutex
	out io.Writer
}

// synchronizedFile is Synchronized over a terminal, re-exposing the terminal's
// own identity through it.
//
// Serializing writes must not cost the consumer the ability to see what it is
// writing to. Bubble Tea decides whether its output is a real terminal by
// type-asserting it to term.File and calling term.IsTerminal on the Fd() that
// interface provides (initInput, tty_unix.go). An output that is only an
// io.Writer leaves p.ttyOutput nil, and handleResize() then starts neither
// checkResize() nor listenForResize() — the program gets no WindowSizeMsg,
// ever. Both the layout and the renderer's own r.width/r.height stay frozen,
// and at r.width == 0 flush() never appends ansi.EraseLineRight, so every line
// that shrinks between two frames keeps the previous frame's tail on screen.
//
// So the wrapper reports the terminal identity of whatever it wraps — no more
// (a plain io.Writer stays a plain io.Writer; a manufactured descriptor would
// send term.IsTerminal/term.GetSize after some unrelated fd) and no less.
// Fd/Read/Close forward untouched: the mutex exists to keep one escape
// sequence from being cut in half by another, which concerns writes only, and
// the wrapper does not own the terminal it was handed — it behaves exactly
// like that terminal, minus the interleaving.
type synchronizedFile struct {
	*Synchronized
	file term.File
}

var _ term.File = (*synchronizedFile)(nil)

// NewSynchronized wraps out so complete writes to it are serialized, keeping
// out recognizable as a terminal when it is one.
func NewSynchronized(out io.Writer) io.Writer {
	w := &Synchronized{out: out}
	file, ok := out.(term.File)
	if !ok {
		return w
	}
	return &synchronizedFile{Synchronized: w, file: file}
}

func (w *Synchronized) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}

func (w *synchronizedFile) Fd() uintptr { return w.file.Fd() }

func (w *synchronizedFile) Read(p []byte) (int, error) { return w.file.Read(p) }

func (w *synchronizedFile) Close() error { return w.file.Close() }

// System writes to the native clipboard and falls back to OSC-52.
func System(text string, output io.Writer) error {
	return Write(clipboard.WriteAll, text, output)
}

func Write(native Writer, text string, output io.Writer) error {
	if err := native(text); err == nil {
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

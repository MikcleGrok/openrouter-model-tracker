// Package clipboard defines the clipboard port and terminal adapters.
package clipboard

import (
	"io"
	"os"
	"sync"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
)

// Writer is the only dependency selection logic needs for copying text.
type Writer func(string) error

// Synchronized serializes complete terminal writes. Bubble Tea and OSC-52
// must never interleave escape sequences on the alternate screen.
type Synchronized struct {
	mu  sync.Mutex
	out io.Writer
}

func NewSynchronized(out io.Writer) io.Writer { return &Synchronized{out: out} }

func (w *Synchronized) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}

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

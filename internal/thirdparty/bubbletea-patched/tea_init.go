package tea

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

// initBackgroundQueryTimeout caps how long the startup background-color
// probe below is allowed to block program startup. Some terminals never
// answer the OSC 11 / CSI 6n query this triggers (or answer it in a way
// termenv's reader does not expect), which otherwise hangs every program
// import indefinitely with no visible output. termenv's own OSCTimeout
// (5s) only bounds a single byte read, not the whole two-query exchange,
// so it does not actually cap this reliably.
const initBackgroundQueryTimeout = 2 * time.Second

func init() {
	// XXX: This is a workaround to make assure that Lip Gloss and Termenv
	// query the terminal before any Bubble Tea Program runs and acquires the
	// terminal. Without this, Programs that use Lip Gloss/Termenv might hang
	// while waiting for a a [termenv.OSCTimeout] while querying the terminal
	// for its background/foreground colors.
	//
	// This happens because Bubble Tea acquires the terminal before termenv
	// reads any responses.
	//
	// Note that this will only affect programs running on the default IO i.e.
	// [os.Stdout] and [os.Stdin].
	//
	// This workaround will be removed in v2.
	//
	// Patched locally: bounded with a hard deadline so a terminal that
	// never answers (or answers unexpectedly) can never block program
	// startup beyond initBackgroundQueryTimeout. See
	// internal/thirdparty/bubbletea-patched/README.md.
	done := make(chan struct{})
	go func() {
		_ = lipgloss.HasDarkBackground()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(initBackgroundQueryTimeout):
	}
}

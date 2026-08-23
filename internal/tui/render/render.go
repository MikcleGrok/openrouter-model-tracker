// Package render owns the terminal frame contract used by the TUI.
package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Frame converts content into a complete terminal frame.
//
// The result has exactly height newline-separated rows. Rows are truncated
// before they reach Bubble Tea, while the renderer remains responsible for
// clearing the right edge of changed terminal lines. This keeps layout data
// independent from Bubble Tea's incremental line cache.
func Frame(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

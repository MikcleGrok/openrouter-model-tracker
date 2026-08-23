package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFrameHasStableDimensionsAndClearsShorterRows(t *testing.T) {
	for _, tc := range []struct {
		name          string
		content       string
		width, height int
	}{
		{"empty", "", 8, 4},
		{"unicode", "модель\n🙂", 8, 3},
		{"shorter", "long row\nshort", 12, 2},
		{"truncated", "0123456789", 4, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(Frame(tc.content, tc.width, tc.height), "\n")
			if len(lines) != tc.height {
				t.Fatalf("rows=%d, want %d", len(lines), tc.height)
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got > tc.width {
					t.Fatalf("row %d width=%d, want <= %d: %q", i, got, tc.width, line)
				}
			}
		})
	}
}

func TestFramePreservesValidANSIWhileTruncating(t *testing.T) {
	content := "\x1b[1mtitle\x1b[0m\n\x1b]8;;https://example.test\x07link\x1b]8;;\x07"
	frame := Frame(content, 10, 2)
	if ansi.Strip(frame) != "title\nlink" {
		t.Fatalf("stripped frame=%q", ansi.Strip(frame))
	}
	if !strings.Contains(frame, "\x1b[1m") || !strings.Contains(frame, "https://example.test") {
		t.Fatalf("ANSI sequences were lost: %q", frame)
	}
}

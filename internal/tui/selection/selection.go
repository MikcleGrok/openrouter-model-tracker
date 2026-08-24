// Package selection contains terminal-independent text selection operations.
package selection

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type Point struct{ Line, Column int }

func Ordered(a, b Point) (Point, Point) {
	if a.Line > b.Line || a.Line == b.Line && a.Column > b.Column {
		return b, a
	}
	return a, b
}

// Text extracts cell ranges from a plain-or-ANSI frame without leaking ANSI.
func Text(lines []string, a, b Point) string {
	a, b = Ordered(a, b)
	if a.Line < 0 || b.Line >= len(lines) || a.Line > b.Line {
		return ""
	}
	parts := make([]string, 0, b.Line-a.Line+1)
	for line := a.Line; line <= b.Line; line++ {
		start, finish := 0, ansi.StringWidth(strings.TrimRight(lines[line], " "))
		if line == a.Line {
			start = a.Column
		}
		if line == b.Line {
			finish = b.Column
		}
		if finish < start {
			finish = start
		}
		parts = append(parts, ansi.Cut(lines[line], start, finish))
	}
	return strings.TrimRight(ansi.Strip(strings.Join(parts, "\n")), " \n")
}

func Paint(rendered string, active bool, a, b Point) string {
	if !active {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	a, b = Ordered(a, b)
	for line := a.Line; line <= b.Line && line < len(lines); line++ {
		plain := ansi.Strip(lines[line])
		start, finish := 0, ansi.StringWidth(strings.TrimRight(plain, " "))
		if line == a.Line {
			start = a.Column
		}
		if line == b.Line {
			finish = b.Column
		}
		if finish > start {
			lines[line] = ansi.Cut(lines[line], 0, start) + ansi.SelectGraphicRendition(ansi.ReverseAttr) + ansi.Cut(lines[line], start, finish) + ansi.SelectGraphicRendition(ansi.NoReverseAttr) + ansi.Cut(lines[line], finish, ansi.StringWidth(plain))
		}
	}
	return strings.Join(lines, "\n")
}

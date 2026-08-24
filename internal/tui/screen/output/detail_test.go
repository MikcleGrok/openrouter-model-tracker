package output

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDetailUsesOneLocalizedLineSourceForViewportAndOffset(t *testing.T) {
	frame := Detail(DetailData{Width: 20, Height: 5, Offset: 99, Lines: []string{"-- Идентичность --", "Производитель: Длинное", "Описание", "Еще одна строка"}, Footer: "Детали"})
	if frame.MaxOffset != 1 || frame.Offset != 1 || len(frame.Lines) != 5 || frame.FooterLine != 4 {
		t.Fatalf("detail frame = %#v", frame)
	}
}

func TestWrapKeepsUnicodeWithinDisplayWidth(t *testing.T) {
	for _, line := range Wrap("модель 🙂 очень длинное значение", 8) {
		if display := ansi.StringWidth(line); display > 8 {
			t.Fatalf("wrapped line %q has display width %d", line, display)
		}
	}
}

func TestFrameClearsStaleRowsAndClipsAnsiByTerminalWidth(t *testing.T) {
	old := Frame("first\nsecond\nthird", 8, 3)
	if len(strings.Split(old, "\n")) != 3 {
		t.Fatalf("old frame rows = %d", len(strings.Split(old, "\n")))
	}
	current := Frame("updated", 8, 3)
	rows := strings.Split(current, "\n")
	if len(rows) != 3 || rows[1] != "" || rows[2] != "" {
		t.Fatalf("stale rows survived: %#v", rows)
	}
	styled := Frame("\x1b[38;5;74m链接\x1b[0m", 4, 2)
	if ansi.StringWidth(ansi.Strip(strings.Split(styled, "\n")[0])) > 4 {
		t.Fatalf("styled row exceeds width: %q", styled)
	}
}

func TestWrapPreservesCJKAndExplicitEmptyParagraphs(t *testing.T) {
	lines := Wrap("中文测试\n\n链接", 4)
	if len(lines) != 4 || lines[2] != "" {
		t.Fatalf("wrapped paragraphs = %#v", lines)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 4 {
			t.Fatalf("line %q exceeds width", line)
		}
	}
}

func TestDetailFrameHasStableRowsAfterLongToShortRender(t *testing.T) {
	long := Detail(DetailData{Width: 12, Height: 6, Lines: []string{"one", "two", "three", "four", "five"}, Footer: "footer"})
	short := Detail(DetailData{Width: 12, Height: 6, Lines: []string{"one"}, Footer: "footer"})
	if len(long.Lines) != 6 || len(short.Lines) != 6 {
		t.Fatalf("frame row counts = %d/%d, want 6/6", len(long.Lines), len(short.Lines))
	}
	if short.Lines[1] != "" || short.Lines[3] != "" || short.Lines[4] != "" || short.Lines[5] != "" {
		t.Fatalf("short frame retained stale rows: %#v", short.Lines)
	}
	if short.FooterLine != 2 || short.Lines[2] != "footer" {
		t.Fatalf("footer placement = %#v at %d", short.Lines, short.FooterLine)
	}
}

func TestFrameKeepsOSC8AndUnicodeWidthBoundaries(t *testing.T) {
	link := "\x1b]8;;https://example.test\x07界🙂\x1b]8;;\x07"
	frame := Frame(link+"\nold\nrows", 4, 3)
	rows := strings.Split(frame, "\n")
	if len(rows) != 3 || ansi.StringWidth(ansi.Strip(rows[0])) > 4 {
		t.Fatalf("OSC8 frame = %#v", rows)
	}
	if rows[1] != "old" || rows[2] != "rows" {
		t.Fatalf("frame changed non-overflow rows: %#v", rows)
	}
}

func TestBoxUsesDTOLinesAndClearsUnusedRows(t *testing.T) {
	plain := Box([]string{"title", "body"}, 30, 8)
	if ansi.StringWidth(ansi.Strip(plain)) == 0 || !strings.Contains(ansi.Strip(plain), "title") {
		t.Fatalf("box omitted DTO content: %q", plain)
	}
	narrow := Box([]string{"first", "second", "third"}, 8, 4)
	if got := strings.Split(narrow, "\n"); len(got) != 4 || got[3] != "" {
		t.Fatalf("narrow box rows = %#v", got)
	}
}

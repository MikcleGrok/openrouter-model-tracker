package output

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDetailUsesOneLocalizedLineSourceForViewportAndOffset(t *testing.T) {
	frame := Detail(DetailData{Width: 20, Height: 5, Offset: 99, Lines: []string{"-- Идентичность --", "Производитель: Длинное", "Описание", "Еще одна строка"}, Footer: "Детали"})
	if frame.MaxOffset != 2 || frame.Offset != 2 || len(frame.Lines) != 5 || frame.FooterLine != 4 {
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

func TestDetailFrameReturnsAnOwnerForEveryPhysicalRow(t *testing.T) {
	frame := Detail(DetailData{Width: 12, Height: 6, Offset: 0, Lines: []string{"-- Pricing --", "A very long value that wraps"}, Regions: []Region{{Name: "pricing", Lines: []string{"-- Pricing --", "A very long value that wraps"}}, {Name: "metadata", Lines: []string{"source"}}}, Footer: "footer"})
	if len(frame.Lines) != 6 || len(frame.Owners) != 6 {
		t.Fatalf("frame dimensions = %d/%d", len(frame.Lines), len(frame.Owners))
	}
	for i, owner := range frame.Owners {
		if owner == "" {
			t.Fatalf("physical row %d has no owner", i)
		}
		if ansi.StringWidth(ansi.Strip(frame.Lines[i])) > 12 {
			t.Fatalf("physical row %d exceeds width: %q", i, frame.Lines[i])
		}
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

func TestFrameWrapsLongDetailValueInsteadOfDroppingIt(t *testing.T) {
	url := "https://example.test/metadata/" + strings.Repeat("segment-", 12) + "final"
	detail := Detail(DetailData{Width: 20, Height: 20, Lines: []string{"Metadata source: " + url}})
	rows := detail.Lines
	if !strings.Contains(strings.Join(rows, ""), url) {
		t.Fatalf("long URL was truncated: %#v", rows)
	}
	if len(rows) <= 2 {
		t.Fatalf("long URL did not wrap: %#v", rows)
	}
	for i, row := range rows {
		if ansi.StringWidth(ansi.Strip(row)) > 20 {
			t.Fatalf("row %d exceeds width: %q", i, row)
		}
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

func TestDetailDecodesEscapedNewlinesAndSanitizesTerminalPayloads(t *testing.T) {
	frame := Detail(DetailData{Width: 40, Height: 12, Lines: []string{
		"Provider: Acme\\nLicense: Apache-2.0",
		"Source: https://example.test/a\\nnext",
		"Unsafe: \x1b]8;;https://evil.test\x07visible\x1b]8;;\x07",
	}})
	joined := strings.Join(frame.Lines, "\n")
	if strings.Contains(joined, `\n`) || strings.Contains(joined, "\x1b") {
		t.Fatalf("detail retained escaped/control payload: %q", joined)
	}
	if !strings.Contains(joined, "Provider: Acme\nLicense: Apache-2.0") || !strings.Contains(joined, "Source: https://example.test/a\nnext") {
		t.Fatalf("detail did not preserve logical fields as rows: %q", joined)
	}
}

func TestDetailAdversarialPhysicalBoundaryKeepsOwnersAndGridExact(t *testing.T) {
	lines := []string{
		"Boundary: " + strings.Repeat("x", 10) + `\n界🙂tail`,
		"  " + strings.Repeat("unique-note-", 5) + "\rnext",
		"Source: https://example.test/" + strings.Repeat("segment/", 8),
	}
	regions := []Region{{Name: "identity", Lines: lines[:1]}, {Name: "long-text", Lines: lines[1:]}}
	frame := Detail(DetailData{Width: 16, Height: 20, Lines: lines, Regions: regions, Footer: "footer"})
	if len(frame.Lines) != 20 || len(frame.Owners) != len(frame.Lines) {
		t.Fatalf("frame dimensions = %d/%d", len(frame.Lines), len(frame.Owners))
	}
	for i, line := range frame.Lines {
		if ansi.StringWidth(ansi.Strip(line)) > 16 {
			t.Fatalf("row %d exceeds width: %q", i, line)
		}
		if frame.Owners[i] == "" {
			t.Fatalf("row %d has no owner", i)
		}
	}
	joined := strings.Join(frame.Lines, "\n")
	compact := strings.ReplaceAll(joined, "\n", "")
	for _, want := range []string{"Boundary:", "界🙂tail", "unique-note-", "https://example.test/"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("physical grid lost %q: %q", want, joined)
		}
	}
	short := Detail(DetailData{Width: 16, Height: 12, Lines: []string{"short"}, Footer: "footer"})
	if short.FooterLine != 2 || strings.TrimSpace(short.Lines[3]) != "" {
		t.Fatalf("long-to-short frame retained stale rows: %#v", short.Lines)
	}
}

func TestDetailRemovesMixedControlPayloadsFromVisibleBaseText(t *testing.T) {
	frame := Detail(DetailData{Width: 32, Height: 10, Lines: []string{
		"Provider: before\x1b[31mred\x1b[0m\rnext",
		"Link: \x1b]8;;https://evil.test\aVISIBLE\x1b]8;;\a",
		"CSI: \x1b[2Cvalue\x1b[K",
	}})
	visible := strings.Join(frame.Lines, "\n")
	if strings.ContainsAny(visible, "\x1b\x00\x01\x07\x0b\x0c\x0d") || strings.Contains(visible, `\n`) {
		t.Fatalf("visible base text retained control payload: %q", visible)
	}
}

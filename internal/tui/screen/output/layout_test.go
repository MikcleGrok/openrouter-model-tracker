package output

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func TestComposeBoundsRowsAndPreservesRegionOwnership(t *testing.T) {
	link := "\x1b]8;;https://example.test/a/very/long/path\a界🙂\x1b]8;;\a"
	frame := Compose(18, 8,
		Region{Name: "pricing", Lines: []string{"Price history: " + link, "  2026-08-01: $1 -> $2"}},
		Region{Name: "benchmarks", Lines: []string{"Benchmarks:", "SWE: 中文 96.2%", "LMArena: 1234"}},
		Region{Name: "long-text", Lines: []string{"Description: " + strings.Repeat("emoji 🙂 CJK 中文 ", 5)}},
	)
	if len(frame.Lines) != 8 || len(frame.Owners) != 8 {
		t.Fatalf("frame dimensions = %d/%d", len(frame.Lines), len(frame.Owners))
	}
	for i, line := range frame.Lines {
		if ansi.StringWidth(line) > 18 {
			t.Fatalf("row %d width = %d: %q", i, ansi.StringWidth(line), line)
		}
		if frame.Owners[i] == "" {
			t.Fatalf("row %d has no owner", i)
		}
	}
	if frame.Owners[0] != "pricing" || frame.Owners[4] != "benchmarks" || frame.Owners[7] != "long-text" {
		t.Fatalf("owners = %#v", frame.Owners)
	}
	full := Compose(18, 20, Region{Name: "metadata", Lines: []string{link}})
	if !strings.Contains(ansi.Strip(strings.Join(full.Lines, "\n")), "界🙂") {
		t.Fatalf("wrapped hyperlink lost visible text: %#v", full.Lines)
	}
	joined := strings.Join(full.Lines, "\n")
	if strings.Count(joined, "\x1b]8;;https") != 1 || strings.Count(joined, "\x1b]8;;\x07") != 1 {
		t.Fatalf("hyperlink sequences are unbalanced: %q", joined)
	}
}

func TestComposeDoesNotConcatenateLogicalRowsOrInterpretCursorControls(t *testing.T) {
	frame := Compose(24, 4,
		Region{Name: "identity", Lines: []string{"Manufacturer: OpenAI\rProvider: wrong"}},
		Region{Name: "pricing", Lines: []string{"Price history: $1 -> $2"}},
		Region{Name: "benchmarks", Lines: []string{"SWE: 96.2%"}},
	)
	if strings.Contains(frame.Lines[0], "Provider") {
		t.Fatalf("cursor control leaked unrelated text into row: %q", frame.Lines[0])
	}
	if frame.Owners[0] != "identity" || frame.Owners[1] != "identity" || frame.Owners[2] != "pricing" {
		t.Fatalf("owners = %#v", frame.Owners)
	}
	if strings.Contains(frame.Lines[0], "\x1b[") || strings.Contains(frame.Lines[0], "Provider") {
		t.Fatalf("unsafe control stream leaked: %q", frame.Lines[0])
	}
	emu, err := emulateTerminal(strings.Join(frame.Lines, "\n"), 24, len(frame.Lines))
	if err != nil {
		t.Fatal(err)
	}
	for y, line := range frame.Lines {
		want := []rune(strings.Repeat(" ", 24))
		x := 0
		for _, r := range ansi.Strip(line) {
			w := ansi.StringWidth(string(r))
			for cell := 0; cell < w && x+cell < len(want); cell++ {
				want[x+cell] = r
			}
			x += w
		}
		if string(emu.cells[y]) != string(want) {
			t.Fatalf("screen row %d = %q, want %q", y, string(emu.cells[y]), string(want))
		}
	}
}

type terminalEmulator struct {
	cells  [][]rune
	writes [][]bool
	x, y   int
}

func emulateTerminal(stream string, width, height int) (terminalEmulator, error) {
	emu := terminalEmulator{cells: make([][]rune, height), writes: make([][]bool, height)}
	for y := range emu.cells {
		emu.cells[y] = []rune(strings.Repeat(" ", width))
		emu.writes[y] = make([]bool, width)
	}
	for i := 0; i < len(stream); {
		if stream[i] == '\x1b' {
			sequence, next := terminalEscape(stream, i)
			if len(sequence) > 0 {
				emu.applyCSI(sequence)
			}
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(stream[i:])
		switch r {
		case '\n':
			emu.y++
			emu.x = 0
		case '\r':
			emu.x = 0
		default:
			w := ansi.StringWidth(string(r))
			if w > 0 {
				if err := emu.write(r, w); err != nil {
					return emu, err
				}
			}
		}
		i += size
	}
	if emu.y >= height {
		return emu, fmt.Errorf("terminal moved below frame: row %d >= %d", emu.y, height)
	}
	return emu, nil
}

func terminalEscape(stream string, start int) (string, int) {
	if start+1 >= len(stream) {
		return "", len(stream)
	}
	if stream[start+1] == ']' {
		i := start + 2
		for i < len(stream) && stream[i] != '\a' && !(stream[i] == '\x1b' && i+1 < len(stream) && stream[i+1] == '\\') {
			i++
		}
		if i < len(stream) && stream[i] == '\a' {
			i++
		} else if i+1 < len(stream) {
			i += 2
		}
		return "", i
	}
	if stream[start+1] != '[' {
		return "", min(len(stream), start+2)
	}
	for i := start + 2; i < len(stream); i++ {
		if stream[i] >= 0x40 && stream[i] <= 0x7e {
			return stream[start : i+1], i + 1
		}
	}
	return "", len(stream)
}

func (e *terminalEmulator) applyCSI(sequence string) {
	final := sequence[len(sequence)-1]
	if final == 'm' {
		return
	}
	params := strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b["), string(final))
	values := []int{1}
	if params != "" {
		values = make([]int, 0, 2)
		for _, raw := range strings.Split(params, ";") {
			value := 1
			if raw != "" {
				value, _ = strconv.Atoi(raw)
			}
			values = append(values, value)
		}
	}
	n := values[0]
	switch final {
	case 'A':
		e.y -= n
	case 'B':
		e.y += n
	case 'C':
		e.x += n
	case 'D':
		e.x -= n
	case 'G':
		e.x = n - 1
	case 'd':
		e.y = n - 1
	case 'H', 'f':
		e.y, e.x = n-1, 0
		if len(values) > 1 {
			e.x = values[1] - 1
		}
	case 'J':
		for y := range e.cells {
			for x := range e.cells[y] {
				e.cells[y][x] = ' '
			}
		}
	case 'K':
		if e.y >= 0 && e.y < len(e.cells) {
			for x := e.x; x < len(e.cells[e.y]); x++ {
				e.cells[e.y][x] = ' '
			}
		}
	}
}

func (e *terminalEmulator) write(r rune, width int) error {
	if e.y < 0 || e.y >= len(e.cells) || e.x < 0 || e.x+width > len(e.cells[e.y]) {
		return fmt.Errorf("write outside frame at (%d,%d), width %d", e.x, e.y, width)
	}
	for x := e.x; x < e.x+width; x++ {
		if e.writes[e.y][x] {
			return fmt.Errorf("terminal cell (%d,%d) written twice", x, e.y)
		}
		e.writes[e.y][x] = true
		e.cells[e.y][x] = r
	}
	e.x += width
	return nil
}

func TestTerminalEmulatorRejectsCursorOverwrite(t *testing.T) {
	if _, err := emulateTerminal("abcd\x1b[2DXY", 8, 1); err == nil {
		t.Fatal("cursor overwrite was not detected")
	}
	if _, err := emulateTerminal("ok\x1b[2J", 8, 1); err != nil {
		t.Fatalf("erase sequence was not modelled: %v", err)
	}
}

func TestRegionsFromLinesAssignsWholeRowsToSections(t *testing.T) {
	regions := RegionsFromLines([]string{"Model", "", "-- Pricing --", "Price history", "-- Benchmarks --", "SWE", "-- Fit and notes --", "Description"})
	if len(regions) != 4 {
		t.Fatalf("regions = %#v", regions)
	}
	for _, region := range regions {
		if len(region.Lines) == 0 || region.Name == "" {
			t.Fatalf("invalid region = %#v", region)
		}
	}
}

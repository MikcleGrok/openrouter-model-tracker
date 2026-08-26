package output

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/termwidth"
)

// Region is a vertical, single-owner part of a terminal frame. Lines are
// logical rows; the compositor is the only code allowed to assign y rows.
type Region struct {
	Name  string
	Lines []string
}

// ComposedFrame contains the complete physical frame and its row ownership.
// Owners is intentionally exposed to tests and terminal-independent callers;
// it makes accidental cross-region writes observable.
type ComposedFrame struct {
	Lines  []string
	Owners []string
}

// Compose stacks regions into one bounded viewport. A region owns whole rows,
// never a horizontal stream, so a long value cannot overwrite another block.
func Compose(width, height int, regions ...Region) ComposedFrame {
	if width <= 0 || height <= 0 {
		return ComposedFrame{}
	}
	frame := ComposedFrame{Lines: make([]string, height), Owners: make([]string, height)}
	for i := range frame.Owners {
		frame.Owners[i] = "empty"
	}
	row := 0
	for _, region := range regions {
		for _, raw := range region.Lines {
			for _, line := range wrapSafeLine(raw, width) {
				if row >= height {
					break
				}
				frame.Lines[row] = line
				frame.Owners[row] = region.Name
				row++
			}
			if row >= height {
				break
			}
		}
		if row >= height {
			break
		}
	}
	return frame
}

// RegionsFromLines preserves the existing detail ordering while assigning
// physical ownership at section boundaries. Continuation rows belong to the
// section that introduced them, including history and long prose.
func RegionsFromLines(lines []string) []Region {
	regions := make([]Region, 0, 5)
	current := Region{Name: "header"}
	for _, line := range lines {
		if len(current.Lines) > 0 && strings.HasPrefix(ansi.Strip(line), "-- ") && strings.HasSuffix(ansi.Strip(line), " --") {
			regions = append(regions, current)
			current = Region{Name: sectionOwner(ansi.Strip(line))}
		}
		current.Lines = append(current.Lines, line)
	}
	if len(current.Lines) > 0 {
		regions = append(regions, current)
	}
	return regions
}

func sectionOwner(heading string) string {
	switch {
	case strings.Contains(heading, "Цены"), strings.Contains(heading, "Pricing"):
		return "pricing"
	case strings.Contains(heading, "Бенчмарки"), strings.Contains(heading, "Benchmarks"):
		return "benchmarks"
	case strings.Contains(heading, "Происхождение"), strings.Contains(heading, "Provenance"):
		return "metadata"
	case strings.Contains(heading, "Соответствие"), strings.Contains(heading, "Fit"):
		return "long-text"
	default:
		return "identity"
	}
}

func normalizeLine(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '\x1b' {
			r, size := decodeRune(value[i:])
			if r < 0x20 || r == 0x7f {
				out.WriteByte(' ')
			} else {
				out.WriteString(value[i : i+size])
			}
			i += size
			continue
		}
		sequence, next, keep := escapeSequence(value, i)
		if keep {
			out.WriteString(sequence)
		}
		i = next
	}
	return out.String()
}

func wrapSafeLine(value string, width int) []string {
	value = normalizeLine(value)
	if value == "" {
		return []string{""}
	}
	if termwidth.String(value) <= width {
		return []string{value}
	}
	indent := value[:len(value)-len(strings.TrimLeft(value, " "))]
	content := strings.TrimLeft(value, " ")
	wrapped := Wrap(content, max(1, width-termwidth.String(indent)))
	for i := range wrapped {
		wrapped[i] = indent + wrapped[i]
	}
	return wrapped
}

func splitEscapedLines(value string) []string {
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\r`, "\r")
	return strings.Split(value, "\n")
}

func decodeRune(value string) (rune, int) {
	for _, r := range value {
		return r, len(string(r))
	}
	return ' ', 1
}

func escapeSequence(value string, start int) (string, int, bool) {
	if start+1 >= len(value) {
		return "", len(value), false
	}
	switch value[start+1] {
	case '[':
		for i := start + 2; i < len(value); i++ {
			if value[i] >= 0x40 && value[i] <= 0x7e {
				return value[start : i+1], i + 1, value[i] == 'm'
			}
		}
		return "", len(value), false
	case ']':
		end := start + 2
		for end < len(value) && value[end] != '\a' && !(value[end] == '\x1b' && end+1 < len(value) && value[end+1] == '\\') {
			end++
		}
		endNext := end
		if end < len(value) && value[end] == '\a' {
			endNext++
		} else if end+1 < len(value) {
			endNext += 2
		}
		payload := value[start+2 : end]
		return value[start:endNext], endNext, strings.HasPrefix(payload, "8;")
	default:
		return "", min(len(value), start+2), false
	}
}

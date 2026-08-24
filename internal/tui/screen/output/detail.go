package output

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type DetailData struct {
	Width, Height int
	Offset        int
	Lines         []string
	Regions       []Region
	Footer        string
	FooterFunc    func(offset, end, total int) string
}

type DetailFrame struct {
	Lines      []string
	Owners     []string
	Offset     int
	MaxOffset  int
	FooterLine int
}

// MaxOffset is the single viewport boundary primitive used by every screen.
// Callers must pass the logical lines that are rendered; no display-specific
// line count may be maintained beside them.
func MaxOffset(lineCount, height int) int { return max(0, lineCount-max(1, height)) }

// Frame is the terminal frame contract. It always returns exactly height
// physical rows and truncates each row by display width, including ANSI text.
// Bubble Tea receives the complete frame and can therefore clear stale rows
// without a private writer or an incremental screen cache.
func Frame(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(normalizeLine(lines[i]), width, "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// Viewport returns exactly height ANSI-safe rows from logical lines.
func Viewport(lines []string, offset, width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	offset = max(0, min(offset, MaxOffset(len(lines), height)))
	page := make([]string, height)
	for i := range page {
		if offset+i < len(lines) {
			page[i] = ansi.Truncate(normalizeLine(lines[offset+i]), width, "")
		}
	}
	return page
}

// Box composes an overlay frame. The caller owns only the DTO text; border,
// padding and terminal dimensions stay in this package.
func Box(lines []string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 10 || height < 5 {
		return strings.Join(Viewport(lines, 0, width, height), "\n")
	}
	innerWidth := max(1, min(width-6, 90))
	textWidth := max(1, innerWidth-4)
	contentHeight := max(1, height-4)
	content := append([]string(nil), lines...)
	if len(content) > contentHeight {
		content = content[:contentHeight]
	}
	for i := range content {
		content[i] = ansi.Truncate(content[i], textWidth, "")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(innerWidth).Render(strings.Join(content, "\n"))
}

// Detail composes the only physical detail viewport. The same line slice is
// used for max offset, slicing and footer coordinates, preventing localized
// line-count drift from producing stale or blank rows.
func Detail(data DetailData) DetailFrame {
	if data.Width <= 0 || data.Height <= 0 {
		return DetailFrame{}
	}
	physicalLines := physicalizeLines(data.Lines, data.Width)
	physicalRegions := physicalizeRegions(data.Regions, data.Width)
	bodyHeight := max(1, data.Height-2)
	maxOffset := MaxOffset(len(physicalLines), bodyHeight)
	offset := min(max(0, data.Offset), maxOffset)
	viewport := Viewport(physicalLines, offset, data.Width, bodyHeight)
	if len(physicalRegions) > 0 {
		visibleRegions := sliceRegions(physicalRegions, offset, bodyHeight)
		viewport = Compose(data.Width, bodyHeight, visibleRegions...).Lines
	} else {
		viewport = Compose(data.Width, bodyHeight, Region{Name: "detail", Lines: viewport}).Lines
	}
	contentCount := min(bodyHeight, max(0, len(physicalLines)-offset))
	visible := append([]string(nil), viewport[:contentCount]...)
	owners := make([]string, len(visible))
	if len(physicalRegions) > 0 {
		owners = Compose(data.Width, bodyHeight, sliceRegions(physicalRegions, offset, bodyHeight)...).Owners[:contentCount]
	} else {
		owners = Compose(data.Width, bodyHeight, Region{Name: "detail", Lines: viewport}).Owners[:contentCount]
	}
	if len(visible) == 0 {
		visible = []string{""}
		owners = []string{"detail"}
	}
	if len(visible)+2 <= data.Height {
		visible = append(visible, "")
		owners = append(owners, "separator")
	}
	footer := data.Footer
	if data.FooterFunc != nil {
		footer = data.FooterFunc(offset, offset+contentCount, len(physicalLines))
	}
	visible = append(visible, footer)
	owners = append(owners, "footer")
	footerLine := len(visible) - 1
	for i := range visible {
		visible[i] = ansi.Truncate(visible[i], data.Width, "")
	}
	for len(visible) < data.Height {
		visible = append(visible, "")
		owners = append(owners, "empty")
	}
	if len(visible) > data.Height {
		visible = visible[:data.Height]
		owners = owners[:data.Height]
	}
	return DetailFrame{Lines: visible, Owners: owners, Offset: offset, MaxOffset: maxOffset, FooterLine: footerLine}
}

func physicalizeLines(lines []string, width int) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, wrapSafeLine(line, width)...)
	}
	return result
}

func physicalizeRegions(regions []Region, width int) []Region {
	result := make([]Region, 0, len(regions))
	for _, region := range regions {
		result = append(result, Region{Name: region.Name, Lines: physicalizeLines(region.Lines, width)})
	}
	return result
}

func sliceRegions(regions []Region, offset, height int) []Region {
	result := make([]Region, 0, len(regions))
	row := 0
	remaining := height
	for _, region := range regions {
		if remaining <= 0 {
			break
		}
		start := max(0, offset-row)
		if start < len(region.Lines) {
			lines := region.Lines[start:min(len(region.Lines), start+remaining)]
			result = append(result, Region{Name: region.Name, Lines: lines})
			remaining -= len(lines)
		}
		row += len(region.Lines)
	}
	return result
}

// AlignRows justifies labelled DTO lines without knowing anything about the
// model domain. Indented lines are prose/details and intentionally stay
// untouched, matching the legacy detail layout.
func AlignRows(lines []string, width int) []string {
	if width < 140 {
		return append([]string(nil), lines...)
	}
	labelWidth := 0
	for _, line := range lines {
		index := strings.Index(line, ": ")
		if index > 0 && !strings.HasPrefix(line, "  ") {
			labelWidth = max(labelWidth, ansi.StringWidth(line[:index]))
		}
	}
	if labelWidth == 0 {
		return append([]string(nil), lines...)
	}
	result := append([]string(nil), lines...)
	for i, line := range result {
		index := strings.Index(line, ": ")
		if index <= 0 || strings.HasPrefix(line, "  ") {
			continue
		}
		label := line[:index]
		result[i] = label + ": " + strings.Repeat(" ", labelWidth-ansi.StringWidth(label)) + line[index+2:]
	}
	return result
}

func Wrap(value string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, word := range words {
			if ansi.StringWidth(word) > width {
				if current != "" {
					lines = append(lines, current)
				}
				chunks := wrapWord(word, width)
				lines = append(lines, chunks[:len(chunks)-1]...)
				current = chunks[len(chunks)-1]
				continue
			}
			if current == "" {
				current = word
			} else if ansi.StringWidth(current)+1+ansi.StringWidth(word) > width {
				lines = append(lines, current)
				current = word
			} else {
				current += " " + word
			}
		}
		lines = append(lines, current)
	}
	return lines
}

// Justify expands wrapped paragraph lines to the terminal width. It is kept
// separate from Wrap so callers can preserve paragraph and bullet boundaries.
func Justify(value string, width int) []string {
	lines := Wrap(value, width)
	for i := 0; i < len(lines)-1; i++ {
		lines[i] = justifyLine(lines[i], width)
	}
	return lines
}

// JustifyLine expands one already wrapped line to width.
func JustifyLine(line string, width int) string { return justifyLine(line, width) }

// JustifyLines applies paragraph justification while retaining blank, tabular
// and literal-tab lines. Bullet continuations receive a two-column indent.
func JustifyLines(lines []string, width int) []string {
	if width <= 0 {
		return append([]string(nil), lines...)
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case line == "", strings.Contains(line, `\t`), strings.HasPrefix(line, "\t"):
			result = append(result, line)
		case strings.HasPrefix(line, "- "):
			wrapped := Justify(line[2:], max(1, width-2))
			for i, value := range wrapped {
				if i == 0 {
					result = append(result, "- "+value)
				} else {
					result = append(result, "  "+value)
				}
			}
		default:
			result = append(result, Justify(line, width)...)
		}
	}
	return result
}

func justifyLine(line string, width int) string {
	words := strings.Fields(line)
	if len(words) < 2 {
		return line
	}
	extra := width - ansi.StringWidth(line)
	if extra <= 0 {
		return line
	}
	gaps := len(words) - 1
	base, remainder := extra/gaps, extra%gaps
	var out strings.Builder
	for i, word := range words {
		if i > 0 {
			out.WriteString(strings.Repeat(" ", 1+base))
			if i-1 < remainder {
				out.WriteByte(' ')
			}
		}
		out.WriteString(word)
	}
	return out.String()
}

func wrapWord(word string, width int) []string {
	var chunks []string
	for word != "" {
		head := ansi.Cut(word, 0, width)
		if head == "" {
			_, size := utf8.DecodeRuneInString(word)
			head = word[:size]
		}
		chunks = append(chunks, head)
		word = word[len(head):]
	}
	return chunks
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

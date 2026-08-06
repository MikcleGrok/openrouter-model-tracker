package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"golang.org/x/term"
)

type tuiColumn string

const (
	colName    tuiColumn = "name"
	colSlug    tuiColumn = "slug"
	colClaude  tuiColumn = "claude"
	colStatus  tuiColumn = "status"
	colQuality tuiColumn = "q/p"
	colContext tuiColumn = "context"
	colInput   tuiColumn = "input"
	colOutput  tuiColumn = "output"
	colTask    tuiColumn = "task-fit"
	colNote    tuiColumn = "note"
)

var tuiColumns = []tuiColumn{colName, colSlug, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colTask, colNote}
var tuiSortKeys = []string{"name", "slug", "context", "input", "output", "price", "quality", "q/p"}

type tuiRefreshMsg struct {
	generation uint64
	models     []model.Model
	err        error
}
type tuiTickMsg struct{}

type tuiModel struct {
	ctx                context.Context
	dataDir            string
	refreshOpts        refresh.Options
	interval           time.Duration
	models, visible    []model.Model
	columns            []tuiColumn
	sortKey            string
	reverse            bool
	cursor             int
	width, height      int
	filter             string
	taskLong, lastNote bool
	status, err        string
	refreshing         bool
	generation         uint64
	selectedSlug       string
	overlay            string
	helpPage           int
	columnCursor       int
	pendingColumns     []tuiColumn
	input, inputMode   string
	limit              int
}

func newTUIModel(ctx context.Context, dataDir string, opts refresh.Options, interval time.Duration, models []model.Model) tuiModel {
	m := tuiModel{ctx: ctx, dataDir: dataDir, refreshOpts: opts, interval: interval, models: models, columns: []tuiColumn{colName, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colTask}, sortKey: "q/p", width: 100, height: 24, limit: 0}
	m.rebuild()
	if len(m.visible) > 0 {
		m.selectedSlug = m.visible[0].Slug
	}
	return m
}

var tuiIsTTY = func(w io.Writer) bool { f, ok := w.(*os.File); return ok && term.IsTerminal(int(f.Fd())) }

func runTUI(ctx context.Context, out io.Writer, dataDir string, opts refresh.Options, interval time.Duration) error {
	return runTUIWithConfig(ctx, out, dataDir, opts, interval, "q/p", false, "", 0, false)
}

func runTUIWithConfig(ctx context.Context, out io.Writer, dataDir string, opts refresh.Options, interval time.Duration, sortKey string, reverse bool, filter string, limit int, showSlug bool) error {
	if !tuiIsTTY(out) {
		return fmt.Errorf("openrouter tui requires a TTY on stdout")
	}
	models, err := loadLocalModels(dataDir)
	if err != nil {
		return err
	}
	m := newTUIModel(ctx, dataDir, opts, interval, models)
	m.sortKey, m.reverse, m.filter, m.limit = sortKey, reverse, filter, limit
	if showSlug {
		m.replaceColumn(colName, colSlug)
	}
	m.rebuild()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m *tuiModel) rebuild() {
	filtered := append([]model.Model(nil), m.models...)
	if m.filter != "" {
		var err error
		filtered, err = filterTableModels(filtered, strings.Split(m.filter, ","))
		if err != nil {
			m.err = err.Error()
			return
		}
	}
	_ = sortTableModels(filtered, m.sortKey, m.reverse)
	filtered = limitTableModels(filtered, m.limit)
	m.visible = filtered
	m.restoreSelection()
}

func (m tuiModel) Init() tea.Cmd {
	if m.interval <= 0 {
		return nil
	}
	return tuiTick(m.interval)
}
func tuiTick(d time.Duration) tea.Cmd { return func() tea.Msg { time.Sleep(d); return tuiTickMsg{} } }
func (m tuiModel) refreshCmd() tea.Cmd {
	generation, opts, dir := m.generation, m.refreshOpts, m.dataDir
	return func() tea.Msg {
		if opts.OutputPath == "" {
			return tuiRefreshMsg{generation: generation, err: fmt.Errorf("tui: live refresh requires --output or default_output")}
		}
		_, err := refresh.Run(m.ctx, opts)
		if err != nil {
			return tuiRefreshMsg{generation: generation, err: err}
		}
		rows, err := loadLocalModels(dir)
		return tuiRefreshMsg{generation: generation, models: rows, err: err}
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tuiTickMsg:
		if m.interval <= 0 {
			return m, nil
		}
		next := tuiTick(m.interval)
		if m.refreshing {
			return m, next
		}
		m.generation++
		m.refreshing = true
		return m, tea.Batch(m.refreshCmd(), next)
	case tuiRefreshMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.refreshing = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.models, m.err, m.status = msg.models, "", "refreshed"
		m.rebuild()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m tuiModel) key(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	key := msg.String()
	if m.inputMode != "" {
		return m.inputKey(msg)
	}
	if m.overlay == "help" {
		switch key {
		case "esc", "?":
			m.overlay = ""
		case "tab", "right":
			if m.helpPage < tuiHelpPageCount-1 {
				m.helpPage++
			}
		case "left":
			if m.helpPage > 0 {
				m.helpPage--
			}
		}
		return m, nil
	}
	if m.overlay == "columns" {
		return m.columnKey(key)
	}
	switch key {
	case "x", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if len(m.visible) > 0 {
			m.cursor = max(0, m.cursor-1)
		}
	case "down", "j":
		if len(m.visible) > 0 {
			m.cursor = min(len(m.visible)-1, m.cursor+1)
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, len(m.visible)-1)
	case "pgup":
		m.cursor = max(0, m.cursor-max(1, m.height-5))
	case "pgdown":
		m.cursor = min(len(m.visible)-1, m.cursor+max(1, m.height-5))
	case "s":
		m.sortKey = tuiSortKeys[(indexOf(tuiSortKeys, m.sortKey)+1)%len(tuiSortKeys)]
		m.rebuild()
	case "S":
		m.reverse = !m.reverse
		m.rebuild()
	case "c":
		m.overlay, m.pendingColumns, m.columnCursor = "columns", append([]tuiColumn(nil), m.columns...), 0
	case "t":
		m.taskLong = !m.taskLong
	case "n":
		m.lastNote = !m.lastNote
		m.toggleLastColumn()
	case "/":
		m.inputMode, m.input = "search", ""
	case "f":
		m.inputMode, m.input = "filter", m.filter
	case "?":
		m.overlay, m.helpPage = "help", 0
	case "q", "p", "r":
		m.sortKey = map[string]string{"q": "quality", "p": "price", "r": "q/p"}[key]
		m.rebuild()
	case "R":
		if len(m.visible) > 0 {
			m.selectedSlug = m.visible[m.cursor].Slug
		}
		if !m.refreshing {
			m.generation++
			m.refreshing = true
			m.status = "refreshing"
			return m, m.refreshCmd()
		}
	}
	if len(m.visible) > 0 {
		m.selectedSlug = m.visible[m.cursor].Slug
	}
	return m, nil
}

func (m tuiModel) inputKey(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = ""
	case "enter":
		candidate := m.input
		if m.inputMode == "search" {
			needle := strings.ToLower(plainTableText(candidate))
			filtered := make([]model.Model, 0, len(m.models))
			for _, row := range m.models {
				if strings.Contains(strings.ToLower(plainTableText(row.Slug)), needle) || strings.Contains(strings.ToLower(plainTableText(row.DisplayName)), needle) {
					filtered = append(filtered, row)
				}
			}
			m.inputMode, m.err = "", ""
			m.visible = filtered
			m.restoreSelection()
			return m, nil
		}
		m.inputMode = ""
		if strings.TrimSpace(candidate) == "" {
			m.filter, m.err = "", ""
			m.rebuild()
			return m, nil
		}
		filtered, err := filterTableModels(append([]model.Model(nil), m.models...), strings.Split(candidate, ","))
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.filter, m.err = candidate, ""
		m.rebuildWith(filtered)
	case "backspace":
		_, n := utf8.DecodeLastRuneInString(m.input)
		if n > 0 {
			m.input = m.input[:len(m.input)-n]
		}
	default:
		m.input += string(msg.Runes)
	}
	return m, nil
}

func (m *tuiModel) rebuildWith(filtered []model.Model) {
	_ = sortTableModels(filtered, m.sortKey, m.reverse)
	filtered = limitTableModels(filtered, m.limit)
	m.visible = filtered
	m.restoreSelection()
}

func (m *tuiModel) restoreSelection() {
	if m.selectedSlug != "" {
		for i, row := range m.visible {
			if row.Slug == m.selectedSlug {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.visible)-1))
	if len(m.visible) == 0 {
		m.selectedSlug = ""
		return
	}
	m.selectedSlug = m.visible[m.cursor].Slug
}
func (m *tuiModel) toggleLastColumn() {
	if m.lastNote {
		m.replaceColumn(colTask, colNote)
	} else {
		m.replaceColumn(colNote, colTask)
	}
}
func (m *tuiModel) replaceColumn(from, to tuiColumn) {
	for i, col := range m.columns {
		if col == from {
			m.columns[i] = to
		}
	}
}
func (m tuiModel) columnKey(key string) (tuiModel, tea.Cmd) {
	switch key {
	case "esc":
		m.overlay = ""
	case "up", "k":
		m.columnCursor = max(0, m.columnCursor-1)
	case "down", "j":
		m.columnCursor = min(len(tuiColumns)-1, m.columnCursor+1)
	case " ":
		if len(m.pendingColumns) > 1 || !containsColumn(m.pendingColumns, tuiColumns[m.columnCursor]) {
			m.togglePending(tuiColumns[m.columnCursor])
		}
	case "enter":
		m.columns, m.overlay = append([]tuiColumn(nil), m.pendingColumns...), ""
	}
	return m, nil
}
func (m *tuiModel) togglePending(col tuiColumn) {
	for i, existing := range m.pendingColumns {
		if existing == col {
			m.pendingColumns = append(m.pendingColumns[:i], m.pendingColumns[i+1:]...)
			return
		}
	}
	m.pendingColumns = append(m.pendingColumns, col)
}

func (m tuiModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.overlay == "help" {
		return tuiHelpView(m.helpPage, m.width, m.height)
	}
	if m.overlay == "columns" {
		lines := []string{"Columns (Space toggle, Enter apply, Esc cancel)", ""}
		for i, col := range tuiColumns {
			mark := "[ ]"
			if containsColumn(m.pendingColumns, col) {
				mark = "[x]"
			}
			prefix := "  "
			if i == m.columnCursor {
				prefix = "> "
			}
			lines = append(lines, prefix+mark+" "+string(col))
		}
		return tuiBox(strings.Join(lines, "\n"), m.width, m.height)
	}
	title := truncateTable(plainTableText(fmt.Sprintf("OpenRouter models  sort:%s%s  filter:%q", m.sortKey, reverseLabel(m.reverse), m.filter)), m.width)
	lines := []string{lipgloss.NewStyle().Bold(true).Render(title)}
	columns := m.renderColumns()
	lines = append(lines, m.renderTUILine(columns, nil, false))
	start := max(0, min(m.cursor-max(1, m.height-8)/2, max(0, len(m.visible)-1)))
	end := min(len(m.visible), start+max(1, m.height-6))
	for i := start; i < end; i++ {
		values := make([]string, len(columns))
		for j, col := range columns {
			values[j] = tuiCell(m.visible[i], col, m.taskLong, m.lastNote)
		}
		prefix := " "
		if i == m.cursor {
			prefix = ">"
		}
		lines = append(lines, m.renderTUILine(columns, values, prefix == ">"))
	}
	status := "↑↓ navigate · q quality · p price · r q/p · R refresh · x quit · ? help"
	if m.refreshing {
		status = "refreshing... · " + status
	}
	if m.err != "" {
		status = "error: " + m.err
	}
	lines = append(lines, "", truncateTable(plainTableText(status), m.width))
	if m.inputMode != "" {
		lines = append(lines, truncateTable(plainTableText("/ "+m.input+"_"), m.width))
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderColumns() []tuiColumn {
	columns := append([]tuiColumn(nil), m.columns...)
	for len(columns) > 1 && m.tuiColumnsWidth(columns) > m.width {
		removed := false
		for _, secondary := range []tuiColumn{colNote, colTask, colOutput, colInput, colContext, colStatus, colClaude, colSlug} {
			for i, col := range columns {
				if col == secondary {
					columns = append(columns[:i], columns[i+1:]...)
					removed = true
					break
				}
			}
			if removed {
				break
			}
		}
		if !removed {
			columns = columns[:len(columns)-1]
		}
	}
	return columns
}

func (m tuiModel) tuiColumnsWidth(columns []tuiColumn) int {
	return tableDisplayWidth("  ") + len(columns) + 3*(len(columns)-1)
}

func (m tuiModel) renderTUILine(columns []tuiColumn, values []string, selected bool) string {
	if m.width <= 0 || len(columns) == 0 {
		return ""
	}
	prefix := "  "
	if values != nil {
		if selected {
			prefix = "> "
		}
	}
	available := m.width - tableDisplayWidth(prefix) - 3*(len(columns)-1)
	widths := tuiCellWidths(columns, available)
	parts := make([]string, len(columns))
	for i, col := range columns {
		value := string(col)
		if values != nil {
			if i < len(values) {
				value = values[i]
			} else {
				value = ""
			}
		}
		value = plainTableText(ansi.Strip(value))
		parts[i] = tuiPadCell(truncateTable(value, widths[i]), widths[i], tuiNumericColumn(col))
	}
	return truncateTable(prefix+strings.Join(parts, " | "), m.width)
}

func tuiCellWidths(columns []tuiColumn, available int) []int {
	widths := make([]int, len(columns))
	if len(columns) == 0 {
		return widths
	}
	if available < len(columns) {
		for i := range widths {
			widths[i] = 1
		}
		return widths
	}
	weights := 0
	for _, column := range columns {
		if column == colName {
			weights += 2
			continue
		}
		weights++
	}
	remaining := available
	for i, column := range columns {
		weight := 1
		if column == colName {
			weight = 2
		}
		widths[i] = max(1, available*weight/weights)
		remaining -= widths[i]
	}
	for i := 0; remaining > 0; i++ {
		widths[i%len(widths)]++
		remaining--
	}
	return widths
}

func tuiNumericColumn(column tuiColumn) bool {
	switch column {
	case colQuality, colContext, colInput, colOutput:
		return true
	default:
		return false
	}
}

func tuiPadCell(value string, width int, rightAligned bool) string {
	padding := width - tableDisplayWidth(value)
	if padding <= 0 {
		return value
	}
	if rightAligned {
		return strings.Repeat(" ", padding) + value
	}
	return value + strings.Repeat(" ", padding)
}
func tuiBox(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 10 {
		parts := strings.Split(content, "\n")
		for i := range parts {
			parts[i] = truncateTable(plainTableText(parts[i]), width)
		}
		return strings.Join(parts, "\n")
	}
	innerWidth := max(1, min(width-6, 90))
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = truncateTable(plainTableText(lines[i]), innerWidth)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(innerWidth).Render(strings.Join(lines, "\n"))
}

const tuiHelpPageCount = 3

func tuiHelpView(page, width, height int) string {
	page = max(0, min(page, tuiHelpPageCount-1))
	lines := strings.Split(tuiHelpPageContent(page), "\n")
	footer := "Tab next · ←/→ pages · Esc/? close"
	if page == 0 {
		footer = "Tab next · → next · Esc/? close"
	}
	if page == tuiHelpPageCount-1 {
		footer = "← previous · Esc/? close"
	}
	lines = append(lines, "", fmt.Sprintf("Page %d/%d", page+1, tuiHelpPageCount), footer)
	return tuiBoxLimited(strings.Join(lines, "\n"), width, height)
}

func tuiHelpPageContent(page int) string {
	switch page {
	case 0:
		return "openrouter tui keys\n\nNavigation\nUp/Down or j/k move through models.\nHome/End or g/G jump; PgUp/PgDown scroll.\n\nSort and task view\nq sorts by quality; p by price; r by quality/price ratio (q/p).\ns cycles sort key; S reverses order.\nt toggles Task fit short/long. Short is IDFT; long shows English keywords.\nn switches the last column between Task fit and Note."
	case 1:
		return "Columns, search, and filters\n\nc opens column selection. Space toggles a column; Enter applies; Esc cancels. The last column stays selected.\n\n/ searches Name/Slug as plain substring text.\nf edits a structured filter and does not change the search.\nExamples: paid, free, scored, tier:*, quality>=N, context>=N, input<=N, output<=N.\nMultiple filters are comma-separated and use AND."
	default:
		return "Refresh and finish\n\nR refreshes the local data now. Auto-refresh runs at the configured --refresh-interval; interval 0 disables it, but R still works.\n\nx or Ctrl-C exits. Esc closes this help (and other overlays); ? toggles help.\n\nUse Tab or Right to advance, Left to go back. Up/Down do not leave help."
	}
}

func tuiBoxLimited(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if height < 5 {
		lines := strings.Split(content, "\n")
		if len(lines) > height {
			lines = lines[:height]
		}
		for i := range lines {
			lines[i] = truncateTable(plainTableText(lines[i]), width)
		}
		return strings.Join(lines, "\n")
	}
	maxLines := height
	if width >= 10 {
		maxLines = height - 4
	}
	if maxLines < 1 {
		maxLines = 1
	}
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := range lines {
		lines[i] = truncateTable(plainTableText(lines[i]), width)
	}
	return tuiBox(strings.Join(lines, "\n"), width, height)
}

func tuiCell(m model.Model, col tuiColumn, long, note bool) string {
	var value string
	switch col {
	case colName:
		value = m.DisplayName
	case colSlug:
		value = m.Slug
	case colClaude:
		value = tableClaude(m)
	case colStatus:
		value = tableStatus(m)
	case colQuality:
		value = m.QualityPriceLabel
	case colContext:
		value = pricing.FormatContext(m.Context)
	case colInput:
		value = pricing.FormatPrice(m.InPerM)
	case colOutput:
		value = pricing.FormatPrice(m.OutPerM)
	case colTask:
		if note {
			value = tableNote(m)
			break
		}
		if long {
			value = tableTaskFit(m, "long")
			break
		}
		value = tableTaskFit(m, "short")
	case colNote:
		value = tableNote(m)
	}
	return plainTableText(value)
}
func reverseLabel(reverse bool) string {
	if reverse {
		return " (reverse)"
	}
	return ""
}
func containsColumn(columns []tuiColumn, wanted tuiColumn) bool {
	for _, col := range columns {
		if col == wanted {
			return true
		}
	}
	return false
}
func indexOf(values []string, wanted string) int {
	for i, value := range values {
		if value == wanted {
			return i
		}
	}
	return -1
}

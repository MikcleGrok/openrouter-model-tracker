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

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
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

var (
	tuiTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	tuiMetaStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("87"))
	tuiSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	tuiStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	tuiErrorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	tuiHintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// tuiLayoutAliases приводит символ, который физическая клавиша печатает в
// русской раскладке ЙЦУКЕН, к той же команде, что и её латинский символ в US
// QWERTY. Таблица статическая и намеренно захардкожена: активная раскладка
// клавиатуры живёт в оконном менеджере ОС и процессу недоступна, а LANG и
// LC_CTYPE говорят о языке сообщений, а не о включённой раскладке, поэтому
// выводить её из окружения нельзя ни формально, ни по существу. Расширенные
// keyboard protocol (Kitty и подобные) не используются: обычный TTY отдаёт уже
// оттранслированную руну, и char-level таблица — единственный работающий
// механизм.
//
// Часть кириллических рун визуально неотличима от латиницы, и часть этих
// латинских букв сама является хоткеем, поэтому каждая запись несёт кодовую
// точку и латинскую клавишу-источник. Особенно коварна 'р' -> "h": при беглом
// чтении выглядит как отображение в "p".
var tuiLayoutAliases = map[rune]string{
	'ч': "x", // U+0447, физическая позиция латинской x
	'л': "k", // U+043B, физическая позиция латинской k
	'о': "j", // U+043E, физическая позиция латинской j (омоглиф латинской o)
	'п': "g", // U+043F, физическая позиция латинской g
	'П': "G", // U+041F, физическая позиция латинской G
	'р': "h", // U+0440, физическая позиция латинской h (омоглиф латинской p)
	'д': "l", // U+0434, физическая позиция латинской l
	'ы': "s", // U+044B, физическая позиция латинской s
	'Ы': "S", // U+042B, физическая позиция латинской S
	'ь': "m", // U+044C, физическая позиция латинской m
	'с': "c", // U+0441, физическая позиция латинской c (омоглиф латинской c)
	'т': "n", // U+0442, физическая позиция латинской n
	'а': "f", // U+0430, физическая позиция латинской f (омоглиф латинской a)
	'й': "q", // U+0439, физическая позиция латинской q
	'з': "p", // U+0437, физическая позиция латинской p
	'к': "r", // U+043A, физическая позиция латинской r (омоглиф латинской k)
	'К': "R", // U+041A, физическая позиция латинской R (омоглиф латинской K)
	'.': "/", // ЙЦУКЕН печатает точку на клавише US QWERTY со слэшем
	',': "?", // та же клавиша с Shift
}

func tuiCommandKey(msg tea.KeyMsg) string {
	if len(msg.Runes) == 1 {
		if command, ok := tuiLayoutAliases[msg.Runes[0]]; ok {
			return command
		}
	}
	return msg.String()
}

type tuiRefreshMsg struct {
	generation uint64
	models     []model.Model
	err        error
}
type tuiTickMsg struct{}

type tuiModel struct {
	ctx              context.Context
	dataDir          string
	refreshOpts      refresh.Options
	interval         time.Duration
	models, visible  []model.Model
	columns          []tuiColumn
	sortKey          string
	reverse          bool
	cursor           int
	width, height    int
	filter           string
	lastNote         bool
	status, err      string
	updatedAt        string
	refreshing       bool
	generation       uint64
	selectedSlug     string
	overlay          string
	helpOffset       int
	detailOffset     int
	helpSearch       string
	helpMatches      []int
	helpMatch        int
	columnCursor     int
	pendingColumns   []tuiColumn
	input, inputMode string
	limit            int
	ranking          string
	scoreSource      string
	priceWeight      float64
	rankingConfig    ranking.Compiled
	rankingConfigSet bool
}

func newTUIModel(ctx context.Context, dataDir string, opts refresh.Options, interval time.Duration, models []model.Model) tuiModel {
	compiled, _ := ranking.Compile(ranking.DefaultConfig())
	m := tuiModel{ctx: ctx, dataDir: dataDir, refreshOpts: opts, interval: interval, models: models, columns: []tuiColumn{colName, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colTask}, sortKey: "q/p", ranking: rankingDefault, scoreSource: scoreSourceDefault, priceWeight: config.DefaultMixedUtilityPriceWeight, rankingConfig: compiled, width: 100, height: 24, limit: 0}
	m.updatedAt = loadLocalUpdatedAt(dataDir)
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
	return runTUIWithRankingConfig(ctx, out, dataDir, opts, interval, sortKey, reverse, filter, limit, showSlug, rankingDefault)
}

func runTUIWithRankingConfig(ctx context.Context, out io.Writer, dataDir string, opts refresh.Options, interval time.Duration, sortKey string, reverse bool, filter string, limit int, showSlug bool, ranking string) error {
	return runTUIWithRankingAndWeight(ctx, out, dataDir, opts, interval, sortKey, reverse, filter, limit, showSlug, ranking, config.DefaultMixedUtilityPriceWeight)
}

func runTUIWithRankingAndWeight(ctx context.Context, out io.Writer, dataDir string, opts refresh.Options, interval time.Duration, sortKey string, reverse bool, filter string, limit int, showSlug bool, rankingName string, priceWeight float64) error {
	c := ranking.DefaultConfig()
	c.PriceWeight = &priceWeight
	compiled, err := ranking.Compile(c)
	if err != nil {
		return err
	}
	return runTUIWithRankingConfigCompiled(ctx, out, dataDir, opts, interval, sortKey, reverse, filter, limit, showSlug, rankingName, compiled, scoreSourceDefault)
}

func runTUIWithRankingConfigCompiled(ctx context.Context, out io.Writer, dataDir string, opts refresh.Options, interval time.Duration, sortKey string, reverse bool, filter string, limit int, showSlug bool, rankingName string, compiled ranking.Compiled, scoreSource string) error {
	if !tuiIsTTY(out) {
		return fmt.Errorf("openrouter tui requires a TTY on stdout")
	}
	m, err := newConfiguredTUIModel(ctx, dataDir, opts, interval, sortKey, reverse, filter, limit, showSlug, rankingName, compiled, scoreSource)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// newConfiguredTUIModel builds the tuiModel exactly the way
// runTUIWithRankingConfigCompiled does, minus the TTY gate and
// tea.NewProgram/p.Run() call — the two things that make that function
// unreachable from a test. It exists so a test can call the real
// construction path directly and assert both that scoreSource lands on the
// model and that loadLocalModelsForSource was actually asked for it,
// instead of every test poking m.scoreSource by hand after the fact.
func newConfiguredTUIModel(ctx context.Context, dataDir string, opts refresh.Options, interval time.Duration, sortKey string, reverse bool, filter string, limit int, showSlug bool, rankingName string, compiled ranking.Compiled, scoreSource string) (tuiModel, error) {
	models, err := loadLocalModelsForSource(dataDir, scoreSource)
	if err != nil {
		return tuiModel{}, err
	}
	m := newTUIModel(ctx, dataDir, opts, interval, models)
	m.rankingConfig = compiled
	m.rankingConfigSet = true
	m.sortKey, m.reverse, m.filter, m.limit = sortKey, reverse, filter, limit
	m.ranking = rankingName
	m.scoreSource = scoreSource
	if showSlug {
		m.replaceColumn(colName, colSlug)
	}
	m.rebuild()
	return m, nil
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
	compiled := m.rankingConfig
	if !m.rankingConfigSet {
		c := ranking.DefaultConfig()
		c.PriceWeight = &m.priceWeight
		compiled, _ = ranking.Compile(c)
	}
	if err := sortTableModelsWithRankingAndConfig(filtered, m.sortKey, m.reverse, m.ranking, compiled); err != nil {
		m.err = err.Error()
		m.visible = nil
		return
	}
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
	generation, opts, dir, source := m.generation, m.refreshOpts, m.dataDir, m.scoreSource
	return func() tea.Msg {
		if opts.OutputPath == "" {
			return tuiRefreshMsg{generation: generation, err: fmt.Errorf("tui: live refresh requires --output or default_output")}
		}
		_, err := refresh.Run(m.ctx, opts)
		if err != nil {
			return tuiRefreshMsg{generation: generation, err: err}
		}
		// Reload through the same projection the session started with, so a
		// refresh can never swap the table back to the other source.
		rows, err := loadLocalModelsForSource(dir, source)
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
		m.updatedAt = loadLocalUpdatedAt(m.dataDir)
		m.rebuild()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m tuiModel) key(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	if m.inputMode != "" {
		return m.inputKey(msg)
	}
	key := tuiCommandKey(msg)
	if m.overlay == "help" {
		switch key {
		case "esc", "?":
			m.overlay = ""
		case "/":
			m.inputMode, m.input = "help-search", m.helpSearch
		case "up", "k":
			m.helpOffset = max(0, m.helpOffset-1)
		case "down", "j":
			m.helpOffset = min(tuiHelpMaxOffset(m.height), m.helpOffset+1)
		case "pgup":
			m.helpOffset = max(0, m.helpOffset-max(1, m.height-1))
		case "pgdown":
			m.helpOffset = min(tuiHelpMaxOffset(m.height), m.helpOffset+max(1, m.height-1))
		case "home", "g":
			m.helpOffset = 0
		case "end", "G":
			m.helpOffset = tuiHelpMaxOffset(m.height)
		case "enter":
			m.helpNextMatch(1)
		}
		return m, nil
	}
	if m.overlay == "detail" {
		row, ok := m.detailRow()
		if !ok {
			m.overlay, m.detailOffset = "", 0
			return m, nil
		}
		maxOffset := tuiDetailMaxOffset(row, m.scoreSource, m.width, m.height)
		switch key {
		case "esc", "left", "h":
			m.overlay, m.detailOffset = "", 0
		case "up", "k":
			m.detailOffset = max(0, m.detailOffset-1)
		case "down", "j":
			m.detailOffset = min(maxOffset, m.detailOffset+1)
		case "pgup":
			m.detailOffset = max(0, m.detailOffset-max(1, m.height-1))
		case "pgdown":
			m.detailOffset = min(maxOffset, m.detailOffset+max(1, m.height-1))
		case "home", "g":
			m.detailOffset = 0
		case "end", "G":
			m.detailOffset = maxOffset
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
	case "m":
		if m.ranking == rankingTier {
			m.ranking = rankingMixed
		} else {
			m.ranking = rankingTier
		}
		m.sortKey = "q/p"
		m.rebuild()
	case "S":
		m.reverse = !m.reverse
		m.rebuild()
	case "c":
		m.overlay, m.pendingColumns, m.columnCursor = "columns", append([]tuiColumn(nil), m.columns...), 0
	case "n":
		m.lastNote = !m.lastNote
		m.toggleLastColumn()
	case "/":
		m.inputMode, m.input = "search", ""
	case "f":
		m.inputMode, m.input = "filter", m.filter
	case "?":
		m.overlay, m.helpOffset = "help", 0
	case "enter", "right", "l":
		if len(m.visible) > 0 {
			m.overlay, m.detailOffset = "detail", 0
		}
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
	if m.inputMode == "help-search" {
		switch msg.String() {
		case "up":
			m.helpNextMatch(-1)
			return m, nil
		case "down":
			m.helpNextMatch(1)
			return m, nil
		}
	}
	switch msg.String() {
	case "esc":
		if m.inputMode == "help-search" {
			m.input = ""
		}
		m.inputMode = ""
	case "enter":
		if m.inputMode == "help-search" {
			m.helpSearch, m.inputMode = m.input, ""
			m.helpMatches = tuiHelpSearch(m.helpSearch)
			m.helpMatch = -1
			m.helpNextMatch(1)
			return m, nil
		}
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
		if m.inputMode == "help-search" {
			m.input += string(msg.Runes)
			return m, nil
		}
		m.input += string(msg.Runes)
	}
	return m, nil
}

func (m *tuiModel) rebuildWith(filtered []model.Model) {
	if err := sortTableModelsWithRankingAndConfig(filtered, m.sortKey, m.reverse, m.ranking, m.rankingConfig); err != nil {
		m.err = err.Error()
		m.visible = nil
		return
	}
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
		return tuiHelpView(m.helpOffset, m.helpSearch, m.width, m.height)
	}
	if m.overlay == "detail" {
		return tuiDetailView(m)
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
	title := truncateTable("OpenRouter models", m.width)
	meta := truncateTable(plainTableText(fmt.Sprintf("ranking:%s  score:%s  sort:%s%s  filter:%q  models:%d  data:%s", rankingLabel(m.ranking), m.scoreSource, m.sortKey, reverseLabel(m.reverse), m.filter, len(m.visible), m.updatedAt)), m.width)
	lines := []string{tuiTitleStyle.Render(title), tuiMetaStyle.Render(meta)}
	columns := m.renderColumns()
	lines = append(lines, tuiHeaderStyle.Render(m.renderTUILine(columns, nil, false)))
	status := "status: ready"
	if m.refreshing {
		status = "status: refreshing..."
	}
	if m.err != "" {
		status = "error: " + m.err
	}
	statusLine := tuiStatusStyle.Render(truncateTable(plainTableText(status), m.width))
	if m.err != "" {
		statusLine = tuiErrorStyle.Render(truncateTable(plainTableText(status), m.width))
	}
	hints := "↑↓ navigate · m ranking · q quality · p price · r q/p · R refresh · x quit · s sort · S reverse · ? help/,"
	hintsLine := tuiHintStyle.Render(truncateTable(hints, m.width))
	inputLine := truncateTable(plainTableText("/ "+m.input+"_"), m.width)
	if m.inputMode == "" {
		inputLine = ""
	}
	rowsBudget := m.height - 6
	if inputLine != "" {
		rowsBudget--
	}
	if rowsBudget > 0 {
		start := max(0, min(m.cursor-max(1, rowsBudget)/2, max(0, len(m.visible)-1)))
		end := min(len(m.visible), start+rowsBudget)
		for i := start; i < end; i++ {
			values := make([]string, len(columns))
			for j, col := range columns {
				values[j] = tuiCell(m.visible[i], col, m.lastNote, m.scoreSource)
			}
			prefix := " "
			if i == m.cursor {
				prefix = ">"
			}
			row := m.renderTUILine(columns, values, prefix == ">")
			if prefix == ">" {
				row = tuiSelectedStyle.Render(row)
			}
			lines = append(lines, row)
		}
	}
	if m.height >= 6 {
		lines = append(lines, statusLine, hintsLine)
		if inputLine != "" {
			lines = append(lines, inputLine)
		}
		return strings.Join(lines, "\n")
	}
	return m.compactView(lines, statusLine, hintsLine, inputLine)
}

func (m tuiModel) compactView(lines []string, statusLine, hintsLine, inputLine string) string {
	if m.height <= 1 {
		return strings.Join(lines[:1], "\n")
	}
	compact := []string{lines[0]}
	if m.height >= 3 {
		compact = append(compact, lines[2])
	}
	if inputLine != "" && m.height >= 4 {
		compact = append(compact, inputLine)
	}
	if m.height >= 5 {
		compact = append(compact, hintsLine)
	}
	if m.height >= 6 {
		compact = append(compact, statusLine)
	}
	return strings.Join(compact[:min(len(compact), m.height)], "\n")
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
	lines := strings.Split(content, "\n")
	if width < 10 || height < 5 {
		return tuiOverlayPlain(lines, width, height)
	}
	innerWidth := max(1, min(width-6, 90))
	textWidth := max(1, innerWidth-4)
	contentHeight := height - 4
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}
	for i := range lines {
		lines[i] = truncateTable(plainTableText(lines[i]), textWidth)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(innerWidth).Render(strings.Join(lines, "\n"))
}

func tuiOverlayPlain(lines []string, width, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = truncateTable(plainTableText(lines[i]), width)
	}
	return strings.Join(lines, "\n")
}

func tuiHelpView(offset int, search string, width, height int) string {
	lines := tuiHelpLines()
	offset = max(0, min(offset, max(0, len(lines)-height)))
	lines = lines[offset:min(len(lines), offset+height)]
	if len(lines) == 0 {
		lines = []string{""}
	}
	footer := fmt.Sprintf("Help %d-%d/%d · / search · Enter next match · Esc close", offset+1, min(len(tuiHelpLines()), offset+height), len(tuiHelpLines()))
	if search != "" {
		footer += fmt.Sprintf(" · %q", search)
	}
	lines = append(lines, footer)
	view := tuiFullscreenText(strings.Join(lines, "\n"), width, height)
	styledLines := strings.Split(view, "\n")
	for i, line := range styledLines {
		plain := ansi.Strip(line)
		if strings.HasSuffix(plain, "keys") || strings.HasSuffix(plain, "view") || strings.HasSuffix(plain, "filters") || strings.HasSuffix(plain, "finish") || strings.HasSuffix(plain, "search") || plain == "Task-fit" {
			styledLines[i] = tuiHeaderStyle.Render(line)
		}
	}
	return strings.Join(styledLines, "\n")
}

// detailRow returns the row the detail overlay is about — the same one the
// list highlights. The second result is false when the list is empty,
// which a failing filter or a broken ranking formula can produce with the
// cursor still at 0; every caller must go through this rather than index
// m.visible directly.
func (m tuiModel) detailRow() (model.Model, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return model.Model{}, false
	}
	return m.visible[m.cursor], true
}

// tuiDetailView renders the model-detail overlay, mirroring tuiHelpView:
// build the content lines, apply the offset, slice to the viewport, append
// a position footer and hand the result to tuiFullscreenText. It differs
// from the help view in one deliberate way — the footer gets a reserved
// line instead of being appended past a full viewport and clipped — so
// what is scrolled and what tuiDetailMaxOffset allows always agree.
func tuiDetailView(m tuiModel) string {
	row, ok := m.detailRow()
	if !ok {
		return tuiFullscreenText("Модель не выбрана · Esc close", m.width, m.height)
	}
	lines := tuiDetailLines(row, m.scoreSource, m.width, time.Now())
	body := tuiDetailBodyHeight(m.height)
	offset := max(0, min(m.detailOffset, max(0, len(lines)-body)))
	end := min(len(lines), offset+body)
	visible := append([]string(nil), lines[offset:end]...)
	if len(visible) == 0 {
		visible = []string{""}
	}
	visible = append(visible, fmt.Sprintf("Detail %d-%d/%d · ↑↓ scroll · Esc close", offset+1, end, len(lines)))
	return tuiFullscreenText(strings.Join(visible, "\n"), m.width, m.height)
}

const tuiHelpDocument = `openrouter tui keys

Navigation
Up/Down or j/k move through models. In help, they scroll this document.
Home/End or g/G jump; PgUp/PgDown scroll.
Enter, Right or l opens the model detail screen; Esc, Left or h closes it.

Sort and task view
q sorts by quality; p by price; r by quality/price ratio (q/p).
m toggles ranking mode: mixed-utility or tier-priority. The default is mixed-utility; use --ranking=legacy for the legacy q/p order. s cycles sort key; S reverses order.
R refreshes; x or Ctrl-C exits. c columns; n switches the last column between Task fit and Note.
f filter; / or . search; ? or , help.

Task-fit
n switches the last column between Task fit and Note.
Task-fit codes:
I - implement: write or change production code.
P - plan: define scope, steps, and decisions.
R - research: investigate options, evidence, or behavior.
D - debug: find and fix a defect or failure.
A - audit: inspect quality, safety, or compliance.
F - refactor: improve structure without changing behavior.
T - test: add or improve automated verification.
No task-fit classification is shown as n/a.

Columns, search, and filters
c opens column selection. Space toggles a column; Enter applies; Esc cancels. The last column stays selected.
/ or . searches Name/Slug as plain substring text.
f edits a structured filter and does not change the search.
Examples: paid, free, scored, tier:*, quality>=N, context>=N, input<=N, output>=N.
Multiple filters are comma-separated and use AND.

Model detail view
Enter, Right or l opens the detail screen for the highlighted model.
Esc, Left or h closes it and returns to the list with the same cursor.
Up/Down or j/k, PgUp/PgDown and Home/End scroll the detail text.
It shows owner, release date, tier, context, full pricing including the long-context tier, both score sources as separate labelled blocks, task fit, note and the vendor description.
The vendor description is wrapped to the terminal width instead of being cut like a table cell.

Refresh and finish
R refreshes the local data now. Auto-refresh runs at the configured --refresh-interval; interval 0 disables it, but R still works.
x or Ctrl-C exits. Esc closes this help. ? or , toggles help.

Ranking modes
tier-priority: rankable models first, then Opus, Sonnet, Haiku, score, and Q/P.
mixed-utility: rankable first, then paid utility from the configured safe YAML formula. Without formula, compatibility is score + price_weight*tier_factor*ln(1+quality_price), with price mix 3:1, factors Opus=1, Sonnet=1, Haiku=0.5, Free=0, and weight 10. Formula vars, operations, depth and node limits are documented in README. Task-fit is never a multiplier.
The CLI --ranking flag accepts legacy, tier, tier-priority, mixed, or mixed-utility; without it, mixed-utility sorting is used.

Score sources
swebench: Status and ranking use SWE-bench Verified, in percent. This is the default.
arena: Status and ranking use the LMArena Elo rating, shown raw and normalised to 0-100 before it enters the ranking formula.
The two are never mixed: in one view a model with no number on the active source shows n/a even when the other source has one. Choose with --score-source; there is no auto mode, and the generated markdown document is always swebench.

Help search
/ starts a search in this document. Type text and press Enter to select the first match.
Enter goes to the next match; Up/Down go to the previous/next match. Esc cancels search.`

func tuiHelpLines() []string { return strings.Split(tuiHelpDocument, "\n") }

func tuiHelpSearch(needle string) []int {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return nil
	}
	matches := []int{}
	for i, line := range tuiHelpLines() {
		if strings.Contains(strings.ToLower(line), needle) {
			matches = append(matches, i)
		}
	}
	return matches
}

func tuiHelpMaxOffset(height int) int { return max(0, len(tuiHelpLines())-max(1, height)) }

func (m *tuiModel) helpNextMatch(direction int) {
	if len(m.helpMatches) == 0 {
		return
	}
	m.helpMatch = (m.helpMatch + direction + len(m.helpMatches)) % len(m.helpMatches)
	m.helpOffset = max(0, min(tuiHelpMaxOffset(m.height), m.helpMatches[m.helpMatch]-max(0, m.height/2)))
}

func tuiBoxLimited(content string, width, height int) string {
	return tuiBox(content, width, height)
}

func tuiFullscreenText(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = truncateTable(plainTableText(lines[i]), width)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// tuiWrapText breaks free-form prose onto lines no wider than width
// display columns, preserving paragraph breaks. Every other layout
// primitive in this file truncates instead (truncateTable), which is the
// right behaviour for a table cell and the wrong one for the vendor
// description: cutting it at the terminal edge would delete exactly the
// content the detail screen exists to show. Width is measured with
// tableDisplayWidth — the same grapheme-aware measure truncateTable uses —
// so non-ASCII prose cannot overflow. A zero or negative width yields nil
// rather than a panic: View() can be called before the first
// tea.WindowSizeMsg sets the terminal size.
func tuiWrapText(value string, width int) []string {
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
			if tableDisplayWidth(word) > width {
				if current != "" {
					lines = append(lines, current)
				}
				chunks := tuiWrapWord(word, width)
				lines = append(lines, chunks[:len(chunks)-1]...)
				current = chunks[len(chunks)-1]
				continue
			}
			switch {
			case current == "":
				current = word
			case tableDisplayWidth(current)+1+tableDisplayWidth(word) > width:
				lines = append(lines, current)
				current = word
			default:
				current += " " + word
			}
		}
		lines = append(lines, current)
	}
	return lines
}

// tuiWrapWord cuts a word that cannot fit on one line into width-sized
// chunks, so a URL or an unbroken identifier still stays inside the
// viewport. tablePrefix returns an empty prefix when even the first
// grapheme is wider than the whole line (a two-column glyph at width 1);
// the chunk is then that one grapheme, which keeps the loop making
// progress instead of spinning forever.
func tuiWrapWord(word string, width int) []string {
	var chunks []string
	for word != "" {
		head := tablePrefix(word, width)
		if head == "" {
			end, _ := tableCluster(word, 0)
			head = word[:end]
		}
		chunks = append(chunks, head)
		word = word[len(head):]
	}
	return chunks
}

// tuiDetailValue is the detail screen's single rule for an absent value:
// the project's н/д placeholder, never a labelled blank.
func tuiDetailValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "н/д"
	}
	return plainTableText(value)
}

// tuiDetailPrice formats a $/M price. pricing.FormatDollar returns an
// empty string for a positive price below one cent, which would render as
// a labelled blank; the detail screen says what it means instead.
func tuiDetailPrice(v float64) string {
	if label := pricing.FormatDollar(v); label != "" {
		return label
	}
	return "< $0.01"
}

// tuiDetailTaskFit spells the task-fit keywords out in full rather than in
// the table's compact codes: the screen has the room, and showing task fit
// and the note at the same time is one of the reasons it exists — in the
// table they share one column and are toggled with n.
func tuiDetailTaskFit(m model.Model) string {
	if len(m.TaskFit) == 0 {
		return "н/д"
	}
	return strings.Join(m.TaskFit, " + ")
}

// tuiDetailWrapped renders a free-prose block: sanitised, wrapped to the
// screen width, indented by two columns, and replaced by the placeholder
// when empty. It sanitises with plainDetailText rather than plainTableText
// so that a real paragraph break in the source value survives into
// tuiWrapText, which has its own branch to preserve it.
func tuiDetailWrapped(value string, width int) []string {
	value = strings.TrimSpace(plainDetailText(value))
	if value == "" {
		return []string{"  н/д"}
	}
	wrapped := tuiWrapText(value, max(1, width-2))
	for i := range wrapped {
		wrapped[i] = "  " + wrapped[i]
	}
	return wrapped
}

// tuiDetailCreated renders the catalogue's publication timestamp in both
// forms the screen promises: the absolute date, which is stable enough to
// live in the snapshot, and the age, which is derived from now and
// therefore must be computed at render time. This is the one deliberate
// exception to the rule that every display string is precomputed in
// package model: the TUI reads a snapshot that can be a week old, so a
// stored "2 месяца назад" would still say that half a year later.
func tuiDetailCreated(created int64, now time.Time) string {
	if created <= 0 {
		return "н/д"
	}
	published := time.Unix(created, 0).UTC()
	return published.Format("2006-01-02") + " (" + tuiDetailAge(published, now) + ")"
}

// tuiDetailAge spells the distance between two instants in whole days,
// months or years — whichever the reader actually cares about at that
// distance.
func tuiDetailAge(published, now time.Time) string {
	days := int(now.UTC().Sub(published).Hours() / 24)
	switch {
	case days < 0:
		return "дата в будущем"
	case days == 0:
		return "сегодня"
	case days < 31:
		return tuiPlural(days, "день", "дня", "дней") + " назад"
	case days < 365:
		return tuiPlural(days/30, "месяц", "месяца", "месяцев") + " назад"
	default:
		return tuiPlural(days/365, "год", "года", "лет") + " назад"
	}
}

// tuiPlural picks the Russian plural form for n: one, few (2-4), many.
// Getting this wrong is visible on every single row of the screen.
func tuiPlural(n int, one, few, many string) string {
	form := many
	switch {
	case n%100 >= 11 && n%100 <= 14:
		form = many
	case n%10 == 1:
		form = one
	case n%10 >= 2 && n%10 <= 4:
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}

// tuiDetailSWEBenchBlock renders the SWE-bench Verified section. In the
// arena view the row has already been through model.ForScoreSource, which
// overwrites Score/ScoreLabel with the Arena projection — printing those
// under a SWE-bench heading would show an Elo rating as a percentage,
// which is the one blend the two independent views exist to prevent. So
// the block reports honestly that this view carries no SWE-bench number.
func tuiDetailSWEBenchBlock(m model.Model, scoreSource string) []string {
	lines := []string{"Оценка SWE-bench Verified (проценты):"}
	if scoreSource != scoreSourceSWEBench {
		return append(lines, "  н/д (активно представление arena, число SWE-bench в него не проецируется)")
	}
	return append(lines, tuiDetailScoreLines(m.Score, m.ScoreLabel)...)
}

// tuiDetailArenaBlock renders the LMArena section from the raw Elo fields,
// which model.ForScoreSource leaves untouched in both views. It is a
// separate block with its own heading and its own scale spelled out: this
// screen is the only place both numbers are visible at once, and that is
// acceptable only because they are labelled and physically apart.
func tuiDetailArenaBlock(m model.Model) []string {
	return append([]string{"Оценка LMArena (рейтинг Elo):"}, tuiDetailScoreLines(m.ArenaScore, m.ArenaLabel)...)
}

// tuiDetailScoreLines prints one score with the provenance the project's
// own rules require: the number, whether it came from a previous run's
// snapshot, what exactly was measured, where it came from, and when it was
// last checked. The table's Status column has room for the number alone.
func tuiDetailScoreLines(info *model.ScoreInfo, label string) []string {
	lines := []string{"  Значение: " + tuiDetailValue(label)}
	if info == nil {
		return lines
	}
	if info.Stale {
		lines = append(lines, "  Устарело: значение взято из прошлого снапшота")
	}
	if info.VariantMeasured != "" {
		lines = append(lines, "  Измеренный вариант: "+plainTableText(info.VariantMeasured))
	}
	return append(lines, "  Источник: "+tuiDetailValue(info.SourceURL), "  Проверено: "+tuiDetailValue(info.Checked))
}

// tuiDetailLines builds the detail screen's content for one model: eleven
// labelled blocks ordered from identity to ever finer detail, with the
// vendor description last because it is the only block of unpredictable
// length and would otherwise push everything else off a short terminal.
// Wrapping happens here, before any scrolling maths, so detailOffset
// counts the same physical lines the terminal shows. now is a parameter
// rather than a time.Now() call inside so a test can pin the release age.
// scoreSource must be the same source m was projected with by
// model.ForScoreSource; passing a mismatched pair defeats the SWE-bench
// block's gate against printing Arena data under the wrong heading.
func tuiDetailLines(m model.Model, scoreSource string, width int, now time.Time) []string {
	context := "н/д"
	if m.Context > 0 {
		context = pricing.FormatContext(m.Context)
	}
	lines := []string{
		tuiDetailValue(m.DisplayName) + " (" + tuiDetailValue(m.Slug) + ")",
		"",
		"Производитель: " + tuiDetailValue(m.Owner),
		"Тир: " + tuiDetailValue(m.Tier),
		"Claude-референс: " + tuiDetailValue(m.ClaudeRef),
		"Дата релиза: " + tuiDetailCreated(m.Created, now),
		"",
		"Контекст: " + context,
		"Вход: " + tuiDetailPrice(m.InPerM) + " за M токенов",
		"Выход: " + tuiDetailPrice(m.OutPerM) + " за M токенов",
	}
	// The three long-context labels are precomputed in MergeWithArena and
	// are all empty when the catalogue reported no override for this slug,
	// which is why there is no HasOverride flag to consult on model.Model.
	// The table has never had room for them; this is the only place they
	// are shown at all.
	if m.LongContextPriceLabel != "" {
		lines = append(lines,
			"Длинный контекст: "+plainTableText(m.LongContextPriceLabel),
			"  вход: "+plainTableText(m.LongContextInLabel),
			"  выход: "+plainTableText(m.LongContextOutLabel),
		)
	}
	lines = append(lines, "Открытые веса: "+tuiDetailValue(m.OpenWeights), "")
	lines = append(lines, tuiDetailSWEBenchBlock(m, scoreSource)...)
	lines = append(lines, "")
	lines = append(lines, tuiDetailArenaBlock(m)...)
	lines = append(lines, "", "Task fit: "+tuiDetailTaskFit(m), "", "Заметка:")
	lines = append(lines, tuiDetailWrapped(tableNote(m), width)...)
	lines = append(lines, "", "Описание:")
	return append(lines, tuiDetailWrapped(m.Description, width)...)
}

// tuiDetailBodyHeight is how many content lines fit above the footer. The
// help overlay appends its footer after already filling the viewport, so
// tuiFullscreenText clips it; the detail screen reserves the line instead,
// which also keeps tuiDetailMaxOffset and the rendered slice in agreement.
func tuiDetailBodyHeight(height int) int { return max(1, height-1) }

// tuiDetailMaxOffset is the detail screen's answer to tuiHelpMaxOffset.
// Unlike the help document, this content is not a constant: its length
// depends on the model, on the active score source and on the terminal
// width the description wraps at, so the maximum is computed from the
// lines actually built for this model rather than from a fixed document.
func tuiDetailMaxOffset(m model.Model, scoreSource string, width, height int) int {
	return max(0, len(tuiDetailLines(m, scoreSource, width, time.Now()))-tuiDetailBodyHeight(height))
}

func tuiCell(m model.Model, col tuiColumn, note bool, scoreSource string) string {
	var value string
	switch col {
	case colName:
		value = m.DisplayName
	case colSlug:
		value = m.Slug
	case colClaude:
		value = tableClaudeForSource(m, scoreSource)
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

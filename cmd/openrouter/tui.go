package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/keymap"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/tier"
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
var tuiSortKeys = []string{"name", "slug", "context", "input", "output", "price", "quality", "q/p", "utility"}

var (
	tuiTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	tuiMetaStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
	tuiSectionStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	tuiSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	tuiStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	tuiErrorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	tuiHintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	// tuiLinkStyle introduces no new colour: 81 is tuiTitleStyle's, and the
	// difference is carried by the attribute, because underlining is the
	// universal terminal convention for "this is a link" and costs nothing
	// in palette. Bold stays exclusive to the screen title, underline stays
	// exclusive to links, so the two remain distinguishable. Reusing an
	// existing style instead was rejected: 220 means "state/attention" on
	// the main screen, 87 is the label colour whose contrast against values
	// this whole change exists to create, and tuiSelectedStyle has a
	// background that would read as a mouse selection on full-screen text.
	tuiLinkStyle         = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("74"))
	tuiMatchStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	tuiCurrentMatchStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("124"))
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
	'щ': "o", // U+0449, physical position of Latin o
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

// tuiCommandKey нормализует нажатие в командную строку, подменяя кириллицу
// раскладочным алиасом. Alt-модификатор и вставка из буфера (bracketed paste)
// обязаны пройти мимо таблицы алиасов и попасть в msg.String() как есть: сам
// String() добавляет им префикс "alt+" и оборачивает paste в "[...]" именно
// затем, чтобы модифицированное или вставленное нажатие никогда не спутали с
// голой командной клавишей (см. комментарий в key.go bubbletea). Таблица
// алиасов не должна отбирать эту защиту у пользователей русской раскладки.
func tuiCommandKey(msg tea.KeyMsg) string {
	if len(msg.Runes) == 1 && !msg.Alt && !msg.Paste {
		if command, ok := tuiLayoutAliases[msg.Runes[0]]; ok {
			return command
		}
	}
	return msg.String()
}

type tuiRefreshMsg struct {
	generation            uint64
	scoreSourceGeneration uint64
	models                []model.Model
	filter                string
	filterSteps           config.TUISteps
	keymap                config.TUIKeymap
	nameWidth             int
	iconGap               int
	iconGaps              config.IconGaps
	iconGapSet            bool
	icons                 config.IconConfig
	iconsSet              bool
	filterFormExplicit    bool
	filterDefaulted       bool
	layout                string
	topN                  int
	err                   error
}
type tuiScoreSourceMsg struct {
	generation uint64
	source     string
	models     []model.Model
	err        error
}
type tuiTickMsg struct{}

type tuiFilterDraft struct {
	free, paid, scored bool
	hasQP              bool
	availability       string
	tier               string
	quality            string
	context            string
	input              string
	output             string
}

type tuiModel struct {
	ctx                   context.Context
	dataDir               string
	configPath            string
	refreshOpts           refresh.Options
	interval              time.Duration
	models, visible       []model.Model
	columns               []tuiColumn
	sortKey               string
	reverse               bool
	cursor                int
	width, height         int
	filter                string
	filterExplicit        bool
	filterFormExplicit    bool
	filterDefaulted       bool
	lastNote              bool
	status, err           string
	updatedAt             string
	refreshing            bool
	generation            uint64
	scoreSourceGeneration uint64
	selectedSlug          string
	overlay               string
	helpOffset            int
	helpSection           int
	detailOffset          int
	helpSearch            string
	helpMatches           []int
	helpMatch             int
	columnCursor          int
	settingsCursor        int
	filterCursor          int
	filterDraft           tuiFilterDraft
	pendingColumns        []tuiColumn
	input, inputMode      string
	search                string
	limit                 int
	layout                string
	topN                  int
	topSeparator          int
	ranking               string
	scoreSource           string
	priceWeight           float64
	priceHistory          *pricehistory.History
	rankingConfig         ranking.Compiled
	rankingConfigSet      bool
	filterSteps           config.TUISteps
	keymap                config.TUIKeymap
	nameWidth             int
	iconGap               int
	iconGaps              config.IconGaps
	icons                 config.IconConfig
	scoreSourceLoading    bool
	pendingScoreSource    string
	// lang selects the TUI's display language: "" (the zero value) means
	// English, today's only behaviour and the persisted default; "ru" means
	// Russian. Keeping "" as English rather than adding an explicit
	// "en" sentinel is what makes a bare tuiModel{} — and a config.yaml
	// without tui_language — render exactly like before this field existed.
	// Toggled at runtime with l (see the language_toggle keymap action) and
	// persisted via config.SaveTUILanguage.
	lang string
}

func newTUIModel(ctx context.Context, dataDir string, opts refresh.Options, interval time.Duration, models []model.Model) tuiModel {
	compiled, _ := ranking.Compile(ranking.DefaultConfig())
	m := tuiModel{ctx: ctx, dataDir: dataDir, refreshOpts: opts, interval: interval, models: models, columns: []tuiColumn{colName, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colTask}, sortKey: "utility", ranking: rankingDefault, scoreSource: scoreSourceDefault, priceWeight: config.DefaultMixedUtilityPriceWeight, rankingConfig: compiled, filterSteps: config.DefaultTUISteps(), keymap: config.DefaultTUIKeymap(), nameWidth: config.DefaultNameWidth, iconGap: int(config.DefaultIconGap), iconGaps: config.DefaultIconGaps(), icons: config.DefaultIconConfig(), width: 100, height: 24, limit: 0, layout: config.DefaultTUILayout, topN: config.DefaultTUITopN, topSeparator: -1}
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
	return runTUIWithRankingConfigCompiled(ctx, out, dataDir, opts, interval, sortKey, reverse, filter, limit, showSlug, rankingName, compiled, scoreSourceDefault, "", false)
}

func runTUIWithRankingConfigCompiled(ctx context.Context, out io.Writer, dataDir string, opts refresh.Options, interval time.Duration, sortKey string, reverse bool, filter string, limit int, showSlug bool, rankingName string, compiled ranking.Compiled, scoreSource, configPath string, filterExplicit bool) error {
	if !tuiIsTTY(out) {
		return fmt.Errorf("openrouter tui requires a TTY on stdout")
	}
	m, err := newConfiguredTUIModel(ctx, dataDir, opts, interval, sortKey, reverse, filter, limit, showSlug, rankingName, compiled, scoreSource)
	if err != nil {
		return err
	}
	m.configPath = configPath
	m.filterExplicit = filterExplicit
	if m.configPath != "" {
		cfg, loadErr := config.Load(m.configPath)
		if loadErr != nil {
			return loadErr
		}
		m.filterSteps = cfg.TUISteps
		m.keymap = cfg.TUIKeymap
		m.nameWidth = cfg.Table.EffectiveNameWidth()
		m.iconGap = cfg.Table.EffectiveIconGap()
		m.iconGaps = cfg.Table.IconGaps
		m.icons = cfg.Icons
		m.layout, m.topN = cfg.TUI.Layout, cfg.TUI.TopN
		if strings.EqualFold(cfg.TUILanguage, "ru") {
			m.lang = "ru"
		}
		m.filterFormExplicit = true
		m.filterDefaulted = !filterExplicit && (!cfg.TUIFilterSet || isLegacyTUIFilter(cfg.TUIFilter))
		m.rebuild()
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
	m.priceHistory, err = pricehistory.Load(pricehistory.Path(dataDir))
	if err != nil {
		return tuiModel{}, err
	}
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
	visible, separator, err := m.buildVisible()
	if err != nil {
		m.err = err.Error()
		m.visible = nil
		m.topSeparator = -1
		return
	}
	m.err = ""
	m.visible, m.topSeparator = visible, separator
	m.restoreSelection()
}

func (m *tuiModel) buildVisible() ([]model.Model, int, error) {
	filtered := append([]model.Model(nil), m.models...)
	if m.filter != "" {
		var err error
		filtered, err = filterTableModels(filtered, strings.Split(m.filter, ","))
		if err != nil {
			return nil, -1, err
		}
	}
	compiled := m.rankingConfig
	if !m.rankingConfigSet {
		c := ranking.DefaultConfig()
		c.PriceWeight = &m.priceWeight
		compiled, _ = ranking.Compile(c)
	}
	if err := sortTableModelsWithRankingAndConfig(filtered, m.sortKey, m.reverse, m.ranking, compiled); err != nil {
		return nil, -1, err
	}
	separator := -1
	if m.layout == "top-paid-free" {
		paidFilters, freeFilters := make([]string, 0), make([]string, 0)
		for _, raw := range splitFilter(m.filter) {
			trimmed := strings.TrimSpace(raw)
			if m.filterDefaulted && strings.EqualFold(trimmed, "availability:paid") {
				continue
			}
			paidFilters = append(paidFilters, raw)
			freeFilters = append(freeFilters, raw)
		}
		paidBase, err := filterTableModels(append([]model.Model(nil), m.models...), paidFilters)
		freeBase, freeErr := filterTableModels(append([]model.Model(nil), m.models...), freeFilters)
		if err != nil {
			return nil, -1, err
		}
		if freeErr != nil {
			return nil, -1, freeErr
		}
		_ = sortTableModelsWithRankingAndConfig(paidBase, m.sortKey, m.reverse, m.ranking, compiled)
		_ = sortTableModelsWithRankingAndConfig(freeBase, m.sortKey, m.reverse, m.ranking, compiled)
		paid, free := make([]model.Model, 0), make([]model.Model, 0)
		for _, row := range paidBase {
			if !row.Free {
				paid = append(paid, row)
			}
		}
		for _, row := range freeBase {
			if row.Free {
				free = append(free, row)
			}
		}
		paid = paid[:min(len(paid), max(0, m.topN))]
		free = free[:min(len(free), max(0, m.topN))]
		if m.search != "" {
			paid = searchTUIModels(paid, m.search)
			free = searchTUIModels(free, m.search)
		}
		if m.limit > 0 {
			if len(paid) >= m.limit {
				paid, free = paid[:m.limit], nil
			} else {
				free = free[:min(len(free), m.limit-len(paid))]
			}
		}
		filtered = append(paid, free...)
		if len(paid) > 0 && len(free) > 0 {
			separator = len(paid)
		}
	} else {
		if m.search != "" {
			filtered = searchTUIModels(filtered, m.search)
		}
	}
	if m.layout != "top-paid-free" {
		filtered = limitTableModels(filtered, m.limit)
	}
	if separator >= len(filtered) {
		separator = -1
	}
	return filtered, separator, nil
}

func searchTUIModels(models []model.Model, query string) []model.Model {
	needle := strings.ToLower(plainTableText(query))
	filtered := make([]model.Model, 0, len(models))
	for _, row := range models {
		if strings.Contains(strings.ToLower(plainTableText(row.Slug)), needle) || strings.Contains(strings.ToLower(plainTableText(row.DisplayName)), needle) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (m tuiModel) Init() tea.Cmd {
	if m.interval <= 0 {
		return nil
	}
	return tuiTick(m.interval)
}
func tuiTick(d time.Duration) tea.Cmd { return func() tea.Msg { time.Sleep(d); return tuiTickMsg{} } }
func (m tuiModel) refreshCmd() tea.Cmd {
	generation, scoreSourceGeneration, opts, dir, source := m.generation, m.scoreSourceGeneration, m.refreshOpts, m.dataDir, m.scoreSource
	return func() tea.Msg {
		filter, filterFormExplicit, filterDefaulted := m.filter, m.filterFormExplicit, m.filterDefaulted
		filterSteps := m.filterSteps
		keymap := m.keymap
		nameWidth := m.nameWidth
		iconGap := m.iconGap
		iconGapSet := false
		iconGaps := m.iconGaps
		icons := m.icons
		iconsSet := false
		layout, topN := m.layout, m.topN
		if m.configPath != "" {
			cfg, err := config.Load(m.configPath)
			if err != nil {
				return tuiRefreshMsg{generation: generation, scoreSourceGeneration: scoreSourceGeneration, iconGap: iconGap, iconGaps: iconGaps, iconGapSet: iconGapSet, err: err}
			}
			filterSteps = cfg.TUISteps
			keymap = cfg.TUIKeymap
			nameWidth = cfg.Table.EffectiveNameWidth()
			iconGap = cfg.Table.EffectiveIconGap()
			iconGapSet = true
			iconGaps = cfg.Table.IconGaps
			icons = cfg.Icons
			iconsSet = true
			if cfg.TUI.Layout != "" {
				layout = cfg.TUI.Layout
			}
			if cfg.TUI.TopN > 0 {
				topN = cfg.TUI.TopN
			}
			filterFormExplicit = true
			filterDefaulted = !m.filterExplicit && (!cfg.TUIFilterSet || isLegacyTUIFilter(cfg.TUIFilter))
			if !m.filterExplicit {
				filter = resolveTUIFilter("", false, cfg.TUIFilter, cfg.TUIFilterSet, cfg.DefaultFilter)
			}
		}
		if opts.OutputPath == "" {
			return tuiRefreshMsg{generation: generation, scoreSourceGeneration: scoreSourceGeneration, iconGap: iconGap, iconGaps: iconGaps, iconGapSet: iconGapSet, layout: layout, topN: topN, err: fmt.Errorf("tui: live refresh requires --output or default_output")}
		}
		_, err := refresh.Run(m.ctx, opts)
		if err != nil {
			return tuiRefreshMsg{generation: generation, scoreSourceGeneration: scoreSourceGeneration, iconGap: iconGap, iconGaps: iconGaps, iconGapSet: iconGapSet, layout: layout, topN: topN, err: err}
		}
		// Reload through the same projection the session started with, so a
		// refresh can never swap the table back to the other source.
		rows, err := loadLocalModelsForSource(dir, source)
		return tuiRefreshMsg{generation: generation, scoreSourceGeneration: scoreSourceGeneration, models: rows, filter: filter, filterSteps: filterSteps, keymap: keymap, nameWidth: nameWidth, iconGap: iconGap, iconGaps: iconGaps, iconGapSet: iconGapSet, icons: icons, iconsSet: iconsSet, filterFormExplicit: filterFormExplicit, filterDefaulted: filterDefaulted, layout: layout, topN: topN, err: err}
	}
}

func (m tuiModel) scoreSourceCmd(source string) tea.Cmd {
	dir, generation := m.dataDir, m.scoreSourceGeneration
	return func() tea.Msg {
		rows, err := loadLocalModelsForSource(dir, source)
		return tuiScoreSourceMsg{generation: generation, source: source, models: rows, err: err}
	}
}

func (m tuiModel) switchScoreSource() (tuiModel, tea.Cmd) {
	if m.scoreSourceLoading {
		return m, nil
	}
	m.scoreSourceGeneration++
	m.scoreSourceLoading = true
	source := scoreSourceArena
	if m.scoreSource == scoreSourceArena {
		source = scoreSourceSWEBench
	}
	m.status, m.err = "loading "+source+" from local snapshot...", ""
	m.pendingScoreSource = source
	return m, m.scoreSourceCmd(source)
}

func (m tuiModel) keyMatches(context, action, key string) bool {
	bindings := m.keymap
	if bindings == nil {
		bindings = config.DefaultTUIKeymap()
	}
	for _, binding := range bindings[context][action] {
		if keymap.CanonicalBinding(binding) == keymap.CanonicalBinding(key) {
			return true
		}
	}
	return false
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampDetailOffset()
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
		if msg.generation != m.generation || msg.scoreSourceGeneration != m.scoreSourceGeneration {
			return m, nil
		}
		m.refreshing = false
		if msg.layout != "" {
			m.layout = msg.layout
		}
		if msg.topN > 0 {
			m.topN = msg.topN
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.models, m.err, m.status = msg.models, "", "refreshed"
		history, err := pricehistory.Load(pricehistory.Path(m.dataDir))
		if err != nil {
			m.err = fmt.Sprintf("price history reload failed: %v", err)
			return m, nil
		}
		m.priceHistory = history
		m.filterSteps = msg.filterSteps
		if msg.nameWidth > 0 {
			m.nameWidth = msg.nameWidth
		}
		if msg.iconGapSet {
			m.iconGap = msg.iconGap
			m.iconGaps = msg.iconGaps
		}
		if msg.iconsSet {
			m.icons = msg.icons
		}
		if msg.keymap != nil {
			m.keymap = msg.keymap
		}
		m.filterFormExplicit = msg.filterFormExplicit
		m.filterDefaulted = msg.filterDefaulted
		if !m.filterExplicit {
			m.filter = msg.filter
		}
		m.updatedAt = loadLocalUpdatedAt(m.dataDir)
		m.rebuild()
		m.clampDetailOffset()
	case tuiScoreSourceMsg:
		if msg.generation != m.scoreSourceGeneration {
			return m, nil
		}
		if msg.err != nil {
			m.scoreSourceLoading = false
			m.pendingScoreSource = ""
			m.err = fmt.Sprintf("score source %s: %v", msg.source, msg.err)
			m.status = "score source switch failed"
			return m, nil
		}
		m.scoreSourceLoading = false
		m.pendingScoreSource = ""
		m.scoreSource, m.models, m.err, m.status = msg.source, msg.models, "", "score source changed"
		m.updatedAt = loadLocalUpdatedAt(m.dataDir)
		m.rebuild()
		m.clampDetailOffset()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

// closeOverlay is the single close path for every overlay: it clears the
// overlay itself plus every piece of transient overlay-scoped state (the
// detail scroll offset; any in-progress text-input draft), the same way each
// overlay's own dedicated close key already did before x delegated to it.
// Resetting detailOffset/inputMode/input unconditionally is harmless even
// when the closing overlay never touched them — they are zero already.
func (m *tuiModel) closeOverlay() {
	m.overlay, m.detailOffset = "", 0
	m.inputMode, m.input = "", ""
}

func (m tuiModel) key(msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	// The universal exit key must never win a race against active text
	// input: m.inputMode != "" (typing a search or a help search) always
	// routes to inputKey first, so a literal "x" (or its Cyrillic "ч" alias,
	// via tuiCommandKey) is inserted into the draft like any other letter —
	// e.g. searching for the real model-map.tsv slug "x-ai/grok-4.5" no
	// longer quits the app the moment "x" is typed. Esc remains the only way
	// to cancel out of an active text input.
	if m.inputMode != "" {
		return m.inputKey(msg)
	}
	key := tuiCommandKey(msg)
	originalKey := key
	// x is a hardcoded, always-on universal exit — not part of the
	// customizable keymap — so it is checked here before any of the
	// keymap-driven routing below. tuiCommandKey (not raw msg.String())
	// makes it Cyrillic-aware: "ч" sits at the physical position of Latin
	// "x" on a ЙЦУКЕН layout and must close/quit exactly like "x" does,
	// while still being blocked above whenever text input is active, and
	// still excluded for Alt/paste (tuiCommandKey never aliases those).
	if key == "x" {
		if m.overlay != "" {
			m.closeOverlay()
			return m, nil
		}
		return m, tea.Quit
	}
	// language_toggle (l, plus its Cyrillic ЙЦУКЕН-position alias д via
	// tuiCommandKey) is checked unconditionally here too, the same way x
	// is above: it is not gated on m.overlay == "" the way open_settings,
	// open_details, help and full_help below are, because none of them
	// share a meaning with l inside any overlay's own switch (checked: no
	// overlay context binds a letter to "l" today) — a user mid-overlay
	// (help, detail, settings, columns, filter) can flip the whole
	// interface's language without backing out first, matching how x
	// already reaches every overlay. It is still, like every other command
	// key, unreachable while m.inputMode != "" — that branch already
	// returned above — so l/д types literally into an active search or
	// help-search draft, exactly as before.
	if m.keyMatches("main", "language_toggle", key) {
		m.toggleLanguage()
		return m, nil
	}
	if m.overlay == "" && m.keyMatches("main", "open_settings", key) {
		key = "o"
	}
	if m.overlay == "" && m.keyMatches("main", "open_details", key) {
		key = "enter"
	}
	if m.overlay == "" && m.keyMatches("main", "help", key) {
		key = "?"
	}
	if m.overlay == "" && m.keyMatches("main", "full_help", key) {
		key = "f1"
	}
	if m.overlay == "settings" && m.settingsCursor == 1 && m.keyMatches("settings", "switch_source", key) {
		key = " "
	}
	if m.overlay == "" && m.keyMatches("main", "switch_source", key) {
		key = " "
	}
	context := "main"
	if m.overlay != "" {
		context = m.overlay
	}
	if m.keyMatches(context, "close", key) {
		key = "esc"
	}
	if (context == "columns" || context == "filter") && m.keyMatches(context, "toggle", key) {
		key = " "
	}
	if (context == "columns" || context == "filter") && m.keyMatches(context, "apply", key) {
		key = "enter"
	}
	if m.keyMatches(context, "navigate_up", key) {
		key = "up"
	}
	if m.keyMatches(context, "navigate_down", key) {
		key = "down"
	}
	if m.overlay == "help" {
		if m.keyMatches("help", "full_help", originalKey) {
			m.setHelpSection(0)
			return m, nil
		}
		switch key {
		case "esc":
			if m.keyMatches("help", "close", originalKey) {
				m.closeOverlay()
			}
		case "?":
			if m.keyMatches("help", "close", originalKey) {
				m.closeOverlay()
			}
		case "/":
			m.inputMode, m.input = "help-search", m.helpSearch
		case "up", "k":
			m.helpOffset = max(0, m.helpOffset-1)
		case "down", "j":
			m.helpOffset = min(m.helpMaxOffset(), m.helpOffset+1)
		case "pgup":
			m.helpOffset = max(0, m.helpOffset-max(1, m.height-1))
		case "pgdown":
			m.helpOffset = min(m.helpMaxOffset(), m.helpOffset+max(1, m.height-1))
		case "home", "g":
			m.helpOffset = 0
		case "end", "G":
			m.helpOffset = m.helpMaxOffset()
		case "n":
			m.helpNextMatch(1)
		case "N":
			m.helpNextMatch(-1)
		case "1", "2", "3", "4", "5", "6":
			// Digit jump and Left/Right step navigate sections of the one
			// sectioned help document; both F1 and ? open the same overlay
			// now, so there is no separate unsectioned mode to guard against.
			m.setHelpSection(int(key[0] - '1'))
		case "left":
			m.setHelpSection(m.helpSection - 1)
		case "right":
			m.setHelpSection(m.helpSection + 1)
		}
		return m, nil
	}
	if m.overlay == "detail" {
		row, ok := m.detailRow()
		if !ok {
			m.closeOverlay()
			return m, nil
		}
		maxOffset := tuiDetailMaxOffsetWithHistoryAndIconsAndGaps(row, m.scoreSource, m.width, m.height, m.priceHistory, m.icons, m.iconGap, m.iconGaps)
		switch key {
		case "esc", "left", "h":
			if !m.keyMatches("detail", "close", originalKey) {
				break
			}
			m.closeOverlay()
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
		return m.columnKey(key, originalKey)
	}
	if m.overlay == "settings" {
		return m.settingsKey(key, originalKey)
	}
	if m.overlay == "filter" {
		return m.filterKey(key, msg)
	}
	switch key {
	case "ctrl+c":
		// "x" is handled unconditionally above (translated through
		// tuiCommandKey, so this covers its Cyrillic "ч" alias too) — it
		// never reaches this switch, since m.overlay == "" and m.inputMode
		// == "" are both already guaranteed by the time control gets here.
		return m, tea.Quit
	case "esc", "left", "h":
		if !m.keyMatches("main", "close", originalKey) {
			break
		}
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
	case " ":
		if !m.keyMatches("main", "switch_source", originalKey) {
			break
		}
		return m.switchScoreSource()
	case "m":
		if m.ranking == rankingTier {
			m.ranking = rankingMixed
		} else {
			m.ranking = rankingTier
		}
		m.sortKey = "utility"
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
		m.inputMode, m.input = "search", m.search
	case "f":
		m.openFilterEditor()
	case "p":
		if m.keyMatches("main", "cycle_availability", originalKey) {
			m.cycleAvailability()
			m.rebuild()
		}
	case "v":
		if m.keyMatches("main", "toggle_layout", originalKey) {
			if m.layout == "top-paid-free" {
				m.layout = "all"
			} else {
				m.layout = "top-paid-free"
			}
			m.persistLayout()
			m.rebuild()
		}
	case "o":
		if !m.keyMatches("main", "open_settings", originalKey) {
			break
		}
		m.overlay, m.settingsCursor = "settings", 0
	case "?":
		if !m.keyMatches("main", "help", originalKey) {
			break
		}
		// ? is a faster entry point into the same sectioned full help F1
		// opens, landing directly on Hotkeys (index 2) instead of Overview.
		// It is otherwise identical: fully navigable, and closes the same
		// way (Esc, or ? again — handled by the overlay == "help" case
		// above, keyed off help.close, which already includes "?").
		m.overlay = "help"
		m.setHelpSection(2)
	case "f1":
		if !m.keyMatches("main", "full_help", originalKey) {
			break
		}
		m.overlay = "help"
		m.setHelpSection(0)
	case "enter", "right":
		if !m.keyMatches("main", "open_details", originalKey) {
			break
		}
		if len(m.visible) > 0 {
			m.overlay, m.detailOffset = "detail", 0
		}
	case "q", "r":
		m.sortKey = map[string]string{"q": "quality", "r": "q/p"}[key]
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
		if m.inputMode == "help-search" {
			m.input = ""
		}
		m.inputMode = ""
	case "enter":
		if m.inputMode == "help-search" {
			m.helpSearch, m.inputMode = m.input, ""
			m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
			m.helpMatch = -1
			m.helpNextMatch(1)
			return m, nil
		}
		candidate := m.input
		inputMode := m.inputMode
		if m.inputMode == "search" {
			m.inputMode, m.search = "", strings.TrimSpace(candidate)
			m.rebuild()
			if m.search == "" {
				m.status = "search: none (cleared)"
			} else {
				m.status = fmt.Sprintf("search: %q (%d matches)", m.search, len(m.visible))
			}
			return m, nil
		}
		m.inputMode = ""
		if strings.TrimSpace(candidate) == "" {
			m.filter, m.err = "", ""
			if inputMode == "filter" {
				m.status = "filter: none (cleared)"
			}
			if inputMode == "filter" && m.configPath != "" {
				if err := config.SaveTUIFilter(m.configPath, ""); err != nil {
					m.err = err.Error()
				}
			}
			m.rebuild()
			return m, nil
		}
		_, err := filterTableModels(append([]model.Model(nil), m.models...), strings.Split(candidate, ","))
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.filter, m.err = candidate, ""
		if inputMode == "filter" {
			m.status = "filter: " + candidate
		}
		if inputMode == "filter" && m.configPath != "" {
			if err := config.SaveTUIFilter(m.configPath, candidate); err != nil {
				m.err = err.Error()
			}
		}
		m.rebuild()
		return m, nil
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

func (m *tuiModel) cycleAvailability() {
	m.cycleAvailabilityDir(1)
}

// cycleAvailabilityDir cycles the availability filter through any/free/paid.
// dir > 0 moves forward (any -> free -> paid -> any); dir < 0 moves backward.
func (m *tuiModel) cycleAvailabilityDir(dir int) {
	parts := make([]string, 0, len(splitFilter(m.filter)))
	current := "any"
	for _, raw := range splitFilter(m.filter) {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(strings.ToLower(trimmed), "availability:") {
			current = strings.TrimPrefix(strings.ToLower(trimmed), "availability:")
			continue
		}
		parts = append(parts, trimmed)
	}
	forward := map[string]string{"any": "free", "free": "paid", "paid": "any"}
	backward := map[string]string{"any": "paid", "paid": "free", "free": "any"}
	next := forward[current]
	if dir < 0 {
		next = backward[current]
	}
	if next != "any" {
		parts = append(parts, "availability:"+next)
	}
	m.filter = strings.Join(parts, ",")
	m.filterFormExplicit = true
	m.filterDefaulted = false
}

func (m *tuiModel) persistLayout() {
	if m.configPath == "" {
		return
	}
	if err := config.SaveTUILayout(m.configPath, m.layout, m.topN); err != nil {
		m.err = err.Error()
	}
}

// toggleLanguage flips m.lang between English ("") and Russian ("ru") and
// persists the choice, the same write-through-only-when-configured shape
// persistLayout uses. Unlike layout/topN, the language is never re-read
// from disk on a periodic refresh (refreshCmd's tuiRefreshMsg carries no
// language field): the toggle already writes through immediately, so a
// live session and its own config.yaml can never disagree with each
// other, and there is no CLI flag for language whose precedence a
// mid-session reload would need to protect (unlike TUIFilter's
// filterExplicit/filterDefaulted machinery). See the key() call site for
// why this is checked from every overlay, not just the main list.
func (m *tuiModel) toggleLanguage() {
	if m.lang == "ru" {
		m.lang = ""
	} else {
		m.lang = "ru"
	}
	if m.configPath == "" {
		return
	}
	if err := config.SaveTUILanguage(m.configPath, m.lang); err != nil {
		m.err = err.Error()
	}
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
func (m tuiModel) columnKey(key, originalKey string) (tuiModel, tea.Cmd) {
	switch key {
	case "esc":
		m.closeOverlay()
	case "up", "k":
		m.columnCursor = max(0, m.columnCursor-1)
	case "down", "j":
		m.columnCursor = min(len(tuiColumns)-1, m.columnCursor+1)
	case " ":
		if !m.keyMatches("columns", "toggle", originalKey) {
			break
		}
		if len(m.pendingColumns) > 1 || !containsColumn(m.pendingColumns, tuiColumns[m.columnCursor]) {
			m.togglePending(tuiColumns[m.columnCursor])
		}
	case "enter":
		if !m.keyMatches("columns", "apply", originalKey) {
			break
		}
		m.columns, m.overlay = append([]tuiColumn(nil), m.pendingColumns...), ""
	}
	return m, nil
}

func (m tuiModel) settingsKey(key, originalKey string) (tuiModel, tea.Cmd) {
	const settingsItems = 6
	switch key {
	case "esc", "o":
		m.closeOverlay()
	case "up", "k":
		m.settingsCursor = max(0, m.settingsCursor-1)
	case "down", "j":
		m.settingsCursor = min(settingsItems-1, m.settingsCursor+1)
	case "enter", " ":
		switch m.settingsCursor {
		case 0:
			if m.ranking == rankingTier {
				m.ranking = rankingMixed
			} else {
				m.ranking = rankingTier
			}
			m.sortKey = "q/p"
			m.rebuild()
		case 1:
			if !m.keyMatches("settings", "switch_source", originalKey) {
				return m, nil
			}
			return m.switchScoreSource()
		case 2:
			m.openFilterEditor()
		case 3:
			m.cycleAvailability()
			m.rebuild()
		case 4:
			if m.layout == "top-paid-free" {
				m.layout = "all"
			} else {
				m.layout = "top-paid-free"
			}
			m.persistLayout()
			m.rebuild()
		case 5:
			m.overlay, m.pendingColumns, m.columnCursor = "columns", append([]tuiColumn(nil), m.columns...), 0
		}
	case "left", "right":
		switch m.settingsCursor {
		case 3:
			m.cycleAvailabilityDir(map[string]int{"left": -1, "right": 1}[key])
			m.rebuild()
		case 4:
			m.topN = max(1, m.topN+map[string]int{"left": -1, "right": 1}[key])
			m.persistLayout()
			m.rebuild()
		}
	}
	return m, nil
}

func tuiAvailabilityFromFilter(value string) string {
	for _, raw := range splitFilter(value) {
		trimmed := strings.ToLower(strings.TrimSpace(raw))
		if strings.HasPrefix(trimmed, "availability:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "availability:"))
		}
	}
	return "any"
}

func (m *tuiModel) openFilterEditor() {
	m.overlay = "filter"
	m.inputMode = ""
	m.filterCursor = 0
	if m.filterFormExplicit || m.filter != config.DefaultFilter {
		m.filterDraft = tuiFilterDraftFromString(m.filter)
	} else {
		m.filterDraft = tuiFilterDraft{}
	}
}

func (m tuiModel) filterKey(key string, msg tea.KeyMsg) (tuiModel, tea.Cmd) {
	const filterFields = 10
	switch key {
	case "esc":
		m.closeOverlay()
	case "up":
		m.filterCursor = max(0, m.filterCursor-1)
	case "k":
		m.filterCursor = max(0, m.filterCursor-1)
	case "down":
		m.filterCursor = min(filterFields-1, m.filterCursor+1)
	case "left":
		if m.filterCursor == 3 {
			m.filterDraft.tier = tuiPreviousFilterTier(m.filterDraft.tier)
		} else if m.filterCursor == 9 {
			m.filterDraft.availability = tuiPreviousAvailability(m.filterDraft.availability)
		} else if m.filterCursor >= 4 {
			m.filterDraft.step(m.filterCursor, -1, m.filterSteps)
		}
	case "right":
		if m.filterCursor == 3 {
			m.filterDraft.tier = tuiNextFilterTier(m.filterDraft.tier)
		} else if m.filterCursor == 9 {
			m.filterDraft.availability = tuiNextAvailability(m.filterDraft.availability)
		} else if m.filterCursor >= 4 {
			m.filterDraft.step(m.filterCursor, 1, m.filterSteps)
		}
	case "j", "tab":
		m.filterCursor = min(filterFields-1, m.filterCursor+1)
	case "shift+tab":
		m.filterCursor = max(0, m.filterCursor-1)
	case " ":
		switch m.filterCursor {
		case 0:
			m.filterDraft.free = !m.filterDraft.free
		case 1:
			m.filterDraft.paid = !m.filterDraft.paid
		case 2:
			m.filterDraft.scored = !m.filterDraft.scored
		case 3:
			m.filterDraft.tier = tuiNextFilterTier(m.filterDraft.tier)
		case 8:
			m.filterDraft.hasQP = !m.filterDraft.hasQP
		case 9:
			m.filterDraft.availability = tuiNextAvailability(m.filterDraft.availability)
		}
	case "c":
		m.filterDraft = tuiFilterDraft{}
	case "backspace":
		m.filterDraft.deleteLast(m.filterCursor)
	case "enter":
		return m.applyFilterDraft()
	default:
		if len(msg.Runes) > 0 && m.filterCursor >= 3 {
			if m.filterCursor != 3 {
				m.filterDraft.append(m.filterCursor, string(msg.Runes))
			}
		}
	}
	return m, nil
}

func (m tuiModel) applyFilterDraft() (tuiModel, tea.Cmd) {
	m.filterDraft.clampNumeric()
	candidate := m.filterDraft.string()
	if _, err := filterTableModels(append([]model.Model(nil), m.models...), splitFilter(candidate)); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.filter, m.err, m.overlay = candidate, "", ""
	m.filterFormExplicit = true
	m.filterDefaulted = false
	m.status = "filter: " + filterStatusValue(candidate)
	if m.configPath != "" {
		if err := config.SaveTUIFilter(m.configPath, candidate); err != nil {
			m.err = err.Error()
		}
	}
	m.rebuild()
	return m, nil
}

func filterStatusValue(filter string) string {
	if strings.TrimSpace(filter) == "" {
		return "none (cleared)"
	}
	return filter
}

func tuiFilterTierValues() []string {
	return append([]string{""}, tier.Values()...)
}

func tuiNextFilterTier(current string) string {
	values := tuiFilterTierValues()
	for i, value := range values {
		if strings.EqualFold(value, current) {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

func tuiPreviousFilterTier(current string) string {
	values := tuiFilterTierValues()
	for i, value := range values {
		if strings.EqualFold(value, current) {
			return values[(i+len(values)-1)%len(values)]
		}
	}
	return values[0]
}

func tuiFilterDraftFromString(filter string) tuiFilterDraft {
	draft := tuiFilterDraft{}
	for _, raw := range splitFilter(filter) {
		value := strings.TrimSpace(raw)
		lower := strings.ToLower(value)
		switch {
		case lower == "free":
			draft.free = true
		case lower == "paid":
			draft.paid = true
		case lower == "scored":
			draft.scored = true
		case lower == "has-q/p":
			draft.hasQP = true
		case strings.HasPrefix(lower, "availability:"):
			draft.availability = strings.TrimSpace(value[len("availability:"):])
		case strings.HasPrefix(lower, "tier:"):
			draft.tier = strings.TrimSpace(value[len("tier:"):])
		case strings.HasPrefix(lower, "quality>="):
			draft.quality = tuiCanonicalDraftValue(4, strings.TrimSpace(value[len("quality>="):]))
		case strings.HasPrefix(lower, "context>="):
			draft.context = tuiCanonicalDraftValue(5, strings.TrimSpace(value[len("context>="):]))
			if draft.context == "0" {
				draft.context = ""
			}
		case strings.HasPrefix(lower, "input<="):
			draft.input = tuiCanonicalDraftValue(6, strings.TrimSpace(value[len("input<="):]))
			if draft.input == "0.00" {
				draft.input = ""
			}
		case strings.HasPrefix(lower, "output<="):
			draft.output = tuiCanonicalDraftValue(7, strings.TrimSpace(value[len("output<="):]))
			if draft.output == "0.00" {
				draft.output = ""
			}
		}
	}
	return draft
}

func (d tuiFilterDraft) string() string {
	filters := make([]string, 0, 10)
	if d.free {
		filters = append(filters, "free")
	}
	if d.paid {
		filters = append(filters, "paid")
	}
	if d.scored {
		filters = append(filters, "scored")
	}
	if d.hasQP {
		filters = append(filters, "has-q/p")
	}
	if d.availability != "" && d.availability != "any" {
		filters = append(filters, "availability:"+d.availability)
	}
	for _, item := range []struct{ name, value, operator string }{{"tier", d.tier, ":"}, {"quality", d.quality, ">="}, {"context", d.context, ">="}, {"input", d.input, "<="}, {"output", d.output, "<="}} {
		if strings.TrimSpace(item.value) != "" {
			field := map[string]int{"quality": 4, "context": 5, "input": 6, "output": 7}[item.name]
			value := strings.TrimSpace(item.value)
			if field != 0 {
				value = tuiCanonicalDraftValue(field, value)
				if (field == 5 && value == "0") || ((field == 6 || field == 7) && value == "0.00") {
					continue
				}
			}
			filters = append(filters, item.name+item.operator+value)
		}
	}
	return strings.Join(filters, ",")
}

func tuiNextAvailability(current string) string {
	values := []string{"", "any", "free", "paid"}
	for i, value := range values {
		if value == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

// tuiPreviousAvailability cycles the Filter overlay's Availability field
// backward through "", "any", "free", "paid" — the reverse of tuiNextAvailability.
func tuiPreviousAvailability(current string) string {
	values := []string{"", "any", "free", "paid"}
	for i, value := range values {
		if value == current {
			return values[(i-1+len(values))%len(values)]
		}
	}
	return values[0]
}

func (d *tuiFilterDraft) step(field, direction int, steps config.TUISteps) {
	steps = steps.WithDefaults()
	values := []*string{nil, nil, nil, nil, &d.quality, &d.context, &d.input, &d.output}
	if field < 0 || field >= len(values) || values[field] == nil {
		return
	}
	value := 0.0
	if strings.TrimSpace(*values[field]) != "" {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(*values[field]), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return
		}
		value = parsed
		if field == 4 && value > 0 && value <= 1 {
			value *= 100
		}
		if direction < 0 && value <= 0 {
			*values[field] = ""
			return
		}
	} else {
		if direction < 0 {
			return
		}
		if field == 4 || field == 5 {
			if field == 4 {
				*values[field] = tuiCanonicalDraftValue(field, strconv.Itoa(steps.QualityPoints))
			} else {
				*values[field] = tuiCanonicalDraftValue(field, strconv.Itoa(steps.ContextTokens))
			}
		} else {
			base := steps.InputCents
			if field == 7 {
				base = steps.OutputCents
			}
			*values[field] = tuiCanonicalDraftValue(field, fmt.Sprintf("%.2f", float64(base)/100))
		}
		return
	}
	previous := value
	if field == 4 {
		step := steps.QualityPoints
		if steps.Legacy {
			step = steps.Quality
		}
		value += float64(direction * step)
	} else {
		if steps.Legacy {
			step := []int{0, 0, 0, 0, 0, steps.Context, steps.Input, steps.Output}[field]
			value *= 1 + float64(direction)*float64(step)/100
			value = tuiSteppedInteger(value, previous, direction)
		} else if field == 5 {
			value += float64(direction * tuiContextStep(int(math.Round(previous)), steps.ContextTokens))
			value = tuiSteppedInteger(value, previous, direction)
		} else {
			cents := int(math.Round(previous * 100))
			base := steps.InputCents
			if field == 7 {
				base = steps.OutputCents
			}
			cents += direction * tuiPriceStep(cents, base)
			value = float64(maxInt(0, cents)) / 100
			*values[field] = tuiPriceFilterValue(value)
			return
		}
	}
	if value < 0 {
		value = 0
	}
	if field == 4 && value > 100 {
		value = 100
	}
	*values[field] = tuiIntegerValue(value)
}

func tuiContextStep(current, base int) int {
	if current < 128000 {
		return base
	}
	if current < 1000000 {
		return base * 4
	}
	return base * 16
}

func tuiPriceStep(currentCents, baseCents int) int {
	return maxInt(1, baseCents)
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func tuiSteppedInteger(value, previous float64, direction int) float64 {
	current := math.Round(math.Max(0, previous))
	stepped := math.Round(math.Max(0, value))
	if current > 0 && direction > 0 && stepped <= current {
		return current + 1
	}
	if current > 0 && direction < 0 && stepped >= current {
		return current - 1
	}
	return stepped
}

func tuiIntegerValue(value float64) string {
	return strconv.FormatInt(int64(math.Round(math.Max(0, value))), 10)
}

func tuiPriceFilterValue(value float64) string {
	return strconv.FormatFloat(math.Max(0, value), 'f', 2, 64)
}

func (d *tuiFilterDraft) clampNumeric() {
	values := []*string{nil, nil, nil, nil, &d.quality, &d.context, &d.input, &d.output}
	for field, value := range values {
		if value == nil || strings.TrimSpace(*value) == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(*value), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			continue
		}
		*value = tuiCanonicalDraftValue(field, strconv.FormatFloat(parsed, 'f', -1, 64))
	}
}

func tuiCanonicalDraftValue(field int, raw string) string {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return strings.TrimSpace(raw)
	}
	value = math.Max(0, value)
	switch field {
	case 4:
		if value > 0 && value <= 1 {
			value *= 100
		}
		return tuiIntegerValue(math.Min(100, value))
	case 5:
		return tuiIntegerValue(value)
	case 6, 7:
		return tuiPriceFilterValue(math.Round(value*100) / 100)
	default:
		return strings.TrimSpace(raw)
	}
}

func (d *tuiFilterDraft) append(field int, value string) {
	switch field {
	case 3:
		d.tier += value
	case 4:
		d.quality += value
	case 5:
		d.context += value
	case 6:
		d.input += value
	case 7:
		d.output += value
	}
}

func (d *tuiFilterDraft) deleteLast(field int) {
	values := []*string{nil, nil, nil, &d.tier, &d.quality, &d.context, &d.input, &d.output}
	if field < len(values) && values[field] != nil {
		value := *values[field]
		if value != "" {
			_, size := utf8.DecodeLastRuneInString(value)
			*values[field] = value[:len(value)-size]
		}
	}
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
		return tuiHelpView(m)
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
	if m.overlay == "filter" {
		return tuiFilterView(m)
	}
	if m.overlay == "settings" {
		rankingName := "mixed-utility"
		if m.ranking == rankingTier {
			rankingName = "tier-priority"
		}
		columns := make([]string, 0, len(m.columns))
		for _, col := range m.columns {
			columns = append(columns, string(col))
		}
		lines := []string{
			"Settings (Enter/Space change, Esc close)",
			"",
			"> Ranking: " + rankingName,
			"  Score source: " + m.scoreSource + " (Space switches SWE-bench/Arena)",
			"  Filter: " + tuiDetailValue(m.filter),
			"  Availability: " + tuiAvailabilityFromFilter(m.filter),
			"  Layout: " + m.layout + " (top N=" + strconv.Itoa(m.topN) + ")",
			"  Columns: " + strings.Join(columns, ", "),
			"",
			"Move Down to Score source, then press Space to switch.",
			"Source uses the local snapshot; R refreshes data.",
			"Select Filter to reuse the structured filter input.",
		}
		// The Status/Error line reports live state, not part of the settings
		// list above it, and must not sit flush against the last static hint
		// line — hence the blank separator ahead of it, added only when
		// there is actually a status/error line to separate.
		if m.scoreSourceLoading {
			lines = append(lines, "", "Status: loading "+m.pendingScoreSource+" from local snapshot...")
		} else if m.err != "" {
			lines = append(lines, "", "Error: "+m.err)
		} else if m.status != "" {
			lines = append(lines, "", "Status: "+m.status)
		}
		for i := 0; i < 6; i++ {
			prefix := "  "
			if i == m.settingsCursor {
				prefix = "> "
			}
			lines[i+2] = prefix + strings.TrimSpace(lines[i+2])
		}
		return tuiBox(strings.Join(lines, "\n"), m.width, m.height)
	}
	title := truncateTable("OpenRouter models", m.width)
	searchContext := "none"
	if m.search != "" {
		searchContext = fmt.Sprintf("%q (%d matches)", m.search, len(m.visible))
	}
	meta := truncateTable(plainTableText(fmt.Sprintf("ranking:%s  score:%s  sort:%s%s  layout:%s  top-n:%d  filter:%q  search:%s  models:%d  data:%s", rankingLabel(m.ranking), m.scoreSource, m.sortKey, reverseLabel(m.reverse), m.layout, m.topN, m.filter, searchContext, len(m.visible), m.updatedAt)), m.width)
	lines := []string{tuiTitleStyle.Render(title), tuiMetaStyle.Render(meta)}
	columns := m.renderColumns()
	lines = append(lines, tuiHeaderStyle.Render(m.renderTUILine(columns, nil, false)))
	status := m.status
	if status == "" {
		status = "status: ready"
	}
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
	hints := "↑↓ navigate · o settings · R refresh · x quit · f filter · p availability · q quality · r q/p"
	if m.search != "" {
		hints += " · / search · Enter empty search to clear"
	}
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
			if i == m.topSeparator {
				lines = append(lines, "")
			}
			values := make([]string, len(columns))
			for j, col := range columns {
				values[j] = tuiCellWithIconsAndGaps(m.visible[i], col, m.lastNote, m.scoreSource, m.icons, m.iconGap, m.iconGaps)
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
		// footer is the status/hints/input cluster reported at the bottom of
		// the screen — state, not table content — and it must never sit
		// flush against the last table row: a blank separator line goes in
		// front of it whenever the budget has room for one. rowsBudget above
		// already reserves exactly one spare row for this (it was previously
		// unused, silently rendering as blank space below the footer instead
		// of above it), so the common case always fits; the length check is
		// only a floor for pathological cases where the visible list is
		// shorter than rowsBudget and doesn't need the spare row at all.
		footer := []string{statusLine, hintsLine}
		if inputLine != "" {
			footer = append(footer, inputLine)
		}
		if len(lines)+1+len(footer) <= m.height {
			lines = append(lines, "")
		}
		lines = append(lines, footer...)
		return strings.Join(lines, "\n")
	}
	return m.compactView(lines, statusLine, hintsLine, inputLine)
}

func tuiFilterView(m tuiModel) string {
	values := []string{tuiFilterCheck(m.filterDraft.free), tuiFilterCheck(m.filterDraft.paid), tuiFilterCheck(m.filterDraft.scored), m.filterDraft.tier, m.filterDraft.quality, m.filterDraft.context, m.filterDraft.input, m.filterDraft.output, tuiFilterCheck(m.filterDraft.hasQP), m.filterDraft.availability}
	labels := []string{"Free", "Paid", "Scored", "Tier", "Quality minimum", "Context minimum", "Input max", "Output max", "Has Q/P", "Availability"}
	lines := []string{"Filter", "", "↑/↓ move · ←/→ step values · Space toggles/cycles Tier · type to edit", "Tier options: (any), " + tier.ValuesString(), ""}
	for i, label := range labels {
		prefix := "  "
		if i == m.filterCursor {
			prefix = "> "
		}
		value := values[i]
		if i >= 4 {
			value = tuiFilterDisplayValue(i, value)
		}
		if i >= 3 && value == "" {
			value = "(any)"
		}
		lines = append(lines, prefix+label+": "+value)
	}
	steps := m.filterSteps.WithDefaults()
	stepText := fmt.Sprintf("Steps: quality ±%d points · context ±%d tokens · input/output ±%d/%d cents · prices use two decimals · values >= 0", steps.QualityPoints, steps.ContextTokens, steps.InputCents, steps.OutputCents)
	if steps.Legacy {
		stepText = fmt.Sprintf("Steps (legacy): quality ±%d points · context/input/output ±%d%%/%d%%/%d%% · display rounds to integers · values >= 0", steps.Quality, steps.Context, steps.Input, steps.Output)
	}
	lines = append(lines, "", "Enter apply · Esc cancel · c clear · Tab/Shift+Tab move", "Tier options: (any), "+tier.ValuesString(), stepText)
	return tuiBox(strings.Join(lines, "\n"), m.width, m.height)
}

func tuiFilterDisplayValue(field int, value string) string {
	return tuiCanonicalDraftValue(field, value)
}

func tuiFilterCheck(checked bool) string {
	if checked {
		return "[x]"
	}
	return "[ ]"
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
	if m.width > 0 && m.width < 40 {
		return tableDisplayWidth("  ") + len(columns) + 3*(len(columns)-1)
	}
	width := tableDisplayWidth("  ") + 3*(len(columns)-1)
	for _, column := range columns {
		width += tuiColumnMinimumWidth(column, m.scoreSource)
	}
	return width
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
	widths := tuiCellWidths(columns, available, m.nameWidth, m.scoreSource)
	parts := make([]string, len(columns))
	for i, col := range columns {
		value := tuiColumnLabel(col, m.scoreSource)
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

func tuiCellWidths(columns []tuiColumn, available, nameWidth int, scoreSource string) []int {
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
	if nameWidth <= 0 {
		nameWidth = config.DefaultNameWidth
	}
	minimums := make([]int, len(columns))
	minimumWidth := 0
	for i, column := range columns {
		minimums[i] = tuiColumnMinimumWidth(column, scoreSource)
		minimumWidth += minimums[i]
	}
	if minimumWidth > available {
		for i := range widths {
			widths[i] = 1
		}
		return widths
	}
	nameIndex := -1
	for i, column := range columns {
		if column == colName {
			nameIndex = i
			break
		}
	}
	remaining := available
	if nameIndex >= 0 {
		otherMinimum := minimumWidth - minimums[nameIndex]
		widths[nameIndex] = min(max(minimums[nameIndex], nameWidth), max(minimums[nameIndex], available-otherMinimum))
		remaining -= widths[nameIndex]
	}
	for i := range columns {
		if i == nameIndex {
			continue
		}
		widths[i] = minimums[i]
		remaining -= widths[i]
	}
	for i := 0; remaining > 0; i++ {
		index := i % len(widths)
		if index == nameIndex && len(widths) > 1 {
			continue
		}
		widths[index]++
		remaining--
	}
	return widths
}

func tuiColumnMinimumWidth(column tuiColumn, scoreSource string) int {
	if scoreSource == "" {
		scoreSource = scoreSourceDefault
	}
	return max(1, tableDisplayWidth(tuiColumnLabel(column, scoreSource)))
}

func tuiNumericColumn(column tuiColumn) bool {
	switch column {
	case colQuality, colContext, colInput, colOutput:
		return true
	default:
		return false
	}
}

func tuiColumnLabel(column tuiColumn, scoreSource string) string {
	switch column {
	case colName:
		return "Name"
	case colSlug:
		return "Slug"
	case colClaude:
		return "Claude"
	case colStatus:
		if scoreSource == scoreSourceArena {
			return "Arena Elo"
		}
		return "SWE %"
	case colQuality:
		return "Q/P score/$M"
	case colContext:
		return "Context tok"
	case colInput:
		return "In $/M"
	case colOutput:
		return "Out $/M"
	case colTask:
		return "Task fit"
	case colNote:
		return "Note"
	default:
		return string(column)
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

// tuiHelpView renders the full-screen help overlay. It takes the whole
// model rather than the four scalars it used to, the way tuiDetailView
// already does: growing past four scalars would have meant a
// six-parameter form — two adjacent strings and two adjacent ints — which
// is exactly the signature shape that eventually gets called with its
// arguments swapped, and the overlay needs more of the model than it used
// to.
//
// While a help search is being typed — m.inputMode == "help-search",
// before Enter — one extra line "/ <input>_" is inserted between the
// scrolled content and the footer, and the content budget shrinks by
// exactly one row to pay for it. Both halves mirror the model list's own
// input line and its rowsBudget-- (View, above): without the shrink the
// overlay would build height+2 lines and tuiFullscreenText would clip the
// input line away again, leaving the bug exactly where it was. m.input is
// run through plainTableText before it is appended, exactly like the
// model list's own inputLine (View, above): tuiFullscreenText splits its
// input on "\n" before sanitizing each resulting line, so a raw m.input
// containing an embedded newline (reachable via bracketed paste, which
// this program leaves enabled) would silently split into two screen rows
// instead of being neutralised into one; pre-sanitizing here is what
// actually prevents that. Running plainTableText again inside
// tuiFullscreenText's own per-line pass is harmless on already-plain text
// and stays as defense in depth.
//
// The input line is skipped in the styling post-pass below by its
// tracked index (inputLineIndex), not by any property of its trailing
// text. plainTableText strips markdown "__" and "**" markers from every
// line before that pass runs, so a query ending in "_" can lose its
// cursor marker to that stripping — "search_" plus the cursor becomes
// "/ search__", which plainTableText reduces to "/ search", matching the
// HasSuffix(plain, "search") heading rule below. Skipping by index is
// what actually prevents that false match; nothing about how the line
// ends does.
func (m tuiModel) helpLines() []string {
	lines := tuiHelpSectionLines(m.helpSection)
	lines[0] = fmt.Sprintf("%s (version %s)", lines[0], version)
	return tuiConfiguredHelpLines(lines, m.keymap)
}

// tuiHelpSectionLines builds the lines the F1 overlay actually renders for
// the given section: one section at a time, framed by a page title and the
// tab bar (tuiHelpTabBarLine) that names all six and highlights the active
// one. F1 and ? both open this same overlay — F1 lands on section 0
// (Overview), ? on section 2 (Hotkeys) — there is no separate unsectioned
// mode anymore. This is deliberately a different function from the
// package-level tuiHelpLines(), which keeps returning
// strings.Split(tuiHelpDocument, "\n") — the flat concatenation of all six
// section bodies behind one English-only title line. That legacy view
// exists purely for content and structural tests ("does the full help still
// document X", tab-column audits, the Cyrillic-free checks) that were
// written against one document and do not need to change just because the
// overlay now shows it one section at a time; this function is what
// actually reaches the screen.
func tuiHelpSectionLines(section int) []string {
	section = tuiClampHelpSection(section)
	lines := []string{tuiHelpTitleLine, "", tuiHelpTabBarLine(section), ""}
	return append(lines, strings.Split(tuiHelpSections[section].Body, "\n")...)
}

// tuiClampHelpSection keeps a section index in range without wrapping,
// matching every other cursor-style movement in this file — m.cursor,
// columnCursor, settingsCursor and filterCursor all clamp at their ends
// instead of wrapping round. Left/Right (setHelpSection) rely on this to
// stop at "Overview" and "Methodology" instead of cycling past them.
func tuiClampHelpSection(section int) int {
	return max(0, min(len(tuiHelpSections)-1, section))
}

// tuiHelpTabBarLine renders the section indicator shown above the F1
// overlay's content, e.g. "[1 Overview] 2 Score Sources 3 Hotkeys
// 4 Filters 5 Model Detail 6 Methodology" with the active section
// bracketed. The bracket is the plain-text signal of which section is
// active; tuiHelpView additionally colours that bracketed span with
// tuiSelectedStyle, the same style the main table uses for its selected
// row, so the active tab reads the same way the app already marks "the
// highlighted one" everywhere else.
func tuiHelpTabBarLine(active int) string {
	active = tuiClampHelpSection(active)
	parts := make([]string, len(tuiHelpSections))
	for i, section := range tuiHelpSections {
		label := fmt.Sprintf("%d %s", i+1, section.Title)
		if i == active {
			label = "[" + label + "]"
		}
		parts[i] = label
	}
	return strings.Join(parts, " ")
}

func tuiConfiguredHelpLines(lines []string, keymap config.TUIKeymap) []string {
	if keymap == nil {
		return lines
	}
	keymap = keymap.WithDefaults()
	result := append([]string(nil), lines...)
	replacements := map[string]config.TUIBindings{
		`\tUp\tnavigate\t`:                                                   keymap["main"]["navigate_up"],
		`\tDown\tnavigate\t`:                                                 keymap["main"]["navigate_down"],
		`\tj / k\tmove\t`:                                                    append(keymap["main"]["navigate_down"], keymap["main"]["navigate_up"]...),
		`\tEnter / Right\tdetail\t`:                                          keymap["main"]["open_details"],
		`\tEsc / Left / h\tclose\t`:                                          keymap["detail"]["close"],
		`\to\tsettings\topen settings.`:                                      keymap["main"]["open_settings"],
		`\t?\thelp\topen help at Hotkeys.`:                                   keymap["main"]["help"],
		`\tF1\thelp\topen full help.`:                                        keymap["main"]["full_help"],
		`\tSpace\tswitch\t(main) switch between SWE-bench and Arena.`:        keymap["main"]["switch_source"],
		`\tSpace\tswitch\t(in Settings) switch between SWE-bench and Arena.`: keymap["settings"]["switch_source"],
		`\tSpace\tcolumns\t`:                                                 keymap["columns"]["toggle"],
		`\tSpace\ttier\t`:                                                    keymap["filter"]["toggle"],
		`\tEnter\tapply\t`:                                                   keymap["filter"]["apply"],
		`\tEsc\tcancel\t`:                                                    keymap["filter"]["close"],
		`\tEsc\tcolumns\t`:                                                   keymap["columns"]["close"],
		`\tEnter\tcolumns\t`:                                                 keymap["columns"]["apply"],
		`\tEnter\tcolumns apply\t`:                                           keymap["columns"]["apply"],
		`\t?\thelp\tclose help.`:                                             keymap["help"]["close"],
		`\tEsc\tclose\tclose help.`:                                          keymap["help"]["close"],
		`\tUp\tsettings navigate\t`:                                          keymap["settings"]["navigate_up"],
		`\tDown\tsettings navigate\t`:                                        keymap["settings"]["navigate_down"],
		`\tEsc\tsettings close\t`:                                            keymap["settings"]["close"],
		`\tUp\tdetail navigate\t`:                                            keymap["detail"]["navigate_up"],
		`\tDown\tdetail navigate\t`:                                          keymap["detail"]["navigate_down"],
		`\tUp\thelp navigate\t`:                                              keymap["help"]["navigate_up"],
		`\tDown\thelp navigate\t`:                                            keymap["help"]["navigate_down"],
		`\tUp\tcolumns navigate\t`:                                           keymap["columns"]["navigate_up"],
		`\tDown\tcolumns navigate\t`:                                         keymap["columns"]["navigate_down"],
		`\tEsc\tcolumns close\t`:                                             keymap["columns"]["close"],
		`\tUp\tfilter navigate\t`:                                            keymap["filter"]["navigate_up"],
		`\tDown\tfilter navigate\t`:                                          keymap["filter"]["navigate_down"],
		`\tEsc\tfilter close\t`:                                              keymap["filter"]["close"],
		`\tEsc\tmain close\t`:                                                keymap["main"]["close"],
	}
	for i := range result {
		for marker, bindings := range replacements {
			if strings.Contains(result[i], marker) {
				parts := strings.Split(marker, `\t`)
				replacement := `\t` + strings.Join(bindings, " / ") + `\t` + strings.Join(parts[2:], `\t`)
				result[i] = strings.Replace(result[i], marker, replacement, 1)
			}
		}
	}
	return result
}

// helpViewportHeight reserves room for the trailing status cluster —  a
// blank separator plus the position footer, and the search input line when
// one is active — so tuiHelpView's footer (and, while searching, its input
// line) is never silently clipped away by tuiFullscreenText the way it used
// to be whenever a section's body filled or exceeded the viewport. tuiHelpView
// still gates the blank separator itself on the exact remaining space
// (see tuiHelpView), so a viewport too small to spare it still shows the
// footer — and the input line, while searching — rather than losing them to
// a cosmetic-only row.
func (m tuiModel) helpViewportHeight() int {
	reserved := 2 // blank separator + footer
	if m.inputMode == "help-search" {
		reserved = 3 // blank separator + input line + footer
	}
	return max(1, m.height-reserved)
}

func (m tuiModel) helpMaxOffset() int { return max(0, len(m.helpLines())-m.helpViewportHeight()) }

// setHelpSection switches the active help section — on open (F1 lands on
// section 0/Overview, ? lands on section 2/Hotkeys) and while already open
// (digit keys 1-5, Left/Right, and F1 pressed again to jump back to
// Overview — see the overlay == "help" key handling above). It resets
// helpOffset to 0 rather than remembering a per-section scroll position:
// every section starts back at its own top on arrival — simplest correct
// behaviour, and no section here is long enough for "resume where I left
// off" to earn a second int per section. Search matches are rebuilt
// against the new section's own lines because search is section-scoped: a
// helpMatches slice computed against the previous section would index into
// the wrong document once the section changes.
func (m *tuiModel) setHelpSection(section int) {
	m.helpSection, m.helpOffset = tuiClampHelpSection(section), 0
	m.helpMatches = tuiHelpSearchInLines(m.helpSearch, m.helpLines())
	m.helpMatch = -1
}

func tuiHelpView(m tuiModel) string {
	lines := m.helpLines()
	inputActive := m.inputMode == "help-search"
	body := m.helpViewportHeight()
	offset := max(0, min(m.helpOffset, max(0, len(lines)-body)))
	// The tab bar sits at a fixed absolute line (tuiHelpTabBarAbsoluteIndex,
	// see tuiHelpSectionLines) in every help view — there is only one
	// sectioned overlay now, however it was opened. tabBarLineIndex is
	// computed against the same slice this function hands to
	// tuiFullscreenText below, exactly like inputLineIndex and
	// footerLineIndex are, so it still points at the right physical line
	// once styledLines is split back out of the finished view.
	tabBarLineIndex := -1
	if idx := tuiHelpTabBarAbsoluteIndex - offset; idx >= 0 && idx < len(lines)-offset {
		tabBarLineIndex = idx
	}
	lines = lines[offset:min(len(lines), offset+body)]
	for i := range lines {
		lines[i] = tuiFormatHelpLine(lines[i], m.width)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	// The trailing status cluster (an optional input line, then the position
	// footer) reports state, not document content, and must not sit flush
	// against the last content line. helpViewportHeight already reserves
	// room for a blank separator ahead of it, so the common case always has
	// space; this length check is only a floor for a viewport too small to
	// spare the extra row, where showing the input line and footer at all
	// matters more than separating them. It always appends its own blank
	// line rather than checking whether the document's own last visible
	// line already is one — a section body ending its viewport exactly on
	// one of its own paragraph breaks occasionally doubles it up, which is
	// harmless (still separated, just by two rows instead of one); skipping
	// the append there would instead leave the line short of height, and
	// tuiFullscreenText's own trailing pad would then land after the
	// footer, displacing it from the screen's last line.
	trailingRows := 1 // footer
	if inputActive {
		trailingRows++ // + input line
	}
	blankLineIndex := -1
	if len(lines)+1+trailingRows <= m.height {
		blankLineIndex = len(lines)
		lines = append(lines, "")
	}
	inputLineIndex := -1
	if inputActive {
		inputLineIndex = len(lines)
		lines = append(lines, plainTableText("/ "+m.input+"_"))
	}
	matchStatus := "0 matches"
	if len(m.helpMatches) > 0 {
		position := 0
		if m.helpMatch >= 0 && m.helpMatch < len(m.helpMatches) {
			position = m.helpMatch + 1
		}
		matchStatus = fmt.Sprintf("%d/%d matches", position, len(m.helpMatches))
	}
	footer := fmt.Sprintf("Help %d-%d/%d · %s", offset+1, min(len(m.helpLines()), offset+body), len(m.helpLines()), matchStatus)
	if inputActive {
		footer += " · / search · Enter confirm search · Esc cancel"
	} else {
		footer += " · / search · n next match · N previous match · Esc close"
	}
	if m.helpSearch != "" {
		footer += fmt.Sprintf(" · %q", m.helpSearch)
	}
	footerLineIndex := len(lines)
	lines = append(lines, footer)
	view := tuiFullscreenText(strings.Join(lines, "\n"), m.width, m.height)
	needle := strings.ToLower(strings.TrimSpace(m.helpSearch))
	styledLines := strings.Split(view, "\n")
	for i, line := range styledLines {
		if i == inputLineIndex || i == footerLineIndex || i == blankLineIndex {
			continue
		}
		if i == tabBarLineIndex {
			styledLines[i] = tuiStyleHelpTabBar(line, m.helpSection)
			continue
		}
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "omt tui ") || strings.HasSuffix(plain, "keys") || plain == "Hotkeys" || plain == "Navigation" || plain == "Data/view" || plain == "Filters/settings" || plain == "Task-fit codes" || plain == "General/help" || strings.HasSuffix(plain, "view") || strings.HasSuffix(plain, "filters") || strings.HasSuffix(plain, "finish") || strings.HasSuffix(plain, "search") {
			styledLines[i] = tuiHeaderStyle.Render(line)
			continue
		}
		current := m.helpMatch >= 0 && m.helpMatch < len(m.helpMatches) && m.helpMatches[m.helpMatch] == offset+i
		styledLines[i] = tuiHighlightHelpMatches(line, needle, current)
	}
	return strings.Join(styledLines, "\n")
}

func tuiFormatHelpLine(line string, width int) string {
	if !strings.Contains(line, `\t`) {
		return line
	}
	parts := strings.Split(line, `\t`)
	if len(parts) == 4 && parts[0] == "" {
		parts = parts[1:]
	}
	if len(parts) != 3 {
		return line
	}
	available := max(1, width)
	if available < 15 {
		return truncateTable(parts[0]+" "+parts[1]+" "+parts[2], available)
	}
	keyWidth := min(8, max(3, available/7))
	actionWidth := min(16, max(7, available/4))
	descriptionWidth := max(1, available-keyWidth-actionWidth-4)
	return tuiPadCell(truncateTable(parts[0], keyWidth), keyWidth, false) + "  " + tuiPadCell(truncateTable(parts[1], actionWidth), actionWidth, false) + "  " + truncateTable(parts[2], descriptionWidth)
}

// tuiStyleHelpTabBar colours the active section's bracketed label within an
// already-rendered (and possibly width-truncated) tab-bar line, reusing
// tuiSelectedStyle — the same bold-on-blue the main table applies to its
// selected row — rather than inventing a new style for "the current one",
// per the confirmed design. It looks the target label up by exact text
// rather than by position, so a line truncated by a narrow terminal simply
// renders unstyled (the label it would colour is not there to find)
// instead of colouring the wrong span.
func tuiStyleHelpTabBar(line string, active int) string {
	if active < 0 || active >= len(tuiHelpSections) {
		return line
	}
	target := "[" + fmt.Sprintf("%d %s", active+1, tuiHelpSections[active].Title) + "]"
	index := strings.Index(line, target)
	if index < 0 {
		return line
	}
	return line[:index] + tuiSelectedStyle.Render(target) + line[index+len(target):]
}

func tuiHighlightHelpMatches(line, needle string, current bool) string {
	if needle == "" || line == "" || ansi.Strip(line) != line {
		return line
	}
	lower := strings.ToLower(line)
	if len(lower) != len(line) {
		return line
	}
	var out strings.Builder
	for from := 0; from < len(line); {
		index := strings.Index(lower[from:], needle)
		if index < 0 {
			out.WriteString(line[from:])
			break
		}
		start, end := from+index, from+index+len(needle)
		out.WriteString(line[from:start])
		if current {
			out.WriteString(tuiCurrentMatchStyle.Render(line[start:end]))
		} else {
			out.WriteString(tuiMatchStyle.Render(line[start:end]))
		}
		from = end
	}
	return out.String()
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
// Colour is applied by tuiStyleDetail strictly afterwards, to the
// finished text: nothing styled may ever flow back into the width
// arithmetic above, none of which is ANSI-aware.
func tuiDetailView(m tuiModel) string {
	row, ok := m.detailRow()
	if !ok {
		return tuiFullscreenText("Модель не выбрана · Esc close", m.width, m.height)
	}
	lines := tuiDetailLinesWithHistoryAndIconsAndGaps(row, m.scoreSource, m.width, time.Now(), m.priceHistory, m.icons, m.iconGap, m.iconGaps)
	body := tuiDetailBodyHeight(m.height)
	offset := max(0, min(m.detailOffset, max(0, len(lines)-body)))
	end := min(len(lines), offset+body)
	visible := append([]string(nil), lines[offset:end]...)
	if len(visible) == 0 {
		visible = []string{""}
	}
	footer := fmt.Sprintf("Detail %d-%d/%d · ↑↓ scroll · Esc close", offset+1, end, len(lines))
	// The footer is state (scroll position), not detail content, and must
	// not sit flush against the last content line. tuiDetailBodyHeight
	// already reserves two rows (blank + footer) for exactly this, so the
	// blank fits whenever the body actually filled up; the length check is
	// only a floor for a viewport too small to spare the extra row at all,
	// where showing the footer itself matters more than separating it. It
	// always appends its own blank line rather than checking whether the
	// page's own last visible line already is one — occasionally doubling
	// it up when a page ends exactly on one of the detail screen's own
	// section-break blanks is harmless (still separated, just by two rows);
	// skipping the append there would instead leave the page short of
	// height, and tuiFullscreenText's own trailing pad would then land
	// after the footer, displacing it from the screen's last line.
	if len(visible)+2 <= m.height {
		visible = append(visible, "")
	}
	visible = append(visible, footer)
	view := tuiFullscreenText(strings.Join(visible, "\n"), m.width, m.height)
	return tuiStyleDetail(view, offset == 0, len(visible)-1)
}

// tuiStyleDetail paints the detail screen's finished output. Everything
// above it — line building, wrapping, the scrolling arithmetic, the
// viewport slice and truncateTable's cut — has already run on plain text,
// which is the whole point: tableDisplayWidth and plainTableText know
// nothing about escape sequences, so a styled string entering them would
// be measured by its raw byte length, cut mid-escape, and end up on
// screen as visible "[38;5;87m" garbage. tuiHelpView takes exactly this
// approach for the same reason. Two lines are addressed by index rather
// than by text because their position is known for certain: the title is
// the first line whenever the screen is not scrolled, and the footer is
// the line tuiDetailView appended itself. A footer index past the end of
// the output simply never matches — that happens only at height 1, where
// tuiFullscreenText clips the footer away.
func tuiStyleDetail(view string, header bool, footer int) string {
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		switch {
		case i == 0 && header:
			lines[i] = tuiTitleStyle.Render(line)
		case i == footer:
			lines[i] = tuiHintStyle.Render(line)
		default:
			lines[i] = tuiStyleDetailLine(line)
		}
	}
	return strings.Join(lines, "\n")
}

// tuiStyleDetailLine styles one finished line by reading its own plain
// text: a field label up to and including the first ": ", a whole line
// that is a block heading, a greyed-out placeholder, an underlined URL.
// Values keep the terminal's default colour on purpose — hierarchy comes
// from there being less colour on screen than not, and the default colour
// is the one that reads correctly on any user theme.
//
// The ansi.Strip comparison is the function's safety interlock rather
// than decoration. By construction the line cannot carry an escape: every
// producer above is plain text, and tuiFullscreenText runs each line
// through plainTableText, which no escape survives. That is exactly what
// makes it safe to cut the line at a byte offset found in its text. If
// the assumption is ever broken, the line is handed back untouched
// instead of being sliced through the middle of an escape sequence.
//
// Misfires are possible — a sentence containing ": " gets a coloured
// opening, a prose line ending in a colon is read as a heading — and are
// accepted: layout, wrapping, scrolling and truncation are all already
// computed, so the only consequence is the colour of one line, and
// lipgloss closes its sequence at the end of every Render, so nothing can
// bleed into the next line.
func tuiStyleDetailLine(line string) string {
	plain := ansi.Strip(line)
	if plain != line || strings.TrimSpace(plain) == "" {
		return line
	}
	index := strings.Index(plain, ": ")
	if index < 0 {
		switch {
		case strings.HasPrefix(plain, "-- ") && strings.HasSuffix(plain, " --"):
			return tuiSectionStyle.Render(plain)
		case strings.HasSuffix(plain, ":"):
			return tuiHeaderStyle.Render(plain)
		case strings.HasPrefix(strings.TrimSpace(plain), tuiDetailPlaceholder):
			return tuiHintStyle.Render(plain)
		default:
			return plain
		}
	}
	label, value := plain[:index+2], plain[index+2:]
	switch {
	case strings.HasPrefix(value, tuiDetailPlaceholder):
		return tuiHeaderStyle.Render(label) + tuiHintStyle.Render(value)
	case strings.HasPrefix(value, "https://"), strings.HasPrefix(value, "http://"):
		return tuiHeaderStyle.Render(label) + tuiLinkStyle.Render(value)
	default:
		return tuiHeaderStyle.Render(label) + value
	}
}

// tuiHelpSection is one F1 full-help section: an English tab-bar label (the
// one piece of this document that is UI chrome, not reference prose — see
// tuiHelpTabBarLine) and an English body, exactly like the rest of this
// document has always been. See tuiHelpSections for the full list and
// tuiHelpDocument for why the bodies are also kept concatenated.
type tuiHelpSection struct {
	Title string
	Body  string
}

// tuiHelpTitleLine is the literal top line of the sectioned F1 overlay,
// shared by every section (see tuiHelpSectionLines) and by the legacy
// concatenated tuiHelpDocument below, so a structural test locating "the
// title line" finds the same text either way.
const tuiHelpTitleLine = "omt tui keys"

// tuiHelpTabBarAbsoluteIndex is the tab bar's fixed line position in every
// full-mode tuiHelpSectionLines result: 0 title, 1 blank, 2 tab bar, 3
// blank, 4+ the active section's body. tuiHelpView uses it to find the tab
// bar again after slicing to the viewport (see tabBarLineIndex there).
const tuiHelpTabBarAbsoluteIndex = 2

// tuiHelpSections is the ordered list of F1 full-help sections. Section
// index order is both the tab-bar order and what digit keys 1-6 and
// Left/Right (setHelpSection) navigate — index+1 is the digit that jumps to
// it. tuiHelpDocument below concatenates every Body, in this order, behind
// one shared title: that flat view is what content/structural tests still
// read, while the overlay itself (tuiHelpSectionLines) renders exactly one
// Body at a time, framed by the title and the tab bar.
var tuiHelpSections = []tuiHelpSection{
	{Title: "Overview", Body: tuiHelpSectionOverviewBody},
	{Title: "Score Sources", Body: tuiHelpSectionScoreSourcesBody},
	{Title: "Hotkeys", Body: tuiHelpSectionHotkeysBody},
	{Title: "Filters", Body: tuiHelpSectionFiltersBody},
	{Title: "Model Detail", Body: tuiHelpSectionDetailBody},
	{Title: "Methodology", Body: tuiHelpSectionMethodologyBody},
}

// tuiHelpSectionOverviewBody is the "Overview" section: what the tool does,
// the three independent kinds of data behind every row (price, quality,
// tier), the identity-gate philosophy that matches a leaderboard row to an
// OpenRouter slug, and the ranking formula (relocated verbatim from the
// pre-sectioning document's own "Ranking modes" block — see git history for
// the original single-page tuiHelpDocument this was split out of). The
// "three kinds of data" and "identity matching" blocks are broken into
// "- " bulleted points, one per original sentence, instead of run-on
// paragraphs — every sentence kept verbatim, only regrouped for
// scannability. The program self-reference in the opening line is "omt",
// the repo's short CLI name, not "openrouter".
const tuiHelpSectionOverviewBody = `omt tracks AI models available on OpenRouter and ranks them by quality and price.
- Quality comes from SWE-bench Verified or LMArena Elo scores; price comes from the OpenRouter
catalogue.
- Models are grouped into tiers matched against Claude Opus, Sonnet, and Haiku, so relative quality is
easy to judge.

What feeds every row: three independent kinds of data
Three kinds of data feed every row, and none of them is derived from another.
- Price and context come live from the OpenRouter catalogue.
- Quality is an independent benchmark score, SWE-bench Verified (from vals.ai or swebench.com) or
LMArena Elo, never both at once; see the Score sources section for how the two differ and why they are
never mixed.
- Tier is a hand-assigned, Claude-relative capability estimate from model-map.tsv, not something
computed from the score. It exists so an unrelated model family becomes comparable on one familiar
scale, "about Sonnet-class" or "about Haiku-class", even when that family has no benchmark number at
all.
- Quality and quality>= always mean a rankable, exact-product benchmark observation, never a vendor
claim and never the tier.

Identity matching: never fuzzy name matching
Matching a leaderboard row to an OpenRouter model is never done by fuzzy name matching.
- A hand-maintained map, model-map.tsv, is the only path from one site's row to the other site's slug.
- No entry in the map means no automatically collected score for that model, on purpose: a
plausible-looking name match across two independently run sites is exactly how a wrong number ends up
attached to the wrong model.
- The mapped source= key is itself the identity claim. If the source returns a row for that key, the row
is trusted (exact_product) even when the key's spelling differs from the OpenRouter slug, which is
normal between two sites with different naming conventions.
- When a mapped row actually measures a different checkpoint or variant of the same family, the map
entry is marked !variant (for example vals!variant=some/other-checkpoint), and the row stays out of the
ranking (variant_mismatch) despite the key match.
- A human, editing the map, is the only thing that can make that call; nothing in the code catches it
automatically.

Ranking modes
tier-priority: rankable models first, then Opus, Sonnet, Haiku, score, and Q/P.
mixed-utility: rankable first, then paid utility from the configured safe YAML formula. Without formula, compatibility is score + price_weight*tier_factor*ln(1+quality_price), with price mix 3:1, factors Opus=1, Sonnet=1, Haiku=0.5, Free=0, and weight 10. Formula vars, operations, depth and node limits are documented in README. Task-fit is never a multiplier.
Use o, then Down to Score source, then Space to switch between SWE-bench and Arena.
The CLI --ranking flag accepts legacy, tier, tier-priority, mixed, or mixed-utility; without it, mixed-utility sorting is used.`

// tuiHelpSectionScoreSourcesBody is the "Score Sources" section: the
// original short "Score sources" block (relocated verbatim), expanded with
// what actually distinguishes the three measurements — vals.ai's
// independent single-harness runs versus swebench.com's self-submitted,
// median-of-scaffolds leaderboard, and LMArena Elo's incomparable crowd
// preference scale — grounded in model-map.tsv's own header comment and the
// internal/sources package docs (valsai.go, swebench.go, arena.go). Each
// mini-header's paragraph is broken into "- " bulleted points, one per
// original sentence — every sentence, including the exact swebench.com
// median/one-vote-per-scaffold mechanics and the Bradley-Terry Elo caveat,
// is kept verbatim; only the grouping changed.
const tuiHelpSectionScoreSourcesBody = `Score sources
swebench: Status and ranking use SWE-bench Verified, in percent. This is the default.
arena: Status and ranking use the LMArena Elo rating, shown raw and normalised to 0-100 before it enters the ranking formula.
- The two are never mixed: in one view a model with no number on the active source shows n/a even when
the other source has one.
- Choose with --score-source or Settings; the switch reads the local snapshot, and the generated
markdown document is always swebench.

SWE-bench Verified: two sources, not two alternatives
SWE-bench Verified itself has two possible sources, and they are fallbacks for each other, not interchangeable measurements.
- vals.ai runs every submitted model itself, on one fixed, independent harness, and its own leaderboard
row echoes back the exact model key it was found by; that echo is what lets this project trust the row's
identity by default (see the Overview section for the identity-gate mechanics).
- swebench.com is different: it is a self-submitted leaderboard where anyone can submit a run with their
own agentic scaffold, so the same model can appear under several different scaffolds with different
scores. To blunt the incentive to game the leaderboard with one aggressive scaffold, this project takes
the median across every distinct scaffold submitted for a model (one vote per scaffold; a resubmission
of the same scaffold replaces it rather than adding a second vote) instead of the single best run, and
the row's own text says "median of N scaffolds" when that happened.
- vals.ai wins whenever it has a usable, identity-checked row for a model; swebench.com is used only as
a fallback, when vals.ai has no row at all or its row fails the identity check.

LMArena Elo: a different scale, not a third alternative
LMArena Elo is not a third way to arrive at the same number.
- It is a crowd preference rating (Bradley-Terry, roughly 950-1550) built from head-to-head human votes
on model output, not a score on a fixed set of real pull requests the way SWE-bench Verified is.
- It is rescaled to 0-100 before it enters the ranking formula, but rescaling does not make it
comparable to a SWE-bench percentage: two models scoring 60 on each are not "equally good" by the same
yardstick; they were measured by two different experiments.
- That is why the app never shows both at once for the same model and never lets a filter mix them.

Switching sources
Space, in the main view or in Settings, changes which single yardstick is active everywhere at once.
- Status, ranking, and quality>= filters all move together, and a model with no number on the newly
active source shows n/a even if it had one on the other.
- Trusting the ranking starts with knowing which of these three measurements produced the number on
screen; this section, and the source column in the model detail view, are how to check.`

// tuiHelpSectionHotkeysBody is the "Hotkeys" section: every
// keybinding table from the pre-sectioning document (Navigation, Data/view,
// Filters/settings, Task-fit codes, Refresh and finish, Help search,
// General/help), relocated verbatim and in their original relative order.
const tuiHelpSectionHotkeysBody = `Hotkeys

Navigation
\tUp\tsettings navigate\tprevious Settings field.
\tDown\tsettings navigate\tnext Settings field.
\tEsc\tsettings close\tclose Settings.
\tUp\tdetail navigate\tprevious detail field.
\tDown\tdetail navigate\tnext detail field.
\tUp\thelp navigate\tscroll help up.
\tDown\thelp navigate\tscroll help down.
\tUp\tcolumns navigate\tprevious column.
\tDown\tcolumns navigate\tnext column.
\tEsc\tcolumns close\tcancel column selection.
\tUp\tfilter navigate\tprevious filter field.
\tDown\tfilter navigate\tnext filter field.
\tEsc\tfilter close\tcancel filter editing.
\tUp\tnavigate\tprevious model; in help, scroll up.
\tDown\tnavigate\tnext model; in help, scroll down.
\tj / k\tnavigate\tmove through models; in help, scroll.
\tHome / g\tjump\tfirst item.
\tEnd / G\tjump\tlast item.
\tPgUp / PgDown\tscroll\tpage through models or help.
\tEnter / Right\tdetail\topen the model detail screen.
\tEsc / Left / h\tclose\tEsc, Left or h closes it and returns to the list.

Data/view
\tq\tsort\tquality.
\tp\tavailability\tcycle any/free/paid.
\tr\tsort\tquality/price ratio (q/p).
\tv\tview\ttoggle all/top-paid-free.
\tm\tranking\ttoggle ranking mode: mixed-utility or tier-priority.
\ts\tordering\tcycle sort key.
\tS\tordering\treverse order.
\to\tsettings\topen settings.
\tDown\tnavigate\tmove to Score source in Settings.
\tSpace\tswitch\t(main) switch between SWE-bench and Arena.
\tSpace\tswitch\t(in Settings) switch between SWE-bench and Arena.
\tR\trefresh\trefresh local data.
\tc\tcolumns\topen selection.
\tn\tview\tswitch the last column between Task fit and Note.

Filters/settings
\tf\tfilter\tedit a structured filter.
\t/\tsearch\tsearch Name/Slug.
\tSpace\tcolumns\ttoggle a column.
\tEnter\tcolumns apply\tapply the column selection.
\tSpace\ttier\tcycle Tier.
\tEnter\tapply\tapply the current editor.
\tEsc\tcancel\tcancel the current editor.

Task-fit codes
\tI\ttask-fit code\timplement: write or change production code.
\tP\ttask-fit code\tplan: define scope, steps, and decisions.
\tR\ttask-fit code\tresearch: investigate options, evidence, or behavior.
\tD\ttask-fit code\tdebug: find and fix a defect or failure.
\tA\ttask-fit code\taudit: inspect quality, safety, or compliance.
\tF\ttask-fit code\trefactor: improve structure without changing behavior.
\tT\ttask-fit code\ttest: add or improve automated verification.
No task-fit classification is shown as n/a.

Refresh and finish
\tR\trefresh\trefresh local data now. Auto-refresh uses --refresh-interval; 0 disables it.
\tx / Ctrl-C\texit\texit the TUI.
\tEsc\tclose\tclose help.
\tEsc\tback\treturn to the list from the current overlay.
\t?\thelp\tclose help.
\tF1\thelp\topen full help.

Help search
\t/\tsearch\tstart a search in this document; type text and press Enter.
\tn\tmatches\tgo to the next match; search results stay selected.
\tN\tmatches\tgo to the previous match; search results stay selected.
\tUp / Down\tscroll\tscroll this help; they do not change the selected match.
\t0 matches\tstatus\tsearches with no matches are reported explicitly.
\tEsc\tclose\tcancel search.

General/help
\tEsc\tmain close\tclose the main view.
\tx / Ctrl-C\texit\texit the TUI.
\t?\thelp\topen help at Hotkeys.
\t?\thelp\tclose help.
\tF1\thelp\topen full help.`

// tuiHelpSectionFiltersBody is the "Filters" section: the structured
// filter syntax block, relocated verbatim out of what used to be the
// single Hotkeys section (its original heading, "Columns, search, and
// filters", stays as this section's own first line).
const tuiHelpSectionFiltersBody = `Columns, search, and filters
\tc\tcolumns\topen selection.
\tSpace\tcolumns\ttoggle a column.
\tEnter\tcolumns\tapply the column selection.
\tEsc\tcolumns\tcancel the column selection.
The last column stays selected.
\t/\tsearch\tsearches Name/Slug as plain substring text.
\tf\tfilter\tedits a structured filter and does not change the search.
	CLI example: omt table --filter 'paid,quality>=80' --filter 'tier:sonnet'.
	TUI example: press f, enable Paid, type sonnet in Tier and 0.8 in Quality minimum, then Enter.
	Filter editor: Up/Down always move between fields, including Tier. Left/Right select Tier or step numeric values; Space cycles Tier. Tab/Shift+Tab also move; typing, Backspace, Enter and c remain available.
	Numeric steps: Quality uses percentage points; Context uses integer token steps; Input and Output use configured absolute cents per $/M. Prices are displayed and serialized with two decimal places, and all draft values are canonicalized on load/apply. Numeric values are never below zero.
	Predicates: paid, free, scored; tier:VALUE; quality>=N; context>=N; input<=N; output<=N.
	Operators: ':' selects a value; '>=' sets a minimum; '<=' sets a maximum.
	Multiple filters are comma-separated (or repeated with CLI --filter) and always use AND.
	quality uses the active score source: SWE-bench is 0..100%; Arena is normalized to 0..100.
	For quality, both 0..100 and 0..1 input are accepted: quality>=0.8 means quality>=80.`

// tuiHelpSectionDetailBody is the "Model Detail" section: the model
// detail screen's own block, relocated verbatim out of what used to be the
// single Hotkeys section.
const tuiHelpSectionDetailBody = `Model detail view
\tEnter or Right\tdetail\tEnter or Right opens the detail screen for the highlighted model.
\tEsc, Left or h\tdetail\tclose it and return to the list with the same cursor.
\tUp/Down or j/k\tscroll\tscroll the detail text; PgUp/PgDown and Home/End also work.
It shows owner, release date, tier, context, full pricing including the long-context tier, both score sources as separate labelled blocks, task fit, note and the vendor description.
The vendor description is wrapped to the terminal width instead of being cut like a table cell.
The screen also links to the model's OpenRouter page and, when the catalogue knows one, to its HuggingFace repository. Links are shown as plain text; there are no clickable terminal hyperlinks.
Field labels, block headings, links and missing values are colour-coded; the colours never change the layout.`

// tuiHelpSectionMethodologyBody is the "Methodology" section: the
// sixth F1 section, a brief but complete synthesis of how every row's data
// and the ranking itself are built, drawing together — not restating
// verbatim — points the Overview and Score Sources sections already make,
// plus the ranking mechanics (the exact mixed-utility formula, the 3:1
// price blend, and the identity states that keep a row out of ranking
// despite carrying a real number). Grounded in model-map.tsv's own header
// comment, internal/model/model.go's classifyIdentity/selectRow doc
// comments and the IdentityExact/IdentityVariantMismatch/IdentityMissing/
// IdentityLegacyUnknown/IdentityObservationOnly constants, and the
// internal/sources package docs (valsai.go, swebench.go, arena.go). The
// same content, with the extra room a web page allows, lives in
// docs/methodology.md in the project repository; this is its
// terminal-friendly cut, not a different text.
const tuiHelpSectionMethodologyBody = `Methodology: how the table and ranking are built
The longer version of this same story, meant for reading outside the terminal, lives in
docs/methodology.md in the project repository. Nothing here is new: it draws together points the
Overview and Score Sources sections already make, plus the ranking mechanics itself.

What feeds every row
Three independent kinds of data feed every row, and none of them is derived from another.
- Price and context come live from the OpenRouter catalogue.
- Quality is an independent benchmark observation, SWE-bench Verified (from vals.ai or swebench.com)
or LMArena Elo, never both at once.
- Tier is a hand-assigned, Claude-relative capability estimate from model-map.tsv, set by a human,
not computed from the score.

Identity gate: model-map.tsv is the only path to a score
A benchmark row is attached to an OpenRouter model only through model-map.tsv, never by matching
names that merely look alike.
- No entry in the map for a source means no automatically collected score from that source, on
purpose.
- The mapped key is itself the identity claim: once the source returns a row for it, the row is
trusted (exact_product) even when its spelling differs from the OpenRouter slug.
- A mapped row that actually measures a different checkpoint or variant is marked !variant on the
source name (for example vals!variant=some/other-checkpoint) and stays out of the ranking
(variant_mismatch) despite the key match; only a human editing the file catches this, never the code.

Three measurements, never mixed
- vals.ai runs every model itself on one fixed, independent harness and echoes back the exact key it
was found by; it wins whenever it has a usable, identity-checked row.
- swebench.com is a self-submitted leaderboard: the median across every distinct scaffold submitted
for a model (one vote per scaffold) is used instead of the single best run; it is only a fallback,
used when vals.ai has no row at all or its row fails identity.
- LMArena Elo is a crowd preference rating (Bradley-Terry, roughly 950-1550), rescaled to 0-100
before entering the ranking formula; it is never shown alongside SWE-bench and never enters the same
column.

How the ranking actually computes
- tier-priority: rankable models first, then Opus, Sonnet, Haiku, score, and Q/P.
- mixed-utility (default): rankable first, then paid utility from the configured safe YAML formula.
Without a formula, utility is score + price_weight*tier_factor*ln(1+quality_price), with
price_weight=10, factors Opus=1, Sonnet=1, Haiku=0.5, Free=0, and price itself a 3:1 input:output
blend: (3*input+output)/4 per $/M tokens. Task-fit is never a multiplier.
- Free rankable models are compared by score alone; quality/price is undefined at $0.

Why a row does not rank
A row can carry a real number and still not rank: variant_mismatch, missing_identity, and
legacy_unknown (an old snapshot saved before identity status existed) are all shown with their
number but excluded, the same as a model with no score source at all, or a manual notes.yaml
observation-only override, which is a vendor claim rather than an independent measurement.`

// stored in a section body, which is exactly what keeps this constant
// entirely English: the content/structural tests that scan tuiHelpDocument
// for stray Cyrillic are checking reference prose, not the tab-bar labels
// the sectioned overlay renders alongside it (the tab-bar labels are
// themselves English titles, see tuiHelpSections, but this constant never
// includes them either way). It exists for those tests — "does the full
// help still document X", the tab-column audits, the English-only checks —
// that were written against one flat document and do not need to change
// just because the F1 overlay now shows its content one section at a time;
// see tuiHelpSectionLines for what the overlay actually renders.
const tuiHelpDocument = tuiHelpTitleLine + "\n\n" +
	tuiHelpSectionOverviewBody + "\n\n" +
	tuiHelpSectionScoreSourcesBody + "\n\n" +
	tuiHelpSectionHotkeysBody + "\n\n" +
	tuiHelpSectionFiltersBody + "\n\n" +
	tuiHelpSectionDetailBody + "\n\n" +
	tuiHelpSectionMethodologyBody

func tuiHelpLines() []string { return strings.Split(tuiHelpDocument, "\n") }

func tuiHelpSearch(needle string) []int {
	return tuiHelpSearchInLines(needle, tuiHelpLines())
}

func tuiHelpSearchInLines(needle string, lines []string) []int {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return nil
	}
	matches := []int{}
	for i, line := range lines {
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
	if m.helpMatch < 0 {
		if direction < 0 {
			m.helpMatch = len(m.helpMatches) - 1
		} else {
			m.helpMatch = 0
		}
	} else {
		m.helpMatch = (m.helpMatch + direction + len(m.helpMatches)) % len(m.helpMatches)
	}
	m.helpOffset = max(0, min(m.helpMaxOffset(), m.helpMatches[m.helpMatch]-max(0, m.height/2)))
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

const (
	// tuiDetailPlaceholder is the detail screen's stand-in for an absent
	// value. It is a constant rather than a literal because the styling
	// pass in tuiStyleDetailLine recognises it by text: a literal repeated
	// in six places and a rule that greys out a seventh copy of it would
	// drift apart the first time one of them is reworded.
	tuiDetailPlaceholder = "n/a"

	// The two link prefixes are constants for the same reason no ready-made
	// URL is stored in model.Model or in the snapshot: the address is
	// entirely derived from an identifier plus a fixed prefix, so one place
	// to change is strictly better than a value duplicated across every
	// model in the snapshot file.
	tuiOpenRouterModelURL  = "https://openrouter.ai/"
	tuiHuggingFaceModelURL = "https://huggingface.co/"
)

// tuiDetailValue is the detail screen's single rule for an absent value:
// the project's н/д placeholder, never a labelled blank.
func tuiDetailValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return tuiDetailPlaceholder
	}
	return plainTableText(value)
}

// tuiDetailURL renders a link line's value: the fixed prefix plus the
// catalogue identifier, or the placeholder when the catalogue gave none.
// The identifier goes through the same sanitisation as every other value
// on this screen, so nothing raw from the network is ever printed.
func tuiDetailURL(prefix, id string) string {
	id = strings.TrimSpace(plainTableText(id))
	if id == "" {
		return tuiDetailPlaceholder
	}
	return prefix + id
}

func tuiDetailLicense(m model.Model) string {
	license := m.License
	if strings.TrimSpace(license) == "" {
		license = m.OpenWeights
	}
	return tuiDetailValue(license)
}

func tuiDetailOpenRouterURL(m model.Model) string {
	id := m.CanonicalSlug
	if strings.TrimSpace(id) == "" {
		id = m.Slug
	}
	return tuiDetailURL(tuiOpenRouterModelURL, id)
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
		return tuiDetailPlaceholder
	}
	return strings.Join(m.TaskFit, " + ")
}

func tuiDetailPriceHistory(history *pricehistory.History, slug string) []string {
	if history == nil || len(history.Observations) < 2 {
		return nil
	}
	lines := make([]string, 0)
	var previous pricehistory.Price
	var havePrevious bool
	for _, observation := range history.Observations {
		current, ok := observation.Prices[slug]
		if !ok || !current.Found || (havePrevious && pricehistory.Equal(previous, current)) {
			continue
		}
		if !havePrevious {
			previous, havePrevious = current, true
			continue
		}
		lines = append(lines, "  "+observation.ObservedAt.UTC().Format("2006-01-02")+": "+pricehistory.Format(previous)+" -> "+pricehistory.Format(current))
		previous = current
	}
	if len(lines) == 0 {
		return nil
	}
	return append([]string{"Динамика цен:"}, lines...)
}

// tuiDetailWrapped renders a free-prose block: sanitised, wrapped to the
// screen width, indented by two columns, and replaced by the placeholder
// when empty. It sanitises with plainDetailText rather than plainTableText
// so that a real paragraph break in the source value survives into
// tuiWrapText, which has its own branch to preserve it.
func tuiDetailWrapped(value string, width int) []string {
	value = strings.TrimSpace(plainDetailText(value))
	if value == "" {
		return []string{"  " + tuiDetailPlaceholder}
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
		return tuiDetailPlaceholder
	}
	published := time.Unix(created, 0).UTC()
	return published.Format("2006-01-02") + " (" + tuiDetailAge(published, now) + ")"
}

func tuiDetailReleaseDate(created int64, now time.Time) string {
	if created <= 0 {
		return tuiDetailPlaceholder
	}
	return tuiDetailCreated(created, now) + "; дата создания записи каталога, релиз неизвестен"
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
		return append(lines, "  "+tuiDetailPlaceholder+" (активно представление arena, число SWE-bench в него не проецируется)")
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
		return append(lines, "  Provenance: "+model.FormatScoreProvenance(nil))
	}
	if info.Stale {
		lines = append(lines, "  Устарело: значение взято из прошлого снапшота")
	}
	return append(lines,
		"  Измеренный вариант: "+tuiDetailValue(model.NormalizeMissingLabels(info.VariantMeasured)),
		"  Метрика: "+tuiDetailValue(model.NormalizeMissingLabels(info.Metric)),
		"  Единица: "+tuiDetailValue(model.NormalizeMissingLabels(info.Unit)),
		"  Согласованность identity: "+tuiDetailValue(model.NormalizeMissingLabels(info.IdentityStatus)),
		"  Источник: "+tuiDetailValue(model.NormalizeMissingLabels(info.SourceURL)),
		"  Проверено: "+tuiDetailValue(model.NormalizeMissingLabels(info.Checked)),
		"  Provenance: "+plainTableText(model.FormatScoreProvenance(info)))
}

// tuiDetailLines builds the detail screen's content for one model: twelve
// labelled blocks ordered from identity to ever finer detail, with the
// vendor description is wrapped near the identity fields so provenance is seen
// before the longer benchmark and pricing sections.
// Wrapping happens here, before any scrolling maths, so detailOffset
// counts the same physical lines the terminal shows. now is a parameter
// rather than a time.Now() call inside so a test can pin the release age.
// scoreSource must be the same source m was projected with by
// model.ForScoreSource; passing a mismatched pair defeats the SWE-bench
// block's gate against printing Arena data under the wrong heading.
func tuiDetailLines(m model.Model, scoreSource string, width int, now time.Time) []string {
	return tuiDetailLinesWithHistory(m, scoreSource, width, now, nil)
}

func tuiDetailLinesWithHistory(m model.Model, scoreSource string, width int, now time.Time, history *pricehistory.History) []string {
	return tuiDetailLinesWithHistoryAndIcons(m, scoreSource, width, now, history, config.DefaultIconConfig())
}

func tuiDetailLinesWithHistoryAndIcons(m model.Model, scoreSource string, width int, now time.Time, history *pricehistory.History, icons config.IconConfig) []string {
	return tuiDetailLinesWithHistoryAndIconsAndGaps(m, scoreSource, width, now, history, icons, int(config.DefaultIconGap), config.DefaultIconGaps())
}

func tuiDetailLinesWithHistoryAndIconsAndGap(m model.Model, scoreSource string, width int, now time.Time, history *pricehistory.History, icons config.IconConfig, iconGap int) []string {
	return tuiDetailLinesWithHistoryAndIconsAndGaps(m, scoreSource, width, now, history, icons, iconGap, nil)
}

func tuiDetailLinesWithHistoryAndIconsAndGaps(m model.Model, scoreSource string, width int, now time.Time, history *pricehistory.History, icons config.IconConfig, iconGap int, iconGaps config.IconGaps) []string {
	context := tuiDetailPlaceholder
	if m.Context > 0 {
		context = pricing.FormatContext(m.Context)
	}
	lines := tuiWrapText(tuiDetailValue(m.DisplayName)+" ("+tuiDetailValue(m.Slug)+")", max(1, width))
	lines = append(lines,
		"",
		"-- Identity --",
	)
	manufacturerLine := "Производитель: " + tuiDetailValue(manufacturerDisplayWithIconsAndGaps(m, icons, iconGaps, iconGap))
	lines = append(lines, tuiWrapWord(manufacturerLine, max(1, width))...)
	lines = append(lines,
		"Провайдер: "+tuiDetailValue(m.Provider),
		"Лицензия: "+tuiDetailLicense(m),
		"Тир: "+tuiDetailValue(m.Tier),
		"Claude-референс: "+tuiDetailValue(m.ClaudeRef),
		"Task fit: "+tuiDetailTaskFit(m),
	)
	lines = append(lines,
		"",
		"-- Pricing --",
		"Контекст: "+context+" токенов",
		"Вход: "+tuiDetailPrice(m.InPerM)+" за M токенов",
		"Выход: "+tuiDetailPrice(m.OutPerM)+" за M токенов",
	)
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
	if historyLines := tuiDetailPriceHistory(history, m.Slug); len(historyLines) > 0 {
		lines = append(lines, historyLines...)
	}
	lines = append(lines, "Открытые веса: "+tuiDetailValue(m.OpenWeights), "")
	lines = append(lines, "-- Benchmarks --")
	lines = append(lines, tuiDetailSWEBenchBlock(m, scoreSource)...)
	lines = append(lines, "")
	lines = append(lines, tuiDetailArenaBlock(m)...)
	lines = append(lines, "", "-- Provenance and metadata --", "Дата релиза: "+tuiDetailReleaseDate(m.Created, now), "Страница OpenRouter: "+tuiDetailOpenRouterURL(m))
	if m.MetadataSourceURL != "" {
		lines = append(lines, "Источник метаданных: "+tuiDetailValue(m.MetadataSourceURL))
	}
	if strings.TrimSpace(m.HuggingFaceID) != "" {
		lines = append(lines, "Репозиторий HuggingFace: "+tuiDetailURL(tuiHuggingFaceModelURL, m.HuggingFaceID))
	}
	lines = append(lines, "Описание:")
	lines = append(lines, tuiDetailWrapped(m.Description, width)...)
	lines = append(lines, "", "-- Fit and notes --", "Заметка:")
	lines = append(lines, tuiDetailWrapped(tableNote(m), width)...)
	return tuiDetailAlignRows(lines, width)
}

// tuiDetailAlignRows keeps the compact label/value form on narrow terminals,
// but turns the same plain lines into a quiet two-column table when there is
// room. Padding is inserted after ": " so existing labels and links remain
// readable, and all of it still happens before ANSI styling.
func tuiDetailAlignRows(lines []string, width int) []string {
	if width < 140 {
		return lines
	}
	labelWidth := 0
	for _, line := range lines {
		index := strings.Index(line, ": ")
		if index > 0 && !strings.HasPrefix(line, "  ") {
			labelWidth = max(labelWidth, tableDisplayWidth(line[:index]))
		}
	}
	if labelWidth == 0 {
		return lines
	}
	result := append([]string(nil), lines...)
	for i, line := range result {
		index := strings.Index(line, ": ")
		if index <= 0 || strings.HasPrefix(line, "  ") {
			continue
		}
		label := line[:index]
		value := line[index+2:]
		result[i] = label + ": " + strings.Repeat(" ", labelWidth-tableDisplayWidth(label)) + value
	}
	return result
}

// tuiDetailBodyHeight is how many content lines fit above the footer
// cluster — a blank separator line plus the position footer, two rows
// always reserved rather than one, which also keeps tuiDetailMaxOffset and
// the rendered slice in agreement. The help overlay used to append its
// footer after already filling the viewport, relying on tuiFullscreenText
// to clip it when there was no room; the detail screen has always reserved
// its own line instead (now two), and the help overlay's own
// helpViewportHeight follows the same reservation discipline.
func tuiDetailBodyHeight(height int) int { return max(1, height-2) }

// tuiDetailMaxOffset is the detail screen's answer to tuiHelpMaxOffset.
// Unlike the help document, this content is not a constant: its length
// depends on the model, on the active score source and on the terminal
// width the description wraps at, so the maximum is computed from the
// lines actually built for this model rather than from a fixed document.
func tuiDetailMaxOffset(m model.Model, scoreSource string, width, height int) int {
	return tuiDetailMaxOffsetWithHistory(m, scoreSource, width, height, nil)
}

func tuiDetailMaxOffsetWithHistory(m model.Model, scoreSource string, width, height int, history *pricehistory.History) int {
	return tuiDetailMaxOffsetWithHistoryAndIcons(m, scoreSource, width, height, history, config.DefaultIconConfig())
}

func tuiDetailMaxOffsetWithHistoryAndIcons(m model.Model, scoreSource string, width, height int, history *pricehistory.History, icons config.IconConfig) int {
	return tuiDetailMaxOffsetWithHistoryAndIconsAndGaps(m, scoreSource, width, height, history, icons, int(config.DefaultIconGap), config.DefaultIconGaps())
}

func tuiDetailMaxOffsetWithHistoryAndIconsAndGap(m model.Model, scoreSource string, width, height int, history *pricehistory.History, icons config.IconConfig, iconGap int) int {
	return tuiDetailMaxOffsetWithHistoryAndIconsAndGaps(m, scoreSource, width, height, history, icons, iconGap, nil)
}

func tuiDetailMaxOffsetWithHistoryAndIconsAndGaps(m model.Model, scoreSource string, width, height int, history *pricehistory.History, icons config.IconConfig, iconGap int, iconGaps config.IconGaps) int {
	return max(0, len(tuiDetailLinesWithHistoryAndIconsAndGaps(m, scoreSource, width, time.Now(), history, icons, iconGap, iconGaps))-tuiDetailBodyHeight(height))
}

func (m *tuiModel) clampDetailOffset() {
	if m.overlay != "detail" {
		return
	}
	row, ok := m.detailRow()
	if !ok {
		m.detailOffset = 0
		return
	}
	m.detailOffset = max(0, min(m.detailOffset, tuiDetailMaxOffsetWithHistoryAndIconsAndGaps(row, m.scoreSource, m.width, m.height, m.priceHistory, m.icons, m.iconGap, m.iconGaps)))
}

func tuiCell(m model.Model, col tuiColumn, note bool, scoreSource string) string {
	return tuiCellWithIcons(m, col, note, scoreSource, config.DefaultIconConfig())
}

func tuiCellWithIcons(m model.Model, col tuiColumn, note bool, scoreSource string, icons config.IconConfig) string {
	return tuiCellWithIconsAndGaps(m, col, note, scoreSource, icons, int(config.DefaultIconGap), config.DefaultIconGaps())
}

func tuiCellWithIconsAndGap(m model.Model, col tuiColumn, note bool, scoreSource string, icons config.IconConfig, iconGap int) string {
	return tuiCellWithIconsAndGaps(m, col, note, scoreSource, icons, iconGap, nil)
}

func tuiCellWithIconsAndGaps(m model.Model, col tuiColumn, note bool, scoreSource string, icons config.IconConfig, iconGap int, iconGaps config.IconGaps) string {
	var value string
	switch col {
	case colName:
		value = modelIdentityWithIconsAndGaps(m, icons, iconGaps, iconGap)
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

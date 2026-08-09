package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/pflag"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
	"golang.org/x/term"
)

const defaultTableWidth = 120
const minTableWidth = 40
const maxTableIdentityWidth = 40

const tableSortHelp = "name, slug, context, input, output, price, quality (Q), q/p (QP), P"

const (
	rankingLegacy  = "legacy"
	rankingTier    = "tier"
	rankingMixed   = "mixed"
	rankingDefault = rankingMixed
)

const (
	scoreSourceSWEBench = model.ScoreSourceSWEBench
	scoreSourceArena    = model.ScoreSourceArena
	scoreSourceDefault  = scoreSourceSWEBench
)

// validateScoreSource rejects anything but the two registered views. There
// is deliberately no "auto": picking a source per row, or blending them, is
// exactly what two separate views exist to prevent — an Elo and a SWE-bench
// percentage are not the same kind of number and must never share a column.
func validateScoreSource(source string) error {
	if source != scoreSourceSWEBench && source != scoreSourceArena {
		return fmt.Errorf("table: invalid --score-source %q; allowed values: swebench, arena", source)
	}
	return nil
}

// scoreSourceLabel is the one-line banner that says which scale the Status
// and Q/P columns are on.
func scoreSourceLabel(source string) string {
	if source == scoreSourceArena {
		return "arena (LMArena Elo; нормализован в 0-100 для ранжирования и для " +
			"показанного Q/P — диапазон зависит от текущего набора моделей)"
	}
	return "swebench (SWE-bench Verified, %)"
}

func normalizeRanking(ranking string) string {
	switch ranking {
	case "tier-priority":
		return rankingTier
	case "mixed-utility":
		return rankingMixed
	default:
		return ranking
	}
}

// loadLocalModelsForSource reads the last run's snapshot and returns the rows
// already projected onto one score source, so no caller can accidentally
// hand a mixed set to the renderer.
func loadLocalModelsForSource(dataDir, source string) ([]model.Model, error) {
	entries, err := modelmap.Load(filepath.Join(dataDir, "model-map.tsv"))
	if err != nil {
		return nil, err
	}
	nt, err := notes.Load(filepath.Join(dataDir, "notes.yaml"))
	if err != nil {
		return nil, err
	}
	snapshotPath := filepath.Join(dataDir, "cache", "last-run-snapshot.json")
	if _, err := os.Stat(snapshotPath); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("table: local snapshot is missing: %s; run refresh first", snapshotPath)
	} else if err != nil {
		return nil, fmt.Errorf("table: inspect local snapshot: %w", err)
	}
	snapshot, err := refresh.LoadSnapshot(snapshotPath)
	if err != nil {
		return nil, err
	}
	prices := make(map[string]sources.PriceInfo, len(snapshot.Models))
	scores := make([]sources.ScoreRow, 0, len(snapshot.Models))
	arena := make([]sources.ScoreRow, 0, len(snapshot.Models))
	for slug, entry := range snapshot.Models {
		prices[slug] = sources.PriceInfo{
			Slug: slug, InPerM: entry.InPerM, OutPerM: entry.OutPerM, Context: entry.Context,
			Free: entry.InPerM == 0 && entry.OutPerM == 0, Found: true,
			HasOverride: entry.HasOverride, OverrideMinTokens: entry.OverrideMinTokens,
			OverrideInPerM: entry.OverrideInPerM, OverrideOutPerM: entry.OverrideOutPerM,
			Created: entry.Created, Description: entry.Description,
		}
		if entry.Score != nil {
			scores = append(scores, sources.ScoreRow{Slug: slug, Metric: entry.Score.Metric, Value: entry.Score.Value, VariantMeasured: entry.Score.VariantMeasured, SourceURL: entry.Score.SourceURL, Checked: entry.Score.Checked})
		}
		if entry.ArenaScore != nil {
			arena = append(arena, sources.ScoreRow{Slug: slug, Metric: entry.ArenaScore.Metric, Value: entry.ArenaScore.Value, VariantMeasured: entry.ArenaScore.VariantMeasured, SourceURL: entry.ArenaScore.SourceURL, Checked: entry.ArenaScore.Checked})
		}
	}
	models := model.MergeWithArena(entries, prices, scores, arena, nt)
	if len(models) == 0 {
		return nil, errors.New("table: local snapshot contains no usable tracked model data")
	}
	return model.ForScoreSource(models, source), nil
}

func loadLocalUpdatedAt(dataDir string) string {
	snapshot, err := refresh.LoadSnapshot(filepath.Join(dataDir, "cache", "last-run-snapshot.json"))
	if err != nil {
		return "unknown"
	}
	if snapshot.UpdatedAt != "" {
		return snapshot.UpdatedAt
	}
	if snapshot.FetchedAt != "" {
		return snapshot.FetchedAt
	}
	return "unknown"
}

func sortTableModels(models []model.Model, key string, reverse bool) error {
	return sortTableModelsWithRankingAndWeight(models, key, reverse, rankingDefault, config.DefaultMixedUtilityPriceWeight)
}

func sortTableModelsWithRanking(models []model.Model, key string, reverse bool, ranking string) error {
	return sortTableModelsWithRankingAndWeight(models, key, reverse, ranking, config.DefaultMixedUtilityPriceWeight)
}

func sortTableModelsWithRankingAndWeight(models []model.Model, key string, reverse bool, rankingName string, priceWeight float64) error {
	c := ranking.DefaultConfig()
	c.PriceWeight = &priceWeight
	compiled, err := ranking.Compile(c)
	if err != nil {
		return err
	}
	return sortTableModelsWithRankingAndConfig(models, key, reverse, rankingName, compiled)
}

func sortTableModelsWithRankingAndConfig(models []model.Model, key string, reverse bool, rankingName string, compiled ranking.Compiled) error {
	rankingName = normalizeRanking(rankingName)
	if rankingName != rankingLegacy && rankingName != rankingTier && rankingName != rankingMixed {
		return fmt.Errorf("table: invalid --ranking %q; allowed values: legacy, tier, tier-priority, mixed, mixed-utility", rankingName)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if alias, ok := map[string]string{"q": "quality", "p": "price", "qp": "q/p"}[key]; ok {
		key = alias
	}
	if key == "" {
		key = "q/p"
	}
	valid := map[string]bool{"name": true, "slug": true, "context": true, "input": true, "output": true, "price": true, "quality": true, "q/p": true}
	if !valid[key] {
		return fmt.Errorf("table: unknown sort %q; allowed values: %s", key, tableSortHelp)
	}
	if rankingName != rankingLegacy && key == "q/p" {
		utilities, err := rankingUtilities(models, compiled)
		if err != nil {
			return err
		}
		sort.SliceStable(models, func(i, j int) bool {
			comparison := compareRanking(models[i], models[j], rankingName, utilities)
			if comparison == 0 {
				return models[i].Slug < models[j].Slug
			}
			if reverse {
				return comparison > 0
			}
			return comparison < 0
		})
		return nil
	}
	sort.SliceStable(models, func(i, j int) bool {
		left, right := models[i], models[j]
		if key == "quality" || key == "q/p" || key == "price" {
			leftValue, leftOK := tableNumericSortValue(left, key)
			rightValue, rightOK := tableNumericSortValue(right, key)
			if leftOK != rightOK {
				return leftOK
			}
			if leftOK && leftValue != rightValue {
				descending := key == "quality" || key == "q/p"
				if reverse {
					descending = !descending
				}
				if descending {
					return leftValue > rightValue
				}
				return leftValue < rightValue
			}
			return left.Slug < right.Slug
		}
		comparison := 0
		switch key {
		case "name":
			comparison = strings.Compare(left.DisplayName, right.DisplayName)
		case "slug":
			comparison = strings.Compare(left.Slug, right.Slug)
		case "context":
			comparison = compareInts(left.Context, right.Context)
		case "input":
			comparison = compareFloats(left.InPerM, right.InPerM)
		case "output":
			comparison = compareFloats(left.OutPerM, right.OutPerM)
		case "price":
			comparison = compareFloats(left.MixedPrice, right.MixedPrice)
		}
		if comparison == 0 {
			return left.Slug < right.Slug
		}
		return (comparison < 0) != reverse
	})
	return nil
}

func rankingUtilities(models []model.Model, compiled ranking.Compiled) (map[string]float64, error) {
	utilities := make(map[string]float64, len(models))
	for _, m := range models {
		if m.Score == nil || !m.Rankable || m.Free {
			continue
		}
		utility, err := mixedUtility(m, compiled)
		if err != nil {
			return nil, fmt.Errorf("table: cannot rank model %q: %w", m.Slug, err)
		}
		utilities[m.Slug] = utility
	}
	return utilities, nil
}

func compareRanking(left, right model.Model, rankingName string, utilities map[string]float64) int {
	leftRankable, rightRankable := left.Score != nil && left.Rankable, right.Score != nil && right.Rankable
	if leftRankable != rightRankable {
		if leftRankable {
			return -1
		}
		return 1
	}
	if !leftRankable {
		return compareFloats(left.MixedPrice, right.MixedPrice)
	}
	if rankingName == rankingTier {
		if leftTier, rightTier := rankingTierValue(left.Tier), rankingTierValue(right.Tier); leftTier != rightTier {
			return compareInts(rightTier, leftTier)
		}
		if left.Score.Value != right.Score.Value {
			return compareFloats(right.Score.Value, left.Score.Value)
		}
		return compareFloats(right.QualityPrice, left.QualityPrice)
	}
	if left.Free || right.Free {
		if left.Free && right.Free {
			return compareFloats(right.Score.Value, left.Score.Value)
		}
		return compareInts(rankingTierValue(right.Tier), rankingTierValue(left.Tier))
	}
	leftUtility := utilities[left.Slug]
	rightUtility := utilities[right.Slug]
	if comparison := compareFloats(rightUtility, leftUtility); comparison != 0 {
		return comparison
	}
	return compareInts(rankingTierValue(right.Tier), rankingTierValue(left.Tier))
}

func mixedUtility(m model.Model, compiled ranking.Compiled) (float64, error) {
	context := compiled.Context(m.Score.Value, m.InPerM, m.OutPerM, m.Tier)
	if m.InPerM == 0 && m.OutPerM == 0 {
		context.QualityPrice = m.QualityPrice
	}
	return compiled.Evaluate(context)
}

func rankingTierValue(tier string) int {
	switch strings.ToLower(tier) {
	case "opus":
		return 3
	case "sonnet":
		return 2
	case "haiku":
		return 1
	case "free":
		return 0
	default:
		return -1
	}
}

func rankingLabel(ranking string) string {
	ranking = normalizeRanking(ranking)
	switch ranking {
	case rankingTier:
		return "tier-priority"
	case rankingMixed:
		return "mixed-utility"
	default:
		return "q/p (legacy)"
	}
}

func parseTableArgs(args []string, flags *pflag.FlagSet) error {
	rewritten := make([]string, 0, len(args)+1)
	for index, arg := range args {
		if len(arg) > 1 && arg[0] == '-' && arg[1] >= '0' && arg[1] <= '9' && (index == 0 || !tableFlagExpectsValue(args[index-1], flags)) {
			value, err := strconv.Atoi(arg[1:])
			if err != nil {
				return fmt.Errorf("table: invalid limit shorthand %q: %w", arg, err)
			}
			rewritten = append(rewritten, "--limit", strconv.Itoa(value))
			continue
		}
		rewritten = append(rewritten, arg)
	}
	if err := flags.Parse(rewritten); err != nil {
		return fmt.Errorf("table: %w", err)
	}
	if len(flags.Args()) > 0 {
		return fmt.Errorf("table: unexpected argument %q", flags.Args()[0])
	}
	return nil
}

func tableFlagExpectsValue(arg string, flags *pflag.FlagSet) bool {
	if strings.Contains(arg, "=") || arg == "-" || arg == "--" {
		return false
	}
	var flag *pflag.Flag
	if strings.HasPrefix(arg, "--") {
		flag = flags.Lookup(strings.TrimPrefix(arg, "--"))
	} else if strings.HasPrefix(arg, "-") && len(arg) == 2 {
		flag = flags.ShorthandLookup(arg[1:])
	}
	return flag != nil && flag.NoOptDefVal == ""
}

func filterTableModels(models []model.Model, filters []string) ([]model.Model, error) {
	parsed := make([]func(model.Model) bool, 0, len(filters))
	for _, raw := range filters {
		filter := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case filter == "paid":
			parsed = append(parsed, func(m model.Model) bool { return !m.Free })
		case filter == "free":
			parsed = append(parsed, func(m model.Model) bool { return m.Free })
		case filter == "scored":
			parsed = append(parsed, func(m model.Model) bool { return m.Score != nil && m.Rankable })
		case strings.HasPrefix(filter, "tier:"):
			tier := strings.TrimSpace(strings.TrimPrefix(filter, "tier:"))
			if tier == "" {
				return nil, fmt.Errorf("table: malformed filter %q; tier must not be empty", raw)
			}
			parsed = append(parsed, func(m model.Model) bool { return strings.EqualFold(m.Tier, tier) })
		case strings.HasPrefix(filter, "quality>="):
			threshold, err := parseFiniteTableThreshold(raw, "quality", strings.TrimSpace(strings.TrimPrefix(filter, "quality>=")))
			if err != nil {
				return nil, err
			}
			parsed = append(parsed, func(m model.Model) bool { return m.Score != nil && m.Rankable && m.Score.Value >= threshold })
		case strings.HasPrefix(filter, "context>="):
			threshold, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(filter, "context>=")))
			if err != nil {
				return nil, fmt.Errorf("table: malformed filter %q; context threshold must be an integer", raw)
			}
			parsed = append(parsed, func(m model.Model) bool { return m.Context >= threshold })
		case strings.HasPrefix(filter, "input<="):
			field, value := "input", strings.TrimSpace(strings.TrimPrefix(filter, "input<="))
			threshold, err := parseFiniteTableThreshold(raw, field, value)
			if err != nil {
				return nil, err
			}
			parsed = append(parsed, func(m model.Model) bool { return m.InPerM <= threshold })
		case strings.HasPrefix(filter, "output<="):
			field, value := "output", strings.TrimSpace(strings.TrimPrefix(filter, "output<="))
			threshold, err := parseFiniteTableThreshold(raw, field, value)
			if err != nil {
				return nil, err
			}
			parsed = append(parsed, func(m model.Model) bool { return m.OutPerM <= threshold })
		default:
			return nil, fmt.Errorf("table: unknown filter %q; allowed values: paid, free, scored, tier:*, quality>=N, context>=N, input<=N, output<=N", raw)
		}
	}
	filtered := make([]model.Model, 0, len(models))
	for _, candidate := range models {
		matches := true
		for _, predicate := range parsed {
			if !predicate(candidate) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}

func parseFiniteTableThreshold(raw, field, value string) (float64, error) {
	threshold, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return 0, fmt.Errorf("table: malformed filter %q; %s threshold must be a finite number", raw, field)
	}
	return threshold, nil
}

func limitTableModels(models []model.Model, limit int) []model.Model {
	if limit <= 0 || limit >= len(models) {
		return models
	}
	return models[:limit]
}

func compareInts(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareFloats(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func tableNumericSortValue(m model.Model, key string) (float64, bool) {
	if key == "price" {
		return m.MixedPrice, true
	}
	if key == "quality" {
		if m.Score == nil || !m.Rankable {
			return 0, false
		}
		return m.Score.Value, true
	}
	if m.Free || m.Score == nil || !m.Rankable {
		return 0, false
	}
	return m.QualityPrice, true
}

func tableWidth() (int, error) {
	width, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || os.Getenv("COLUMNS") == "" {
		return defaultTableWidth, nil
	}
	if width < minTableWidth {
		return 0, fmt.Errorf("table: terminal width %d is too narrow; minimum is %d columns", width, minTableWidth)
	}
	return width, nil
}

func truncateTable(value string, width int) string {
	if width <= 0 || tableDisplayWidth(value) <= width {
		return value
	}
	if width <= tableDisplayWidth("...") {
		return tablePrefix(value, width)
	}
	return tablePrefix(value, width-tableDisplayWidth("...")) + "..."
}

func tableDisplayWidth(value string) int {
	width := 0
	for index := 0; index < len(value); {
		end, clusterWidth := tableCluster(value, index)
		width += clusterWidth
		index = end
	}
	return width
}

func tableCluster(value string, start int) (int, int) {
	index := start
	r, size := utf8.DecodeRuneInString(value[index:])
	clusterWidth := tableRuneWidth(r)
	index += size
	if tableIsRegionalIndicator(r) && index < len(value) {
		next, nextSize := utf8.DecodeRuneInString(value[index:])
		if tableIsRegionalIndicator(next) {
			index += nextSize
			clusterWidth = 2
		}
	}
	for index < len(value) {
		r, size = utf8.DecodeRuneInString(value[index:])
		if tableIsClusterContinuation(r) {
			index += size
			continue
		}
		if r != '\u200d' {
			break
		}
		index += size
		if index >= len(value) {
			break
		}
		_, size = utf8.DecodeRuneInString(value[index:])
		index += size
		for index < len(value) {
			r, size = utf8.DecodeRuneInString(value[index:])
			if !tableIsClusterContinuation(r) {
				break
			}
			index += size
		}
	}
	return index, clusterWidth
}

func tableIsRegionalIndicator(r rune) bool {
	return r >= 0x1f1e6 && r <= 0x1f1ff
}

func tableIsClusterContinuation(r rune) bool {
	return r == '\ufe0e' || r == '\ufe0f' || (r >= 0x1f3fb && r <= 0x1f3ff) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

func tableRuneWidth(r rune) int {
	if tableIsClusterContinuation(r) || r == '\u200d' {
		return 0
	}
	if tableIsRegionalIndicator(r) {
		return 2
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a || (r >= 0x2e80 && r <= 0xa4cf) || (r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0xfe10 && r <= 0xfe19) || (r >= 0xfe30 && r <= 0xfe6f) || (r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6) || (r >= 0x1f300 && r <= 0x1faff)) {
		return 2
	}
	return 1
}

func tablePrefix(value string, width int) string {
	if width <= 0 {
		return ""
	}
	current := 0
	for index := 0; index < len(value); {
		end, clusterWidth := tableCluster(value, index)
		if current+clusterWidth > width {
			return value[:index]
		}
		current += clusterWidth
		index = end
	}
	return value
}

func padTableCell(value string, width int) string {
	padding := width - tableDisplayWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func tableStatus(m model.Model) string {
	status := m.ScoreLabel
	if status == "" {
		status = "No score"
	}
	return plainTableText(status)
}

func tableClaude(m model.Model) string {
	return refresh.ClaudeEquivalent(m)
}

// tableClaudeForSource neutralizes the Claude cell for haiku/free-tier rows
// when the active score source is arena. ClaudeEquivalent's haiku/free
// thresholds (>=70, >=60) are calibrated on SWE-bench Verified percentage
// points; after projection through model.ForScoreSource, an arena-mode
// Score.Value instead holds a min-max-normalized Arena position, so running
// those thresholds on it would silently read an Elo rank as a SWE-bench
// score — exactly the cross-scale blending --score-source exists to prevent.
// There is no established mapping from a normalized Arena position onto a
// Claude tier, so this deliberately does not attempt one, regardless of
// whether the row actually has an Arena number. Opus/sonnet rows are
// unaffected: ClaudeEquivalent derives their label from Tier alone, never
// from a score value, so it stays correct under either source.
func tableClaudeForSource(m model.Model, source string) string {
	if source == scoreSourceArena && (m.Tier == "haiku" || m.Tier == "free") {
		return "н/д"
	}
	return tableClaude(m)
}

func tableNote(m model.Model) string {
	if m.Note == "" || m.Note == notes.NeedsReview {
		return ""
	}
	return plainTableText(m.Note)
}

func plainTableText(value string) string {
	for _, marker := range []string{"**", "__", "`"} {
		value = strings.ReplaceAll(value, marker, "")
	}
	value = strings.ReplaceAll(value, "|", "/")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func renderTable(models []model.Model, width int, showSlug bool) string {
	return renderTableMode(models, width, showSlug, "notes", scoreSourceDefault)
}

func renderTableMode(models []model.Model, width int, showSlug bool, columnMode string, scoreSource string) string {
	identityHeader := "Name"
	if showSlug {
		identityHeader = "Slug"
	}
	columnHeader := "Task fit"
	if columnMode == "notes" {
		columnHeader = "Note"
	}
	headers := []string{identityHeader, "Claude", "Status", "Q/P", "Context", "Input $/M", "Output $/M", columnHeader}
	rows := make([][]string, 0, len(models))
	maxClaudeWidth := 0
	maxNoteWidth := 0
	for _, m := range models {
		identity := m.DisplayName
		if showSlug {
			identity = m.Slug
		}
		last := tableTaskFit(m, columnMode)
		values := []string{identity, tableClaudeForSource(m, scoreSource), tableStatus(m), m.QualityPriceLabel, pricing.FormatContext(m.Context), pricing.FormatPrice(m.InPerM), pricing.FormatPrice(m.OutPerM), last}
		for i := range values {
			values[i] = plainTableText(values[i])
		}
		rows = append(rows, values)
		maxClaudeWidth = max(maxClaudeWidth, tableDisplayWidth(values[1]))
		maxNoteWidth = max(maxNoteWidth, tableDisplayWidth(values[7]))
	}
	preferred := []int{maxTableIdentityWidth, max(tableDisplayWidth(headers[1]), maxClaudeWidth), 8, 5, 8, 13, 13, max(tableDisplayWidth(headers[7]), maxNoteWidth)}
	minimum := []int{30, max(tableDisplayWidth(headers[1]), maxClaudeWidth), 6, 3, 7, 9, 10, max(tableDisplayWidth(headers[7]), maxNoteWidth)}
	// Claude keeps its full width; structural columns and the selected last column use compact fallback.
	compactMinimum := []int{4, max(1, maxClaudeWidth), 1, 3, 1, 1, 1, max(1, maxNoteWidth)}
	widths := append([]int(nil), preferred...)
	target := width - (3*len(widths) + 1)
	if width <= minTableWidth {
		minimum = compactMinimum
	}
	if target < sum(widths) {
		deficit := sum(widths) - target
		for _, index := range []int{0, 1, 4, 5, 6, 7, 3, 2} {
			shrink := min(widths[index]-minimum[index], deficit)
			widths[index] -= shrink
			deficit -= shrink
			if deficit == 0 {
				break
			}
		}
	} else if target > sum(widths) {
		widths[4] += target - sum(widths)
	}
	var b strings.Builder
	separator := func() {
		b.WriteString("+")
		for _, columnWidth := range widths {
			b.WriteString(strings.Repeat("-", columnWidth+2))
			b.WriteString("+")
		}
		b.WriteByte('\n')
	}
	row := func(values []string) {
		b.WriteString("|")
		for i, value := range values {
			if i == 0 {
				value = truncateTable(value, min(widths[i], maxTableIdentityWidth))
			} else {
				value = truncateTable(value, widths[i])
			}
			b.WriteString(" " + padTableCell(value, widths[i]) + " |")
		}
		b.WriteByte('\n')
	}
	separator()
	row(headers)
	separator()
	for _, values := range rows {
		row(values)
	}
	separator()
	return b.String()
}

func tableTaskFit(m model.Model, mode string) string {
	if mode == "notes" {
		return tableNote(m)
	}
	if len(m.TaskFit) == 0 {
		return "n/a"
	}
	if mode == "long" {
		return strings.Join(m.TaskFit, " + ")
	}
	short := make([]string, 0, len(m.TaskFit))
	tokens := map[string]string{"implement": "I", "plan": "P", "research": "R", "debug": "D", "audit": "A", "refactor": "F", "test": "T"}
	for _, keyword := range m.TaskFit {
		short = append(short, tokens[keyword])
	}
	return strings.Join(short, "")
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var tableIsTTY = func(stdout io.Writer) bool {
	file, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func tableShouldPage(stdout io.Writer, noPager bool) bool {
	return !noPager && tableIsTTY(stdout)
}

var runTablePager = func(output string, stdout, stderr io.Writer) error {
	pager := exec.Command("less", "-S")
	pager.Stdin = strings.NewReader(output)
	pager.Stdout = stdout
	pager.Stderr = stderr
	if err := pager.Run(); err != nil {
		return fmt.Errorf("table: run less -S: %w", err)
	}
	return nil
}

func writeTableOutput(output string, stdout, stderr io.Writer, shouldPage bool) error {
	if shouldPage {
		return runTablePager(output, stdout, stderr)
	}
	_, err := io.WriteString(stdout, output)
	return err
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

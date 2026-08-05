package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

const defaultTableWidth = 120
const minTableWidth = 40

func loadLocalModels(dataDir string) ([]model.Model, error) {
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
	for slug, entry := range snapshot.Models {
		prices[slug] = sources.PriceInfo{
			Slug: slug, InPerM: entry.InPerM, OutPerM: entry.OutPerM, Context: entry.Context,
			Free: entry.InPerM == 0 && entry.OutPerM == 0, Found: true,
			HasOverride: entry.HasOverride, OverrideMinTokens: entry.OverrideMinTokens,
			OverrideInPerM: entry.OverrideInPerM, OverrideOutPerM: entry.OverrideOutPerM,
		}
		if entry.Score != nil {
			scores = append(scores, sources.ScoreRow{Slug: slug, Metric: entry.Score.Metric, Value: entry.Score.Value, VariantMeasured: entry.Score.VariantMeasured, SourceURL: entry.Score.SourceURL, Checked: entry.Score.Checked})
		}
	}
	models := model.Merge(entries, prices, scores, nt)
	if len(models) == 0 {
		return nil, errors.New("table: local snapshot contains no usable tracked model data")
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Slug < models[j].Slug })
	return models, nil
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

func tableIsClusterContinuation(r rune) bool {
	return r == '\ufe0e' || r == '\ufe0f' || (r >= 0x1f3fb && r <= 0x1f3ff) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

func tableRuneWidth(r rune) int {
	if tableIsClusterContinuation(r) || r == '\u200d' {
		return 0
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
	if m.Note != "" && m.Note != notes.NeedsReview {
		status += "; " + m.Note
	}
	return plainTableText(status)
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

func renderTable(models []model.Model, width int) string {
	preferred := []int{30, 7, 12, 13, 20}
	minimum := []int{6, 3, 5, 6, 3}
	widths := append([]int(nil), preferred...)
	target := width - 16
	if target < minTableWidth-16 {
		target = minTableWidth - 16
	}
	if target < sum(widths) {
		deficit := sum(widths) - target
		for _, i := range []int{0, 4, 3, 2, 1} {
			shrink := widths[i] - minimum[i]
			if shrink > deficit {
				shrink = deficit
			}
			widths[i] -= shrink
			deficit -= shrink
			if deficit == 0 {
				break
			}
		}
	} else {
		widths[4] += target - sum(widths)
	}
	headers := []string{"Model", "Context", "Input $/M", "Output $/M", "Status/Note"}
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
			value = plainTableText(value)
			value = truncateTable(value, widths[i])
			b.WriteString(" " + padTableCell(value, widths[i]) + " |")
		}
		b.WriteByte('\n')
	}
	separator()
	row(headers)
	separator()
	for _, m := range models {
		row([]string{m.DisplayName, pricing.FormatContext(m.Context), pricing.FormatPrice(m.InPerM), pricing.FormatPrice(m.OutPerM), tableStatus(m)})
	}
	separator()
	return b.String()
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

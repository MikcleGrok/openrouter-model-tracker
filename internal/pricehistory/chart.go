package pricehistory

import (
	"fmt"
	"strings"
	"time"
)

const (
	chartRows = 6
	// DefaultChartWidth is the plotted column count RenderChart falls back
	// to when the caller passes width <= 0 — e.g. when stdout isn't a TTY
	// and there is no real terminal width to size against.
	DefaultChartWidth = 60
	chartMinCols      = 8
)

// chartBlocks indexes by eighths filled: 0 is blank, 8 is a full block.
var chartBlocks = []rune(" ▁▂▃▄▅▆▇█")

// RenderChart draws a labeled terminal bar chart for one named value
// series: values[i] is the value observed on days[i], with nil marking a
// gap (no valid observation that day) rendered as a blank column rather
// than interpolated across it. Every row of the Y-axis carries an actual
// dollar value and the X-axis carries a handful of real dates, so the
// result is never an unlabeled wall of bar characters.
//
// width caps the number of plotted columns; a longer series is
// downsampled by keeping the last non-gap value in each bucket — "the
// price as of that day" is already what a step series like this means, so
// carrying the latest value forward is the right reduction, not an
// average.
func RenderChart(label string, days []time.Time, values []*float64, width int) string {
	if len(values) == 0 || len(values) != len(days) {
		return label + ": n/a\n"
	}
	// min/max (and the flat-series check) are computed from the FULL
	// series, before any resampling — resampleColumns only keeps one
	// value per bucket, so deriving the range from its output could both
	// miscount the real number of observed days and, for a downsampled
	// series, drop the true extreme if it fell on a bucket's discarded
	// point.
	minValue, maxValue, ok := seriesRange(values)
	if !ok {
		return label + ": n/a\n"
	}
	if minValue == maxValue {
		return fmt.Sprintf("%s: constant at %s across %d observed day(s)\n", label, formatDollarG(minValue), countPoints(values))
	}
	if width <= 0 {
		width = DefaultChartWidth
	}
	if width < chartMinCols {
		width = chartMinCols
	}
	plotDays, plotValues := resampleColumns(days, values, width)
	cols := len(plotValues)
	rowHeight := (maxValue - minValue) / float64(chartRows)
	grid := make([][]rune, chartRows)
	for r := range grid {
		grid[r] = make([]rune, cols)
	}
	for col, v := range plotValues {
		if v == nil {
			for r := 0; r < chartRows; r++ {
				grid[r][col] = ' '
			}
			continue
		}
		units := (*v - minValue) / rowHeight
		if units < 0 {
			units = 0
		}
		if units > float64(chartRows) {
			units = float64(chartRows)
		}
		for r := 0; r < chartRows; r++ {
			bottomIndex := chartRows - 1 - r
			fill := units - float64(bottomIndex)
			switch {
			case fill <= 0:
				grid[r][col] = ' '
			case fill >= 1:
				grid[r][col] = chartBlocks[8]
			default:
				idx := int(fill*8 + 0.5)
				if idx < 1 {
					idx = 1
				}
				if idx > 8 {
					idx = 8
				}
				grid[r][col] = chartBlocks[idx]
			}
		}
	}
	// labels[r] is the price at row r's top edge, for r in [0, chartRows];
	// labels[chartRows] is therefore the baseline value (== minValue).
	labels := make([]string, chartRows+1)
	labelWidth := 0
	for r := 0; r <= chartRows; r++ {
		value := maxValue - float64(r)*rowHeight
		labels[r] = formatDollarG(value)
		labelWidth = max(labelWidth, len(labels[r]))
	}
	var b strings.Builder
	b.WriteString(label)
	b.WriteByte('\n')
	for r := 0; r < chartRows; r++ {
		b.WriteString(padLeft(labels[r], labelWidth))
		b.WriteString(" ┤")
		b.WriteString(string(grid[r]))
		b.WriteByte('\n')
	}
	b.WriteString(padLeft(labels[chartRows], labelWidth))
	b.WriteString(" └")
	b.WriteString(strings.Repeat("─", cols))
	b.WriteByte('\n')
	b.WriteString(strings.Repeat(" ", labelWidth+2))
	b.WriteString(xAxisLabels(plotDays, cols))
	b.WriteByte('\n')
	return b.String()
}

// resampleColumns maps values/days onto exactly width plotted columns,
// keeping the last non-gap value (and its day) that falls in each column's
// source range. With fewer points than width this stretches each point
// into a wider bar — giving a short series room to actually show its
// per-run X-axis date labels instead of collapsing into a
// few-characters-wide sliver — and with more points than width it
// downsamples the usual way, keeping the latest ("as of that day") value
// per bucket rather than averaging it away.
func resampleColumns(days []time.Time, values []*float64, width int) ([]time.Time, []*float64) {
	n := len(values)
	if n == 0 || width <= 0 {
		return nil, nil
	}
	outDays := make([]time.Time, width)
	outValues := make([]*float64, width)
	for col := 0; col < width; col++ {
		start := col * n / width
		end := (col + 1) * n / width
		if end <= start {
			end = start + 1
		}
		if end > n {
			end = n
		}
		outDays[col] = days[end-1]
		for i := end - 1; i >= start; i-- {
			if values[i] != nil {
				outValues[col] = values[i]
				break
			}
		}
	}
	return outDays, outValues
}

func seriesRange(values []*float64) (minValue, maxValue float64, ok bool) {
	for _, v := range values {
		if v == nil {
			continue
		}
		if !ok {
			minValue, maxValue, ok = *v, *v, true
			continue
		}
		if *v < minValue {
			minValue = *v
		}
		if *v > maxValue {
			maxValue = *v
		}
	}
	return minValue, maxValue, ok
}

func countPoints(values []*float64) int {
	n := 0
	for _, v := range values {
		if v != nil {
			n++
		}
	}
	return n
}

// xAxisLabels lays out a handful of non-overlapping "YYYY-MM-DD" date
// labels under a cols-wide chart, evenly spaced across the plotted days —
// never one label per column (dates are far wider than one character).
func xAxisLabels(days []time.Time, cols int) string {
	if cols == 0 || len(days) == 0 {
		return ""
	}
	line := []rune(strings.Repeat(" ", cols))
	const dateWidth = len("2006-01-02")
	numLabels := cols / (dateWidth + 2)
	if numLabels < 1 {
		numLabels = 1
	}
	if numLabels > len(days) {
		numLabels = len(days)
	}
	denominator := numLabels - 1
	if denominator < 1 {
		denominator = 1
	}
	lastEnd := -1
	for i := 0; i < numLabels; i++ {
		pos := 0
		if numLabels > 1 {
			pos = i * (len(days) - 1) / denominator
		}
		text := []rune(days[pos].Format("2006-01-02"))
		start := pos
		if start+len(text) > cols {
			start = cols - len(text)
		}
		if start < 0 {
			start = 0
		}
		if start <= lastEnd {
			continue
		}
		for j, r := range text {
			if start+j >= cols {
				break
			}
			line[start+j] = r
		}
		lastEnd = start + len(text)
	}
	return string(line)
}

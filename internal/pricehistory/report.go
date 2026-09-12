package pricehistory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
)

// PriceRun is one contiguous stretch of a slug's observations that all
// carried the same Price, collapsed from the raw (often many-per-day)
// observation log. It is the building block for a deduplicated,
// human-readable price-history table: a long stable period becomes one row
// spanning [From, To] instead of one repeated row per daily observation.
type PriceRun struct {
	Price Price
	From  time.Time
	To    time.Time
	// Days is the number of distinct calendar days (UTC) this run was
	// observed on — not a raw observation count, since a single day can
	// carry several observations (frequent refreshes) that all collapse
	// into the same run.
	Days int
}

// Runs collapses h's observations for slug into deduplicated PriceRun
// entries, in chronological order. An observation where slug was not found
// is a gap: it neither breaks the current run nor starts a new one,
// matching the "not found" skip already used by the recorded-change lines
// in cmd/openrouter/tui.go's tuiDetailPriceHistoryLines.
func Runs(h *History, slug string) []PriceRun {
	if h == nil {
		return nil
	}
	type building struct {
		run  PriceRun
		seen map[string]bool
	}
	var runs []building
	for _, observation := range h.Observations {
		price, ok := observation.Prices[slug]
		if !ok || !price.Found {
			continue
		}
		when := observation.ObservedAt.UTC()
		dayKey := when.Format("2006-01-02")
		if n := len(runs); n > 0 && Equal(runs[n-1].run.Price, price) {
			current := &runs[n-1]
			current.run.To = when
			if !current.seen[dayKey] {
				current.seen[dayKey] = true
				current.run.Days++
			}
			continue
		}
		runs = append(runs, building{run: PriceRun{Price: price, From: when, To: when, Days: 1}, seen: map[string]bool{dayKey: true}})
	}
	out := make([]PriceRun, len(runs))
	for i, r := range runs {
		out[i] = r.run
	}
	return out
}

// FormatRunsTable renders runs as a column-aligned table: one row per
// distinct price, with the calendar-date range it held and how many
// distinct days it was observed on. A single-day run shows just that one
// date; a run spanning several days shows "from → to". This is the
// deduplicated replacement for dumping one line per daily observation
// regardless of whether the price actually changed.
func FormatRunsTable(runs []PriceRun) string {
	if len(runs) == 0 {
		return "no price history\n"
	}
	type row struct{ dateRange, input, output, context, days, note string }
	rows := make([]row, len(runs))
	for i, r := range runs {
		// Compare calendar dates, not exact instants: a run can carry
		// several same-day observations before the next real change, so
		// From and To are rarely bit-identical even within one calendar
		// day — comparing the formatted dates is what "single day vs.
		// range" actually means to a reader.
		fromDate, toDate := r.From.Format("2006-01-02"), r.To.Format("2006-01-02")
		dateRange := fromDate
		if fromDate != toDate {
			dateRange = fromDate + " → " + toDate
		}
		note := ""
		if r.Price.HasOverride {
			note = fmt.Sprintf("long-context %s/%s from %s+", formatDollarG(r.Price.OverrideInPerM), formatDollarG(r.Price.OverrideOutPerM), pricing.FormatContext(r.Price.OverrideMinTokens))
		}
		rows[i] = row{
			dateRange: dateRange,
			input:     formatDollarG(r.Price.InPerM),
			output:    formatDollarG(r.Price.OutPerM),
			context:   pricing.FormatContext(r.Price.Context),
			days:      strconv.Itoa(r.Days),
			note:      note,
		}
	}
	headers := row{dateRange: "Date range", input: "Input $/M", output: "Output $/M", context: "Context", days: "Days", note: "Notes"}
	widths := [5]int{displayWidth(headers.dateRange), displayWidth(headers.input), displayWidth(headers.output), displayWidth(headers.context), displayWidth(headers.days)}
	hasNotes := false
	for _, r := range rows {
		widths[0] = max(widths[0], displayWidth(r.dateRange))
		widths[1] = max(widths[1], displayWidth(r.input))
		widths[2] = max(widths[2], displayWidth(r.output))
		widths[3] = max(widths[3], displayWidth(r.context))
		widths[4] = max(widths[4], displayWidth(r.days))
		if r.note != "" {
			hasNotes = true
		}
	}
	var b strings.Builder
	writeRow := func(r row) {
		b.WriteString(padRight(r.dateRange, widths[0]))
		b.WriteString("  ")
		b.WriteString(padLeft(r.input, widths[1]))
		b.WriteString("  ")
		b.WriteString(padLeft(r.output, widths[2]))
		b.WriteString("  ")
		b.WriteString(padLeft(r.context, widths[3]))
		b.WriteString("  ")
		b.WriteString(padLeft(r.days, widths[4]))
		if hasNotes && r.note != "" {
			b.WriteString("  ")
			b.WriteString(r.note)
		}
		b.WriteByte('\n')
	}
	writeRow(headers)
	total := widths[0] + widths[1] + widths[2] + widths[3] + widths[4] + 8
	b.WriteString(strings.Repeat("-", total))
	b.WriteByte('\n')
	for _, r := range rows {
		writeRow(r)
	}
	return b.String()
}

// formatDollarG formats a $/M price the same way pricehistory.Format
// already renders the compact change-log style ("$3/$15, 1049K"): up to 4
// significant digits, trimmed. This project's real price data carries more
// than 2 decimal digits (e.g. 1.0423 vs 1.0393) — a fixed 2-decimal format
// would round genuinely different prices to identical-looking labels,
// which is exactly wrong for a table/chart whose entire point is showing
// where the price actually changed.
func formatDollarG(v float64) string {
	return fmt.Sprintf("$%.4g", v)
}

// displayWidth counts runes, not bytes — every string this package pads or
// measures a column by is plain ASCII except the "→" range separator
// (U+2192, a single-width rune but 3 UTF-8 bytes). Using len() here padded
// a "from → to" row 2 columns short of every plain single-date row,
// visibly misaligning the Input/Output/Context/Days columns whenever a
// table mixed range and single-date rows. Content here never includes a
// double-width grapheme (CJK, emoji, ZWJ sequences) the way a model's
// display name can elsewhere in this project, so a straight rune count is
// enough — no need for cmd/openrouter/table.go's full grapheme-cluster
// width oracle (which this package cannot import without a cycle anyway).
func displayWidth(s string) int { return utf8.RuneCountInString(s) }

func padRight(s string, width int) string {
	if pad := width - displayWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func padLeft(s string, width int) string {
	if pad := width - displayWidth(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DailySeries collapses h's observations into one point per calendar day
// (UTC): input[i]/output[i] hold slug's last *valid* (found) price observed
// on days[i], or nil when slug had no valid observation that day (a gap,
// rendered as a break in the chart rather than silently interpolated).
// Days come from every observation in h, not only the ones where slug was
// found — a day where the model was checked but came back missing is a
// real gap, not a day that never happened.
func DailySeries(h *History, slug string) (days []time.Time, input, output []*float64) {
	if h == nil {
		return nil, nil, nil
	}
	order := make([]string, 0, len(h.Observations))
	values := make(map[string]*Price, len(h.Observations))
	for _, observation := range h.Observations {
		when := observation.ObservedAt.UTC()
		key := when.Format("2006-01-02")
		if _, seen := values[key]; !seen {
			values[key] = nil
			order = append(order, key)
		}
		if price, ok := observation.Prices[slug]; ok && price.Found {
			p := price
			values[key] = &p
		}
	}
	days = make([]time.Time, len(order))
	input = make([]*float64, len(order))
	output = make([]*float64, len(order))
	for i, key := range order {
		day, _ := time.Parse("2006-01-02", key)
		days[i] = day
		if p := values[key]; p != nil {
			in, out := p.InPerM, p.OutPerM
			input[i] = &in
			output[i] = &out
		}
	}
	return days, input, output
}

// FilterSince returns a copy of h containing only observations at or after
// cutoff. A zero cutoff returns h unchanged.
func FilterSince(h *History, cutoff time.Time) *History {
	if h == nil || cutoff.IsZero() {
		return h
	}
	filtered := &History{SchemaVersion: h.SchemaVersion}
	for _, observation := range h.Observations {
		if !observation.ObservedAt.Before(cutoff) {
			filtered.Observations = append(filtered.Observations, observation)
		}
	}
	return filtered
}

// RenderModelReport builds the full human-readable price-history report for
// one model: a deduplicated table of distinct price runs (see Runs and
// FormatRunsTable), followed by a labeled bar chart for each of the input
// and output price series (see RenderChart). width caps the charts'
// plotted columns; 0 uses the package default.
func RenderModelReport(h *History, slug string, width int) string {
	runs := Runs(h, slug)
	if len(runs) == 0 {
		return fmt.Sprintf("No price history for %s.\n", slug)
	}
	days, input, output := DailySeries(h, slug)
	var b strings.Builder
	fmt.Fprintf(&b, "Price history for %s\n\n", slug)
	b.WriteString(FormatRunsTable(runs))
	b.WriteByte('\n')
	b.WriteString(RenderChart("Input $/M", days, input, width))
	b.WriteByte('\n')
	b.WriteString(RenderChart("Output $/M", days, output, width))
	return b.String()
}

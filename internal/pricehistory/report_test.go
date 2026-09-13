package pricehistory

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func day(t *testing.T, y int, m time.Month, d int) time.Time {
	t.Helper()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TestRunsCollapsesConsecutiveIdenticalPrices is the direct regression test
// for the bug report: a long run of daily observations at the same price
// must collapse into a single PriceRun spanning the whole date range,
// instead of one entry per day.
func TestRunsCollapsesConsecutiveIdenticalPrices(t *testing.T) {
	const slug = "a/model"
	h := &History{Observations: []Observation{
		{ObservedAt: day(t, 2026, 9, 3), Prices: map[string]Price{slug: {Found: true, InPerM: 3, OutPerM: 15}}},
		{ObservedAt: day(t, 2026, 9, 4), Prices: map[string]Price{slug: {Found: true, InPerM: 3, OutPerM: 15}}},
		{ObservedAt: day(t, 2026, 9, 5), Prices: map[string]Price{slug: {Found: true, InPerM: 3, OutPerM: 15}}},
		{ObservedAt: day(t, 2026, 9, 6), Prices: map[string]Price{slug: {Found: true, InPerM: 2.5, OutPerM: 12.5}}},
	}}
	runs := Runs(h, slug)
	if len(runs) != 2 {
		t.Fatalf("runs = %#v, want 2", runs)
	}
	if !runs[0].From.Equal(day(t, 2026, 9, 3)) || !runs[0].To.Equal(day(t, 2026, 9, 5)) || runs[0].Days != 3 {
		t.Errorf("run 0 = %+v, want From=09-03 To=09-05 Days=3", runs[0])
	}
	if !runs[1].From.Equal(day(t, 2026, 9, 6)) || !runs[1].To.Equal(day(t, 2026, 9, 6)) || runs[1].Days != 1 {
		t.Errorf("run 1 = %+v, want From=To=09-06 Days=1", runs[1])
	}
}

// TestRunsSkipsGapsWithoutBreakingTheRun mirrors the existing "not found"
// skip semantics of tuiDetailPriceHistoryLines (cmd/openrouter/tui.go): a
// day where the model was checked but came back missing does not start a
// new run, and does not count toward Days either.
func TestRunsSkipsGapsWithoutBreakingTheRun(t *testing.T) {
	const slug = "a/model"
	h := &History{Observations: []Observation{
		{ObservedAt: day(t, 2026, 9, 3), Prices: map[string]Price{slug: {Found: true, InPerM: 1, OutPerM: 2}}},
		{ObservedAt: day(t, 2026, 9, 4), Prices: map[string]Price{slug: {Found: false}}},
		{ObservedAt: day(t, 2026, 9, 5), Prices: map[string]Price{slug: {Found: true, InPerM: 1, OutPerM: 2}}},
	}}
	runs := Runs(h, slug)
	if len(runs) != 1 {
		t.Fatalf("runs = %#v, want 1 (gap must not split the run)", runs)
	}
	if !runs[0].From.Equal(day(t, 2026, 9, 3)) || !runs[0].To.Equal(day(t, 2026, 9, 5)) || runs[0].Days != 2 {
		t.Errorf("run = %+v, want From=09-03 To=09-05 Days=2 (the gap day doesn't count)", runs[0])
	}
}

// TestRunsSameDayTwoChanges is the other half of the bug report: two price
// changes landing on the same calendar day must produce two distinct rows
// dated the same day, not get merged or dropped.
func TestRunsSameDayTwoChanges(t *testing.T) {
	const slug = "a/model"
	h := &History{Observations: []Observation{
		{ObservedAt: time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC), Prices: map[string]Price{slug: {Found: true, InPerM: 2.1, OutPerM: 10.53}}},
		{ObservedAt: time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC), Prices: map[string]Price{slug: {Found: true, InPerM: 2.34, OutPerM: 11.7}}},
	}}
	runs := Runs(h, slug)
	if len(runs) != 2 {
		t.Fatalf("runs = %#v, want 2", runs)
	}
	for i, r := range runs {
		// From/To keep the exact observation instant (needed so a later
		// run's From is genuinely after this run's To); only the calendar
		// date — not the time of day — has to match "09-11" here.
		if r.From.Format("2006-01-02") != "2026-09-11" || r.To.Format("2006-01-02") != "2026-09-11" || r.Days != 1 {
			t.Errorf("run %d = %+v, want a single 09-11 day", i, r)
		}
	}
	if runs[0].Price.InPerM != 2.1 || runs[1].Price.InPerM != 2.34 {
		t.Errorf("runs out of order or wrong prices: %#v", runs)
	}
}

func TestFormatRunsTableCollapsesRangeAndKeepsSingleDayRows(t *testing.T) {
	runs := []PriceRun{
		{Price: Price{Found: true, InPerM: 3, OutPerM: 15, Context: 1048576}, From: day(t, 2026, 9, 3), To: day(t, 2026, 9, 10), Days: 7},
		{Price: Price{Found: true, InPerM: 2.1, OutPerM: 10.53, Context: 1048576}, From: day(t, 2026, 9, 11), To: day(t, 2026, 9, 11), Days: 1},
	}
	table := FormatRunsTable(runs)
	if !strings.Contains(table, "2026-09-03 → 2026-09-10") {
		t.Errorf("table is missing the collapsed range row:\n%s", table)
	}
	if strings.Contains(table, "2026-09-04") || strings.Contains(table, "2026-09-05") {
		t.Errorf("table spelled out individual days inside a collapsed range:\n%s", table)
	}
	if !strings.Contains(table, "2026-09-11") || strings.Contains(table, "2026-09-11 →") {
		t.Errorf("single-day run should show one bare date, not a range:\n%s", table)
	}
	if !strings.Contains(table, "$3") || !strings.Contains(table, "$15") || !strings.Contains(table, "$2.1") || !strings.Contains(table, "$10.53") {
		t.Errorf("table is missing formatted dollar prices:\n%s", table)
	}
	if !strings.Contains(table, "Date range") || !strings.Contains(table, "Days") {
		t.Errorf("table is missing its header row:\n%s", table)
	}
	if !strings.Contains(table, "7") {
		t.Errorf("table is missing the 7-day count for the collapsed run:\n%s", table)
	}
	if strings.Contains(table, "Slug") {
		t.Errorf("FormatRunsTable must not render a Slug column:\n%s", table)
	}
}

func TestFormatRunsTableNotesLongContextOverrideOnlyWhenPresent(t *testing.T) {
	runs := []PriceRun{
		{Price: Price{Found: true, InPerM: 1, OutPerM: 2, Context: 1000, HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 1.5, OverrideOutPerM: 4}, From: day(t, 2026, 8, 1), To: day(t, 2026, 8, 1), Days: 1},
	}
	table := FormatRunsTable(runs)
	if !strings.Contains(table, "long-context $1.5/$4 from 500K+") {
		t.Errorf("table is missing the long-context note:\n%s", table)
	}
}

func TestFormatRunsTableEmpty(t *testing.T) {
	if got := FormatRunsTable(nil); got != "no price history\n" {
		t.Errorf("FormatRunsTable(nil) = %q", got)
	}
}

func TestDailySeriesLastValidValuePerDayAndGaps(t *testing.T) {
	const slug = "a/model"
	h := &History{Observations: []Observation{
		{ObservedAt: time.Date(2026, 9, 3, 6, 0, 0, 0, time.UTC), Prices: map[string]Price{slug: {Found: true, InPerM: 1, OutPerM: 2}}},
		{ObservedAt: time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC), Prices: map[string]Price{slug: {Found: true, InPerM: 1.5, OutPerM: 3}}},
		{ObservedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), Prices: map[string]Price{slug: {Found: false}}},
		{ObservedAt: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), Prices: map[string]Price{slug: {Found: true, InPerM: 2, OutPerM: 4}}},
	}}
	days, input, output := DailySeries(h, slug)
	if len(days) != 3 {
		t.Fatalf("days = %v, want 3 distinct calendar days", days)
	}
	if input[0] == nil || *input[0] != 1.5 {
		t.Errorf("day 0 should carry the day's LAST valid observation (1.5), got %v", input[0])
	}
	if input[1] != nil || output[1] != nil {
		t.Errorf("day 1 (not found) should be a gap, got input=%v output=%v", input[1], output[1])
	}
	if input[2] == nil || *input[2] != 2 {
		t.Errorf("day 2 = %v, want 2", input[2])
	}
}

func TestRenderModelReportNoHistory(t *testing.T) {
	got := RenderModelReport(&History{}, "a/model", 0)
	if !strings.Contains(got, "No price history for a/model") {
		t.Errorf("RenderModelReport empty = %q", got)
	}
}

func TestFilterSinceKeepsOnlyObservationsAtOrAfterCutoff(t *testing.T) {
	h := &History{Observations: []Observation{
		{ObservedAt: day(t, 2026, 9, 1)},
		{ObservedAt: day(t, 2026, 9, 2)},
		{ObservedAt: day(t, 2026, 9, 3)},
	}}
	filtered := FilterSince(h, day(t, 2026, 9, 2))
	if len(filtered.Observations) != 2 {
		t.Fatalf("filtered = %+v, want 2 observations", filtered.Observations)
	}
	if FilterSince(h, time.Time{}) != h {
		t.Errorf("a zero cutoff should return h unchanged")
	}
}

// TestAllRunsEnumeratesEverySlugSortedWithPerModelDedup covers three models
// at once: one stable (single run), one that changes price mid-range (two
// runs), and one with a Found:false gap that must not split its run — same
// per-model semantics Runs already guarantees, now aggregated and ordered
// slug-ascending-then-chronological.
func TestAllRunsEnumeratesEverySlugSortedWithPerModelDedup(t *testing.T) {
	h := &History{Observations: []Observation{
		{ObservedAt: day(t, 2026, 9, 1), Prices: map[string]Price{
			"z/model": {Found: true, InPerM: 1, OutPerM: 2},
			"a/model": {Found: true, InPerM: 3, OutPerM: 6},
			"m/model": {Found: true, InPerM: 5, OutPerM: 10},
		}},
		{ObservedAt: day(t, 2026, 9, 2), Prices: map[string]Price{
			"z/model": {Found: true, InPerM: 1, OutPerM: 2},
			"a/model": {Found: true, InPerM: 4, OutPerM: 8},
			"m/model": {Found: false},
		}},
		{ObservedAt: day(t, 2026, 9, 3), Prices: map[string]Price{
			"z/model": {Found: true, InPerM: 1, OutPerM: 2},
			"m/model": {Found: true, InPerM: 5, OutPerM: 10},
		}},
	}}
	runs := AllRuns(h)
	if len(runs) != 4 {
		t.Fatalf("AllRuns returned %d rows, want 4 (a/model x2, m/model x1, z/model x1): %+v", len(runs), runs)
	}
	wantSlugs := []string{"a/model", "a/model", "m/model", "z/model"}
	for i, want := range wantSlugs {
		if runs[i].Slug != want {
			t.Errorf("runs[%d].Slug = %q, want %q (slug-ascending, then chronological within slug)", i, runs[i].Slug, want)
		}
	}
	if runs[0].Run.Price.InPerM != 3 || runs[1].Run.Price.InPerM != 4 {
		t.Errorf("a/model runs out of chronological/price order: %+v, %+v", runs[0].Run, runs[1].Run)
	}
	if runs[2].Run.From.Format("2006-01-02") != "2026-09-01" || runs[2].Run.To.Format("2006-01-02") != "2026-09-03" || runs[2].Run.Days != 2 {
		t.Errorf("m/model run = %+v, want From=09-01 To=09-03 Days=2 (a Found:false gap must not split the run)", runs[2].Run)
	}
}

// TestFormatModelRunsTableAlignsSlugColumnAcrossModels uses mixed slug
// lengths and checks that every row's Date range column starts at the same
// rune offset as the header's — computed in runes, not bytes, since a naive
// byte-offset comparison would be silently wrong for a row containing a
// multi-byte "→" range separator (see report.go's displayWidth comment).
func TestFormatModelRunsTableAlignsSlugColumnAcrossModels(t *testing.T) {
	rows := []ModelRun{
		{Slug: "a/short", Run: PriceRun{Price: Price{Found: true, InPerM: 1, OutPerM: 2}, From: day(t, 2026, 9, 1), To: day(t, 2026, 9, 1), Days: 1}},
		{Slug: "very-long-vendor/a-much-longer-model-name", Run: PriceRun{Price: Price{Found: true, InPerM: 3, OutPerM: 15}, From: day(t, 2026, 9, 3), To: day(t, 2026, 9, 10), Days: 7}},
	}
	table := FormatModelRunsTable(rows)
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("table has %d lines, want 4 (header, separator, 2 rows):\n%s", len(lines), table)
	}
	headerByteOffset := strings.Index(lines[0], "Date range")
	if headerByteOffset < 0 {
		t.Fatalf("header is missing the Date range column:\n%s", lines[0])
	}
	wantRuneOffset := utf8.RuneCountInString(lines[0][:headerByteOffset])

	dates := []string{"2026-09-01", "2026-09-03"} // the From date of each row, in order
	for i, dataLine := range lines[2:] {
		byteOffset := strings.Index(dataLine, dates[i])
		if byteOffset < 0 {
			t.Fatalf("row %d is missing its expected date %q:\n%s", i, dates[i], dataLine)
		}
		gotRuneOffset := utf8.RuneCountInString(dataLine[:byteOffset])
		if gotRuneOffset != wantRuneOffset {
			t.Errorf("row %d Date range column starts at rune offset %d, want %d (header offset) — Slug column misaligned:\nheader: %q\nrow:    %q", i, gotRuneOffset, wantRuneOffset, lines[0], dataLine)
		}
	}
}

func TestFormatModelRunsTableEmpty(t *testing.T) {
	if got := FormatModelRunsTable(nil); got != "no price history\n" {
		t.Errorf("FormatModelRunsTable(nil) = %q", got)
	}
}

// TestRenderAllModelsReportHasNoCharts proves the aggregate report never
// renders a per-model bar chart. It uses a newline-anchored check, since a
// bare "Input $/M" substring check would be vacuous — that text is also the
// table's own column header (mirrors cmd/openrouter/history_test.go's
// strings.Cut(report, "\nInput $/M") idiom for the same reason).
func TestRenderAllModelsReportHasNoCharts(t *testing.T) {
	h := &History{Observations: []Observation{
		{ObservedAt: day(t, 2026, 9, 1), Prices: map[string]Price{
			"a/model": {Found: true, InPerM: 1, OutPerM: 2},
			"b/model": {Found: true, InPerM: 3, OutPerM: 6},
		}},
	}}
	out := RenderAllModelsReport(h)
	if strings.Contains(out, "\nInput $/M") {
		t.Errorf("aggregate report should have no per-model bar charts, but found a chart title line:\n%s", out)
	}
	if !strings.Contains(out, "Input $/M") {
		t.Errorf("aggregate report is missing its Input $/M column header:\n%s", out)
	}
}

// TestRenderAllModelsReportRespectsSinceThroughFilterSince confirms
// FilterSince composes with RenderAllModelsReport for free — no new
// filtering logic is needed in RenderAllModelsReport itself.
func TestRenderAllModelsReportRespectsSinceThroughFilterSince(t *testing.T) {
	h := &History{Observations: []Observation{
		{ObservedAt: day(t, 2026, 9, 1), Prices: map[string]Price{"a/model": {Found: true, InPerM: 1, OutPerM: 2}}},
		{ObservedAt: day(t, 2026, 9, 5), Prices: map[string]Price{"a/model": {Found: true, InPerM: 1, OutPerM: 2}}},
	}}
	full := RenderAllModelsReport(h)
	if !strings.Contains(full, "2026-09-01") {
		t.Fatalf("unfiltered report should include the early observation:\n%s", full)
	}
	filtered := RenderAllModelsReport(FilterSince(h, day(t, 2026, 9, 5)))
	if strings.Contains(filtered, "2026-09-01") {
		t.Errorf("filtered report should not include the pre-cutoff observation:\n%s", filtered)
	}
	if !strings.Contains(filtered, "2026-09-05") {
		t.Errorf("filtered report should include the retained observation:\n%s", filtered)
	}
}

package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
)

func TestRenderHistoryFormatsAndFilters(t *testing.T) {
	history := &pricehistory.History{SchemaVersion: pricehistory.SchemaVersion, Observations: []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{"a/model": {Found: true, InPerM: 1, OutPerM: 2, Context: 1000}, "b/model": {Found: true, InPerM: 3, OutPerM: 4, Context: 2000}}},
		{ObservedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{"a/model": {Found: true, InPerM: 1, OutPerM: 2, Context: 1000, HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 1.5, OverrideOutPerM: 4}}},
	}}
	markdown, err := renderHistory(history, "a/model", "2026-08-01", "markdown", 0)
	if err != nil || !strings.Contains(markdown, "a/model") || !strings.Contains(markdown, "long-context threshold") || !strings.Contains(markdown, "500K+") || !strings.Contains(markdown, "long-context $1.5/$4") || strings.Contains(markdown, "b/model") {
		t.Fatalf("markdown = %q, %v", markdown, err)
	}
	// `openrouter history` output is English-only, headers included (see the
	// "timestamp | slug | ..." row above) — it must never bake in the
	// Russian preposition "от" for a change's long-context clause.
	if strings.Contains(markdown, "от") {
		t.Errorf("markdown change column carries the Russian preposition \"от\"; CLI output is English-only:\n%s", markdown)
	}
	if !strings.Contains(markdown, "long-context $1.5/$4 from 500K+") {
		t.Errorf("markdown change column is missing the English long-context clause:\n%s", markdown)
	}
	tsv, err := renderHistory(history, "", "2026-08-02T00:00:00Z", "tsv", 0)
	if err != nil || !strings.HasPrefix(tsv, "timestamp\tslug\tinput_per_million") || !strings.Contains(tsv, "a/model\t1\t2\t1000\t500000\t1.5\t4") {
		t.Fatalf("tsv = %q, %v", tsv, err)
	}
}

func TestRenderHistoryEmpty(t *testing.T) {
	output, err := renderHistory(&pricehistory.History{}, "", "", "markdown", 0)
	if err != nil || !strings.Contains(output, "No price history, or no observations match the current filters.") {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestRenderHistoryInvalidFormat(t *testing.T) {
	if _, err := renderHistory(&pricehistory.History{}, "", "", "csv", 0); err == nil || !strings.Contains(err.Error(), "--format must be markdown, tsv, or report") {
		t.Fatalf("err = %v, want a format error", err)
	}
}

// TestRenderHistoryReportDedupesRepeatedDailyObservations is the direct
// regression test for the bug report: a long run of daily observations at
// the same price must collapse into one table row spanning the date
// range, never one repeated row per day, and a day where the price
// changed twice must still show up as two distinct rows dated the same
// day.
func TestRenderHistoryReportDedupesRepeatedDailyObservations(t *testing.T) {
	const slug = "openai/report-model"
	observations := []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{slug: {Found: true, InPerM: 3, OutPerM: 15, Context: 1048576}}},
	}
	// Seven more daily observations (8 total, spanning 2026-09-03 through
	// 2026-09-10) at the identical price — the exact shape the user's bug
	// report complained about ("this exact line repeats 15+ times").
	for day := 4; day <= 10; day++ {
		observations = append(observations, pricehistory.Observation{ObservedAt: time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{slug: {Found: true, InPerM: 3, OutPerM: 15, Context: 1048576}}})
	}
	// Two distinct price changes on the same calendar day.
	observations = append(observations,
		pricehistory.Observation{ObservedAt: time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{slug: {Found: true, InPerM: 2.1, OutPerM: 10.53, Context: 1048576}}},
		pricehistory.Observation{ObservedAt: time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{slug: {Found: true, InPerM: 2.34, OutPerM: 11.7, Context: 1048576}}},
		pricehistory.Observation{ObservedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{slug: {Found: true, InPerM: 2.303, OutPerM: 11.55, Context: 1048576}}},
	)
	history := &pricehistory.History{SchemaVersion: pricehistory.SchemaVersion, Observations: observations}
	report, err := renderHistory(history, slug, "", "report", 72)
	if err != nil {
		t.Fatalf("renderHistory report: %v", err)
	}
	tableSection, _, hasCharts := strings.Cut(report, "\nInput $/M")
	if !hasCharts {
		t.Fatalf("report has no chart section:\n%s", report)
	}
	// The 8-day stable run collapses into one row spanning the range with
	// its day count — not eight identical rows.
	if !strings.Contains(tableSection, "2026-09-03 → 2026-09-10") || !strings.Contains(tableSection, "$3") || !strings.Contains(tableSection, "$15") {
		t.Fatalf("report did not collapse the stable run into a dated range:\n%s", report)
	}
	if count := strings.Count(tableSection, "$3"); count != 1 {
		t.Errorf("table = %q, want exactly one collapsed row for the stable $3 run, got %d occurrences", tableSection, count)
	}
	// The same-day double change still yields two distinct rows.
	if count := strings.Count(tableSection, "2026-09-11"); count != 2 {
		t.Errorf("table should list 2026-09-11 twice (two price changes that day), got %d:\n%s", count, tableSection)
	}
	if !strings.Contains(tableSection, "$2.1") || !strings.Contains(tableSection, "$2.34") || !strings.Contains(tableSection, "$2.303") {
		t.Fatalf("table is missing one of the same-day/next-day price changes:\n%s", tableSection)
	}
	// Charts for both series are present, each carrying real labeled
	// values and dates — never an unlabeled wall of bar characters.
	if !strings.Contains(report, "Input $/M") || !strings.Contains(report, "Output $/M") {
		t.Fatalf("report is missing a labeled series heading:\n%s", report)
	}
	if !strings.Contains(report, "$2.1") && !strings.Contains(report, "$3") {
		t.Fatalf("chart Y-axis has no recognizable dollar labels:\n%s", report)
	}
	if !strings.Contains(report, "2026-09-03") || !strings.Contains(report, "2026-09-12") {
		t.Fatalf("chart X-axis is missing its date labels:\n%s", report)
	}
	if strings.Count(report, "input $3 / output $15") > 1 {
		t.Fatalf("report still repeats a raw per-day observation line:\n%s", report)
	}
}

// TestRenderHistoryReportWithoutModelAggregatesAllModels is the direct
// regression test for report's new no-`--model` behavior: instead of
// erroring, it must aggregate every tracked model into one flat table —
// stable models keep one row, a model that changed price gets one row per
// run, and a Found:false gap must not split a model's run into extra rows.
func TestRenderHistoryReportWithoutModelAggregatesAllModels(t *testing.T) {
	const stableSlug = "aaa/stable-model"
	const changingSlug = "bbb/changing-model"
	const gappedSlug = "ccc/gapped-model"
	observations := []pricehistory.Observation{
		{ObservedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{
			stableSlug:   {Found: true, InPerM: 1, OutPerM: 2, Context: 1000},
			changingSlug: {Found: true, InPerM: 5, OutPerM: 10, Context: 2000},
			gappedSlug:   {Found: true, InPerM: 3, OutPerM: 6, Context: 500},
		}},
		{ObservedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{
			stableSlug:   {Found: true, InPerM: 1, OutPerM: 2, Context: 1000},
			changingSlug: {Found: true, InPerM: 5, OutPerM: 10, Context: 2000},
			gappedSlug:   {Found: false},
		}},
		{ObservedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{
			stableSlug:   {Found: true, InPerM: 1, OutPerM: 2, Context: 1000},
			changingSlug: {Found: true, InPerM: 7, OutPerM: 14, Context: 2000},
			gappedSlug:   {Found: true, InPerM: 3, OutPerM: 6, Context: 500},
		}},
		{ObservedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{
			stableSlug:   {Found: true, InPerM: 1, OutPerM: 2, Context: 1000},
			changingSlug: {Found: true, InPerM: 7, OutPerM: 14, Context: 2000},
			gappedSlug:   {Found: true, InPerM: 3, OutPerM: 6, Context: 500},
		}},
	}
	history := &pricehistory.History{SchemaVersion: pricehistory.SchemaVersion, Observations: observations}
	out, err := renderHistory(history, "", "", "report", 0)
	if err != nil {
		t.Fatalf("renderHistory report without --model: %v", err)
	}
	if !strings.Contains(out, "Slug") || !strings.Contains(out, "Date range") {
		t.Fatalf("aggregate report header is missing Slug/Date range columns:\n%s", out)
	}
	for _, slug := range []string{stableSlug, changingSlug, gappedSlug} {
		if !strings.Contains(out, slug) {
			t.Fatalf("aggregate report is missing slug %q:\n%s", slug, out)
		}
	}
	if count := strings.Count(out, stableSlug); count != 1 {
		t.Errorf("stable model %q appears %d times, want exactly 1 (one run):\n%s", stableSlug, count, out)
	}
	if count := strings.Count(out, changingSlug); count < 2 {
		t.Errorf("changing model %q appears %d times, want at least 2 (two runs):\n%s", changingSlug, count, out)
	}
	if count := strings.Count(out, gappedSlug); count != 1 {
		t.Errorf("gapped model %q appears %d times, want exactly 1 (gap must not split the run):\n%s", gappedSlug, count, out)
	}
	// The newline-anchored check proves no bar charts appear in aggregate
	// mode — a bare "Input $/M" substring check would be vacuous, since
	// that text is also the table's own column header.
	if strings.Contains(out, "\nInput $/M") {
		t.Fatalf("aggregate report unexpectedly contains a bar chart section:\n%s", out)
	}
}

// TestRenderHistoryReportWithoutModelIsEmptyStateInEnglish confirms empty
// history reaches pricehistory.RenderAllModelsReport's own English
// empty-string through renderHistory's routing.
func TestRenderHistoryReportWithoutModelIsEmptyStateInEnglish(t *testing.T) {
	out, err := renderHistory(&pricehistory.History{}, "", "", "report", 0)
	if err != nil || out != "No price history.\n" {
		t.Fatalf("renderHistory report on empty history = %q, %v, want \"No price history.\\n\"", out, err)
	}
}

// TestRenderHistoryDefaultFormatIsReport confirms the --format flag's new
// default and that a bare `history` invocation with no --format flag at
// all actually takes the report code path (the empty-state string), not
// the old markdown pipe-table.
func TestRenderHistoryDefaultFormatIsReport(t *testing.T) {
	root := newRootCmd()
	historyCmd, _, err := root.Find([]string{"history"})
	if err != nil {
		t.Fatalf("find history: %v", err)
	}
	formatFlag := historyCmd.Flags().Lookup("format")
	if formatFlag == nil || formatFlag.DefValue != "report" {
		t.Fatalf("history --format flag = %+v, want DefValue \"report\"", formatFlag)
	}

	dataDir := t.TempDir()
	config := writeConfig(t, "data_dir: "+dataDir+"\n")
	output := executeCLI(t, "history", "--config", config)
	if output != "No price history.\n" {
		t.Fatalf("history with no --format = %q, want the report format's empty state", output)
	}
}

// TestHistoryPagerDecision mirrors TestTablePagerDecision, applied to
// history's own --no-pager flag wiring: the flag exists, defaults to
// false, and shouldPage (the shared decision function history's RunE
// calls) behaves the same way regardless of which command wired it up.
func TestHistoryPagerDecision(t *testing.T) {
	root := newRootCmd()
	historyCmd, _, err := root.Find([]string{"history"})
	if err != nil {
		t.Fatalf("find history: %v", err)
	}
	noPagerFlag := historyCmd.Flags().Lookup("no-pager")
	if noPagerFlag == nil || noPagerFlag.DefValue != "false" {
		t.Fatalf("history --no-pager flag = %+v, want a bool flag defaulting to false", noPagerFlag)
	}

	var output strings.Builder
	if shouldPage(&output, false) || shouldPage(&output, true) {
		t.Fatal("buffer output must never use pager")
	}
	previous := pagerIsTTY
	pagerIsTTY = func(io.Writer) bool { return true }
	t.Cleanup(func() { pagerIsTTY = previous })
	if !shouldPage(&output, false) {
		t.Fatal("TTY output should use pager when --no-pager is not set")
	}
	if shouldPage(&output, true) {
		t.Fatal("--no-pager must disable the pager in a TTY")
	}
}

// TestHistoryUsesPagerInTTY mirrors TestTablePagerBoundsIdentityFields's
// swap-seam pattern: with pagerIsTTY forced true and runPager swapped to
// capture instead of exec'ing less, running `history` must land its
// output in the captured pager buffer instead of stdout — and --no-pager
// must reverse that, sending output straight to stdout instead.
func TestHistoryUsesPagerInTTY(t *testing.T) {
	previousTTY := pagerIsTTY
	previousPager := runPager
	t.Cleanup(func() {
		pagerIsTTY = previousTTY
		runPager = previousPager
	})
	pagerIsTTY = func(io.Writer) bool { return true }
	var paged strings.Builder
	runPager = func(output string, _, _ io.Writer) error {
		paged.WriteString(output)
		return nil
	}

	dataDir := t.TempDir()
	config := writeConfig(t, "data_dir: "+dataDir+"\n")

	stdout := executeCLI(t, "history", "--config", config)
	if stdout != "" {
		t.Fatalf("history output went to stdout instead of the pager: %q", stdout)
	}
	if !strings.Contains(paged.String(), "No price history.") {
		t.Fatalf("history output did not reach the pager: %q", paged.String())
	}

	paged.Reset()
	stdout = executeCLI(t, "history", "--config", config, "--no-pager")
	if stdout != "No price history.\n" {
		t.Fatalf("history --no-pager output = %q, want it printed directly to stdout", stdout)
	}
	if paged.Len() != 0 {
		t.Fatalf("--no-pager still routed output through the pager: %q", paged.String())
	}
}

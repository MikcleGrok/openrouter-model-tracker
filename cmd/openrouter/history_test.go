package main

import (
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
	if err != nil || !strings.Contains(output, "История цен пуста") {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestRenderHistoryInvalidFormat(t *testing.T) {
	if _, err := renderHistory(&pricehistory.History{}, "", "", "csv", 0); err == nil || !strings.Contains(err.Error(), "--format must be markdown, tsv, or report") {
		t.Fatalf("err = %v, want a format error", err)
	}
}

// TestRenderHistoryReportRequiresModel guards --format report's one hard
// requirement: unlike markdown/tsv (which happily dump every model), a
// deduplicated table + chart only makes sense for one named model.
func TestRenderHistoryReportRequiresModel(t *testing.T) {
	if _, err := renderHistory(&pricehistory.History{}, "", "", "report", 0); err == nil || !strings.Contains(err.Error(), "--format report requires --model") {
		t.Fatalf("err = %v, want the missing-model error", err)
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

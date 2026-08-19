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
	markdown, err := renderHistory(history, "a/model", "2026-08-01", "markdown")
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
	tsv, err := renderHistory(history, "", "2026-08-02T00:00:00Z", "tsv")
	if err != nil || !strings.HasPrefix(tsv, "timestamp\tslug\tinput_per_million") || !strings.Contains(tsv, "a/model\t1\t2\t1000\t500000\t1.5\t4") {
		t.Fatalf("tsv = %q, %v", tsv, err)
	}
}

func TestRenderHistoryEmpty(t *testing.T) {
	output, err := renderHistory(&pricehistory.History{}, "", "", "markdown")
	if err != nil || !strings.Contains(output, "История цен пуста") {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
)

// ValsGPQAURL is the vals.ai GPQA Diamond leaderboard. It is the same Astro
// page shape as the vals.ai SWE-bench leaderboard — no JSON API, the
// leaderboard lives in the props attribute of the <astro-island> whose
// component-url names BenchmarkView — so this source reuses benchmarkProps,
// unwrapAstro and valsPage from valsai.go rather than re-deriving them. Only
// the URL, the metric and the guard below differ.
//
// Note the slug: the site serves GPQA Diamond at /benchmarks/gpqa, not at
// /benchmarks/gpqa_diamond (which is a 404).
var ValsGPQAURL = "https://www.vals.ai/benchmarks/gpqa"

// MetricGPQADiamond is the graduate-level multiple-choice reasoning score
// vals.ai publishes (a 0–100 percentage over the GPQA Diamond set).
//
// It is a percentage like MetricSWEBenchVerified, and that coincidence is
// exactly why the two must never share a column: SWE-bench Verified measures
// resolved real pull requests by an agent, GPQA Diamond measures answers to
// Google-proof science questions in a single turn. Two models on 90% here and
// 90% there are not "equally good" on one scale — they sat two different
// exams. Unlike MetricArenaElo, whose different range makes the confusion
// self-evident, this one looks comparable and is not.
const MetricGPQADiamond = "GPQA Diamond"

// valsGPQASlug is the benchmark slug the page must report for its numbers to
// be accepted. See valsPage.Metadata.Benchmark/Slug for why this is checked.
const valsGPQASlug = "gpqa"

// FetchValsGPQA returns one row per tracked slug present in
// benchmarkView.default.tasks.overall on the GPQA Diamond leaderboard. names
// maps an OpenRouter slug to the exact model key vals.ai uses, e.g.
// "anthropic/claude-opus-4-8"; matching is an exact map lookup and never a
// guess, exactly as for every other source here.
//
// Identity has the same counter-check the vals.ai SWE-bench source has, and
// for the same reason: the row is looked up by key, so VariantMeasured is set
// to that very key and internal/model can reject a row whose echo disagrees
// with the configured mapping. There is one extra, page-level check on top —
// the benchmark slug the page reports about itself — because a second vals.ai
// benchmark differs from this one only by a path segment.
func FetchValsGPQA(ctx context.Context, c *httpcache.Client, names map[string]string) ([]ScoreRow, error) {
	body, err := c.Get(ctx, ValsGPQAURL)
	if err != nil {
		return nil, fmt.Errorf("gpqa: fetch: %w", err)
	}
	plain, err := benchmarkProps(body)
	if err != nil {
		return nil, fmt.Errorf("gpqa: %s: %w", ValsGPQAURL, err)
	}
	var page valsPage
	if err := json.Unmarshal(plain, &page); err != nil {
		return nil, fmt.Errorf("gpqa: decode benchmarkView: %w", err)
	}
	meta := page.BenchmarkView.Default.Metadata
	// Fail closed, never fall through: a page that is not GPQA Diamond is
	// "no GPQA data this run" (the orchestrator then keeps the snapshot),
	// which is strictly better than importing another benchmark's numbers
	// under this metric's name.
	if meta.Slug != valsGPQASlug {
		return nil, fmt.Errorf("gpqa: %s: page reports benchmark slug %q (%q), want %q — refusing to read another benchmark's numbers", ValsGPQAURL, meta.Slug, meta.Benchmark, valsGPQASlug)
	}
	overall, ok := page.BenchmarkView.Default.Tasks["overall"]
	if !ok {
		return nil, fmt.Errorf("gpqa: %s: tasks.overall is missing", ValsGPQAURL)
	}

	out := make([]ScoreRow, 0, len(names))
	for slug, key := range names {
		entry, ok := overall[key]
		if !ok {
			continue // tracked, but not on this leaderboard: report.go tells the human
		}
		out = append(out, ScoreRow{
			Slug:               slug,
			SourceFamily:       "gpqa",
			ConfiguredIdentity: key,
			Metric:             MetricGPQADiamond,
			Value:              entry.Accuracy,
			Unit:               "%",
			VariantMeasured:    key,
			SourceURL:          ValsGPQAURL,
			Checked:            meta.Updated,
			Provider:           entry.Provider,
			Harness:            "vals.ai fixed harness",
			Scaffold:           "n/a",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

package sources

import (
	"context"
	"strings"
	"testing"
)

func TestFetchValsGPQA(t *testing.T) {
	srv, c := serveFixture(t, "testdata/vals-gpqa.html")
	old := ValsGPQAURL
	ValsGPQAURL = srv.URL
	t.Cleanup(func() { ValsGPQAURL = old })

	rows, err := FetchValsGPQA(context.Background(), c, map[string]string{
		"anthropic/claude-opus-4.8":  "anthropic/claude-opus-4-8",
		"anthropic/claude-fable-5.1": "anthropic/claude-fable-5-1",
		"x-ai/grok-4.6":              "grok/grok-4.6",
		"openai/gpt-6-astra":         "openai/gpt-6-astra", // tracked, but vals.ai has not measured it
	})
	if err != nil {
		t.Fatalf("FetchValsGPQA: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 — gemini is on the page but untracked, gpt-6-astra is tracked but absent: %+v", len(rows), rows)
	}

	byslug := map[string]ScoreRow{}
	for _, r := range rows {
		byslug[r.Slug] = r
	}

	opus := byslug["anthropic/claude-opus-4.8"]
	if opus.Value != 92.424 {
		t.Errorf("opus.Value = %v, want 92.424", opus.Value)
	}
	if opus.Metric != MetricGPQADiamond || opus.Unit != "%" {
		t.Errorf("opus metric/unit = %q/%q, want %q/%q", opus.Metric, opus.Unit, MetricGPQADiamond, "%")
	}
	if opus.SourceFamily != "gpqa" {
		t.Errorf("opus.SourceFamily = %q, want the model-map source id %q", opus.SourceFamily, "gpqa")
	}
	if opus.Checked != "2026-09-01" {
		t.Errorf("opus.Checked = %q, want metadata.updated", opus.Checked)
	}
	if opus.SourceURL != srv.URL {
		t.Errorf("opus.SourceURL = %q, want %q", opus.SourceURL, srv.URL)
	}
	if opus.Provider != "Anthropic" {
		t.Errorf("opus.Provider = %q, want the leaderboard's own provider", opus.Provider)
	}

	// The key echo is what internal/model checks a mapping against, so it
	// must be the configured key itself and not a display name.
	for _, r := range rows {
		if r.VariantMeasured != r.ConfiguredIdentity {
			t.Errorf("%s: VariantMeasured = %q, ConfiguredIdentity = %q — the row must echo the key it was looked up by", r.Slug, r.VariantMeasured, r.ConfiguredIdentity)
		}
	}

	if _, ok := byslug["google/gemini-3.1-pro-preview"]; ok {
		t.Error("gemini is on the leaderboard but not in the map — it must be ignored, never guessed into a slug")
	}

	// Only tasks.overall feeds the column; the per-subject breakdowns on the
	// same page are a different population of questions.
	if opus.Value == 88.1 {
		t.Error("opus.Value came from the physics task map, not from tasks.overall")
	}
}

// TestFetchValsGPQARefusesAnotherBenchmarksPage is the page-level half of the
// identity gate: every vals.ai benchmark renders through the same component
// at a URL differing by one path segment, so a redirect or a renamed slug
// would otherwise hand SWE-bench percentages to the GPQA column, where they
// would look entirely plausible.
func TestFetchValsGPQARefusesAnotherBenchmarksPage(t *testing.T) {
	srv, c := serveFixture(t, "testdata/valsai.html") // a real BenchmarkView page, but slug "swebench"
	old := ValsGPQAURL
	ValsGPQAURL = srv.URL
	t.Cleanup(func() { ValsGPQAURL = old })

	rows, err := FetchValsGPQA(context.Background(), c, map[string]string{"openai/gpt-5.6-luna": "openai/gpt-5.6-luna"})
	if err == nil {
		t.Fatalf("want an error when the page is another benchmark, got %d rows", len(rows))
	}
	if !strings.Contains(err.Error(), "swebench") {
		t.Errorf("error = %v, want it to name the benchmark slug actually found", err)
	}
	if rows != nil {
		t.Errorf("rows = %+v, want nothing at all — a wrong page must never contribute a partial result", rows)
	}
}

func TestFetchValsGPQAErrorsWhenIslandIsGone(t *testing.T) {
	srv, c := serveFixture(t, "testdata/swebench.html") // страница без BenchmarkView
	old := ValsGPQAURL
	ValsGPQAURL = srv.URL
	t.Cleanup(func() { ValsGPQAURL = old })

	if _, err := FetchValsGPQA(context.Background(), c, map[string]string{"a/b": "a/b"}); err == nil {
		t.Fatal("want an error when the BenchmarkView island is missing, so the orchestrator falls back to the snapshot")
	}
}

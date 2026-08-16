package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchSWEBenchVerified(t *testing.T) {
	srv, c := serveFixture(t, "testdata/swebench.html")
	old := SWEBenchURL
	SWEBenchURL = srv.URL
	t.Cleanup(func() { SWEBenchURL = old })

	rows, err := FetchSWEBenchVerified(context.Background(), c, map[string]string{
		"openai/gpt-5.6-sol":                     "Model: gpt-5.6-sol",
		"nvidia/nemotron-3-ultra-550b-a55b:free": "Model: nemotron-3-ultra",
		"minimax/minimax-m3":                     "Model: minimax-m3", // нет такой записи на лидерборде
	})
	if err != nil {
		t.Fatalf("FetchSWEBenchVerified: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (a tracked slug with no leaderboard entry is silently absent, not an error): %+v", len(rows), rows)
	}

	byslug := map[string]ScoreRow{}
	for _, r := range rows {
		byslug[r.Slug] = r
	}

	sol, ok := byslug["openai/gpt-5.6-sol"]
	if !ok {
		t.Fatalf("openai/gpt-5.6-sol missing from %+v", rows)
	}
	// Two submissions share the "Model: gpt-5.6-sol" tag: 71.2 and 79.2.
	// The reported score is their median, not the higher one — see the
	// package doc on why swebench.com is aggregated by median rather than
	// max.
	if sol.Value != 75.2 {
		t.Errorf("sol.Value = %v, want 75.2 (the median of the two entries sharing the Model tag: (71.2+79.2)/2)", sol.Value)
	}
	// Value is nobody's individual result once several submissions are
	// aggregated, so neither the variant nor the date may name one specific
	// submission: every renderer prints Value, VariantMeasured and Checked
	// side by side.
	if want := "gpt-5.6-sol (median of 2 scaffolds)"; sol.VariantMeasured != want {
		t.Errorf("sol.VariantMeasured = %q, want %q — naming the top submission next to a median it did not produce is a false provenance claim", sol.VariantMeasured, want)
	}
	if sol.Metric != "SWE-bench Verified" {
		t.Errorf("sol.Metric = %q, want %q", sol.Metric, "SWE-bench Verified")
	}
	if want := "2026-02-17..2026-03-04"; sol.Checked != want {
		t.Errorf("sol.Checked = %q, want %q — the span the aggregated submissions cover, not the top submission's own date", sol.Checked, want)
	}
	if sol.SourceURL != srv.URL {
		t.Errorf("sol.SourceURL = %q, want %q", sol.SourceURL, srv.URL)
	}
	if want := "median of 2 scaffolds (highest individual: OpenHands + GPT-5.6 Sol)"; sol.Scaffold != want {
		t.Errorf("sol.Scaffold = %q, want %q — it must not attribute the median score to one scaffold's name", sol.Scaffold, want)
	}

	nemo, ok := byslug["nvidia/nemotron-3-ultra-550b-a55b:free"]
	if !ok {
		t.Fatalf("nemotron missing from %+v", rows)
	}
	// Only one submission is tagged for nemotron, so its median is trivially
	// that one value.
	if nemo.Value != 65.0 {
		t.Errorf("nemo.Value = %v, want 65.0", nemo.Value)
	}
	if want := "single scaffold (SWE-agent + Nemotron 3 Ultra)"; nemo.Scaffold != want {
		t.Errorf("nemo.Scaffold = %q, want %q", nemo.Scaffold, want)
	}
	// A single submission IS the value, so it keeps naming itself and its own
	// date — the honesty rule only bites once n > 1.
	if nemo.VariantMeasured != "SWE-agent + Nemotron 3 Ultra" {
		t.Errorf("nemo.VariantMeasured = %q, want the submission's own name — with one submission there is nothing to aggregate", nemo.VariantMeasured)
	}
	if nemo.Checked != "2026-05-11" {
		t.Errorf("nemo.Checked = %q, want the submission's own date", nemo.Checked)
	}

	if rows[0].Slug > rows[1].Slug {
		t.Errorf("rows are not sorted by slug: %+v", rows)
	}
}

// TestFetchSWEBenchVerifiedDeduplicatesResubmittedScaffold covers the
// grouping guard: the leaderboard can list one scaffold twice (a resubmission
// or a re-verification), and counting both would give that scaffold two votes
// in the median — exactly the weighting the median exists to prevent. Here
// the same scaffold appears at 90.0 and, later, at 92.0; a second, genuinely
// different scaffold sits at 60.0. One vote each makes the median 76.0, while
// counting the duplicate would pull it up to 90.0.
func TestFetchSWEBenchVerifiedDeduplicatesResubmittedScaffold(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "swebench-dupes.html")
	page := `<html><body><script id="leaderboard-data">[{"name":"Verified","results":[
	  {"name":"OpenHands + Dup Model","resolved":90.0,"date":"2026-01-10","tags":["Model: dup-model"]},
	  {"name":"openhands +  dup model","resolved":92.0,"date":"2026-02-20","tags":["Model: dup-model"]},
	  {"name":"SWE-agent + Dup Model","resolved":60.0,"date":"2026-01-15","tags":["Model: dup-model"]}
	]}]</script></body></html>`
	if err := os.WriteFile(fixture, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, c := serveFixture(t, fixture)
	old := SWEBenchURL
	SWEBenchURL = srv.URL
	t.Cleanup(func() { SWEBenchURL = old })

	rows, err := FetchSWEBenchVerified(context.Background(), c, map[string]string{"a/dup-model": "Model: dup-model"})
	if err != nil {
		t.Fatalf("FetchSWEBenchVerified: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	got := rows[0]
	if got.Value != 76.0 {
		t.Errorf("Value = %v, want 76.0 — one vote per scaffold ((92.0+60.0)/2), not 90.0 from counting the resubmitted scaffold twice", got.Value)
	}
	if want := "median of 2 scaffolds (highest individual: openhands +  dup model)"; got.Scaffold != want {
		t.Errorf("Scaffold = %q, want %q — the deduplicated count, and the superseding submission as the top one", got.Scaffold, want)
	}
	if want := "dup-model (median of 2 scaffolds)"; got.VariantMeasured != want {
		t.Errorf("VariantMeasured = %q, want %q", got.VariantMeasured, want)
	}
	if want := "2026-01-15..2026-02-20"; got.Checked != want {
		t.Errorf("Checked = %q, want %q — the span of the submissions actually counted, with the superseded one gone", got.Checked, want)
	}
}

func TestSupersedes(t *testing.T) {
	tests := []struct {
		name string
		a, b sweResult
		want bool
	}{
		{"later date wins", sweResult{Date: "2026-02-20", Resolved: 10}, sweResult{Date: "2026-01-10", Resolved: 90}, true},
		{"earlier date loses", sweResult{Date: "2026-01-10", Resolved: 90}, sweResult{Date: "2026-02-20", Resolved: 10}, false},
		{"same date, higher score wins", sweResult{Date: "2026-01-10", Resolved: 90}, sweResult{Date: "2026-01-10", Resolved: 80}, true},
		{"same date, lower score loses", sweResult{Date: "2026-01-10", Resolved: 80}, sweResult{Date: "2026-01-10", Resolved: 90}, false},
		{"identical is not a replacement", sweResult{Date: "2026-01-10", Resolved: 80}, sweResult{Date: "2026-01-10", Resolved: 80}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supersedes(tt.a, tt.b); got != tt.want {
				t.Errorf("supersedes(%+v, %+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCheckedSpan(t *testing.T) {
	tests := []struct {
		name    string
		results []sweResult
		want    string
	}{
		{"one date", []sweResult{{Date: "2026-01-10"}}, "2026-01-10"},
		{"same date twice", []sweResult{{Date: "2026-01-10"}, {Date: "2026-01-10"}}, "2026-01-10"},
		{"unsorted span", []sweResult{{Date: "2026-03-04"}, {Date: "2026-02-17"}}, "2026-02-17..2026-03-04"},
		{"undated entries are skipped", []sweResult{{Date: ""}, {Date: "2026-02-17"}}, "2026-02-17"},
		{"all undated", []sweResult{{Date: ""}, {Date: ""}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkedSpan(tt.results); got != tt.want {
				t.Errorf("checkedSpan(%+v) = %q, want %q", tt.results, got, tt.want)
			}
		})
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"single value", []float64{42}, 42},
		{"odd count, already sorted", []float64{10, 20, 30}, 20},
		{"odd count, unsorted", []float64{30, 10, 20}, 20},
		{"even count", []float64{10, 20, 30, 40}, 25},
		{"even count, unsorted", []float64{40, 10, 30, 20}, 25},
		{"empty", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := append([]float64(nil), tt.values...)
			if got := median(values); got != tt.want {
				t.Errorf("median(%v) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

func TestFetchSWEBenchVerifiedErrorsWhenBlockIsGone(t *testing.T) {
	srv, c := serveFixture(t, "testdata/openrouter-models.json") // страница без нужного <script>
	old := SWEBenchURL
	SWEBenchURL = srv.URL
	t.Cleanup(func() { SWEBenchURL = old })

	if _, err := FetchSWEBenchVerified(context.Background(), c, map[string]string{"a/b": "Model: x"}); err == nil {
		t.Fatal("want an error when the leaderboard-data block is missing, so the orchestrator falls back to the snapshot")
	}
}

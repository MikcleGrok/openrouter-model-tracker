package sources

import (
	"context"
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
	if sol.Value != 79.2 {
		t.Errorf("sol.Value = %v, want 79.2 (the higher of two entries sharing the Model tag)", sol.Value)
	}
	if sol.VariantMeasured != "OpenHands + GPT-5.6 Sol" {
		t.Errorf("sol.VariantMeasured = %q, want the winning scaffold's name", sol.VariantMeasured)
	}
	if sol.Metric != "SWE-bench Verified" {
		t.Errorf("sol.Metric = %q, want %q", sol.Metric, "SWE-bench Verified")
	}
	if sol.Checked != "2026-03-04" {
		t.Errorf("sol.Checked = %q, want the winning entry's own date", sol.Checked)
	}
	if sol.SourceURL != srv.URL {
		t.Errorf("sol.SourceURL = %q, want %q", sol.SourceURL, srv.URL)
	}

	nemo, ok := byslug["nvidia/nemotron-3-ultra-550b-a55b:free"]
	if !ok {
		t.Fatalf("nemotron missing from %+v", rows)
	}
	if nemo.Value != 65.0 {
		t.Errorf("nemo.Value = %v, want 65.0", nemo.Value)
	}

	if rows[0].Slug > rows[1].Slug {
		t.Errorf("rows are not sorted by slug: %+v", rows)
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

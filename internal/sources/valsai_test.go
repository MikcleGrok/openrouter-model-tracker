package sources

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnwrapAstro(t *testing.T) {
	const in = `{"a":[0,{"b":[0,"x"],"c":[0,null],"d":[0,1.5]}],"e":[1,[[0,"p"],[0,"q"]]]}`
	var raw any
	if err := json.Unmarshal([]byte(in), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := json.Marshal(unwrapAstro(raw))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"a":{"b":"x","c":null,"d":1.5},"e":["p","q"]}`
	if string(got) != want {
		t.Fatalf("unwrapAstro produced\n  %s\nwant\n  %s", got, want)
	}
}

func TestFetchValsSWEBench(t *testing.T) {
	srv, c := serveFixture(t, "testdata/valsai.html")
	old := ValsSWEBenchURL
	ValsSWEBenchURL = srv.URL
	t.Cleanup(func() { ValsSWEBenchURL = old })

	rows, err := FetchValsSWEBench(context.Background(), c, map[string]string{
		"openai/gpt-5.6-luna": "openai/gpt-5.6-luna",
		"moonshotai/kimi-k3":  "moonshotai/kimi-k3",
		"minimax/minimax-m3":  "minimax/minimax-m3", // на vals.ai такой модели нет
	})
	if err != nil {
		t.Fatalf("FetchValsSWEBench: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 — gpt-5.6-sol is on the page but untracked, minimax is tracked but absent: %+v", len(rows), rows)
	}

	byslug := map[string]ScoreRow{}
	for _, r := range rows {
		byslug[r.Slug] = r
	}

	luna := byslug["openai/gpt-5.6-luna"]
	if luna.Value != 93 {
		t.Errorf("luna.Value = %v, want 93 (an integer accuracy must survive)", luna.Value)
	}
	kimi := byslug["moonshotai/kimi-k3"]
	if kimi.Value != 93.4 {
		t.Errorf("kimi.Value = %v, want 93.4", kimi.Value)
	}
	if kimi.Metric != "SWE-bench Verified" {
		t.Errorf("kimi.Metric = %q, want %q", kimi.Metric, "SWE-bench Verified")
	}
	if kimi.VariantMeasured != "moonshotai/kimi-k3" {
		t.Errorf("kimi.VariantMeasured = %q, want the vals.ai model key", kimi.VariantMeasured)
	}
	if kimi.Checked != "2026-08-03" {
		t.Errorf("kimi.Checked = %q, want metadata.updated", kimi.Checked)
	}
	if kimi.SourceURL != srv.URL {
		t.Errorf("kimi.SourceURL = %q, want %q", kimi.SourceURL, srv.URL)
	}

	if _, ok := byslug["openai/gpt-5.6-sol"]; ok {
		t.Error("gpt-5.6-sol is on the leaderboard but not in the map — it must be ignored, never guessed into a slug")
	}
}

func TestFetchValsSWEBenchErrorsWhenIslandIsGone(t *testing.T) {
	srv, c := serveFixture(t, "testdata/swebench.html") // страница без BenchmarkView
	old := ValsSWEBenchURL
	ValsSWEBenchURL = srv.URL
	t.Cleanup(func() { ValsSWEBenchURL = old })

	if _, err := FetchValsSWEBench(context.Background(), c, map[string]string{"a/b": "a/b"}); err == nil {
		t.Fatal("want an error when the BenchmarkView island is missing, so the orchestrator falls back to the snapshot")
	}
}

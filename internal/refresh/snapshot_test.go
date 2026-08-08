package refresh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
)

func TestLoadSnapshotMissingFileIsEmpty(t *testing.T) {
	s, err := LoadSnapshot(filepath.Join(t.TempDir(), "last-run-snapshot.json"))
	if err != nil {
		t.Fatalf("LoadSnapshot on a missing file returned %v, want nil error", err)
	}
	if s == nil {
		t.Fatal("LoadSnapshot returned a nil snapshot, want an empty one")
	}
	if len(s.Models) != 0 {
		t.Fatalf("empty snapshot has %d models, want 0", len(s.Models))
	}
	if _, ok := s.Models["anything"]; ok {
		t.Fatal("lookup into an empty snapshot must be safe and miss")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "last-run-snapshot.json")
	want := &Snapshot{
		FetchedAt:    "2026-08-04",
		CatalogSlugs: []string{"minimax/minimax-m3", "openai/gpt-5.6-luna"},
		Models: map[string]SnapshotEntry{
			"openai/gpt-5.6-luna": {
				InPerM: 0.5, OutPerM: 3, Context: 1000000,
				Score: &model.ScoreInfo{
					Metric: "SWE-bench Verified", Value: 93.0,
					VariantMeasured: "openai/gpt-5.6-luna",
					SourceURL:       "https://www.vals.ai/benchmarks/swebench",
					Checked:         "2026-08-03",
				},
			},
			"minimax/minimax-m3": {InPerM: 0.3, OutPerM: 1.2, Context: 1000000},
		},
	}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save (must create the parent directory): %v", err)
	}

	got, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if got.FetchedAt != want.FetchedAt || len(got.Models) != 2 || len(got.CatalogSlugs) != 2 {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	luna := got.Models["openai/gpt-5.6-luna"]
	if luna.InPerM != 0.5 || luna.OutPerM != 3 || luna.Context != 1000000 {
		t.Errorf("luna prices = %+v", luna)
	}
	if luna.Score == nil || luna.Score.Value != 93.0 || luna.Score.Checked != "2026-08-03" {
		t.Errorf("luna.Score = %+v, want the score to survive the round trip", luna.Score)
	}
	if m3 := got.Models["minimax/minimax-m3"]; m3.Score != nil {
		t.Errorf("m3.Score = %+v, want nil to survive as nil", m3.Score)
	}
}

func TestLoadSnapshotWithoutCatalogBaselineRemainsCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-run-snapshot.json")
	legacy := `{"fetched_at":"2026-08-03","models":{"a/model":{"in_per_m":1,"out_per_m":2,"context":1000}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy snapshot: %v", err)
	}
	s, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(s.Models) != 1 || len(s.CatalogSlugs) != 0 {
		t.Fatalf("legacy snapshot = %+v, want models and no catalogue baseline", s)
	}
}

func TestNewSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "last-run-snapshot.json")
	models := []model.Model{
		{Slug: "a/one", InPerM: 1, OutPerM: 2, Context: 100000, Score: &model.ScoreInfo{Value: 50}, TaskFit: []string{"implement", "test"}, QualityPrice: 999},
		{Slug: "a/two", InPerM: 3, OutPerM: 4, Context: 200000},
	}
	s := NewSnapshot(models, "2026-08-04")
	if s.FetchedAt != "2026-08-04" || len(s.Models) != 2 {
		t.Fatalf("NewSnapshot = %+v", s)
	}
	if s.Models["a/one"].Score == nil || s.Models["a/one"].Score.Value != 50 {
		t.Errorf("NewSnapshot dropped the score of a/one: %+v", s.Models["a/one"])
	}
	if s.Models["a/two"].OutPerM != 4 {
		t.Errorf("NewSnapshot = %+v", s.Models["a/two"])
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	got := loaded.Models["a/one"]
	if got.InPerM != 1 || got.OutPerM != 2 || got.Context != 100000 || got.Score == nil || got.Score.Value != 50 || got.HasOverride || got.OverrideMinTokens != 0 || got.OverrideInPerM != 0 || got.OverrideOutPerM != 0 {
		t.Fatalf("loaded entry = %+v, want only dynamic snapshot fields", got)
	}
	body, err := json.Marshal(loaded.Models["a/one"])
	if err != nil {
		t.Fatalf("marshal loaded entry: %v", err)
	}
	if string(body) != `{"in_per_m":1,"out_per_m":2,"context":100000,"score":{"metric":"","value":50,"variant_measured":"","source_url":"","checked":""}}` {
		t.Fatalf("loaded entry metadata = %s, want no task-fit or quality/price fields", body)
	}
}

func TestSnapshotRoundTripsTheArenaScore(t *testing.T) {
	models := []model.Model{{
		Slug: "a/high", InPerM: 1, OutPerM: 3, Context: 1000,
		Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70},
		ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1453, VariantMeasured: "hy3", SourceURL: "u", Checked: "2026-08-06"},
	}}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshot(models, "2026-08-08").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	entry := loaded.Models["a/high"]
	if entry.Score == nil || entry.Score.Value != 70 {
		t.Errorf("Score = %+v, want the SWE-bench number preserved", entry.Score)
	}
	if entry.ArenaScore == nil || entry.ArenaScore.Value != 1453 || entry.ArenaScore.Metric != "LMArena Elo" {
		t.Errorf("ArenaScore = %+v, want the raw Elo preserved so the next run can fall back to it", entry.ArenaScore)
	}
}

func TestSnapshotOmitsAnEmptyArenaScore(t *testing.T) {
	models := []model.Model{{Slug: "a/high", InPerM: 1, OutPerM: 3, Context: 1000}}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshot(models, "2026-08-08").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(body), "arena_score") {
		t.Errorf("an empty Arena score must not appear in the snapshot:\n%s", body)
	}
}

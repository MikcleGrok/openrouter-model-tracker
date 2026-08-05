package refresh

import (
	"os"
	"path/filepath"
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
	models := []model.Model{
		{Slug: "a/one", InPerM: 1, OutPerM: 2, Context: 100000, Score: &model.ScoreInfo{Value: 50}},
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
}

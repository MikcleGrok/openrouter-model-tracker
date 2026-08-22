package refresh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
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

func TestLoadSnapshotNormalizesLegacyMissingLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	body := `{"models":{"demo/model":{"provider":"Demo (н/д)","score":{"metric":"н/д","uncertainty":"н/д","sample_size":"н/д"}}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Models["demo/model"]
	if entry.Provider != "Demo (n/a)" || entry.Score.Metric != "n/a" || entry.Score.Uncertainty != "n/a" || entry.Score.SampleSize != "n/a" {
		t.Fatalf("normalized snapshot entry = %+v", entry)
	}
}

func TestLoadSnapshotKeepsArenaProviderWhenTopLevelProviderIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	body := `{"models":{"openai/gpt":{"provider":"","arena_score":{"metric":"LMArena Elo","value":1453,"provider":"OpenAI"}}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Models["openai/gpt"]
	if entry.Provider != "" || entry.ArenaScore == nil || entry.ArenaScore.Provider != "OpenAI" {
		t.Fatalf("legacy Arena provider = %+v, want empty top-level provider and OpenAI Arena provider", entry)
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
	if got.Models["minimax/minimax-m3"].Copyright != "unknown" {
		t.Errorf("m3.Copyright = %q, want unknown for an omitted value", got.Models["minimax/minimax-m3"].Copyright)
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
	if string(body) != `{"in_per_m":1,"out_per_m":2,"context":100000,"copyright":"unknown","copyright_guardrail":"unknown","score":{"metric":"","value":50,"variant_measured":"","source_url":"","checked":"","provenance":"","canonical_id":"","release_variant":"","model_variant":"","reasoning":"","configuration":"","provider":"","uncertainty":"","sample_size":"","harness":"","scaffold":""}}` {
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

func TestSnapshotRoundTripsCopyrightStatus(t *testing.T) {
	models := []model.Model{{Slug: "a/model", Copyright: "non_compliant"}}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshot(models, "2026-08-08").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Models["a/model"].Copyright; got != "non_compliant" {
		t.Errorf("Copyright = %q, want non_compliant", got)
	}
}

func TestSnapshotRoundTripsFullScoreProvenance(t *testing.T) {
	info := &model.ScoreInfo{Metric: "LMArena Elo", Value: 1453, Unit: "Elo", SourceFamily: model.ScoreSourceArena, ConfiguredIdentity: "hy3-tencent-cloud-text", CanonicalID: "hy3-tencent-cloud-text", VariantMeasured: "hy3", SourceURL: "u", Checked: "2026-08-06", IdentityStatus: model.IdentityVariantMismatch, Uncertainty: "95% CI 10", SampleSize: "10000", Harness: "arena", Scaffold: "chat", Provider: "Tencent", Configuration: "default"}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshot([]model.Model{{Slug: "a/hy3", ArenaScore: info}}, "2026-08-08").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	got := loaded.Models["a/hy3"].ArenaScore
	if got == nil || got.IdentityStatus != model.IdentityVariantMismatch || got.SourceFamily != model.ScoreSourceArena || got.ConfiguredIdentity != "hy3-tencent-cloud-text" || got.CanonicalID != "hy3-tencent-cloud-text" || got.Uncertainty != "95% CI 10" || got.SampleSize != "10000" || got.Harness != "arena" || got.Scaffold != "chat" || got.Provider != "Tencent" || got.Configuration != "default" {
		t.Fatalf("provenance = %+v, want every supplied provenance field", got)
	}
}

func TestSnapshotRoundTripsCatalogueIdentityForFallback(t *testing.T) {
	models := []model.Model{{Slug: "a/model"}}
	prices := map[string]sources.PriceInfo{"a/model": {Slug: "a/model", CanonicalSlug: "a/model", Provider: "A", ReleaseVariant: "2026-08", ModelVariant: "model", Reasoning: "high", Configuration: "default"}}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshotWithPrices(models, prices, "2026-08-08").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	entry := got.Models["a/model"]
	if entry.CanonicalSlug != "a/model" || entry.Provider != "A" || entry.ReleaseVariant != "2026-08" || entry.ModelVariant != "model" || entry.Reasoning != "high" || entry.Configuration != "default" {
		t.Fatalf("catalog identity = %+v, want all identity fields preserved", entry)
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

// TestSnapshotRoundTripsCatalogueMetadata guards the least obvious hop of
// the whole feature. The TUI never renders a live model.Model: it re-reads
// this snapshot from disk and rebuilds every row from it. A field that
// model.Model has and SnapshotEntry lacks is therefore invisible on
// screen no matter how correct the rest of the pipeline is — the local
// sources/model tests stay green while the detail screen shows a
// permanent н/д.
func TestSnapshotRoundTripsCatalogueMetadata(t *testing.T) {
	models := []model.Model{
		{Slug: "a/dated", InPerM: 1, OutPerM: 3, Context: 1000, Created: 1786034890, Description: "A dated model.", CatalogName: "Qwen: Qwen3.8 Max", Provider: "Qwen", License: "Apache-2.0", ModelURL: "https://arena.ai/models/qwen", MetadataSourceURL: "https://arena.ai/leaderboard/text"},
		{Slug: "a/bare", InPerM: 1, OutPerM: 3, Context: 1000},
	}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshot(models, "2026-08-08").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	dated := loaded.Models["a/dated"]
	if dated.Created != 1786034890 || dated.Description != "A dated model." {
		t.Errorf("dated = %+v, want the catalogue metadata to survive the disk round trip", dated)
	}
	if dated.CatalogName != "Qwen: Qwen3.8 Max" || dated.Provider != "Qwen" || dated.License != "Apache-2.0" || dated.ModelURL != "https://arena.ai/models/qwen" || dated.MetadataSourceURL != "https://arena.ai/leaderboard/text" {
		t.Errorf("dated metadata = %+v, want catalogue name and sourced Arena metadata", dated)
	}
	bare := loaded.Models["a/bare"]
	if bare.Created != 0 || bare.Description != "" {
		t.Errorf("bare = %+v, want zero catalogue metadata", bare)
	}

	body, err := json.Marshal(bare)
	if err != nil {
		t.Fatalf("marshal bare entry: %v", err)
	}
	if strings.Contains(string(body), "created") || strings.Contains(string(body), "description") {
		t.Errorf("bare entry = %s, want omitempty to keep absent catalogue metadata out of the snapshot", body)
	}
}

// TestSnapshotRoundTripsTheLinkIdentifiers is the disk hop of the link
// pipeline. The TUI never renders a live model.Model: it re-reads this
// snapshot and rebuilds every row from it, so a field model.Model has and
// SnapshotEntry lacks is invisible on screen however correct the rest of
// the pipeline is. The marshalling half of the test is not decoration
// either: TestNewSnapshot compares a serialised entry byte for byte, so
// a new field without omitempty breaks it for every model that has none.
func TestSnapshotRoundTripsTheLinkIdentifiers(t *testing.T) {
	models := []model.Model{
		{Slug: "a/linked", InPerM: 1, OutPerM: 3, Context: 1000, CanonicalSlug: "a/linked-20260804", HuggingFaceID: "a-labs/Linked"},
		{Slug: "a/bare", InPerM: 1, OutPerM: 3, Context: 1000},
	}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := NewSnapshot(models, "2026-08-09").Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	linked := loaded.Models["a/linked"]
	if linked.CanonicalSlug != "a/linked-20260804" || linked.HuggingFaceID != "a-labs/Linked" {
		t.Errorf("linked = %+v, want both link identifiers to survive the disk round trip", linked)
	}
	bare := loaded.Models["a/bare"]
	if bare.CanonicalSlug != "" || bare.HuggingFaceID != "" {
		t.Errorf("bare = %+v, want empty link identifiers", bare)
	}

	body, err := json.Marshal(bare)
	if err != nil {
		t.Fatalf("marshal bare entry: %v", err)
	}
	if strings.Contains(string(body), "canonical_slug") || strings.Contains(string(body), "hugging_face_id") {
		t.Errorf("bare entry = %s, want omitempty to keep absent link identifiers out of the snapshot", body)
	}
}

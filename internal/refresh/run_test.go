package refresh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

const testModelMap = "openai/gpt-5.6-luna\ttier=opus\tvals=openai/gpt-5.6-luna\n" +
	"minimax/minimax-m3\ttier=sonnet\n"

const testNotesYAML = `updated_note: "автоматический прогон"
sections:
  favorites_intro: "интро"
  safety_intro: "безопасность"
  saferai: "saferai"
  open_weights: "веса"
  tiers_intro: "тиры"
  tokens10_intro: "токены"
  free_intro: "бесплатные"
  free_terms: "условия"
fable_verdict: "нет кандидата"
claude_note: "цены Claude"
claude_prices: []
claude_tokens: []
companies: []
caveats: ["одна оговорка"]
favorite_reasons:
  opus:
    openai/gpt-5.6-luna: "лучший"
models:
  openai/gpt-5.6-luna:
    display: GPT-5.6 Luna
    owner: OpenAI (C)
    open_weights: "нет"
    note: "заметка"
  minimax/minimax-m3:
    display: MiniMax M3
    owner: MiniMax (н/д)
    open_weights: "да"
    note: "заметка"
    score:
      label: "80.5% (только вендор)"
      value: 80.5
      rankable: true
      source: "https://minimax.io/"
`

// newDataDir builds a project data directory the orchestrator can run against.
func newDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model-map.tsv"), []byte(testModelMap), 0o644); err != nil {
		t.Fatalf("write model-map.tsv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.yaml"), []byte(testNotesYAML), 0o644); err != nil {
		t.Fatalf("write notes.yaml: %v", err)
	}
	return dir
}

func fixedNow() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }

func okDeps() deps {
	return deps{
		prices: func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
			return map[string]sources.PriceInfo{
				"openai/gpt-5.6-luna": {Slug: "openai/gpt-5.6-luna", InPerM: 0.5, OutPerM: 3, Context: 1000000, Found: true},
				"minimax/minimax-m3":  {Slug: "minimax/minimax-m3", InPerM: 0.3, OutPerM: 1.2, Context: 1000000, Found: true},
			}, nil
		},
		catalog: func(ctx context.Context) ([]string, error) {
			return []string{"openai/gpt-5.6-luna", "openai/gpt-5.7-nova", "minimax/minimax-m3"}, nil
		},
		sources: []scoreSource{
			{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return nil, nil
			}},
			{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return []sources.ScoreRow{{
					Slug: "openai/gpt-5.6-luna", Metric: "SWE-bench Verified", Value: 93,
					VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://www.vals.ai/benchmarks/swebench",
					Checked: "2026-08-03",
				}}, nil
			}},
		},
		now: fixedNow,
	}
}

func TestRunWritesDocumentAndSnapshot(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "docs", "openrouter-model-comparison.md")

	report, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, okDeps())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(report.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none on a fully successful run", report.Warnings)
	}
	if len(report.NewCandidates) != 1 || report.NewCandidates[0] != "openai/gpt-5.7-nova" {
		t.Errorf("NewCandidates = %v", report.NewCandidates)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	doc := string(body)
	for _, want := range []string{
		"Обновлено: 2026-08-04 (автоматический прогон)",
		"| GPT-5.6 Luna | openai/gpt-5.6-luna | $0.50 | $3.00 | 1M | 93.0% | 82.7 |",
		"| MiniMax M3 | minimax/minimax-m3 | $0.30 | $1.20 | 1M | 80.5% (только вендор) | 153 |",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("output does not contain %q\n---\n%s", want, doc)
		}
	}

	snap, err := LoadSnapshot(filepath.Join(dir, "cache", "last-run-snapshot.json"))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if snap.FetchedAt != "2026-08-04" || len(snap.Models) != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if s := snap.Models["openai/gpt-5.6-luna"].Score; s == nil || s.Value != 93 {
		t.Errorf("snapshot did not record luna's score: %+v", snap.Models["openai/gpt-5.6-luna"])
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")

	if _, err := run(context.Background(), Options{DataDir: dir, OutputPath: out, DryRun: true}, okDeps()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--dry-run wrote %s, want it untouched", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "last-run-snapshot.json")); !errors.Is(err, os.ErrNotExist) {
		t.Error("--dry-run overwrote the snapshot, want it untouched")
	}
}

func TestRunFallsBackToSnapshotWhenEverythingFails(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")

	seed := &Snapshot{
		FetchedAt: "2026-08-01",
		Models: map[string]SnapshotEntry{
			"openai/gpt-5.6-luna": {
				InPerM: 0.5, OutPerM: 3, Context: 1000000,
				Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 93, VariantMeasured: "openai/gpt-5.6-luna", Checked: "2026-07-30"},
			},
			"minimax/minimax-m3": {InPerM: 0.3, OutPerM: 1.2, Context: 1000000},
		},
	}
	if err := seed.Save(filepath.Join(dir, "cache", "last-run-snapshot.json")); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	broken := okDeps()
	broken.prices = func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
		return nil, errors.New("catalogue unreachable")
	}
	broken.catalog = func(ctx context.Context) ([]string, error) {
		return nil, errors.New("catalogue unreachable")
	}
	broken.sources = []scoreSource{
		{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
			return nil, errors.New("leaderboard block gone")
		}},
		{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
			return nil, errors.New("astro island gone")
		}},
	}

	report, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, broken)
	if err != nil {
		t.Fatalf("run must not fail when every source is down: %v", err)
	}
	if len(report.Warnings) != 4 {
		t.Errorf("Warnings = %v, want one per failed fetch (prices, catalogue, swebench, vals)", report.Warnings)
	}
	if len(report.Retired) != 0 {
		t.Errorf("Retired = %v, want none — an unreachable catalogue is not a retirement", report.Retired)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the document must still be written: %v", err)
	}
	doc := string(body)
	if !strings.Contains(doc, "93.0% (не удалось проверить на 2026-08-04)") {
		t.Errorf("the fallen-back score is not labelled stale:\n%s", doc)
	}
	if !strings.Contains(doc, "Цену не удалось проверить на 2026-08-04") {
		t.Errorf("the fallen-back price is not labelled stale:\n%s", doc)
	}
	if !strings.Contains(doc, "| $0.50 | $3.00 |") {
		t.Errorf("the snapshot price did not make it into the document:\n%s", doc)
	}
	// The vendor-claimed number lives in notes.yaml and cannot go stale.
	if strings.Contains(doc, "80.5% (только вендор) (не удалось проверить") {
		t.Errorf("a notes.yaml score was wrongly labelled stale:\n%s", doc)
	}
}

func TestRunDryRunNeverHardFailsEvenWhenNothingCanBeMerged(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")

	broken := okDeps()
	broken.prices = func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
		return nil, errors.New("catalogue unreachable")
	}
	broken.catalog = func(ctx context.Context) ([]string, error) {
		return nil, errors.New("catalogue unreachable")
	}
	broken.sources = []scoreSource{
		{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
			return nil, errors.New("leaderboard block gone")
		}},
		{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
			return nil, errors.New("astro island gone")
		}},
	}

	// No snapshot seeded — Fix 1's "Merge would produce zero models" guard
	// (and the pre-existing missing-price-coverage guard) would both fire on
	// a real refresh. `check` (--dry-run) must still report, not hard-fail.
	report, err := run(context.Background(), Options{DataDir: dir, OutputPath: out, DryRun: true}, broken)
	if err != nil {
		t.Fatalf("run must not hard-fail on --dry-run even when nothing can be merged: %v", err)
	}
	if len(report.Warnings) != 4 {
		t.Errorf("Warnings = %v, want one per failed fetch (prices, catalogue, swebench, vals)", report.Warnings)
	}
	if _, statErr := os.Stat(out); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("--dry-run must never write the document")
	}
}

func TestRunRefusesToWriteWhenPricesFailWithNoSnapshotFallback(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")

	broken := okDeps()
	broken.prices = func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
		return nil, errors.New("catalogue unreachable")
	}

	_, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, broken)
	if err == nil {
		t.Fatal("run must return an error when prices fail and a tracked model has no snapshot fallback")
	}
	if _, statErr := os.Stat(out); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("run must not write the document when refusing due to missing price coverage")
	}
}

func TestRunRefusesToWriteWhenMergeProducesZeroModels(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")

	broken := okDeps()
	// Prices succeeds as a whole (pricesOK == true) but reports every tracked
	// slug as gone — the "OpenRouter renamed its slug scheme" scenario the
	// !pricesOK guard alone does not catch.
	broken.prices = func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
		out := make(map[string]sources.PriceInfo, len(slugs))
		for _, slug := range slugs {
			out[slug] = sources.PriceInfo{Slug: slug, Found: false}
		}
		return out, nil
	}

	_, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, broken)
	if err == nil {
		t.Fatal("run must return an error when Merge would produce zero models")
	}
	if _, statErr := os.Stat(out); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("run must not write the document when every tracked model has no usable price data")
	}
}

// loadTestNotes builds a *notes.Notes from inline YAML, for tests that call
// applyFallback directly and need a notes.Notes without going through run().
func loadTestNotes(t *testing.T, yamlContent string) *notes.Notes {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notes.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write notes.yaml: %v", err)
	}
	nt, err := notes.Load(path)
	if err != nil {
		t.Fatalf("notes.Load: %v", err)
	}
	return nt
}

func TestApplyFallbackLeavesLiveNotFoundAlone(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "x-ai/grok-4.1-fast", Tier: "sonnet", Names: map[string]string{"vals": "xai/grok-4.1-fast"}}}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"x-ai/grok-4.1-fast": {InPerM: 0.2, OutPerM: 0.5, Context: 2000000},
	}}
	live := map[string]sources.PriceInfo{"x-ai/grok-4.1-fast": {Slug: "x-ai/grok-4.1-fast"}} // Found == false
	sourceOK := map[string]bool{"vals": false}
	nt := loadTestNotes(t, "{}")

	prices, _, stalePrices, _ := applyFallback(entries, live, true, nil, sourceOK, nt, snap)
	if prices["x-ai/grok-4.1-fast"].Found {
		t.Error("a slug the live catalogue reported as gone was resurrected from the snapshot")
	}
	if len(stalePrices) != 0 {
		t.Errorf("stalePrices = %v, want empty on a successful catalogue fetch", stalePrices)
	}
}

func TestApplyFallbackDoesNotFabricateStaleScoreWhenSourceSucceededButHadNoRow(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "openai/gpt-5.6-luna", Tier: "opus", Names: map[string]string{"vals": "openai/gpt-5.6-luna"}},
	}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"openai/gpt-5.6-luna": {
			Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 93, VariantMeasured: "openai/gpt-5.6-luna", Checked: "2026-07-30"},
		},
	}}
	// vals succeeded this run (no error) but simply returned no row for luna —
	// a genuine "model dropped off the leaderboard or the map entry is
	// misspelled" situation, not a source failure.
	sourceOK := map[string]bool{"vals": true}
	nt := loadTestNotes(t, "{}")
	prices := map[string]sources.PriceInfo{"openai/gpt-5.6-luna": {Slug: "openai/gpt-5.6-luna", Found: true}}

	_, scores, _, staleScores := applyFallback(entries, prices, true, nil, sourceOK, nt, snap)
	for _, r := range scores {
		if r.Slug == "openai/gpt-5.6-luna" {
			t.Fatalf("score was fabricated from the snapshot even though its declared source succeeded this run: %+v", r)
		}
	}
	if staleScores["openai/gpt-5.6-luna"] {
		t.Error("staleScores marked luna stale even though its declared source succeeded — genuine absence, not a failure")
	}
}

func TestApplyFallbackPrefersScoreOverrideOverStaleSnapshot(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "minimax/minimax-m3", Tier: "sonnet", Names: map[string]string{"vals": "minimax/minimax-m3"}},
	}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"minimax/minimax-m3": {
			Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 55.0, VariantMeasured: "minimax/minimax-m3", Checked: "2026-07-01"},
		},
	}}
	// The declared source failed this run — the only case where fallback
	// should even be considered.
	sourceOK := map[string]bool{"vals": false}
	// testNotesYAML carries a manual score override for minimax/minimax-m3
	// labelled "80.5% (только вендор)".
	nt := loadTestNotes(t, testNotesYAML)
	prices := map[string]sources.PriceInfo{"minimax/minimax-m3": {Slug: "minimax/minimax-m3", InPerM: 0.3, OutPerM: 1.2, Found: true}}

	_, scores, _, staleScores := applyFallback(entries, prices, true, nil, sourceOK, nt, snap)
	for _, r := range scores {
		if r.Slug == "minimax/minimax-m3" {
			t.Fatalf("a stale snapshot score was injected even though notes.yaml has a manual override: %+v", r)
		}
	}
	if staleScores["minimax/minimax-m3"] {
		t.Error("staleScores marked minimax stale even though notes.yaml's override should take precedence")
	}

	merged := model.Merge(entries, prices, scores, nt)
	if len(merged) != 1 || merged[0].Score == nil || !strings.Contains(merged[0].ScoreLabel, "только вендор") {
		t.Fatalf("merged model does not carry the notes.yaml override, got: %+v", merged)
	}
}

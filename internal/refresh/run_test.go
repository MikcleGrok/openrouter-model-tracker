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

func TestApplyFallbackLeavesLiveNotFoundAlone(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "x-ai/grok-4.1-fast", Tier: "sonnet", Names: map[string]string{"vals": "xai/grok-4.1-fast"}}}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"x-ai/grok-4.1-fast": {InPerM: 0.2, OutPerM: 0.5, Context: 2000000},
	}}
	live := map[string]sources.PriceInfo{"x-ai/grok-4.1-fast": {Slug: "x-ai/grok-4.1-fast"}} // Found == false

	prices, _, stalePrices, _ := applyFallback(entries, live, true, nil, snap)
	if prices["x-ai/grok-4.1-fast"].Found {
		t.Error("a slug the live catalogue reported as gone was resurrected from the snapshot")
	}
	if len(stalePrices) != 0 {
		t.Errorf("stalePrices = %v, want empty on a successful catalogue fetch", stalePrices)
	}
}

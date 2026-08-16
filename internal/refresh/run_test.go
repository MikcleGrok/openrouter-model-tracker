package refresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
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
		"| MiniMax M3 | minimax/minimax-m3 | $0.30 | $1.20 | 1M | 80.5% (только вендор) | n/a (observation only) |",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("output does not contain %q\n---\n%s", want, doc)
		}
	}

	snap, err := LoadSnapshot(filepath.Join(dir, "model-snapshot.json"))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if snap.FetchedAt != "2026-08-04" || snap.UpdatedAt != "2026-08-04T12:00:00Z" || len(snap.Models) != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if len(snap.CatalogSlugs) != 3 || snap.CatalogSlugs[0] != "openai/gpt-5.6-luna" {
		t.Errorf("snapshot CatalogSlugs = %v, want the complete successful catalogue", snap.CatalogSlugs)
	}
	history, err := pricehistory.Load(pricehistory.Path(dir))
	if err != nil || len(history.Observations) != 1 {
		t.Fatalf("price history = %+v, %v", history, err)
	}
	if s := snap.Models["openai/gpt-5.6-luna"].Score; s == nil || s.Value != 93 {
		t.Errorf("snapshot did not record luna's score: %+v", snap.Models["openai/gpt-5.6-luna"])
	}
}

func TestRunSnapshotPathIsIndependentOfCacheDir(t *testing.T) {
	dir := newDataDir(t)
	custom := filepath.Join(t.TempDir(), "custom-cache")
	out := filepath.Join(t.TempDir(), "report.md")
	if _, err := run(context.Background(), Options{DataDir: dir, OutputPath: out, CacheDir: custom}, okDeps()); err != nil {
		t.Fatalf("run: %v", err)
	}
	// The snapshot is durable, versioned data — it always lands at the data
	// directory root, regardless of where CacheDir points the ephemeral
	// HTTP cache.
	if _, err := LoadSnapshot(filepath.Join(dir, "model-snapshot.json")); err != nil {
		t.Fatalf("snapshot at the tracked data-dir path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(custom, "last-run-snapshot.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot unexpectedly written under the custom cache dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "last-run-snapshot.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot unexpectedly written under the default cache dir: %v", err)
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
	if _, err := os.Stat(filepath.Join(dir, "model-snapshot.json")); !errors.Is(err, os.ErrNotExist) {
		t.Error("--dry-run overwrote the snapshot, want it untouched")
	}
	if _, err := os.Stat(pricehistory.Path(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Error("--dry-run wrote price history, want it untouched")
	}
}

func TestRunDryRunReportsCatalogDeltaWithoutChangingBaseline(t *testing.T) {
	dir := newDataDir(t)
	snapshotPath := filepath.Join(dir, "model-snapshot.json")
	seed := &Snapshot{
		FetchedAt:    "2026-08-03",
		CatalogSlugs: []string{"minimax/minimax-m3", "openai/gpt-5.6-luna"},
		Models: map[string]SnapshotEntry{
			"openai/gpt-5.6-luna": {InPerM: 0.5, OutPerM: 3, Context: 1000000},
			"minimax/minimax-m3":  {InPerM: 0.3, OutPerM: 1.2, Context: 1000000},
		},
	}
	if err := seed.Save(snapshotPath); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	first, err := run(context.Background(), Options{DataDir: dir, OutputPath: filepath.Join(t.TempDir(), "doc.md"), DryRun: true}, okDeps())
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if len(first.CatalogAdded) != 1 || first.CatalogAdded[0] != "openai/gpt-5.7-nova" {
		t.Errorf("first CatalogAdded = %v", first.CatalogAdded)
	}
	if len(first.CatalogRemoved) != 0 {
		t.Errorf("first CatalogRemoved = %v, want none", first.CatalogRemoved)
	}

	second, err := run(context.Background(), Options{DataDir: dir, OutputPath: filepath.Join(t.TempDir(), "doc.md"), DryRun: true}, okDeps())
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if len(second.CatalogAdded) != 1 || second.CatalogAdded[0] != "openai/gpt-5.7-nova" {
		t.Errorf("second CatalogAdded = %v, want the same delta while baseline is unchanged", second.CatalogAdded)
	}
	got, err := LoadSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("load unchanged snapshot: %v", err)
	}
	if got.FetchedAt != seed.FetchedAt || len(got.CatalogSlugs) != len(seed.CatalogSlugs) {
		t.Errorf("dry-run changed baseline: got %+v, want %+v", got, seed)
	}
}

func TestRunWithoutCatalogBaselineDoesNotReportWholeCatalogAsNew(t *testing.T) {
	dir := newDataDir(t)
	report, err := run(context.Background(), Options{DataDir: dir, OutputPath: filepath.Join(t.TempDir(), "doc.md"), DryRun: true}, okDeps())
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if len(report.CatalogAdded) != 0 || len(report.CatalogRemoved) != 0 {
		t.Fatalf("first check catalogue delta = %v/%v, want no false positives", report.CatalogAdded, report.CatalogRemoved)
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
				Created: 1700000000, Description: "OpenAI's long-context flagship, strong at code.",
				CanonicalSlug: "openai/gpt-5.6-luna-20260804", HuggingFaceID: "openai-community/gpt-5-6-luna",
				HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 1, OverrideOutPerM: 4,
				Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 93, VariantMeasured: "openai/gpt-5.6-luna", Checked: "2026-07-30"},
			},
			"minimax/minimax-m3": {InPerM: 0.3, OutPerM: 1.2, Context: 1000000},
		},
	}
	if err := seed.Save(filepath.Join(dir, "model-snapshot.json")); err != nil {
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
	if !strings.Contains(doc, "$0.50") || !strings.Contains(doc, "$3.00") {
		t.Errorf("the snapshot price did not make it into the document:\n%s", doc)
	}
	if !strings.Contains(doc, "$1.00 от 500K+") || !strings.Contains(doc, "$4.00 от 500K+") {
		t.Errorf("the snapshot long-context override did not make it into the document:\n%s", doc)
	}
	// Created/Description are not rendered into the markdown document — they
	// only surface on the TUI detail screen — so their fallback survival is
	// checked in the written snapshot, the other artifact this run produces.
	newSnap, err := LoadSnapshot(filepath.Join(dir, "model-snapshot.json"))
	if err != nil {
		t.Fatalf("reload the written snapshot: %v", err)
	}
	if got := newSnap.Models["openai/gpt-5.6-luna"].Created; got != 1700000000 {
		t.Errorf("Created = %d, want the snapshot's fallback value 1700000000 to survive applyFallback", got)
	}
	if got := newSnap.Models["openai/gpt-5.6-luna"].Description; got != "OpenAI's long-context flagship, strong at code." {
		t.Errorf("Description = %q, want the snapshot's fallback value to survive applyFallback", got)
	}
	// The link identifiers are not rendered into the markdown document
	// either — they only surface on the TUI detail screen — so their
	// survival is checked in the written snapshot, exactly like
	// Created/Description above. This assertion is the guard against
	// repeating the bug a review already caught once on this very
	// function: a single failed catalogue fetch zeroing a catalogue field
	// for every model on disk, silently, until the next successful refresh.
	if got := newSnap.Models["openai/gpt-5.6-luna"].CanonicalSlug; got != "openai/gpt-5.6-luna-20260804" {
		t.Errorf("CanonicalSlug = %q, want the snapshot's fallback value to survive applyFallback", got)
	}
	if got := newSnap.Models["openai/gpt-5.6-luna"].HuggingFaceID; got != "openai-community/gpt-5-6-luna" {
		t.Errorf("HuggingFaceID = %q, want the snapshot's fallback value to survive applyFallback", got)
	}
	// The live score remains stale-visible even when its identity metadata is complete.
	if !strings.Contains(doc, "93.0% (не удалось проверить") {
		t.Errorf("the snapshot score was not labelled stale:\n%s", doc)
	}
	if _, statErr := os.Stat(pricehistory.Path(dir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("fallback prices wrote price history")
	}
	if !sort.StringsAreSorted(report.Warnings) {
		t.Errorf("Warnings = %v, want deterministic sorted order", report.Warnings)
	}
}

func TestRunWarningsHaveDeterministicOrder(t *testing.T) {
	dir := newDataDir(t)
	broken := okDeps()
	broken.prices = func(context.Context, []string) (map[string]sources.PriceInfo, error) {
		return nil, errors.New("prices down")
	}
	broken.catalog = func(context.Context) ([]string, error) { return nil, errors.New("catalog down") }
	broken.sources = []scoreSource{
		{id: "vals", fn: func(context.Context, map[string]string) ([]sources.ScoreRow, error) {
			return nil, errors.New("vals down")
		}},
		{id: "swebench", fn: func(context.Context, map[string]string) ([]sources.ScoreRow, error) {
			return nil, errors.New("swebench down")
		}},
	}
	report, err := run(context.Background(), Options{DataDir: dir, OutputPath: filepath.Join(t.TempDir(), "doc.md"), DryRun: true}, broken)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []string{
		"openrouter: каталог не получен (catalog down) — новые и снятые модели в этом прогоне не искались",
		"openrouter: цены не получены (prices down) — берутся из снимка прошлого прогона",
		"swebench: источник недоступен или изменил структуру (swebench down) — оценки берутся из снимка",
		"vals: источник недоступен или изменил структуру (vals down) — оценки берутся из снимка",
	}
	if strings.Join(report.Warnings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Warnings = %v, want %v", report.Warnings, want)
	}
}

func TestRunHistoryFailureDoesNotAdvanceDocumentOrSnapshot(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")
	oldDoc := "old document\n"
	if err := os.WriteFile(out, []byte(oldDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(dir, "model-snapshot.json")
	seed := &Snapshot{FetchedAt: "2026-08-03", Models: map[string]SnapshotEntry{"openai/gpt-5.6-luna": {InPerM: 0.4, OutPerM: 2, Context: 900000}}}
	if err := seed.Save(snapshotPath); err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	d := okDeps()
	d.saveHistory = func(*pricehistory.History, string) error { return errors.New("history storage unavailable") }
	if _, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, d); err == nil {
		t.Fatal("run unexpectedly succeeded when history persistence failed")
	}
	afterDoc, err := os.ReadFile(out)
	if err != nil || string(afterDoc) != oldDoc {
		t.Fatalf("document advanced after history failure: %q, %v", afterDoc, err)
	}
	afterSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil || string(afterSnapshot) != string(beforeSnapshot) {
		t.Fatalf("snapshot advanced after history failure: %q, %v", afterSnapshot, err)
	}
}

func TestRunPublishFailureRollsBackAllFiles(t *testing.T) {
	for _, target := range []string{"document", "snapshot"} {
		t.Run(target, func(t *testing.T) {
			dir := newDataDir(t)
			out := filepath.Join(t.TempDir(), "openrouter-model-comparison.md")
			snapshotPath := filepath.Join(dir, "model-snapshot.json")
			historyPath := pricehistory.Path(dir)
			oldDoc := "old document\n"
			if err := os.WriteFile(out, []byte(oldDoc), 0o644); err != nil {
				t.Fatal(err)
			}
			seed := &Snapshot{FetchedAt: "2026-08-03", Models: map[string]SnapshotEntry{"openai/gpt-5.6-luna": {InPerM: 0.4, OutPerM: 2, Context: 900000}}}
			if err := seed.Save(snapshotPath); err != nil {
				t.Fatal(err)
			}
			oldSnapshot, err := os.ReadFile(snapshotPath)
			if err != nil {
				t.Fatal(err)
			}
			oldHistory := &pricehistory.History{SchemaVersion: pricehistory.SchemaVersion, Observations: []pricehistory.Observation{{ObservedAt: fixedNow().Add(-24 * time.Hour), Prices: map[string]pricehistory.Price{"openai/gpt-5.6-luna": {Found: true, InPerM: 0.4, OutPerM: 2, Context: 900000}}}}}
			if err := oldHistory.Save(historyPath); err != nil {
				t.Fatal(err)
			}
			oldHistoryBytes, err := os.ReadFile(historyPath)
			if err != nil {
				t.Fatal(err)
			}

			d := okDeps()
			failPath := out
			if target == "snapshot" {
				failPath = snapshotPath
			}
			d.rename = func(oldPath, newPath string) error {
				base := filepath.Base(oldPath)
				if newPath == failPath && strings.HasPrefix(base, ".refresh-") && !strings.HasPrefix(base, ".refresh-backup-") {
					return errors.New("publish target unavailable")
				}
				return os.Rename(oldPath, newPath)
			}
			if _, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, d); err == nil {
				t.Fatal("run unexpectedly succeeded")
			}
			gotDoc, err := os.ReadFile(out)
			if err != nil || string(gotDoc) != oldDoc {
				t.Fatalf("document changed after %s publish failure: %q, %v", target, gotDoc, err)
			}
			gotSnapshot, err := os.ReadFile(snapshotPath)
			if err != nil || string(gotSnapshot) != string(oldSnapshot) {
				t.Fatalf("snapshot changed after %s publish failure: %q, %v", target, gotSnapshot, err)
			}
			gotHistory, err := os.ReadFile(historyPath)
			if err != nil || string(gotHistory) != string(oldHistoryBytes) {
				t.Fatalf("history changed after %s publish failure: %q, %v", target, gotHistory, err)
			}
		})
	}
}

func TestPublishRollbackAttemptsEveryTargetAfterRestoreFailure(t *testing.T) {
	dir := t.TempDir()
	files := make([]publishFile, 3)
	for i := range files {
		path := filepath.Join(dir, fmt.Sprintf("target-%d", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("old-%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		files[i] = publishFile{path: path, data: []byte(fmt.Sprintf("new-%d", i))}
	}
	var restored []string
	rename := func(oldPath, newPath string) error {
		if strings.HasPrefix(filepath.Base(oldPath), ".refresh-backup-") {
			restored = append(restored, newPath)
			if strings.HasSuffix(newPath, "target-1") {
				return errors.New("restore target-1 unavailable")
			}
		}
		if strings.HasSuffix(newPath, "target-2") && strings.HasPrefix(filepath.Base(oldPath), ".refresh-") && !strings.HasPrefix(filepath.Base(oldPath), ".refresh-backup-") {
			return errors.New("publish target-2 unavailable")
		}
		return os.Rename(oldPath, newPath)
	}
	err := publish(files, rename, os.Remove)
	if err == nil || !strings.Contains(err.Error(), "restore target-1 unavailable") {
		t.Fatalf("publish error = %v, want the rollback restore failure", err)
	}
	if len(restored) != 3 {
		t.Fatalf("restored targets = %v, want all three targets attempted", restored)
	}
	if !strings.Contains(strings.Join(restored, "\n"), "target-0") || !strings.Contains(strings.Join(restored, "\n"), "target-2") {
		t.Fatalf("restored targets = %v, want target-0 and target-2", restored)
	}
}

func TestPublishReportsBackupCleanupFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	remove := func(path string) error {
		if strings.HasPrefix(filepath.Base(path), ".refresh-backup-") {
			return errors.New("backup cleanup unavailable")
		}
		return os.Remove(path)
	}
	err := publish([]publishFile{{path: path, data: []byte("new")}}, os.Rename, remove)
	if err == nil || !strings.Contains(err.Error(), "cleanup backup") || !strings.Contains(err.Error(), "backup cleanup unavailable") {
		t.Fatalf("publish error = %v, want explicit backup cleanup failure", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "new" {
		t.Fatalf("published target = %q, %v, want new content", got, readErr)
	}
}

func TestPublishRollbackCleansPreparedFutureTargets(t *testing.T) {
	dir := t.TempDir()
	files := make([]publishFile, 3)
	for i := range files {
		path := filepath.Join(dir, fmt.Sprintf("target-%d", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("old-%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		files[i] = publishFile{path: path, data: []byte(fmt.Sprintf("new-%d", i))}
	}
	rename := func(oldPath, newPath string) error {
		if strings.HasSuffix(newPath, "target-1") && strings.HasPrefix(filepath.Base(oldPath), ".refresh-") && !strings.HasPrefix(filepath.Base(oldPath), ".refresh-backup-") {
			return errors.New("publish target-1 unavailable")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := publish(files, rename, os.Remove); err == nil {
		t.Fatal("publish unexpectedly succeeded")
	}
	for i := range files {
		matches, err := filepath.Glob(filepath.Join(dir, ".refresh-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("temporary or backup files remain after target %d failure: %v", i, matches)
		}
	}
}

func TestPublishReportsPlaceholderBackupCleanupFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	remove := func(path string) error {
		if strings.HasPrefix(filepath.Base(path), ".refresh-backup-") {
			return errors.New("placeholder cleanup unavailable")
		}
		return os.Remove(path)
	}
	err := publish([]publishFile{{path: path, data: []byte("new")}}, os.Rename, remove)
	if err == nil || !strings.Contains(err.Error(), "cleanup backup placeholder") || !strings.Contains(err.Error(), "placeholder cleanup unavailable") {
		t.Fatalf("publish error = %v, want placeholder cleanup failure", err)
	}
}

func TestPublishAggregatesPreparationCleanupFailure(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	remove := func(path string) error {
		if strings.HasPrefix(filepath.Base(path), ".refresh-") {
			return errors.New("temporary cleanup unavailable")
		}
		return os.Remove(path)
	}
	err := publish([]publishFile{{path: first, data: []byte("first")}, {path: second, save: func(string) error { return errors.New("save unavailable") }}}, os.Rename, remove)
	if err == nil || !strings.Contains(err.Error(), "save unavailable") || !strings.Contains(err.Error(), "temporary cleanup unavailable") {
		t.Fatalf("publish error = %v, want source and cleanup errors", err)
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
	if _, statErr := os.Stat(pricehistory.Path(dir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("failed price lookup wrote price history")
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

func TestApplyFallbackKeepsMissingIdentityConservativeAndPreservesCatalogIdentity(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/model", Tier: "sonnet", Names: map[string]string{"vals": "a/model"}}}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"a/model": {
			CanonicalSlug: "a/model", Provider: "A", ReleaseVariant: "2026-08", ModelVariant: "model", Reasoning: "high", Configuration: "default",
			Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 80, VariantMeasured: "a/model", IdentityStatus: model.IdentityExact},
		},
	}}
	nt := loadTestNotes(t, "{}")
	prices, scores, _, stale := applyFallback(entries, map[string]sources.PriceInfo{}, false, nil, map[string]bool{"vals": false}, nt, snap)
	if !stale["a/model"] || len(scores) != 1 {
		t.Fatalf("fallback = prices=%+v scores=%+v stale=%v, want one stale score", prices, scores, stale)
	}
	got := model.Merge(entries, prices, scores, nt)[0]
	if got.Score.IdentityStatus != model.IdentityExact || !got.Rankable {
		t.Fatalf("preserved catalog identity was not revalidated: score=%+v rankable=%v", got.Score, got.Rankable)
	}

	snap.Models["a/model"] = SnapshotEntry{Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 80, VariantMeasured: "a/model", IdentityStatus: model.IdentityExact}}
	_, scores, _, _ = applyFallback(entries, map[string]sources.PriceInfo{"a/model": {Slug: "a/model", Found: true}}, true, nil, map[string]bool{"vals": false}, nt, snap)
	if len(scores) != 1 || scores[0].IdentityStatus != model.IdentityLegacyUnknown {
		t.Fatalf("missing catalog identity was promoted: %+v", scores)
	}
	got = model.Merge(entries, map[string]sources.PriceInfo{"a/model": {Slug: "a/model", Found: true}}, scores, nt)[0]
	if got.Rankable || got.Score.IdentityStatus != model.IdentityLegacyUnknown {
		t.Fatalf("missing fallback identity became rankable: score=%+v rankable=%v", got.Score, got.Rankable)
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

func TestArenaRowsNeverFillTheSWEBenchColumn(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/only-arena", Tier: "sonnet", Names: map[string]string{"arena": "a-only-arena"}}}
	prices := map[string]sources.PriceInfo{"a/only-arena": {Slug: "a/only-arena", InPerM: 1, OutPerM: 3, Context: 1000, Found: true}}
	arena := []sources.ScoreRow{{Slug: "a/only-arena", Metric: sources.MetricArenaElo, Value: 1400, VariantMeasured: "a-only-arena"}}

	models := model.MergeWithArena(entries, prices, nil, arena, loadTestNotes(t, "{}"))
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if models[0].Score != nil || models[0].ScoreLabel != "n/a" {
		t.Errorf("SWE-bench cell = %+v / %q, want it empty: an Elo is not a SWE-bench percentage", models[0].Score, models[0].ScoreLabel)
	}
}

func TestApplyFallbackIgnoresAnArenaOnlyFailure(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/only-arena", Tier: "sonnet", Names: map[string]string{"arena": "a-only-arena"}}}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"a/only-arena": {InPerM: 1, OutPerM: 3, Context: 1000, Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70}},
	}}
	sourceOK := map[string]bool{"swebench": true, "vals": true, "arena": false}

	_, scores, _, staleScores := applyFallback(entries, map[string]sources.PriceInfo{}, true, nil, sourceOK, loadTestNotes(t, "{}"), snap)
	if len(scores) != 0 || len(staleScores) != 0 {
		t.Errorf("scores = %+v, stale = %v; an Arena outage must never resurrect a SWE-bench number", scores, staleScores)
	}
}

func TestApplyArenaFallbackUsesTheSnapshotWhenArenaIsDown(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "a/down", Tier: "sonnet", Names: map[string]string{"arena": "a-down"}},
		{Slug: "a/absent", Tier: "sonnet", Names: map[string]string{"arena": "a-absent"}},
		{Slug: "a/swe", Tier: "sonnet", Names: map[string]string{"vals": "a/swe"}},
	}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"a/down": {ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1400, VariantMeasured: "a-down", SourceURL: "u", Checked: "2026-08-01"}},
		"a/swe":  {ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1200}},
	}}

	// vals is down too, not just arena: if the family guard inside
	// applyArenaFallback were ever dropped and it fell back to a plain
	// "any declared source failed" check, a/swe (vals= only) would
	// incorrectly qualify for arena-fallback. With vals healthy this test
	// would pass either way, since a/swe would already be excluded on other
	// grounds — see the review that caught this.
	arena, stale := applyArenaFallback(entries, nil, map[string]bool{"vals": false, "arena": false}, snap)
	if len(arena) != 1 || arena[0].Slug != "a/down" || arena[0].Value != 1400 {
		t.Fatalf("arena = %+v, want exactly the snapshot row for a/down", arena)
	}
	if !stale["a/down"] || stale["a/swe"] {
		t.Errorf("stale = %v, want only a/down: a slug whose only declared source is a different family has nothing that could have failed for this column", stale)
	}
}

func TestApplyArenaFallbackKeepsQuietWhenArenaSucceeded(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/gone", Tier: "sonnet", Names: map[string]string{"arena": "a-gone"}}}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"a/gone": {ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1400}},
	}}
	arena, stale := applyArenaFallback(entries, nil, map[string]bool{"arena": true}, snap)
	if len(arena) != 0 || len(stale) != 0 {
		t.Errorf("arena = %+v, stale = %v; a model that simply dropped off the leaderboard is a genuine absence, not a failure", arena, stale)
	}
}

// TestDualFamilyFallbacksActIndependently covers the most production-
// representative shape neither fallback test above exercises: a single
// model-map.tsv row that declares both a SWE-bench-family token (vals=) and
// an arena= token, in a run where exactly one family's source failed and the
// other succeeded. 20 of the real rows in model-map.tsv have this dual
// shape today. applyFallback and applyArenaFallback must each look only at
// their own family's sourceOK entries — a failure in one family must never
// leak a stale (or a live) value into the other family's column for the
// same slug.
func TestDualFamilyFallbacksActIndependently(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "dual/model", Tier: "sonnet", Names: map[string]string{"vals": "dual/model", "arena": "dual-model-arena"}},
	}
	snap := &Snapshot{Models: map[string]SnapshotEntry{
		"dual/model": {
			Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 61, VariantMeasured: "dual/model", Checked: "2026-07-20"},
			ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1420, VariantMeasured: "dual-model-arena", Checked: "2026-07-20"},
		},
	}}
	prices := map[string]sources.PriceInfo{"dual/model": {Slug: "dual/model", InPerM: 1, OutPerM: 3, Found: true}}
	nt := loadTestNotes(t, "{}")

	t.Run("vals fails, arena succeeds", func(t *testing.T) {
		sourceOK := map[string]bool{"vals": false, "arena": true}

		// SWE-bench family: vals failed this run and nothing was fetched for
		// this slug — must fall back to the snapshot's Score and be marked
		// stale.
		_, scores, _, staleScores := applyFallback(entries, prices, true, nil, sourceOK, nt, snap)
		if len(scores) != 1 || scores[0].Slug != "dual/model" || scores[0].Value != 61 {
			t.Fatalf("scores = %+v, want the snapshot's SWE-bench row for dual/model", scores)
		}
		if !staleScores["dual/model"] {
			t.Error("staleScores does not mark dual/model stale even though its only SWE-bench-family source (vals) failed")
		}

		// Arena family: arena succeeded this run and already produced a live
		// row — applyArenaFallback must leave it untouched, not overwrite it
		// with the (older) snapshot value.
		liveArena := []sources.ScoreRow{{Slug: "dual/model", Metric: sources.MetricArenaElo, Value: 1500, VariantMeasured: "dual-model-arena"}}
		arena, staleArena := applyArenaFallback(entries, liveArena, sourceOK, snap)
		if len(arena) != 1 || arena[0].Value != 1500 {
			t.Fatalf("arena = %+v, want the live row (1500) untouched, not the stale snapshot value (1420)", arena)
		}
		if len(staleArena) != 0 {
			t.Errorf("staleArena = %v, want empty: arena succeeded this run for dual/model", staleArena)
		}
	})

	t.Run("arena fails, vals succeeds", func(t *testing.T) {
		sourceOK := map[string]bool{"vals": true, "arena": false}

		// SWE-bench family: vals succeeded this run and already produced a
		// live row — applyFallback must leave it untouched.
		liveScores := []sources.ScoreRow{{Slug: "dual/model", Metric: "SWE-bench Verified", Value: 70, VariantMeasured: "dual/model"}}
		_, scores, _, staleScores := applyFallback(entries, prices, true, liveScores, sourceOK, nt, snap)
		if len(scores) != 1 || scores[0].Value != 70 {
			t.Fatalf("scores = %+v, want the live row (70) untouched, not the stale snapshot value (61)", scores)
		}
		if len(staleScores) != 0 {
			t.Errorf("staleScores = %v, want empty: vals succeeded this run for dual/model", staleScores)
		}

		// Arena family: arena failed this run and nothing was fetched for
		// this slug — must fall back to the snapshot's ArenaScore and be
		// marked stale.
		arena, staleArena := applyArenaFallback(entries, nil, sourceOK, snap)
		if len(arena) != 1 || arena[0].Slug != "dual/model" || arena[0].Value != 1420 {
			t.Fatalf("arena = %+v, want the snapshot's Arena row for dual/model", arena)
		}
		if !staleArena["dual/model"] {
			t.Error("staleArena does not mark dual/model stale even though its only Arena-family source (arena) failed")
		}
	})
}

func TestArenaFallbackKeepsArenaProviderWhenSnapshotTopLevelProviderIsEmpty(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "openai/gpt", Names: map[string]string{"arena": "openai-gpt"}}}
	snap := &Snapshot{Models: map[string]SnapshotEntry{"openai/gpt": {ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1453, Provider: "OpenAI"}}}}
	arena, _ := applyArenaFallback(entries, nil, map[string]bool{"arena": false}, snap)
	if len(arena) != 1 || arena[0].Provider != "OpenAI" {
		t.Fatalf("Arena fallback = %+v, want Arena organization OpenAI", arena)
	}
}

func TestMarkStaleLabelsTheArenaColumn(t *testing.T) {
	models := []model.Model{{
		Slug:       "a/down",
		ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1400},
		ArenaLabel: "1400 Elo",
	}}
	markStale(models, nil, nil, map[string]bool{"a/down": true}, "2026-08-08")
	if !models[0].ArenaScore.Stale {
		t.Error("ArenaScore.Stale is false, want true")
	}
	if models[0].ArenaLabel != "1400 Elo (не удалось проверить на 2026-08-08)" {
		t.Errorf("ArenaLabel = %q, want the staleness suffix", models[0].ArenaLabel)
	}
}

// TestLiveDepsSourcesAllHaveASourceFamily guards the registration in liveDeps
// against a source id with no matching model.SourceFamily entry. Such an id
// would fall into the split's default branch — its rows dropped rather than
// silently mistaken for the SWE-bench family — but the safer failure is to
// catch the missing registration here, at test time, before it ever reaches
// that runtime default.
func TestLiveDepsSourcesAllHaveASourceFamily(t *testing.T) {
	d := liveDeps(Options{DataDir: t.TempDir()})
	if len(d.sources) == 0 {
		t.Fatal("liveDeps registered no sources")
	}
	for _, s := range d.sources {
		if model.SourceFamily[s.id] == "" {
			t.Errorf("liveDeps source id %q has no model.SourceFamily entry — its rows would be silently dropped by the family split instead of feeding either view", s.id)
		}
	}
}

// TestLiveDepsPrefersValsOverSWEBench pins down the ordering half of the
// source-priority fix: vals.ai (independent, one fixed harness) must be
// queried before swebench.com (self-submitted, median across scaffolds) so
// that model.MergeWithArena's selectRow, which walks a slug's rows in this
// order and takes the first one whose identity classifies as exact_product,
// prefers the vals.ai number whenever both sources have a usable row for the
// same model.
func TestLiveDepsPrefersValsOverSWEBench(t *testing.T) {
	d := liveDeps(Options{DataDir: t.TempDir()})
	valsIdx, sweIdx := -1, -1
	for i, s := range d.sources {
		switch s.id {
		case "vals":
			valsIdx = i
		case "swebench":
			sweIdx = i
		}
	}
	if valsIdx == -1 || sweIdx == -1 {
		t.Fatalf("liveDeps sources = %+v, want both vals and swebench registered", d.sources)
	}
	if valsIdx >= sweIdx {
		t.Errorf("vals is at index %d and swebench at index %d, want vals before swebench — the independently-measured source must win the shared-slug merge, not the self-submitted one", valsIdx, sweIdx)
	}
}

// TestRunPrefersValsOverSWEBenchForSharedSlug is the end-to-end companion to
// TestLiveDepsPrefersValsOverSWEBench: with sources wired in the real
// (vals-then-swebench) priority order and both returning a row for the same
// slug, the published snapshot must carry the vals.ai number.
func TestRunPrefersValsOverSWEBenchForSharedSlug(t *testing.T) {
	dir := newDataDir(t)
	out := filepath.Join(t.TempDir(), "doc.md")

	d := okDeps()
	d.sources = []scoreSource{
		// Both rows are made identity-valid on purpose (SourceFamily +
		// ConfiguredIdentity set so each independently classifies as
		// exact_product): the point of this test is that vals.ai wins by
		// *priority order*, not merely because it is the only usable
		// candidate — see TestFlaggedTopSourceFallsThroughToTheNextSource in
		// the model package for the "only one is valid" fallback case.
		{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
			return []sources.ScoreRow{{
				Slug: "openai/gpt-5.6-luna", SourceFamily: "vals", ConfiguredIdentity: "openai/gpt-5.6-luna",
				Metric: "SWE-bench Verified", Value: 93,
				VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://www.vals.ai/benchmarks/swebench",
			}}, nil
		}},
		{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
			return []sources.ScoreRow{{
				Slug: "openai/gpt-5.6-luna", SourceFamily: "swebench", ConfiguredIdentity: "Model: gpt-5.6-luna",
				Metric: "SWE-bench Verified", Value: 79.2,
				VariantMeasured: "OpenHands + GPT-5.6 Luna", SourceURL: "https://www.swebench.com/",
			}}, nil
		}},
	}

	if _, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, d); err != nil {
		t.Fatalf("run: %v", err)
	}

	snap, err := LoadSnapshot(filepath.Join(dir, "model-snapshot.json"))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	entry, ok := snap.Models["openai/gpt-5.6-luna"]
	if !ok || entry.Score == nil {
		t.Fatalf("snapshot entry for openai/gpt-5.6-luna = %+v, want a score", entry)
	}
	if entry.Score.Value != 93 {
		t.Errorf("Score.Value = %v, want 93 (vals.ai) — swebench.com must lose the shared-slug merge, not win it", entry.Score.Value)
	}
	if entry.Score.SourceURL != "https://www.vals.ai/benchmarks/swebench" {
		t.Errorf("Score.SourceURL = %q, want the vals.ai URL", entry.Score.SourceURL)
	}
}

// TestRunSplitsArenaRowsFromSWEBenchRows is a run-level test, exercising the
// family split with all three real source ids wired through a full run(),
// not just model.MergeWithArena directly (that invariant is already covered
// by TestArenaRowsNeverFillTheSWEBenchColumn). The split lives inline in
// run(), between the fetch goroutines and applyFallback/applyArenaFallback —
// deleting it (reverting to one shared, unconditionally-concatenated slice)
// must make this test fail even though every other test in the package
// stays green.
func TestRunSplitsArenaRowsFromSWEBenchRows(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model-map.tsv"), []byte("a/only-arena\ttier=sonnet\tarena=a-only-arena\n"), 0o644); err != nil {
		t.Fatalf("write model-map.tsv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write notes.yaml: %v", err)
	}
	out := filepath.Join(t.TempDir(), "doc.md")

	d := deps{
		prices: func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
			return map[string]sources.PriceInfo{"a/only-arena": {Slug: "a/only-arena", InPerM: 1, OutPerM: 3, Context: 1000, Found: true}}, nil
		},
		catalog: func(ctx context.Context) ([]string, error) { return []string{"a/only-arena"}, nil },
		sources: []scoreSource{
			{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) { return nil, nil }},
			{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) { return nil, nil }},
			{id: "arena", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return []sources.ScoreRow{{Slug: "a/only-arena", Metric: sources.MetricArenaElo, Value: 1400, VariantMeasured: "a-only-arena"}}, nil
			}},
		},
		now: fixedNow,
	}

	if _, err := run(context.Background(), Options{DataDir: dir, OutputPath: out}, d); err != nil {
		t.Fatalf("run: %v", err)
	}

	snap, err := LoadSnapshot(filepath.Join(dir, "model-snapshot.json"))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	entry, ok := snap.Models["a/only-arena"]
	if !ok {
		t.Fatalf("snapshot has no entry for a/only-arena")
	}
	if entry.Score != nil {
		t.Errorf("Score = %+v, want nil: a source that fed the arena= column must never fill the SWE-bench one", entry.Score)
	}
	if entry.ArenaScore == nil || entry.ArenaScore.Value != 1400 {
		t.Errorf("ArenaScore = %+v, want the fetched Elo of 1400", entry.ArenaScore)
	}
}

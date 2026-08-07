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
	if _, err := os.Stat(pricehistory.Path(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Error("--dry-run wrote price history, want it untouched")
	}
}

func TestRunDryRunReportsCatalogDeltaWithoutChangingBaseline(t *testing.T) {
	dir := newDataDir(t)
	snapshotPath := filepath.Join(dir, "cache", "last-run-snapshot.json")
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
				HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 1, OverrideOutPerM: 4,
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
	if !strings.Contains(doc, "$0.50 / $3.00") {
		t.Errorf("the snapshot price did not make it into the document:\n%s", doc)
	}
	if !strings.Contains(doc, "$1.00 / $4.00 от 500K+") {
		t.Errorf("the snapshot long-context override did not make it into the document:\n%s", doc)
	}
	// The vendor-claimed number lives in notes.yaml and cannot go stale.
	if strings.Contains(doc, "80.5% (только вендор) (не удалось проверить") {
		t.Errorf("a notes.yaml score was wrongly labelled stale:\n%s", doc)
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
	snapshotPath := filepath.Join(dir, "cache", "last-run-snapshot.json")
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
			snapshotPath := filepath.Join(dir, "cache", "last-run-snapshot.json")
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

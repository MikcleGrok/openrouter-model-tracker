package arrange

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

const modelMap = "demo/high\ttier=sonnet\ndemo/low\ttier=haiku\n"

const notes = "models:\n  demo/high:\n    display: Demo High\n    note: Local fixture\n    task_fit: [implement, test]\n  demo/low:\n    display: Demo Low\n"

// DataDir creates a self-contained offline fixture for CLI acceptance tests.
func DataDir(t *testing.T, marker string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte(modelMap), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := refresh.Snapshot{FetchedAt: marker, Models: map[string]refresh.SnapshotEntry{
		"demo/high": {InPerM: 100, OutPerM: 100, Context: 128000, Score: &model.ScoreInfo{Value: 90, Unit: "%", VariantMeasured: "demo/high", IdentityStatus: model.IdentityExact}},
		"demo/low":  {InPerM: 1, OutPerM: 1, Context: 128000, Score: &model.ScoreInfo{Value: 10, Unit: "%", VariantMeasured: "demo/low", IdentityStatus: model.IdentityExact}},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-snapshot.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

const scoreSourceModelMap = "demo/swe\ttier=sonnet\tvals=demo/swe\ndemo/arena\ttier=sonnet\tarena=demo-arena\n"

const scoreSourceNotes = "models:\n  demo/swe:\n    display: Demo SWE\n  demo/arena:\n    display: Demo Arena\n"

// ScoreSourceDataDir is DataDir's sibling for the two-score-source CLI: one
// model has only a SWE-bench number, the other only an Arena Elo, so a view
// that leaks the other source's data is immediately visible.
func ScoreSourceDataDir(t *testing.T, marker string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte(scoreSourceModelMap), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte(scoreSourceNotes), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := refresh.Snapshot{FetchedAt: marker, Models: map[string]refresh.SnapshotEntry{
		"demo/swe":   {InPerM: 1, OutPerM: 3, Context: 128000, Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70, Unit: "%", VariantMeasured: "demo/swe", IdentityStatus: model.IdentityExact}},
		"demo/arena": {InPerM: 1, OutPerM: 3, Context: 128000, ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1453, VariantMeasured: "demo/arena", Unit: "Elo", IdentityStatus: model.IdentityExact}},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-snapshot.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

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
		"demo/high": {InPerM: 100, OutPerM: 100, Context: 128000, Score: &model.ScoreInfo{Value: 90}},
		"demo/low":  {InPerM: 1, OutPerM: 1, Context: 128000, Score: &model.ScoreInfo{Value: 10}},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

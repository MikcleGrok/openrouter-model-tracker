package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

type SnapshotBuilder struct {
	fetchedAt string
	models    map[string]refresh.SnapshotEntry
}

func NewSnapshot() *SnapshotBuilder {
	return &SnapshotBuilder{fetchedAt: "2026-08-07", models: map[string]refresh.SnapshotEntry{
		"demo/high": {InPerM: 100, OutPerM: 100, Context: 128000, Score: &model.ScoreInfo{Value: 90}},
		"demo/low":  {InPerM: 1, OutPerM: 1, Context: 128000, Score: &model.ScoreInfo{Value: 10}},
	}}
}

func (b *SnapshotBuilder) WithFetchedAt(value string) *SnapshotBuilder {
	b.fetchedAt = value
	return b
}

func (b *SnapshotBuilder) Write(t *testing.T, path string) {
	t.Helper()
	body, err := json.Marshal(refresh.Snapshot{FetchedAt: b.fetchedAt, Models: b.models})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Package refresh orchestrates one run: fetch, merge, render, write, report.
package refresh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
)

// SnapshotEntry is what one model looked like at the end of the previous run.
type SnapshotEntry struct {
	InPerM  float64          `json:"in_per_m"`
	OutPerM float64          `json:"out_per_m"`
	Context int              `json:"context"`
	Score   *model.ScoreInfo `json:"score,omitempty"`
}

// Snapshot is the previous run's result, used to keep the document intact when
// a source is down or has changed shape.
type Snapshot struct {
	FetchedAt string                   `json:"fetched_at"`
	Models    map[string]SnapshotEntry `json:"models"`
}

// LoadSnapshot reads the snapshot at path. A missing file yields an empty
// snapshot and no error: the very first run has nothing to fall back to.
func LoadSnapshot(path string) (*Snapshot, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Snapshot{Models: map[string]SnapshotEntry{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("snapshot: %s: %w", path, err)
	}
	if s.Models == nil {
		s.Models = map[string]SnapshotEntry{}
	}
	return &s, nil
}

// Save writes the snapshot, creating the parent directory if needed.
func (s *Snapshot) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("snapshot: create directory: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	return nil
}

// NewSnapshot captures the current run's values for the next run to fall back to.
func NewSnapshot(models []model.Model, fetchedAt string) *Snapshot {
	s := &Snapshot{FetchedAt: fetchedAt, Models: make(map[string]SnapshotEntry, len(models))}
	for _, m := range models {
		s.Models[m.Slug] = SnapshotEntry{
			InPerM:  m.InPerM,
			OutPerM: m.OutPerM,
			Context: m.Context,
			Score:   m.Score,
		}
	}
	return s
}

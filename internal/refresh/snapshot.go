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
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// SnapshotEntry is what one model looked like at the end of the previous run.
type SnapshotEntry struct {
	InPerM             float64          `json:"in_per_m"`
	OutPerM            float64          `json:"out_per_m"`
	Context            int              `json:"context"`
	Created            int64            `json:"created,omitempty"`
	Description        string           `json:"description,omitempty"`
	CatalogName        string           `json:"catalog_name,omitempty"`
	CanonicalSlug      string           `json:"canonical_slug,omitempty"`
	HuggingFaceID      string           `json:"hugging_face_id,omitempty"`
	Provider           string           `json:"provider,omitempty"`
	ReleaseVariant     string           `json:"release_variant,omitempty"`
	ModelVariant       string           `json:"model_variant,omitempty"`
	Reasoning          string           `json:"reasoning,omitempty"`
	Configuration      string           `json:"configuration,omitempty"`
	License            string           `json:"license,omitempty"`
	ModelURL           string           `json:"model_url,omitempty"`
	MetadataSourceURL  string           `json:"metadata_source_url,omitempty"`
	Copyright          string           `json:"copyright,omitempty"`
	CopyrightGuardrail string           `json:"copyright_guardrail,omitempty"`
	HasOverride        bool             `json:"has_long_context_override,omitempty"`
	OverrideMinTokens  int              `json:"long_context_min_tokens,omitempty"`
	OverrideInPerM     float64          `json:"long_context_input_per_million,omitempty"`
	OverrideOutPerM    float64          `json:"long_context_output_per_million,omitempty"`
	Score              *model.ScoreInfo `json:"score,omitempty"`
	// ArenaScore is the raw Elo, kept apart from Score for the same reason the
	// two columns are apart: one source going down must never put its number
	// into the other's view on the next run's fallback.
	ArenaScore *model.ScoreInfo `json:"arena_score,omitempty"`
}

// Snapshot is the previous run's result, used to keep the document intact when
// a source is down or has changed shape.
type Snapshot struct {
	FetchedAt    string                   `json:"fetched_at"`
	UpdatedAt    string                   `json:"updated_at,omitempty"`
	Models       map[string]SnapshotEntry `json:"models"`
	CatalogSlugs []string                 `json:"catalog_slugs,omitempty"`
}

// SnapshotFileName is the tracked, versioned snapshot's file name. It is
// always resolved relative to DataDir — the same root that holds
// model-map.tsv and notes.yaml — never under the gitignored cache/
// directory, which holds only ephemeral raw fetches from external sources.
const SnapshotFileName = "model-snapshot.json"

// SnapshotPath returns the tracked snapshot's path for a given data
// directory. Every reader/writer of the snapshot goes through this helper so
// the on-disk location has one definition.
func SnapshotPath(dataDir string) string {
	return filepath.Join(dataDir, SnapshotFileName)
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
	for slug, entry := range s.Models {
		entry.Description = model.NormalizeMissingLabels(entry.Description)
		entry.CatalogName = model.NormalizeMissingLabels(entry.CatalogName)
		entry.CanonicalSlug = model.NormalizeMissingLabels(entry.CanonicalSlug)
		entry.HuggingFaceID = model.NormalizeMissingLabels(entry.HuggingFaceID)
		entry.Provider = model.NormalizeMissingLabels(entry.Provider)
		entry.ReleaseVariant = model.NormalizeMissingLabels(entry.ReleaseVariant)
		entry.ModelVariant = model.NormalizeMissingLabels(entry.ModelVariant)
		entry.Reasoning = model.NormalizeMissingLabels(entry.Reasoning)
		entry.Configuration = model.NormalizeMissingLabels(entry.Configuration)
		entry.License = model.NormalizeMissingLabels(entry.License)
		entry.ModelURL = model.NormalizeMissingLabels(entry.ModelURL)
		entry.MetadataSourceURL = model.NormalizeMissingLabels(entry.MetadataSourceURL)
		if entry.Copyright == "" {
			entry.Copyright = notes.CopyrightUnknown
		}
		if entry.CopyrightGuardrail == "" {
			entry.CopyrightGuardrail = copyrightGuardrailFor(entry.Copyright)
		}
		if entry.Copyright == notes.CopyrightUnknown && entry.CopyrightGuardrail != notes.CopyrightGuardrailUnknown {
			entry.Copyright = copyrightForGuardrail(entry.CopyrightGuardrail)
		}
		normalizeScoreInfo(entry.Score)
		normalizeScoreInfo(entry.ArenaScore)
		s.Models[slug] = entry
	}
	return &s, nil
}

func normalizeScoreInfo(info *model.ScoreInfo) {
	if info == nil {
		return
	}
	info.Metric = model.NormalizeMissingLabels(info.Metric)
	info.Unit = model.NormalizeMissingLabels(info.Unit)
	info.ConfiguredIdentity = model.NormalizeMissingLabels(info.ConfiguredIdentity)
	info.VariantMeasured = model.NormalizeMissingLabels(info.VariantMeasured)
	info.SourceURL = model.NormalizeMissingLabels(info.SourceURL)
	info.Checked = model.NormalizeMissingLabels(info.Checked)
	info.IdentityStatus = model.NormalizeMissingLabels(info.IdentityStatus)
	info.Provenance = model.NormalizeMissingLabels(info.Provenance)
	info.CanonicalID = model.NormalizeMissingLabels(info.CanonicalID)
	info.ReleaseVariant = model.NormalizeMissingLabels(info.ReleaseVariant)
	info.ModelVariant = model.NormalizeMissingLabels(info.ModelVariant)
	info.Reasoning = model.NormalizeMissingLabels(info.Reasoning)
	info.Uncertainty = model.NormalizeMissingLabels(info.Uncertainty)
	info.SampleSize = model.NormalizeMissingLabels(info.SampleSize)
	info.Harness = model.NormalizeMissingLabels(info.Harness)
	info.Scaffold = model.NormalizeMissingLabels(info.Scaffold)
	info.Provider = model.NormalizeMissingLabels(info.Provider)
	info.Configuration = model.NormalizeMissingLabels(info.Configuration)
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
// CatalogSlugs is attached by the orchestrator only after a successful catalogue fetch.
func NewSnapshot(models []model.Model, fetchedAt string) *Snapshot {
	s := &Snapshot{FetchedAt: fetchedAt, Models: make(map[string]SnapshotEntry, len(models))}
	for _, m := range models {
		s.Models[m.Slug] = SnapshotEntry{
			InPerM:             m.InPerM,
			OutPerM:            m.OutPerM,
			Context:            m.Context,
			Created:            m.Created,
			Description:        m.Description,
			CatalogName:        m.CatalogName,
			CanonicalSlug:      m.CanonicalSlug,
			HuggingFaceID:      m.HuggingFaceID,
			Provider:           m.Provider,
			License:            m.License,
			ModelURL:           m.ModelURL,
			MetadataSourceURL:  m.MetadataSourceURL,
			Copyright:          m.Copyright,
			CopyrightGuardrail: m.CopyrightGuardrail,
			Score:              m.Score,
			ArenaScore:         m.ArenaScore,
		}
	}
	return s
}

func copyrightGuardrailFor(value string) string {
	switch value {
	case notes.CopyrightCompliant:
		return notes.CopyrightGuardrailEnforces
	case notes.CopyrightNonCompliant:
		return notes.CopyrightGuardrailBypasses
	default:
		return notes.CopyrightGuardrailUnknown
	}
}

func copyrightForGuardrail(value string) string {
	switch value {
	case notes.CopyrightGuardrailEnforces:
		return notes.CopyrightCompliant
	case notes.CopyrightGuardrailBypasses:
		return notes.CopyrightNonCompliant
	default:
		return notes.CopyrightUnknown
	}
}

// NewSnapshotWithPrices also persists catalogue-only long-context fields that
// are not part of the rendered model value.
func NewSnapshotWithPrices(models []model.Model, prices map[string]sources.PriceInfo, fetchedAt string) *Snapshot {
	s := NewSnapshot(models, fetchedAt)
	for slug, price := range prices {
		entry, ok := s.Models[slug]
		if !ok {
			continue
		}
		entry.HasOverride = price.HasOverride
		entry.OverrideMinTokens = price.OverrideMinTokens
		entry.OverrideInPerM = price.OverrideInPerM
		entry.OverrideOutPerM = price.OverrideOutPerM
		entry.CanonicalSlug = price.CanonicalSlug
		entry.HuggingFaceID = price.HuggingFaceID
		entry.Provider = price.Provider
		entry.ReleaseVariant = price.ReleaseVariant
		entry.ModelVariant = price.ModelVariant
		entry.Reasoning = price.Reasoning
		entry.Configuration = price.Configuration
		s.Models[slug] = entry
	}
	return s
}

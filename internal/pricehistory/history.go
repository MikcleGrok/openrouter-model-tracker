package pricehistory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

const (
	SchemaVersion   = 1
	MaxObservations = 365
)

type Price struct {
	Found             bool    `json:"found"`
	InPerM            float64 `json:"input_per_million"`
	OutPerM           float64 `json:"output_per_million"`
	Context           int     `json:"context"`
	HasOverride       bool    `json:"has_long_context_override,omitempty"`
	OverrideMinTokens int     `json:"long_context_min_tokens,omitempty"`
	OverrideInPerM    float64 `json:"long_context_input_per_million,omitempty"`
	OverrideOutPerM   float64 `json:"long_context_output_per_million,omitempty"`
}

type Observation struct {
	ObservedAt time.Time        `json:"observed_at"`
	Prices     map[string]Price `json:"prices"`
}

type History struct {
	SchemaVersion int           `json:"schema_version"`
	Observations  []Observation `json:"observations"`
}

func Path(dataDir string) string { return filepath.Join(dataDir, "cache", "price-history.json") }

func FromPrices(prices map[string]sources.PriceInfo) map[string]Price {
	out := make(map[string]Price, len(prices))
	for slug, p := range prices {
		out[slug] = Price{Found: p.Found, InPerM: p.InPerM, OutPerM: p.OutPerM, Context: p.Context, HasOverride: p.HasOverride, OverrideMinTokens: p.OverrideMinTokens, OverrideInPerM: p.OverrideInPerM, OverrideOutPerM: p.OverrideOutPerM}
	}
	return out
}

func Load(path string) (*History, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &History{SchemaVersion: SchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("price history: read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return &History{SchemaVersion: SchemaVersion}, nil
	}
	var h History
	if err := json.Unmarshal(body, &h); err != nil {
		return nil, fmt.Errorf("price history: decode %s: %w", path, err)
	}
	if h.SchemaVersion == 0 {
		h.SchemaVersion = SchemaVersion
	}
	if h.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("price history: unsupported schema_version %d", h.SchemaVersion)
	}
	sort.SliceStable(h.Observations, func(i, j int) bool { return h.Observations[i].ObservedAt.Before(h.Observations[j].ObservedAt) })
	return &h, nil
}

func (h *History) Add(observedAt time.Time, prices map[string]sources.PriceInfo) {
	h.SchemaVersion = SchemaVersion
	h.Observations = append(h.Observations, Observation{ObservedAt: observedAt.UTC(), Prices: FromPrices(prices)})
	sort.SliceStable(h.Observations, func(i, j int) bool { return h.Observations[i].ObservedAt.Before(h.Observations[j].ObservedAt) })
	if len(h.Observations) > MaxObservations {
		h.Observations = h.Observations[len(h.Observations)-MaxObservations:]
	}
}

func (h *History) Save(path string) error {
	h.SchemaVersion = SchemaVersion
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("price history: encode: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("price history: create directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".price-history-*.tmp")
	if err != nil {
		return fmt.Errorf("price history: create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("price history: chmod temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("price history: write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("price history: close temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("price history: replace %s: %w", path, err)
	}
	return nil
}

func Equal(a, b Price) bool { return a == b }

// Format renders p as a change-log fragment: "$in/$out, contextK" plus, when
// p carries a long-context override, a "; long-context $in/$out <prep>
// thresholdK+" clause. lang follows the same "" (English) / "ru" (Russian)
// convention as the TUI's own language toggle and *ForLang helpers
// (cmd/openrouter/tui.go) — it only picks the override clause's preposition
// ("from" / "от"); nothing else in the string is natural language. A caller
// with no language concept of its own passes a fixed lang instead of varying
// it: the always-Russian refresh report passes "ru", the always-English
// `openrouter history` CLI command passes "".
func Format(p Price, lang string) string {
	base := fmt.Sprintf("$%.4g/$%.4g, %dK", p.InPerM, p.OutPerM, p.Context/1000)
	if !p.HasOverride {
		return base
	}
	preposition := "from"
	if lang == "ru" {
		preposition = "от"
	}
	return fmt.Sprintf("%s; long-context $%.4g/$%.4g %s %dK+", base, p.OverrideInPerM, p.OverrideOutPerM, preposition, p.OverrideMinTokens/1000)
}

package pricehistory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

const (
	SchemaVersion   = 2
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
	Scores     map[string]Score `json:"scores,omitempty"`
}

type Score struct {
	SourceFamily   string    `json:"source_family"`
	SourceID       string    `json:"source_id"`
	Metric         string    `json:"metric,omitempty"`
	IdentityStatus string    `json:"identity_status,omitempty"`
	Value          float64   `json:"value"`
	Unit           string    `json:"unit,omitempty"`
	Provenance     string    `json:"provenance,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`
	QualityPrice   *float64  `json:"quality_price,omitempty"`
	Formula        string    `json:"formula,omitempty"`
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
	if h.SchemaVersion == 0 || h.SchemaVersion == 1 {
		h.SchemaVersion = SchemaVersion
	}
	if h.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("price history: unsupported schema_version %d", h.SchemaVersion)
	}
	sort.SliceStable(h.Observations, func(i, j int) bool { return h.Observations[i].ObservedAt.Before(h.Observations[j].ObservedAt) })
	return &h, nil
}

func (h *History) Add(observedAt time.Time, prices map[string]sources.PriceInfo) {
	h.AddObservation(observedAt, prices, nil, nil)
}

func (h *History) AddObservation(observedAt time.Time, prices map[string]sources.PriceInfo, scores, arena []sources.ScoreRow) {
	h.SchemaVersion = SchemaVersion
	when := observedAt.UTC()
	observation := Observation{ObservedAt: when, Prices: FromPrices(prices), Scores: map[string]Score{}}
	for _, row := range append(append([]sources.ScoreRow(nil), scores...), arena...) {
		family := "swebench"
		if row.Metric == sources.MetricArenaElo || row.SourceFamily == "arena" {
			family = "arena"
		}
		sourceID := row.SourceFamily
		if sourceID != "vals" && sourceID != "swebench" && family == "swebench" {
			sourceID = "swebench"
		}
		if family == "arena" && (row.IdentityStatus != "exact_product" || row.Value < 0 || math.IsNaN(row.Value) || math.IsInf(row.Value, 0)) {
			continue
		}
		point := Score{SourceFamily: family, SourceID: sourceID, Metric: row.Metric, IdentityStatus: row.IdentityStatus, Value: row.Value, Unit: row.Unit, Provenance: row.SourceURL, ObservedAt: when}
		price := prices[row.Slug]
		if family == "swebench" && row.IdentityStatus == "exact_product" && row.Value >= 0 && row.Value <= 100 && !row.IdentityAmbiguous {
			mixed := pricing.MixedPrice(price.InPerM, price.OutPerM)
			if price.Found && mixed > 0 {
				qualityPrice := pricing.QualityPrice(row.Value, mixed)
				point.QualityPrice = &qualityPrice
				point.Formula = "swe_score_pct / mixed_price_3_to_1"
			}
		}
		observation.Scores[row.Slug+"\x00"+family] = point
	}
	if len(observation.Scores) == 0 {
		observation.Scores = nil
	}
	h.Observations = append(h.Observations, observation)
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

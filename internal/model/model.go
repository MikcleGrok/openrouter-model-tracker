// Package model merges the hand-maintained map, the live prices, the fetched
// benchmark rows and the prose into the one row type the renderer iterates. All
// derived arithmetic and every display string are computed here, so the
// template only formats and iterates.
package model

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// ScoreInfo is a benchmark number attached to a model, plus the provenance the
// document's own rules require. It is also what the run snapshot persists.
type ScoreInfo struct {
	Metric          string  `json:"metric"`
	Value           float64 `json:"value"`
	VariantMeasured string  `json:"variant_measured"`
	SourceURL       string  `json:"source_url"`
	Checked         string  `json:"checked"`
	Stale           bool    `json:"stale,omitempty"`
}

// Model is one rendered row.
type Model struct {
	Slug        string
	DisplayName string
	Tier        string

	InPerM  float64
	OutPerM float64
	Context int
	Free    bool

	Score        *ScoreInfo
	MixedPrice   float64
	QualityPrice float64

	Note        string
	Owner       string
	OpenWeights string
	ClaudeRef   string

	// Display strings, precomputed so the template stays logic-free.
	ScoreLabel        string
	QualityPriceLabel string

	// LongContextPriceLabel is the catalogue's long-context pricing tier,
	// pre-formatted as "$in / $out от <threshold>+". Empty when the
	// catalogue reported no override for this slug.
	LongContextPriceLabel string

	// Rankable is false for a row whose number does not belong to the product
	// sold under this slug, or which has no SWE-bench Verified number at all.
	// Such a row never ranks and never becomes a favourite.
	Rankable bool

	// PriceStale is set when the price came from the previous run's snapshot
	// because the live lookup failed.
	PriceStale bool

	Tokens10In    float64
	Tokens10Out   float64
	Tokens10Mixed float64
}

// FormatScore prints a benchmark percentage the way the score column does.
func FormatScore(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

// Merge builds the rendered rows. A slug with no live price entry, or one the
// catalogue does not know, is dropped: report.go tells the human about it.
func Merge(entries []modelmap.Entry, prices map[string]sources.PriceInfo, scores []sources.ScoreRow, nt *notes.Notes) []Model {
	// First row for a slug wins. The caller controls source priority by the
	// order it concatenates the sources' results.
	firstRow := map[string]sources.ScoreRow{}
	for _, r := range scores {
		if _, seen := firstRow[r.Slug]; !seen {
			firstRow[r.Slug] = r
		}
	}

	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		price, ok := prices[e.Slug]
		if !ok || !price.Found {
			continue
		}

		m := Model{
			Slug:        e.Slug,
			DisplayName: nt.DisplayName(e.Slug),
			Tier:        e.Tier,
			InPerM:      price.InPerM,
			OutPerM:     price.OutPerM,
			Context:     price.Context,
			Free:        price.Free,
			Note:        nt.ModelNote(e.Slug),
			Owner:       nt.Owner(e.Slug),
			OpenWeights: nt.OpenWeights(e.Slug),
			ClaudeRef:   nt.ClaudeRef(e.Slug),
		}
		m.MixedPrice = pricing.MixedPrice(m.InPerM, m.OutPerM)
		m.Tokens10In = pricing.Tokens10(m.InPerM)
		m.Tokens10Out = pricing.Tokens10(m.OutPerM)
		m.Tokens10Mixed = pricing.Tokens10(m.MixedPrice)
		if price.HasOverride {
			m.LongContextPriceLabel = fmt.Sprintf("$%s / $%s от %s+",
				pricing.FormatPrice(price.OverrideInPerM), pricing.FormatPrice(price.OverrideOutPerM), pricing.FormatContext(price.OverrideMinTokens))
		}

		if row, has := firstRow[e.Slug]; has {
			m.Score = &ScoreInfo{
				Metric:          row.Metric,
				Value:           row.Value,
				VariantMeasured: row.VariantMeasured,
				SourceURL:       row.SourceURL,
				Checked:         row.Checked,
			}
			m.ScoreLabel = FormatScore(row.Value)
			m.Rankable = true
		} else if ov, has := nt.ScoreOverride(e.Slug); has {
			m.Score = &ScoreInfo{
				Metric:          sources.MetricSWEBenchVerified,
				Value:           ov.Value,
				VariantMeasured: "vendor-claimed",
				SourceURL:       ov.Source,
			}
			m.ScoreLabel = ov.Label
			m.Rankable = ov.Rankable
		} else {
			m.ScoreLabel = "н/д"
		}

		switch {
		case m.Free:
			m.QualityPriceLabel = "н/д (цена $0)"
		case m.Rankable && m.Score != nil:
			m.QualityPrice = pricing.QualityPrice(m.Score.Value, m.MixedPrice)
			m.QualityPriceLabel = pricing.FormatQualityPrice(m.QualityPrice)
		default:
			m.QualityPriceLabel = nt.NoScoreReason(e.Slug)
		}

		out = append(out, m)
	}
	return out
}

// RankFavorites returns the rankable models of one tier, best first. Paid tiers
// rank by quality/price; the free tier ranks by score, because every free model
// costs $0 and quality/price is undefined there.
func RankFavorites(models []Model, tier string) []Model {
	var out []Model
	for _, m := range models {
		if m.Tier != tier || m.Score == nil || !m.Rankable {
			continue
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if tier == "free" {
			if a.Score.Value != b.Score.Value {
				return a.Score.Value > b.Score.Value
			}
			return a.Slug < b.Slug
		}
		if a.QualityPrice != b.QualityPrice {
			return a.QualityPrice > b.QualityPrice
		}
		return a.Slug < b.Slug
	})
	return out
}

// TierRows returns every model of a tier in table order: the ranked ones first,
// then the rows that do not rank, ordered by blended price.
func TierRows(models []Model, tier string) []Model {
	ranked := RankFavorites(models, tier)
	inRanked := make(map[string]bool, len(ranked))
	for _, m := range ranked {
		inRanked[m.Slug] = true
	}

	var rest []Model
	for _, m := range models {
		if m.Tier == tier && !inRanked[m.Slug] {
			rest = append(rest, m)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].MixedPrice != rest[j].MixedPrice {
			return rest[i].MixedPrice < rest[j].MixedPrice
		}
		return rest[i].Slug < rest[j].Slug
	})
	return append(ranked, rest...)
}

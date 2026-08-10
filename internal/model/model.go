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
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
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

// ScoreSourceSWEBench and ScoreSourceArena name the two independent score
// sources a rendered table can be built from. They are never blended: one
// table shows one of them, and a model with no number on the active source
// shows "н/д" even when it has a number on the other one.
const (
	ScoreSourceSWEBench = "swebench"
	ScoreSourceArena    = "arena"
)

// SourceFamily maps a model-map.tsv source id onto the score source it
// feeds. Two ids of one family (swebench.com and vals.ai) are
// priority-ordered alternatives for the same number; two families never
// merge into one column. An unknown id maps to the empty string and feeds
// neither view, which is the safe default for a typo in the map.
var SourceFamily = map[string]string{
	"swebench": ScoreSourceSWEBench,
	"vals":     ScoreSourceSWEBench,
	"arena":    ScoreSourceArena,
}

// arenaNoScoreLabel fills the quality/price cell of a row the Arena view has
// no number for. It deliberately does not reuse notes.yaml's NoScoreReason:
// that text names SWE-bench, which is exactly the confusion two separate
// views exist to prevent.
const arenaNoScoreLabel = "н/д (нет оценки на LMArena)"

// Model is one rendered row.
type Model struct {
	Slug        string
	DisplayName string
	Tier        string

	InPerM  float64
	OutPerM float64
	Context int
	Free    bool

	// Created is the catalogue's publication timestamp (Unix seconds);
	// Description is the vendor's prose. Both are carried verbatim from the
	// catalogue entry and need no staleness flag of their own: a slug with
	// no catalogue entry is dropped whole by the !price.Found check below,
	// so "row exists but catalogue data is missing" is not a reachable
	// state. The relative age ("2 месяца назад") is deliberately NOT
	// precomputed here, even though every other display string is: it
	// depends on when the screen is drawn, not on the data, and the
	// snapshot on disk can be a week old.
	Created     int64
	Description string
	CatalogName string

	// CanonicalSlug and HuggingFaceID are the catalogue identifiers the TUI
	// detail screen turns into links: https://openrouter.ai/<CanonicalSlug>
	// and https://huggingface.co/<HuggingFaceID>. Unlike Created and
	// Description above, they have two reachable empty states, and neither
	// is a defect: a snapshot written before this feature existed carries
	// neither field until the next refresh, and roughly 60% of catalogue
	// entries genuinely have no hugging_face_id at all, because a
	// proprietary model has no repository to link to. Neither is ever
	// guessed from Slug — id and canonical_slug disagree for most of the
	// catalogue, so a guess would be a silently wrong link.
	CanonicalSlug     string
	HuggingFaceID     string
	Provider          string
	License           string
	ModelURL          string
	MetadataSourceURL string

	Score        *ScoreInfo
	MixedPrice   float64
	QualityPrice float64
	InputPrice   float64
	OutputPrice  float64

	Note        string
	TaskFit     []string
	Owner       string
	OpenWeights string
	ClaudeRef   string

	// Display strings, precomputed so the template stays logic-free.
	ScoreLabel        string
	QualityPriceLabel string

	// LongContextPriceLabel is the catalogue's long-context pricing tier,
	// pre-formatted as "$in / $out от <threshold>+", for display next to a
	// combined in/out price cell (the favourites table). LongContextInLabel
	// and LongContextOutLabel carry the same tier split into "$in от
	// <threshold>+" / "$out от <threshold>+" for display next to separate
	// input/output price columns (the per-tier table) — putting the combined
	// label there would land the input price under the output column's
	// header. All three are empty when the catalogue reported no override
	// for this slug.
	LongContextPriceLabel string
	LongContextInLabel    string
	LongContextOutLabel   string

	// Rankable is false for a row whose number does not belong to the product
	// sold under this slug, or which has no SWE-bench Verified number at all.
	// Such a row never ranks and never becomes a favourite.
	Rankable bool

	// Arena is the second, fully independent score source. Its number never
	// mixes with Score: --score-source picks one view or the other, and
	// ForScoreSource projects the chosen one onto the fields the renderer and
	// the ranking already read. ArenaScore.Value is the raw Elo, as fetched
	// and as persisted in the snapshot; ArenaNormalized is that Elo rescaled
	// onto 0–100 over the current Arena set, which is what the mixed-utility
	// formula consumes.
	ArenaScore             *ScoreInfo
	ArenaNormalized        float64
	ArenaLabel             string
	ArenaRankable          bool
	ArenaQualityPrice      float64
	ArenaQualityPriceLabel string

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

// FormatArenaScore prints an Elo rating the way the Status column does when
// the Arena view is active. The unit is spelled out because the number is
// not a percentage and must never be read as one.
func FormatArenaScore(v float64) string {
	return strconv.FormatFloat(v, 'f', 0, 64) + " Elo"
}

// Merge builds the rendered rows with the SWE-bench column only. It stays
// for callers that have no Arena data to pass.
func Merge(entries []modelmap.Entry, prices map[string]sources.PriceInfo, scores []sources.ScoreRow, nt *notes.Notes) []Model {
	return MergeWithArena(entries, prices, scores, nil, nt)
}

// MergeWithArena builds the rendered rows from both score sources at once,
// into two separate sets of fields. A slug with no live price entry, or one
// the catalogue does not know, is dropped: report.go tells the human about it.
func MergeWithArena(entries []modelmap.Entry, prices map[string]sources.PriceInfo, scores, arena []sources.ScoreRow, nt *notes.Notes) []Model {
	// First row for a slug wins, per source. The caller controls priority
	// inside one source by the order it concatenates that source's results.
	// The two sources have their own maps and never fall through to one
	// another: an Elo is not a SWE-bench percentage.
	firstRow := map[string]sources.ScoreRow{}
	for _, r := range scores {
		if _, seen := firstRow[r.Slug]; !seen {
			firstRow[r.Slug] = r
		}
	}
	firstArena := map[string]sources.ScoreRow{}
	for _, r := range arena {
		if _, seen := firstArena[r.Slug]; !seen {
			firstArena[r.Slug] = r
		}
	}

	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		price, ok := prices[e.Slug]
		if !ok || !price.Found {
			continue
		}

		m := Model{
			Slug:          e.Slug,
			DisplayName:   nt.DisplayName(e.Slug),
			Tier:          e.Tier,
			InPerM:        price.InPerM,
			OutPerM:       price.OutPerM,
			Context:       price.Context,
			Free:          price.Free,
			Created:       price.Created,
			Description:   price.Description,
			CatalogName:   price.Name,
			CanonicalSlug: price.CanonicalSlug,
			HuggingFaceID: price.HuggingFaceID,
			Note:          nt.ModelNote(e.Slug),
			TaskFit:       nt.TaskFit(e.Slug),
			Owner:         nt.Owner(e.Slug),
			OpenWeights:   nt.OpenWeights(e.Slug),
			ClaudeRef:     nt.ClaudeRef(e.Slug),
		}
		m.MixedPrice = pricing.MixedPrice(m.InPerM, m.OutPerM)
		m.InputPrice, m.OutputPrice = m.InPerM, m.OutPerM
		m.Tokens10In = pricing.Tokens10(m.InPerM)
		m.Tokens10Out = pricing.Tokens10(m.OutPerM)
		m.Tokens10Mixed = pricing.Tokens10(m.MixedPrice)
		if price.HasOverride {
			threshold := pricing.FormatContext(price.OverrideMinTokens)
			m.LongContextPriceLabel = fmt.Sprintf("%s / %s от %s+", pricing.FormatDollar(price.OverrideInPerM), pricing.FormatDollar(price.OverrideOutPerM), threshold)
			m.LongContextInLabel = fmt.Sprintf("%s от %s+", pricing.FormatDollar(price.OverrideInPerM), threshold)
			m.LongContextOutLabel = fmt.Sprintf("%s от %s+", pricing.FormatDollar(price.OverrideOutPerM), threshold)
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

		// The Arena column has no notes.yaml fallback: manual overrides in
		// notes.yaml describe SWE-bench Verified, and reusing them here would
		// put a percentage on an Elo scale.
		if row, has := firstArena[e.Slug]; has {
			m.Provider, m.License, m.ModelURL, m.MetadataSourceURL = row.Provider, row.License, row.ModelURL, row.MetadataSourceURL
			m.ArenaScore = &ScoreInfo{
				Metric:          row.Metric,
				Value:           row.Value,
				VariantMeasured: row.VariantMeasured,
				SourceURL:       row.SourceURL,
				Checked:         row.Checked,
			}
			m.ArenaLabel = FormatArenaScore(row.Value)
			m.ArenaRankable = true
		} else {
			m.ArenaLabel = "н/д"
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

	// Min-max normalisation needs the whole set at once, so the Arena view's
	// derived numbers are filled after the loop, not inside it.
	fillArenaDerived(out)
	return out
}

// fillArenaDerived rescales every row's raw Elo onto 0–100 over the current
// Arena set and computes that view's quality/price cell.
func fillArenaDerived(models []Model) {
	indexes := make([]int, 0, len(models))
	raw := make([]float64, 0, len(models))
	for i := range models {
		if models[i].ArenaScore == nil {
			continue
		}
		indexes = append(indexes, i)
		raw = append(raw, models[i].ArenaScore.Value)
	}
	normalized := ranking.NormalizeMinMax(raw)
	for n, i := range indexes {
		models[i].ArenaNormalized = normalized[n]
	}
	for i := range models {
		m := &models[i]
		switch {
		case m.Free:
			m.ArenaQualityPriceLabel = "н/д (цена $0)"
		case m.ArenaRankable && m.ArenaScore != nil:
			m.ArenaQualityPrice = pricing.QualityPrice(m.ArenaNormalized, m.MixedPrice)
			m.ArenaQualityPriceLabel = pricing.FormatQualityPrice(m.ArenaQualityPrice)
		default:
			m.ArenaQualityPriceLabel = arenaNoScoreLabel
		}
	}
}

// ForScoreSource returns the rows projected onto one score source: the
// chosen source's number, label and quality/price land in the fields every
// consumer already reads, and the other source's data is simply not there.
// Nothing downstream — the ranking, the filters, the renderer, the TUI —
// needs to know a second source exists, and no table can end up showing two
// scales at once.
//
// The Arena view carries the normalised 0–100 value in Score.Value, because
// that is what the ranking formula is tuned for, and the raw Elo in
// ScoreLabel, because that is what a human should see. The input slice is
// never mutated: the caller keeps a row that still knows both sources.
func ForScoreSource(models []Model, source string) []Model {
	if source != ScoreSourceArena {
		return models
	}
	out := make([]Model, len(models))
	copy(out, models)
	for i := range out {
		m := &out[i]
		m.Score, m.Rankable, m.ScoreLabel = nil, m.ArenaRankable, m.ArenaLabel
		if m.ArenaScore != nil {
			m.Score = &ScoreInfo{
				Metric:          m.ArenaScore.Metric,
				Value:           m.ArenaNormalized,
				VariantMeasured: m.ArenaScore.VariantMeasured,
				SourceURL:       m.ArenaScore.SourceURL,
				Checked:         m.ArenaScore.Checked,
				Stale:           m.ArenaScore.Stale,
			}
		}
		m.QualityPrice, m.QualityPriceLabel = m.ArenaQualityPrice, m.ArenaQualityPriceLabel
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

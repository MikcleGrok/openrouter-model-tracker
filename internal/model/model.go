// Package model merges the hand-maintained map, the live prices, the fetched
// benchmark rows and the prose into the one row type the renderer iterates. All
// derived arithmetic and every display string are computed here, so the
// template only formats and iterates.
package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// ScoreInfo is a benchmark number attached to a model, plus the provenance the
// document's own rules require. It is also what the run snapshot persists.
type ScoreInfo struct {
	Metric             string  `json:"metric"`
	Value              float64 `json:"value"`
	Unit               string  `json:"unit,omitempty"`
	SourceFamily       string  `json:"source_family,omitempty"`
	ConfiguredIdentity string  `json:"configured_identity,omitempty"`
	IdentityAmbiguous  bool    `json:"identity_ambiguous,omitempty"`
	VariantMeasured    string  `json:"variant_measured"`
	SourceURL          string  `json:"source_url"`
	Checked            string  `json:"checked"`
	IdentityStatus     string  `json:"identity_status,omitempty"`
	Provenance         string  `json:"provenance"`
	Stale              bool    `json:"stale,omitempty"`
	CanonicalID        string  `json:"canonical_id"`
	ReleaseVariant     string  `json:"release_variant"`
	ModelVariant       string  `json:"model_variant"`
	Reasoning          string  `json:"reasoning"`
	Configuration      string  `json:"configuration"`
	Provider           string  `json:"provider"`
	Uncertainty        string  `json:"uncertainty"`
	SampleSize         string  `json:"sample_size"`
	Harness            string  `json:"harness"`
	Scaffold           string  `json:"scaffold"`
}

const (
	IdentityExact           = "exact_product"
	IdentityVariantMismatch = "variant_mismatch"
	IdentityLegacyUnknown   = "legacy_unknown"
	IdentityObservationOnly = "observation_only"
	IdentityMissing         = "missing_identity"
)

// ScoreSourceSWEBench and ScoreSourceArena name the two independent score
// sources a rendered table can be built from. They are never blended: one
// table shows one of them, and a model with no number on the active source
// shows "n/a" even when it has a number on the other one.
const (
	ScoreSourceSWEBench = "swebench"
	ScoreSourceArena    = "arena"
)

// sourceIDVals, sourceIDSWEBench and sourceIDArena are the model-map.tsv
// source ids, as stamped into ScoreRow.SourceFamily by each fetcher. They are
// deliberately kept separate from the ScoreSource* family names above even
// where the two strings coincide: classifyIdentity treats the two SWE-bench
// ids differently, so "which site produced this row" is a question the family
// name cannot answer.
const (
	sourceIDVals     = "vals"
	sourceIDSWEBench = "swebench"
	sourceIDArena    = "arena"
)

// SourceFamily maps a model-map.tsv source id onto the score source it
// feeds. Two ids of one family (swebench.com and vals.ai) are
// priority-ordered alternatives for the same number; two families never
// merge into one column. An unknown id maps to the empty string and feeds
// neither view, which is the safe default for a typo in the map.
var SourceFamily = map[string]string{
	sourceIDSWEBench: ScoreSourceSWEBench,
	sourceIDVals:     ScoreSourceSWEBench,
	sourceIDArena:    ScoreSourceArena,
}

// arenaNoScoreLabel fills the quality/price cell of a row the Arena view has
// no number for. It deliberately does not reuse notes.yaml's NoScoreReason:
// that text names SWE-bench, which is exactly the confusion two separate
// views exist to prevent.
const arenaNoScoreLabel = "n/a (no LMArena score)"

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

	Score           *ScoreInfo
	RankingScore    float64
	HasRankingScore bool
	MixedPrice      float64
	QualityPrice    float64
	InputPrice      float64
	OutputPrice     float64

	Note        string
	TaskFit     []string
	Owner       string
	OpenWeights string
	ClaudeRef   string
	Copyright   string
	// CopyrightGuardrail remains for the already merged TUI/filter contract.
	CopyrightGuardrail string
	ManualScore        *notes.ScoreOverride

	// Display strings, precomputed so the template stays logic-free.
	ScoreLabel        string
	QualityPriceLabel string
	HasQualityPrice   bool

	// LongContextPriceLabel is the catalogue's long-context pricing tier,
	// pre-formatted as "$in / $out от <threshold>+", for display next to a
	// combined in/out price cell (the favourites table). LongContextInLabel
	// and LongContextOutLabel carry the same tier split into "$in от
	// <threshold>+" / "$out от <threshold>+" for display next to separate
	// input/output price columns (the per-tier table) — putting the combined
	// label there would land the input price under the output column's
	// header. All three are empty when the catalogue reported no override
	// for this slug. All three are unconditionally Russian and frozen: they
	// feed the comparison-doc renderer (internal/refresh, always Russian)
	// and the TUI Detail overlay's pre-language-toggle render path, both of
	// which must keep reading exactly this text. A language-aware renderer
	// (the TUI's *ForLang path) must NOT read these three fields — it has
	// no way to translate the preposition baked into an already-formatted
	// string — and should instead format its own label from the raw numbers
	// below.
	LongContextPriceLabel string
	LongContextInLabel    string
	LongContextOutLabel   string

	// HasLongContextOverride and the three fields below it are the raw
	// numbers behind LongContextPriceLabel/InLabel/OutLabel above, carried
	// separately so a language-aware renderer can compose its own label
	// (any preposition, any language) instead of reading the frozen,
	// unconditionally-Russian strings.
	HasLongContextOverride       bool
	LongContextOverrideInPerM    float64
	LongContextOverrideOutPerM   float64
	LongContextOverrideMinTokens int

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

// NormalizeMissingLabels keeps legacy persisted data from leaking the old
// Russian placeholder into generated Markdown and the TUI.
func NormalizeMissingLabels(value string) string {
	for _, legacy := range []string{"н/д", "Н/Д", "Н/д", "н/Д"} {
		value = strings.ReplaceAll(value, legacy, "n/a")
	}
	return value
}

// ProviderLabel keeps provider identity useful when catalogue metadata is
// missing or an old snapshot predates the provider field. OpenRouter slugs use
// a provider namespace, but several namespaces are not the display label.
func ProviderLabel(slug, provider string) string {
	if provider = strings.TrimSpace(provider); provider != "" && !IsPlaceholder(provider) {
		return provider
	}
	namespace, _, ok := strings.Cut(strings.TrimSpace(slug), "/")
	if !ok || namespace == "" {
		return ""
	}
	if label, ok := map[string]string{
		"anthropic": "Anthropic", "cohere": "Cohere", "deepseek": "DeepSeek", "google": "Google",
		"inclusionai": "InclusionAI", "meta": "Meta", "meta-llama": "Meta", "minimax": "MiniMax", "mistral": "Mistral",
		"mistralai": "Mistral AI", "moonshotai": "Moonshot AI", "nvidia": "NVIDIA", "openai": "OpenAI",
		"poolside": "Poolside", "qwen": "Qwen", "tencent": "Tencent", "xiaomi": "Xiaomi",
		"x-ai": "xAI", "z-ai": "Z.ai",
	}[strings.ToLower(namespace)]; ok {
		return label
	}
	return namespace
}

// IsPlaceholder reports whether a value is a standalone missing-data marker,
// including the prose forms used in persisted notes and source snapshots.
func IsPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(normalized, "(") && strings.HasSuffix(normalized, ")") {
		normalized = strings.TrimSpace(normalized[1 : len(normalized)-1])
	}
	switch normalized {
	case "n/a", "n/d", "н/д", notes.NeedsReview:
		return true
	default:
		return strings.HasPrefix(normalized, "n/a ") || strings.HasPrefix(normalized, "n/a(") || strings.HasPrefix(normalized, "n/d ") || strings.HasPrefix(normalized, "n/d(") || strings.HasPrefix(normalized, "н/д ") || strings.HasPrefix(normalized, "н/д(") || strings.HasPrefix(normalized, strings.ToLower(notes.NeedsReview)+" ") || strings.HasPrefix(normalized, strings.ToLower(notes.NeedsReview)+"(")
	}
}

// FormatScoreProvenance keeps the overview compact while making every missing
// provenance field explicit rather than silently dropping it.
func FormatScoreProvenance(info *ScoreInfo) string {
	value := func(s string) string {
		if s == "" {
			return "n/a"
		}
		return NormalizeMissingLabels(s)
	}
	if info == nil {
		return "raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a"
	}
	return fmt.Sprintf("raw=%s; metric=%s; unit=%s; variant=%s; identity=%s; checked=%s; source=%s; uncertainty=%s; sample=%s; harness=%s; scaffold=%s; provider=%s; configuration=%s", strconv.FormatFloat(info.Value, 'f', -1, 64), value(info.Metric), value(info.Unit), value(info.VariantMeasured), value(info.IdentityStatus), value(info.Checked), value(info.SourceURL), value(info.Uncertainty), value(info.SampleSize), value(info.Harness), value(info.Scaffold), value(info.Provider), value(info.Configuration))
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
	// Every row a slug has, in the caller's order — which inside one family
	// is that family's source priority. selectRow picks one of them per
	// slug. The two families keep their own maps and never fall through to
	// one another: an Elo is not a SWE-bench percentage.
	rowsBySlug := map[string][]sources.ScoreRow{}
	for _, r := range scores {
		rowsBySlug[r.Slug] = append(rowsBySlug[r.Slug], r)
	}
	arenaBySlug := map[string][]sources.ScoreRow{}
	for _, r := range arena {
		arenaBySlug[r.Slug] = append(arenaBySlug[r.Slug], r)
	}

	out := make([]Model, 0, len(entries))
	for _, e := range entries {
		price, ok := prices[e.Slug]
		if !ok || !price.Found {
			continue
		}

		m := Model{
			Slug:               e.Slug,
			DisplayName:        nt.DisplayName(e.Slug),
			Tier:               e.Tier,
			InPerM:             price.InPerM,
			OutPerM:            price.OutPerM,
			Context:            price.Context,
			Free:               price.Free,
			Created:            price.Created,
			Description:        price.Description,
			CatalogName:        price.Name,
			CanonicalSlug:      price.CanonicalSlug,
			HuggingFaceID:      price.HuggingFaceID,
			Provider:           ProviderLabel(e.Slug, price.Provider),
			Note:               nt.ModelNote(e.Slug),
			TaskFit:            nt.TaskFit(e.Slug),
			Owner:              nt.Owner(e.Slug),
			OpenWeights:        nt.OpenWeights(e.Slug),
			ClaudeRef:          nt.ClaudeRef(e.Slug),
			Copyright:          nt.Copyright(e.Slug),
			CopyrightGuardrail: nt.CopyrightGuardrail(e.Slug),
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
			m.HasLongContextOverride = true
			m.LongContextOverrideInPerM = price.OverrideInPerM
			m.LongContextOverrideOutPerM = price.OverrideOutPerM
			m.LongContextOverrideMinTokens = price.OverrideMinTokens
		}

		if row, identity, has := selectRow(rowsBySlug[e.Slug], e, price); has {
			m.Score = &ScoreInfo{
				Metric:             row.Metric,
				Value:              row.Value,
				Unit:               row.Unit,
				SourceFamily:       sourceFamilyForRow(row),
				ConfiguredIdentity: row.ConfiguredIdentity,
				IdentityAmbiguous:  row.IdentityAmbiguous,
				VariantMeasured:    row.VariantMeasured,
				SourceURL:          row.SourceURL,
				Checked:            row.Checked,
				IdentityStatus:     identity,
				Provenance:         row.SourceURL,
				CanonicalID:        row.CanonicalID,
				ReleaseVariant:     row.ReleaseVariant,
				ModelVariant:       row.ModelVariant,
				Reasoning:          row.Reasoning,
				Configuration:      row.Configuration,
				Provider:           row.Provider,
				Uncertainty:        row.Uncertainty,
				SampleSize:         row.SampleSize,
				Harness:            row.Harness,
				Scaffold:           row.Scaffold,
			}
			m.ScoreLabel = FormatScore(row.Value)
			m.Rankable = identity == IdentityExact
			if !m.Rankable {
				m.ScoreLabel += " [" + identity + "]"
			}
		} else if ov, has := nt.ScoreOverride(e.Slug); has {
			m.Score = &ScoreInfo{
				Metric:          sources.MetricSWEBenchVerified,
				Value:           ov.Value,
				Unit:            "%",
				VariantMeasured: "vendor-claimed",
				SourceURL:       ov.Source,
				IdentityStatus:  IdentityObservationOnly,
				Provenance:      ov.Source,
			}
			m.ScoreLabel = ov.Label
			m.Rankable = false
		} else {
			m.ScoreLabel = "n/a"
		}
		if ov, has := nt.ScoreOverride(e.Slug); has {
			m.ManualScore = &ov
		}

		// The Arena column has no notes.yaml fallback: manual overrides in
		// notes.yaml describe SWE-bench Verified, and reusing them here would
		// put a percentage on an Elo scale.
		if row, identity, has := selectRow(arenaBySlug[e.Slug], e, price); has {
			if IsPlaceholder(m.Provider) && strings.TrimSpace(row.Provider) != "" {
				m.Provider = ProviderLabel(e.Slug, row.Provider)
			}
			if row.License != "" {
				m.License = row.License
			}
			if row.ModelURL != "" {
				m.ModelURL = row.ModelURL
			}
			if row.MetadataSourceURL != "" {
				m.MetadataSourceURL = row.MetadataSourceURL
			}
			m.ArenaScore = &ScoreInfo{
				Metric:             row.Metric,
				Value:              row.Value,
				Unit:               row.Unit,
				SourceFamily:       sourceFamilyForRow(row),
				ConfiguredIdentity: row.ConfiguredIdentity,
				IdentityAmbiguous:  row.IdentityAmbiguous,
				VariantMeasured:    row.VariantMeasured,
				SourceURL:          row.SourceURL,
				Checked:            row.Checked,
				IdentityStatus:     identity,
				Provenance:         row.SourceURL,
				CanonicalID:        row.CanonicalID,
				ReleaseVariant:     row.ReleaseVariant,
				ModelVariant:       row.ModelVariant,
				Reasoning:          row.Reasoning,
				Configuration:      row.Configuration,
				Provider:           row.Provider,
				Uncertainty:        row.Uncertainty,
				SampleSize:         row.SampleSize,
				Harness:            row.Harness,
				Scaffold:           row.Scaffold,
			}
			m.ArenaLabel = FormatArenaScore(row.Value)
			m.ArenaRankable = identity == IdentityExact
		} else {
			m.ArenaLabel = "n/a"
		}

		switch {
		case m.Free:
			m.QualityPriceLabel = "n/a (free)"
		case m.Rankable && m.Score != nil && validScore(m.scoreValue()) && m.MixedPrice > 0:
			m.QualityPrice = pricing.QualityPrice(m.scoreValue(), m.MixedPrice)
			m.QualityPriceLabel = pricing.FormatQualityPrice(m.QualityPrice)
			m.HasQualityPrice = true
		case m.Score != nil && m.Score.IdentityStatus == IdentityVariantMismatch:
			m.QualityPriceLabel = "n/a (variant mismatch)"
		case m.Score != nil && m.Score.IdentityStatus == IdentityLegacyUnknown:
			m.QualityPriceLabel = "n/a (legacy)"
		case m.Score != nil && m.Score.IdentityStatus == IdentityMissing:
			m.QualityPriceLabel = "n/a (missing identity)"
		case m.Score != nil && m.Score.IdentityStatus == IdentityObservationOnly:
			m.QualityPriceLabel = "n/a (observation only)"
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

// classifyIdentity accepts the catalogue id, its explicit canonical id, or —
// for the SWE-bench family (swebench.com, vals.ai) — the exact key
// model-map.tsv already maps this OpenRouter model to on that source. A
// dated release or source alias remains visible evidence, not product
// quality.
func classifyIdentity(row sources.ScoreRow, entry modelmap.Entry, price sources.PriceInfo) string {
	if sourceFamilyForRow(row) == ScoreSourceArena {
		if row.IdentityAmbiguous || row.ConfiguredIdentity == "" || row.CanonicalID == "" {
			return IdentityMissing
		}
		if row.CanonicalID != row.ConfiguredIdentity {
			return IdentityVariantMismatch
		}
		if row.Provider != "" && price.Provider != "" && row.Provider != price.Provider {
			return IdentityVariantMismatch
		}
		for _, pair := range [][2]string{{row.ReleaseVariant, price.ReleaseVariant}, {row.ModelVariant, price.ModelVariant}, {row.Reasoning, price.Reasoning}, {row.Configuration, price.Configuration}} {
			if pair[0] != "" && pair[1] != "" && pair[0] != pair[1] {
				return IdentityVariantMismatch
			}
		}
		return IdentityExact
	}
	if row.CanonicalID == "" && row.VariantMeasured == "" && row.ReleaseVariant == "" && row.ModelVariant == "" {
		return IdentityMissing
	}
	if namesAnotherProduct(row.CanonicalID, entry, price) {
		return IdentityVariantMismatch
	}
	switch {
	case row.ConfiguredIdentity != "":
		// model-map.tsv already asserts that entry.Slug IS the product behind
		// row.ConfiguredIdentity on this source: the human who wrote that
		// mapping line is the identity check for this row, not a second
		// guess against OpenRouter's own naming. Both FetchValsSWEBench and
		// FetchSWEBenchVerified only ever emit a row for a slug when the
		// exact configured key matched something on the source, so trust it
		// as exact_product — unless this specific mapping was explicitly
		// flagged as a genuine variant/checkpoint with the "!variant" marker.
		if entry.Variants[row.SourceFamily] {
			return IdentityVariantMismatch
		}
		// The one extra sanity check on top of that trust is per-source,
		// because only one of the two SWE-bench sources can carry it.
		// FetchValsSWEBench sets VariantMeasured to the very key it looked
		// up, so on a vals.ai row an inequality here cannot be a naming
		// difference — it can only be a forged or stale row, and it stays a
		// mismatch. FetchSWEBenchVerified has no equivalent field: its
		// VariantMeasured describes the leaderboard submission (a scaffold
		// name, or the median wording when several were aggregated), which
		// by construction never equals the configured "Model: " tag.
		// Demanding equality there would reject every correctly mapped
		// swebench.com row, which is the opposite of what the mapping means.
		// Any other source id — including the empty one a pre-SourceFamily
		// snapshot carries — keeps the strict check, as the safe default.
		if row.SourceFamily != sourceIDSWEBench && row.VariantMeasured != "" && row.VariantMeasured != row.ConfiguredIdentity {
			return IdentityVariantMismatch
		}
	case namesAnotherProduct(row.VariantMeasured, entry, price):
		// No model-map.tsv entry at all for this source (ConfiguredIdentity
		// empty): nothing to trust, so fall back to the original check
		// against the OpenRouter identifiers themselves.
		return IdentityVariantMismatch
	}
	if row.Provider != "" && price.Provider != "" && row.Provider != price.Provider {
		return IdentityVariantMismatch
	}
	for _, pair := range [][2]string{{row.ReleaseVariant, price.ReleaseVariant}, {row.ModelVariant, price.ModelVariant}, {row.Reasoning, price.Reasoning}, {row.Configuration, price.Configuration}} {
		if pair[0] != "" && pair[1] != "" && pair[0] != pair[1] {
			return IdentityVariantMismatch
		}
	}
	return IdentityExact
}

// namesAnotherProduct reports whether a non-empty identifier carried by a
// score row names something other than the OpenRouter product this entry is
// about. Both identifiers a row can be judged by — its explicit canonical id
// and, absent any model-map.tsv mapping, the variant it measured — are held
// to exactly this test, so it lives in one place.
func namesAnotherProduct(name string, entry modelmap.Entry, price sources.PriceInfo) bool {
	return name != "" && name != entry.Slug && name != price.CanonicalSlug
}

// selectRow picks which of a slug's rows becomes the rendered one. Order is
// the caller's priority order, but a higher-priority row that fails identity
// classification must not shadow a lower-priority source that passes it:
// model-map.tsv documents swebench.com as exactly this kind of fallback
// behind vals.ai, and a vals.ai mapping flagged "!variant" is the case where
// the fallback has to actually happen. When no row passes, the
// highest-priority one is kept anyway, so the reason it does not rank stays
// visible in the table instead of the model silently losing its number.
func selectRow(rows []sources.ScoreRow, entry modelmap.Entry, price sources.PriceInfo) (sources.ScoreRow, string, bool) {
	if len(rows) == 0 {
		return sources.ScoreRow{}, "", false
	}
	first := identityForRow(rows[0], entry, price)
	if first == IdentityExact {
		return rows[0], first, true
	}
	for _, row := range rows[1:] {
		if identity := identityForRow(row, entry, price); identity == IdentityExact {
			return row, identity, true
		}
	}
	return rows[0], first, true
}

func sourceFamilyForRow(row sources.ScoreRow) string {
	if row.SourceFamily != "" {
		return row.SourceFamily
	}
	switch row.Metric {
	case sources.MetricArenaElo:
		return ScoreSourceArena
	case sources.MetricSWEBenchVerified:
		return ScoreSourceSWEBench
	default:
		return ""
	}
}

func identityForRow(row sources.ScoreRow, entry modelmap.Entry, price sources.PriceInfo) string {
	if row.IdentityStatus == IdentityLegacyUnknown || row.IdentityStatus == IdentityObservationOnly {
		return row.IdentityStatus
	}
	return classifyIdentity(row, entry, price)
}

// fillArenaDerived rescales every row's raw Elo onto 0–100 over the current
// Arena set and computes that view's quality/price cell.
func fillArenaDerived(models []Model) {
	indexes := make([]int, 0, len(models))
	raw := make([]float64, 0, len(models))
	for i := range models {
		if models[i].ArenaScore == nil || !models[i].ArenaRankable {
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
			m.ArenaQualityPriceLabel = "n/a (free)"
		case m.ArenaRankable && m.ArenaScore != nil && validScore(m.ArenaNormalized) && m.MixedPrice > 0:
			m.ArenaQualityPrice = pricing.QualityPrice(m.ArenaNormalized, m.MixedPrice)
			m.ArenaQualityPriceLabel = pricing.FormatQualityPrice(m.ArenaQualityPrice)
			m.HasQualityPrice = true
		default:
			m.ArenaQualityPriceLabel = arenaNoScoreLabel
		}
	}
}

func validScore(value float64) bool {
	return value >= 0 && value <= 100
}

// ForScoreSource returns the rows projected onto one score source: the
// chosen source's number, label and quality/price land in the fields every
// consumer already reads, and the other source's data is simply not there.
// Nothing downstream — the ranking, the filters, the renderer, the TUI —
// needs to know a second source exists, and no table can end up showing two
// scales at once.
//
// The Arena view keeps raw Elo in Score.Value and carries the normalised value
// separately for ranking. The input slice is
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
		m.RankingScore, m.HasRankingScore = m.ArenaNormalized, true
		if m.ArenaScore != nil {
			m.Score = &ScoreInfo{
				Metric:             m.ArenaScore.Metric,
				Value:              m.ArenaScore.Value,
				Unit:               m.ArenaScore.Unit,
				SourceFamily:       m.ArenaScore.SourceFamily,
				ConfiguredIdentity: m.ArenaScore.ConfiguredIdentity,
				IdentityAmbiguous:  m.ArenaScore.IdentityAmbiguous,
				VariantMeasured:    m.ArenaScore.VariantMeasured,
				SourceURL:          m.ArenaScore.SourceURL,
				Checked:            m.ArenaScore.Checked,
				IdentityStatus:     m.ArenaScore.IdentityStatus,
				Provenance:         m.ArenaScore.Provenance,
				Stale:              m.ArenaScore.Stale,
				CanonicalID:        m.ArenaScore.CanonicalID,
				ReleaseVariant:     m.ArenaScore.ReleaseVariant,
				ModelVariant:       m.ArenaScore.ModelVariant,
				Reasoning:          m.ArenaScore.Reasoning,
				Configuration:      m.ArenaScore.Configuration,
				Provider:           m.ArenaScore.Provider,
				Uncertainty:        m.ArenaScore.Uncertainty,
				SampleSize:         m.ArenaScore.SampleSize,
				Harness:            m.ArenaScore.Harness,
				Scaffold:           m.ArenaScore.Scaffold,
			}
		}
		m.QualityPrice, m.QualityPriceLabel, m.HasQualityPrice = m.ArenaQualityPrice, m.ArenaQualityPriceLabel, m.ArenaQualityPriceLabel != "" && m.ArenaRankable && !m.Free && m.MixedPrice > 0 && validScore(m.ArenaNormalized)
	}
	return out
}

func (m Model) scoreValue() float64 {
	if m.HasRankingScore {
		return m.RankingScore
	}
	if m.Score == nil {
		return 0
	}
	return m.Score.Value
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
			if a.scoreValue() != b.scoreValue() {
				return a.scoreValue() > b.scoreValue()
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

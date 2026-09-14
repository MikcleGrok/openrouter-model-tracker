package refresh

import (
	_ "embed"
	"fmt"
	"io"
	"text/template"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
)

//go:embed comparison.md.tmpl
var comparisonTemplate string

// FavoriteRow is one line of the "Фавориты по категориям" table. A row with no
// Model renders the Fallback text instead — that is how the "≈ Fable 5" line
// says there is no worthy candidate.
type FavoriteRow struct {
	TierLabel string
	Model     *model.Model
	Fallback  string
	Reason    string
}

// TierSection is one quality tier: its heading and its rows in table order.
type TierSection struct {
	Heading string
	Rows    []model.Model
}

// RenderData is everything the template needs. Nothing is computed inside the
// template: it only formats and iterates.
type RenderData struct {
	UpdatedDate string
	UpdatedNote string

	FavoritesIntro string
	Favorites      []FavoriteRow

	ClaudePrices []notes.ClaudePrice
	ClaudeNote   string

	SafetyIntro string
	Companies   []notes.Company
	SaferAI     string
	OpenWeights string

	TiersIntro string
	Tiers      []TierSection

	Tokens10Intro string
	ClaudeTokens  []notes.ClaudeTokens

	Caveats []string

	FreeIntro  string
	FreeModels []model.Model
	FreeTerms  string

	UnmappedIntro string
	Unmapped      []model.Model
}

var tmpl = template.Must(template.New("comparison").Funcs(template.FuncMap{
	"price":      pricing.FormatDollar,
	"ctx":        pricing.FormatContext,
	"tok":        pricing.FormatTokens10,
	"provenance": model.FormatScoreProvenance,
}).Parse(comparisonTemplate))

// Render writes the whole document. The previous file is never read: the
// document is a build artefact, regenerated from scratch every run.
func Render(w io.Writer, data RenderData) error {
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	return nil
}

// paidTiers is the order the quality-tier sections appear in. The free tier is
// rendered separately, in its own section at the bottom of the document.
var paidTiers = []string{"opus", "sonnet", "haiku"}

var tierHeadings = map[string]string{
	"opus":   ">≈ Opus 5",
	"sonnet": "≈ Sonnet 5",
	"haiku":  "<≈ Haiku 4.5",
	"free":   "<≈ Haiku 4.5 (бесплатная)",
}

// ClaudeEquivalent returns the table's Claude-relative label. The manual tier
// selects the Claude reference; only rankable haiku/free scores affect their
// thresholds. Q/P and price are not used.
func ClaudeEquivalent(m model.Model) string {
	switch m.Tier {
	case "opus":
		return ">≈ Opus 5"
	case "sonnet":
		return "≈ Sonnet 5"
	case "haiku", "free":
		if m.Score == nil || !m.Rankable {
			if m.Tier == "free" {
				return "<≈ Haiku 4.5"
			}
			return "≈ Haiku 4.5"
		}
		if scoreValue(m) >= 70 {
			return "≈ Haiku 4.5"
		}
		if scoreValue(m) >= 60 {
			return "<≈ Haiku 4.5"
		}
		return "<<≈ Haiku 4.5"
	}
	return "n/a"
}

// ClaudeEquivalentForSource neutralizes the haiku/free Claude labels under a
// non-SWE-bench score source: ClaudeEquivalent's haiku/free thresholds
// (>=70, >=60) are calibrated on SWE-bench Verified percentage points; after
// projection through model.ForScoreSource, an arena-mode Score.Value instead
// holds a min-max-normalized Arena position and a general-mode one holds a
// GPQA Diamond percentage, so running those thresholds on either would
// silently read one experiment's result as another's — exactly the
// cross-scale blending --score-source exists to prevent.
//
// The GPQA case is the more dangerous of the two and the reason this is a
// blanket "not swebench" rule rather than an arena special case: a GPQA
// percentage would sail through a threshold written for percentages and
// produce a confident, wrong Claude equivalence, where an Elo at least looks
// obviously out of range. There is no established mapping from either onto a
// Claude tier, so this deliberately does not attempt one, regardless of
// whether the row actually has a number on the active source. Opus/sonnet
// rows are unaffected: ClaudeEquivalent derives their label from Tier alone,
// never from a score value, so it stays correct under every source.
func ClaudeEquivalentForSource(m model.Model, source string) string {
	if source != model.ScoreSourceSWEBench && (m.Tier == "haiku" || m.Tier == "free") {
		return "n/a"
	}
	return ClaudeEquivalent(m)
}

func scoreValue(m model.Model) float64 {
	if m.HasRankingScore {
		return m.RankingScore
	}
	if m.Score == nil {
		return 0
	}
	return m.Score.Value
}

// BuildRenderData turns merged models plus the prose file into the flat,
// already-ordered structure the template iterates.
func BuildRenderData(models []model.Model, nt *notes.Notes, updated string) RenderData {
	d := RenderData{
		UpdatedDate:    updated,
		UpdatedNote:    nt.UpdatedNote(),
		FavoritesIntro: nt.Section("favorites_intro"),
		ClaudePrices:   nt.ClaudePrices(),
		ClaudeNote:     nt.ClaudeNote(),
		SafetyIntro:    nt.Section("safety_intro"),
		Companies:      nt.Companies(),
		SaferAI:        nt.Section("saferai"),
		OpenWeights:    nt.Section("open_weights"),
		TiersIntro:     nt.Section("tiers_intro"),
		Tokens10Intro:  nt.Section("tokens10_intro"),
		ClaudeTokens:   nt.ClaudeTokens(),
		Caveats:        nt.Caveats(),
		FreeIntro:      nt.Section("free_intro"),
		FreeTerms:      nt.Section("free_terms"),
		UnmappedIntro:  "Актуальные строки каталога без ручной benchmark identity.",
	}

	d.Favorites = append(d.Favorites, FavoriteRow{
		TierLabel: "≈ Fable 5",
		Fallback:  "нет достойного кандидата",
		Reason:    nt.FableVerdict(),
	})
	for _, tier := range append(append([]string{}, paidTiers...), "free") {
		ranked := model.RankFavorites(models, tier)
		for i := 0; i < 2 && i < len(ranked); i++ {
			label := tierHeadings[tier]
			if i == 1 {
				label = "↳ второй выбор"
			}
			m := ranked[i]
			d.Favorites = append(d.Favorites, FavoriteRow{
				TierLabel: label,
				Model:     &m,
				Reason:    nt.FavoriteReason(tier, m.Slug),
			})
		}
	}

	for _, tier := range paidTiers {
		rows := model.TierRows(models, tier)
		if len(rows) == 0 {
			continue
		}
		d.Tiers = append(d.Tiers, TierSection{Heading: tierHeadings[tier], Rows: rows})
	}
	d.FreeModels = model.TierRows(models, "free")
	for _, m := range models {
		if m.Unmapped {
			d.Unmapped = append(d.Unmapped, m)
		}
	}
	return d
}

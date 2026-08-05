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
}

var tmpl = template.Must(template.New("comparison").Funcs(template.FuncMap{
	"price": pricing.FormatPrice,
	"ctx":   pricing.FormatContext,
	"tok":   pricing.FormatTokens10,
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
	"opus":   "≈ Opus 5",
	"sonnet": "≈ Sonnet 5",
	"haiku":  "≈ Haiku 4.5 и ниже",
	"free":   "≈ Haiku 4.5 и ниже (бесплатная)",
}

// ClaudeEquivalent returns the manually classified Claude-level label for a model.
// It is deliberately based on the hand-maintained tier, not benchmark or price data.
func ClaudeEquivalent(m model.Model) string {
	if m.Tier == "free" && m.ClaudeRef != "" && m.ClaudeRef != notes.NeedsReview {
		return m.ClaudeRef
	}
	if label, ok := tierHeadings[m.Tier]; ok {
		return label
	}
	return "н/д"
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

	return d
}

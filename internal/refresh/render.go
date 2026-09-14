package refresh

import (
	_ "embed"
	"fmt"
	"io"
	"strings"
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

// ModelNote is one bullet of a table's "Заметки" list: a model whose curated
// prose is real, not the empty/needs-review placeholder. Tier and free
// tables carry their long-form Примечание as this list underneath the table
// instead of as an eleventh table column — see D3 in .task/omt-report/plan.md.
type ModelNote struct {
	Model model.Model
	Note  string
}

// TierSection is one quality tier: its heading, its rows in table order, and
// the subset of those rows that have a real note to show underneath.
type TierSection struct {
	Heading string
	Rows    []model.Model
	Notes   []ModelNote
}

// RankedRow is one row of the "Рейтинг моделей по цене и качеству" table:
// the model plus its 1-based position in the caller-supplied order
// (RenderOptions.Ranked). It carries the whole Model, not just display
// strings, because RenderHTML's sortable table needs the raw numeric fields
// behind the formatted cells.
type RankedRow struct {
	Index int
	Model model.Model
}

// ProvenanceRow is one bullet of the "Приложение: происхождение оценок"
// appendix: a model that has a score, plus its full FormatScoreProvenance
// dump. Dump is precomputed here (rather than left to the `provenance`
// template func on .Model.Score) only because that keeps the appendix
// looking uniform with the rest of RenderData's precomputed-string
// convention; either would produce identical output.
type ProvenanceRow struct {
	Model model.Model
	Dump  string
}

// RenderData is everything the template needs. Nothing is computed inside the
// template: it only formats and iterates.
type RenderData struct {
	UpdatedDate string
	UpdatedNote string

	// GenerationLine is the one-line summary of non-default generation
	// parameters ("Ranking: mixed-utility · Sort: q/p · Score source:
	// swebench"), printed right under "Обновлено:" so a reader of a
	// non-default document immediately sees that it is non-default. Empty
	// under the zero-value RenderOptions refresh passes (no Ranked list —
	// there is no ranking to describe), so refresh's document prints nothing
	// extra here.
	GenerationLine string
	// ScoreSource is the raw --score-source value ("swebench"/"arena"/
	// "general", or "" for refresh's default swebench-shaped view), read by
	// the `claude` template func to neutralize haiku/free Claude labels
	// under a non-SWE-bench view in the Ranked table.
	ScoreSource string
	// ScoreColumnHeader names the Ranked table's score column after the
	// active experiment ("SWE %"/"Arena Elo"/"GPQA %") — see
	// docs/reference.md's rule that a score column must never be called
	// something as generic as "Score".
	ScoreColumnHeader string

	FavoritesIntro string
	Favorites      []FavoriteRow

	// Ranked is the "Рейтинг моделей по цене и качеству" table's rows, in
	// the exact order RenderOptions.Ranked was given — nil means the section
	// is not printed at all, which is what refresh's zero-value
	// RenderOptions produces, since internal/refresh has no CLI-level sort
	// to describe (see RenderOptions.Ranked's own doc comment).
	Ranked []RankedRow

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
	FreeNotes  []ModelNote
	FreeTerms  string

	UnmappedIntro string
	Unmapped      []model.Model

	// Provenance is the "Приложение: происхождение оценок" appendix: one row
	// per model that has a score, in RenderOptions.Ranked order when a
	// ranked list was supplied, else in the input models order. Printed only
	// when non-empty.
	Provenance []ProvenanceRow
}

// scoreSourceDisplayNames maps a ScoreInfo.SourceFamily id (the model-map.tsv
// source id a fetched row was stamped with, e.g. "vals") to the
// human-readable site name a tier-table score cell links to. "gpqa" shares
// vals.ai's site under a different experiment, so it maps to the same name —
// see model.SourceFamily's own doc comment for why the two id spaces are
// kept apart even where they coincide.
var scoreSourceDisplayNames = map[string]string{
	"vals":     "vals.ai",
	"swebench": "swebench.com",
	"arena":    "arena.ai",
	"gpqa":     "vals.ai",
}

// scoreRefMarkdown is the "проверка" (tier-table) level of provenance from
// D3's three-level hierarchy: a short, human-readable pointer to where a
// score came from and when it was last checked — as opposed to the bare
// number in the ranked list ("обзор") or the full FormatScoreProvenance dump
// in the appendix ("аудит"). It returns "" for a nil info or one with no
// source URL to link, so a manual/legacy score with nothing to point at adds
// no dangling " · []()" text.
func scoreRefMarkdown(info *model.ScoreInfo) string {
	if info == nil || info.SourceURL == "" {
		return ""
	}
	name, ok := scoreSourceDisplayNames[info.SourceFamily]
	if !ok {
		name = "источник"
	}
	checked := info.Checked
	if checked == "" {
		checked = "n/a"
	}
	return fmt.Sprintf(" · [%s](%s), %s", name, info.SourceURL, checked)
}

// taskFitLine formats TaskFit for the ranked list's long form (docs, not
// terminal, so no width pressure to abbreviate to the IDFT single-letter
// form table.go's short mode uses): "implement + debug + refactor + test",
// or "—" for a model with no manual task-fit keywords.
func taskFitLine(fit []string) string {
	if len(fit) == 0 {
		return "—"
	}
	return strings.Join(fit, " + ")
}

// scoreColumnHeaderFor names the Ranked table's score column after the
// active experiment. It deliberately duplicates cmd/openrouter/table.go's
// scoreColumnHeader instead of importing it: internal/refresh must not
// depend on cmd/openrouter (the inverse of the real dependency direction),
// and the mapping itself is three lines that have to stay in only two
// places, not become a shared package for its own sake.
func scoreColumnHeaderFor(source string) string {
	switch source {
	case model.ScoreSourceArena:
		return "Arena Elo"
	case model.ScoreSourceGeneral:
		return "GPQA %"
	default:
		return "SWE %"
	}
}

var tmpl = template.Must(template.New("comparison").Funcs(template.FuncMap{
	"price":      pricing.FormatDollar,
	"ctx":        pricing.FormatContext,
	"tok":        pricing.FormatTokens10,
	"provenance": model.FormatScoreProvenance,
	"marker":     model.ScoreSourceMarker,
	"scoreref":   scoreRefMarkdown,
	"taskfit":    taskFitLine,
	"claude":     ClaudeEquivalentForSource,
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

// RenderOptions selects how the ranked list is ordered and labelled. The zero
// value reproduces the report-less document exactly — refresh passes it as
// it always has, via BuildRenderData.
type RenderOptions struct {
	// ScoreSource is the raw --score-source value; "" means refresh's
	// default (SWE-bench-shaped) view.
	ScoreSource string
	// SortLabel and RankingLabel are the --sort/--ranking values as already
	// formatted for display (e.g. "q/p", "mixed-utility") — report resolves
	// these itself; RenderOptions only carries strings to print.
	SortLabel    string
	RankingLabel string
	// Ranked is the already-sorted model set for the "Рейтинг" table and,
	// absent a better order, the appendix — nil means the "Рейтинг" section
	// is not printed at all. Sorting lives in cmd/openrouter (it depends on
	// internal/config/internal/ranking), so internal/refresh never sorts;
	// it only ever iterates the order it is handed. See D3 in
	// .task/omt-report/plan.md for why this is a plain slice, not sort keys.
	Ranked []model.Model
}

// modelNotesFor filters rows down to the ones with a real, non-placeholder
// note — the "Заметки" list underneath a tier/free table. A model with no
// note, or one still carrying notes.NeedsReview, contributes nothing: a
// "_нужен обзор_" bullet on every unwritten row would be exactly the
// low-signal noise D3 explicitly retires.
func modelNotesFor(rows []model.Model) []ModelNote {
	var out []ModelNote
	for _, m := range rows {
		if m.Note == "" || m.Note == notes.NeedsReview {
			continue
		}
		out = append(out, ModelNote{Model: m, Note: m.Note})
	}
	return out
}

// BuildRenderData turns merged models plus the prose file into the flat,
// already-ordered structure the template iterates. It is
// BuildRenderDataWithOptions with a zero RenderOptions — refresh, which has
// no CLI-level sort of its own, calls this directly.
func BuildRenderData(models []model.Model, nt *notes.Notes, updated string) RenderData {
	return BuildRenderDataWithOptions(models, nt, updated, RenderOptions{})
}

// BuildRenderDataWithOptions is BuildRenderData plus the "Рейтинг"
// table/appendix ordering and labelling report needs. See RenderOptions for
// what each field controls.
func BuildRenderDataWithOptions(models []model.Model, nt *notes.Notes, updated string, opts RenderOptions) RenderData {
	d := RenderData{
		UpdatedDate:       updated,
		UpdatedNote:       nt.UpdatedNote(),
		ScoreSource:       opts.ScoreSource,
		ScoreColumnHeader: scoreColumnHeaderFor(opts.ScoreSource),
		FavoritesIntro:    nt.Section("favorites_intro"),
		ClaudePrices:      nt.ClaudePrices(),
		ClaudeNote:        nt.ClaudeNote(),
		SafetyIntro:       nt.Section("safety_intro"),
		Companies:         nt.Companies(),
		SaferAI:           nt.Section("saferai"),
		OpenWeights:       nt.Section("open_weights"),
		TiersIntro:        nt.Section("tiers_intro"),
		Tokens10Intro:     nt.Section("tokens10_intro"),
		ClaudeTokens:      nt.ClaudeTokens(),
		Caveats:           nt.Caveats(),
		FreeIntro:         nt.Section("free_intro"),
		FreeTerms:         nt.Section("free_terms"),
		UnmappedIntro:     "Актуальные строки каталога без ручной benchmark identity.",
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

	if opts.Ranked != nil {
		d.GenerationLine = fmt.Sprintf("Ranking: %s · Sort: %s · Score source: %s", opts.RankingLabel, opts.SortLabel, opts.ScoreSource)
		d.Ranked = make([]RankedRow, len(opts.Ranked))
		for i, m := range opts.Ranked {
			d.Ranked[i] = RankedRow{Index: i + 1, Model: m}
		}
	}

	for _, tier := range paidTiers {
		rows := model.TierRows(models, tier)
		if len(rows) == 0 {
			continue
		}
		d.Tiers = append(d.Tiers, TierSection{Heading: tierHeadings[tier], Rows: rows, Notes: modelNotesFor(rows)})
	}
	d.FreeModels = model.TierRows(models, "free")
	d.FreeNotes = modelNotesFor(d.FreeModels)
	for _, m := range models {
		if m.Unmapped {
			d.Unmapped = append(d.Unmapped, m)
		}
	}

	// The appendix follows the ranked order when one was supplied — the same
	// order the reader just saw in "Рейтинг" — and otherwise the input
	// models order, so refresh's document (no ranked list at all) still
	// carries the full audit trail D3 requires, just in merge order.
	provenanceSource := opts.Ranked
	if provenanceSource == nil {
		provenanceSource = models
	}
	for _, m := range provenanceSource {
		if m.Score == nil {
			continue
		}
		d.Provenance = append(d.Provenance, ProvenanceRow{Model: m, Dump: model.FormatScoreProvenance(m.Score)})
	}
	return d
}

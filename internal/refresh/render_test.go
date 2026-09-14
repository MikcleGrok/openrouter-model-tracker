package refresh

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
)

func goldenModels() (luna, sol, nemo model.Model) {
	luna = model.Model{
		Slug: "openai/gpt-5.6-luna", DisplayName: "GPT-5.6 Luna", Tier: "opus",
		InPerM: 0.5, OutPerM: 3, Context: 1000000,
		Score: &model.ScoreInfo{
			Metric: "SWE-bench Verified", Value: 93.0, VariantMeasured: "openai/gpt-5.6-luna",
			SourceFamily: "vals", SourceURL: "https://www.vals.ai/benchmarks/swebench", Checked: "2026-08-01",
		},
		ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Rankable: true,
		Owner: "OpenAI (C)", OpenWeights: "нет", Copyright: "unknown", Note: "Независимая оценка (vals.ai).",
		Tokens10In: pricing.Tokens10(0.5), Tokens10Out: pricing.Tokens10(3), Tokens10Mixed: pricing.Tokens10(1.125),
		LongContextPriceLabel: "$1.00 / $4.00 от 272K+",
		LongContextInLabel:    "$1.00 от 272K+",
		LongContextOutLabel:   "$4.00 от 272K+",
	}
	sol = model.Model{
		Slug: "openai/gpt-5.6-sol", DisplayName: "GPT-5.6 Sol", Tier: "opus",
		InPerM: 5, OutPerM: 30, Context: 1000000,
		Score: &model.ScoreInfo{
			Metric: "SWE-bench Verified", Value: 96.2, VariantMeasured: "openai/gpt-5.6-sol",
			SourceFamily: "swebench", SourceURL: "https://www.swebench.com/", Checked: "2026-08-02",
		},
		ScoreLabel: "96.2%", QualityPriceLabel: "8.6", Rankable: true,
		Owner: "OpenAI (C)", OpenWeights: "нет", Copyright: "unknown", Note: "Оговорка METR сохраняется.",
		Tokens10In: pricing.Tokens10(5), Tokens10Out: pricing.Tokens10(30), Tokens10Mixed: pricing.Tokens10(11.25),
	}
	nemo = model.Model{
		Slug: "nvidia/nemotron-3-ultra-550b-a55b:free", DisplayName: "NVIDIA Nemotron 3 Ultra", Tier: "free",
		Context: 1000000, Free: true,
		Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70.4, VariantMeasured: "vendor-claimed"},
		ScoreLabel: "65–70.4% (только вендор)", QualityPriceLabel: "n/a (free)", Rankable: true,
		// ClaudeRef and Note deliberately carry '<' and '&': the Markdown
		// path passes them through verbatim (this is a plain-text document),
		// but render_html_test.go's escaping tests reuse this same fixture
		// against html/template's auto-escaping, on the real shape notes.yaml
		// already contains today (e.g. "<≈ Haiku 4.5").
		ClaudeRef: "<≈ Haiku 4.5 (бесплатная)", Owner: "NVIDIA", OpenWeights: "да, OpenMDW-1.1", Copyright: "unknown",
		Note: "550B/55B-active MoE, vendor-claimed & не подтверждено независимо.",
	}
	return
}

// goldenRanked returns the three golden models in a fixed, deliberately
// non-quality/price order (sol, luna, nemo) so the ranked-list golden output
// is visibly not "just the tier order" — see TestRenderRankedListHonoursCallerOrder.
func goldenRanked(luna, sol, nemo model.Model) []model.Model {
	return []model.Model{sol, luna, nemo}
}

func goldenData() RenderData {
	luna, sol, nemo := goldenModels()
	ranked := goldenRanked(luna, sol, nemo)
	rankedRows := make([]RankedRow, len(ranked))
	for i, m := range ranked {
		rankedRows[i] = RankedRow{Index: i + 1, Model: m}
	}
	var provenance []ProvenanceRow
	for _, m := range ranked {
		provenance = append(provenance, ProvenanceRow{Model: m, Dump: model.FormatScoreProvenance(m.Score)})
	}
	return RenderData{
		UpdatedDate:       "2026-08-04",
		UpdatedNote:       "цены и оценки собраны автоматически",
		GenerationLine:    "Ranking: mixed-utility · Sort: q/p · Score source: swebench",
		ScoreSource:       "swebench",
		ScoreColumnHeader: "SWE %",
		FavoritesIntro:    "Один лучший вариант на каждый уровень качества Claude.",
		Favorites: []FavoriteRow{
			{TierLabel: "≈ Fable 5", Fallback: "нет достойного кандидата", Reason: "Ни одна проверенная модель независимо не подтверждает Fable-уровень."},
			{TierLabel: ">≈ Opus 5", Model: &luna, Reason: "Лучшее соотношение цена/качество."},
			{TierLabel: "↳ второй выбор", Model: &sol, Reason: "Ближе всего к Opus 5 по сырой оценке."},
		},
		Ranked:       rankedRows,
		ClaudePrices: []notes.ClaudePrice{{Model: "Claude Opus 5", In: "$5", Out: "$25", Context: "1M", Note: "—"}},
		ClaudeNote:   "На OpenRouter цены Claude совпадают с прайсом Anthropic 1:1.",
		SafetyIntro:  "Рейтинг безопасности — оценка компании в целом, а не модели.",
		Companies:    []notes.Company{{Name: "OpenAI", Grade: "C (2.28)", Comment: "Лидирует в категории Risk Assessment"}},
		SaferAI:      "SaferAI Frontier Risk Management Tracker: OpenAI 34%.",
		OpenWeights:  "Полностью закрытые: всё OpenAI.",
		TiersIntro:   "Категории — по примерному уровню качества относительно Claude.",
		Tiers: []TierSection{{Heading: ">≈ Opus 5", Tier: "opus", Rows: []model.Model{luna, sol}, Notes: []ModelNote{
			{Model: luna, Note: luna.Note},
			{Model: sol, Note: sol.Note},
		}}},
		Tokens10Intro: "Смешанное соотношение 3:1 (вход:выход).",
		ClaudeTokens:  []notes.ClaudeTokens{{Model: "Claude Opus 5", In: "2.00", Out: "0.40", Mixed: "1.00"}},
		Caveats:       []string{"Цены на OpenRouter меняются часто.", "Бенчмарки сильно зависят от скаффолда."},
		FreeIntro:     "Модели с ценой $0/$0 — из каталога OpenRouter.",
		FreeModels:    []model.Model{nemo},
		FreeNotes:     []ModelNote{{Model: nemo, Note: nemo.Note}},
		FreeTerms:     "Для всех `:free`-моделей: rate-limit 20 запросов/мин.",
		Provenance:    provenance,
	}
}

func TestRenderGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, goldenData()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenPath := filepath.Join("testdata", "golden.md")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if buf.String() == string(want) {
		return
	}

	gotLines := strings.Split(buf.String(), "\n")
	wantLines := strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := "", ""
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Fatalf("render differs from %s at line %d:\n  got:  %q\n  want: %q", goldenPath, i+1, g, w)
		}
	}
	t.Fatalf("render differs from %s but no differing line was found (trailing bytes?)", goldenPath)
}

// isTableSeparatorLine reports whether a trimmed line is a GFM table's own
// "|---|---|" separator row — the line that opens a table's scope for
// TestRenderProducesNoTableBreakingLines below.
func isTableSeparatorLine(line string) bool {
	if !strings.HasPrefix(line, "|") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ':
		default:
			return false
		}
	}
	return true
}

// assertNoTableBreakingLines walks doc line by line: after any GFM
// "|---|---|" table separator row, every non-blank line up to the first
// blank line must itself start with "|" — a bare line in that span (like the
// original bug's "  Provenance: ...") ends the table early and starts a new
// one-row table on the very next "|"-prefixed line.
func assertNoTableBreakingLines(t *testing.T, doc string) {
	t.Helper()
	lines := strings.Split(doc, "\n")
	inTable := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case isTableSeparatorLine(trimmed):
			inTable = true
		case !inTable:
			continue
		case trimmed == "":
			inTable = false
		case !strings.HasPrefix(trimmed, "|"):
			t.Fatalf("line %d breaks its table (does not start with |): %q", i+1, line)
		}
	}
}

// TestRenderProducesNoTableBreakingLines is the regression test for the bug
// this whole feature exists to fix: internal/refresh/comparison.md.tmpl used
// to print "  Provenance: ..." as a bare, non-"|"-prefixed line right after a
// table row, which GFM reads as the end of the table (and the start of a new
// one-row table). Existed before the redesign, this test would have caught
// the defect at the moment it was introduced.
func TestRenderProducesNoTableBreakingLines(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, goldenData()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNoTableBreakingLines(t, buf.String())
}

func TestRenderRankedListHonoursCallerOrder(t *testing.T) {
	nt, err := notes.Load(filepath.Join("..", "notes", "testdata", "notes.yaml"))
	if err != nil {
		t.Fatalf("notes.Load: %v", err)
	}
	luna, sol, nemo := goldenModels()
	luna.QualityPrice = 82.7
	sol.QualityPrice = 8.6

	// Deliberately not quality/price order (luna would rank first there under
	// RankFavorites/TierRows): sol first, luna second — the kind of order a
	// caller passes under e.g. --sort name.
	models := []model.Model{sol, luna, nemo}
	callerOrder := []model.Model{sol, luna, nemo}
	d := BuildRenderDataWithOptions(models, nt, "2026-08-04", RenderOptions{
		ScoreSource: "swebench", SortLabel: "name", RankingLabel: "mixed-utility", Ranked: callerOrder,
	})

	if len(d.Ranked) != 3 {
		t.Fatalf("Ranked = %+v, want 3 rows", d.Ranked)
	}
	wantSlugs := []string{sol.Slug, luna.Slug, nemo.Slug}
	for i, want := range wantSlugs {
		if d.Ranked[i].Model.Slug != want || d.Ranked[i].Index != i+1 {
			t.Fatalf("Ranked[%d] = %+v, want slug %q index %d", i, d.Ranked[i], want, i+1)
		}
	}
	if want := "Ranking: mixed-utility · Sort: name · Score source: swebench"; d.GenerationLine != want {
		t.Fatalf("GenerationLine = %q, want %q", d.GenerationLine, want)
	}

	// Favorites/Tiers are unaffected by Ranked's order: the opus favourite is
	// still whichever of luna/sol has the higher quality/price.
	if d.Favorites[1].Model.Slug != luna.Slug {
		t.Fatalf("Favorites[1] (opus favourite) = %+v, want luna (higher Q/P) regardless of Ranked order", d.Favorites[1])
	}
	if len(d.Tiers) != 1 || d.Tiers[0].Rows[0].Slug != luna.Slug {
		t.Fatalf("Tiers[0].Rows = %+v, want luna first (ranked by Q/P), independent of Ranked order", d.Tiers[0])
	}
}

func TestClaudeEquivalentForSourceNeutralizesHaikuFree(t *testing.T) {
	haiku := model.Model{Tier: "haiku", Score: &model.ScoreInfo{Value: 80}, Rankable: true}
	free := model.Model{Tier: "free"}
	opus := model.Model{Tier: "opus"}
	sonnet := model.Model{Tier: "sonnet"}

	for _, source := range []string{model.ScoreSourceArena, model.ScoreSourceGeneral} {
		if got := ClaudeEquivalentForSource(haiku, source); got != "n/a" {
			t.Errorf("ClaudeEquivalentForSource(haiku, %q) = %q, want n/a", source, got)
		}
		if got := ClaudeEquivalentForSource(free, source); got != "n/a" {
			t.Errorf("ClaudeEquivalentForSource(free, %q) = %q, want n/a", source, got)
		}
		if got, want := ClaudeEquivalentForSource(opus, source), ClaudeEquivalent(opus); got != want {
			t.Errorf("ClaudeEquivalentForSource(opus, %q) = %q, want unchanged %q", source, got, want)
		}
		if got, want := ClaudeEquivalentForSource(sonnet, source), ClaudeEquivalent(sonnet); got != want {
			t.Errorf("ClaudeEquivalentForSource(sonnet, %q) = %q, want unchanged %q", source, got, want)
		}
	}
	// swebench (the default) is unaffected: haiku keeps its normal
	// threshold-based logic instead of being blanket-neutralized.
	if got, want := ClaudeEquivalentForSource(haiku, model.ScoreSourceSWEBench), ClaudeEquivalent(haiku); got != want {
		t.Errorf("ClaudeEquivalentForSource(haiku, swebench) = %q, want unchanged %q", got, want)
	}
}

func TestBuildRenderData(t *testing.T) {
	nt, err := notes.Load(filepath.Join("..", "notes", "testdata", "notes.yaml"))
	if err != nil {
		t.Fatalf("notes.Load: %v", err)
	}

	luna, sol, nemo := goldenModels()
	weak := model.Model{
		Slug: "deepseek/deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro", Tier: "opus",
		InPerM: 0.435, OutPerM: 0.87, MixedPrice: 0.54375,
		ScoreLabel: "n/a", QualityPriceLabel: "n/a (variant mismatch)", Rankable: false,
	}
	luna.QualityPrice = 82.7
	sol.QualityPrice = 8.6

	d := BuildRenderData([]model.Model{sol, luna, weak, nemo}, nt, "2026-08-04")

	if d.UpdatedDate != "2026-08-04" || d.UpdatedNote != "цены и оценки собраны автоматически" {
		t.Errorf("updated line = %q / %q", d.UpdatedDate, d.UpdatedNote)
	}
	if d.TiersIntro != "Категории — по примерному уровню качества относительно Claude." {
		t.Errorf("TiersIntro = %q, want the notes.yaml section", d.TiersIntro)
	}
	if d.ScoreColumnHeader != "SWE %" {
		t.Errorf("ScoreColumnHeader = %q, want the swebench default of \"SWE %%\" under the zero-value RenderOptions", d.ScoreColumnHeader)
	}
	if d.GenerationLine != "" {
		t.Errorf("GenerationLine = %q, want empty under the zero-value RenderOptions (refresh has no ranked list to describe)", d.GenerationLine)
	}
	if d.Ranked != nil {
		t.Errorf("Ranked = %+v, want nil under the zero-value RenderOptions", d.Ranked)
	}

	if len(d.Favorites) != 4 {
		t.Fatalf("got %d favourite rows, want 4 (Fable placeholder + opus #1 and #2 + free #1): %+v", len(d.Favorites), d.Favorites)
	}
	if d.Favorites[0].Model != nil || d.Favorites[0].TierLabel != "≈ Fable 5" {
		t.Errorf("favourites[0] = %+v, want the Fable placeholder row first", d.Favorites[0])
	}
	if d.Favorites[0].Reason != nt.FableVerdict() {
		t.Errorf("favourites[0].Reason = %q, want the fable_verdict text", d.Favorites[0].Reason)
	}
	if d.Favorites[1].TierLabel != ">≈ Opus 5" || d.Favorites[1].Model.Slug != "openai/gpt-5.6-luna" {
		t.Errorf("favourites[1] = %+v, want luna as the opus favourite (82.7 > 8.6)", d.Favorites[1])
	}
	if d.Favorites[1].Reason != "Лучшее соотношение цена/качество в Opus-тире." {
		t.Errorf("favourites[1].Reason = %q, want the notes.yaml reason", d.Favorites[1].Reason)
	}
	if d.Favorites[2].TierLabel != "↳ второй выбор" || d.Favorites[2].Model.Slug != "openai/gpt-5.6-sol" {
		t.Errorf("favourites[2] = %+v, want sol as the second choice", d.Favorites[2])
	}
	if d.Favorites[2].Reason != notes.NeedsReview {
		t.Errorf("favourites[2].Reason = %q, want %q — the fixture has no reason for sol", d.Favorites[2].Reason, notes.NeedsReview)
	}
	if d.Favorites[3].Model.Slug != nemo.Slug {
		t.Errorf("favourites[3] = %+v, want the free-tier favourite", d.Favorites[3])
	}

	if len(d.Tiers) != 1 || d.Tiers[0].Heading != ">≈ Opus 5" {
		t.Fatalf("Tiers = %+v, want a single non-empty opus section", d.Tiers)
	}
	got := []string{}
	for _, m := range d.Tiers[0].Rows {
		got = append(got, m.Slug)
	}
	want := []string{"openai/gpt-5.6-luna", "openai/gpt-5.6-sol", "deepseek/deepseek-v4-pro"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("opus rows = %v, want %v (unrankable rows go last)", got, want)
		}
	}
	notesGot := map[string]bool{}
	for _, n := range d.Tiers[0].Notes {
		notesGot[n.Model.Slug] = true
	}
	if len(d.Tiers[0].Notes) != 2 || !notesGot[luna.Slug] || !notesGot[sol.Slug] {
		t.Errorf("Tiers[0].Notes = %+v, want exactly luna and sol (weak has no note to show)", d.Tiers[0].Notes)
	}

	if len(d.FreeModels) != 1 || d.FreeModels[0].Slug != nemo.Slug {
		t.Errorf("FreeModels = %+v, want the free tier in its own slice, out of Tiers", d.FreeModels)
	}
	if len(d.FreeNotes) != 1 || d.FreeNotes[0].Model.Slug != nemo.Slug {
		t.Errorf("FreeNotes = %+v, want nemo's real note", d.FreeNotes)
	}
	if len(d.Caveats) != 2 || len(d.Companies) != 2 || len(d.ClaudePrices) != 2 {
		t.Errorf("static blocks not pulled from notes.yaml: caveats %d, companies %d, claude prices %d",
			len(d.Caveats), len(d.Companies), len(d.ClaudePrices))
	}

	if len(d.Provenance) != 3 {
		t.Fatalf("Provenance = %+v, want one row per scored model (luna, sol, nemo — weak has no Score)", d.Provenance)
	}
	// Ranked is nil here, so Provenance falls back to the input models order:
	// sol, luna, weak (skipped, no Score), nemo.
	wantProvenance := []string{sol.Slug, luna.Slug, nemo.Slug}
	for i, want := range wantProvenance {
		if d.Provenance[i].Model.Slug != want {
			t.Fatalf("Provenance[%d].Model.Slug = %q, want %q (input-models order, since Ranked is nil)", i, d.Provenance[i].Model.Slug, want)
		}
	}
}

func TestRenderNormalizesLegacyMissingLabelsInMarkdownProvenance(t *testing.T) {
	var output bytes.Buffer
	m := model.Model{
		DisplayName: "Demo", Slug: "demo/model",
		Score:             &model.ScoreInfo{Value: 1, Metric: "н/д", Uncertainty: "н/д"},
		ScoreLabel:        "1.0%",
		QualityPriceLabel: "n/a", Owner: "Demo (n/a)", OpenWeights: "n/a",
	}
	data := RenderData{
		Tiers:      []TierSection{{Heading: "Tier", Rows: []model.Model{m}}},
		Provenance: []ProvenanceRow{{Model: m, Dump: model.FormatScoreProvenance(m.Score)}},
	}
	if err := Render(&output, data); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "н/д") {
		t.Fatalf("rendered Markdown contains legacy missing label:\n%s", text)
	}
	// The provenance dump moved out of the table and into the appendix (D3):
	// confirm the appendix bullet for this model is actually there, not just
	// that "н/д" is absent — a test that only checks absence would pass
	// vacuously if the appendix silently disappeared.
	if !strings.Contains(text, "## Приложение: происхождение оценок") {
		t.Fatalf("rendered Markdown has no provenance appendix:\n%s", text)
	}
	if !strings.Contains(text, "`demo/model` —") || !strings.Contains(text, "uncertainty=n/a") {
		t.Fatalf("appendix bullet for demo/model missing or malformed:\n%s", text)
	}
}

func TestRenderIncludesUnmappedRowsWithoutRankingIdentity(t *testing.T) {
	var output bytes.Buffer
	data := RenderData{UnmappedIntro: "catalog", Unmapped: []model.Model{{Slug: "new/model", DisplayName: "New Model", InPerM: 1, OutPerM: 2, Context: 4096}}}
	if err := Render(&output, data); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Актуальные модели без ручного сопоставления") || !strings.Contains(text, "new/model") || !strings.Contains(text, "missing identity") {
		t.Fatalf("unmapped row missing from Markdown:\n%s", text)
	}
	if strings.Contains(text, "| New Model | new/model | 1 | 2 | 4K |  |") {
		t.Fatalf("unmapped row was rendered as a ranked row")
	}
}

func TestClaudeHeadingsUseNamedReferencesAndExactOperators(t *testing.T) {
	want := map[string]string{
		"opus":   ">≈ Opus 5",
		"sonnet": "≈ Sonnet 5",
		"haiku":  "<≈ Haiku 4.5",
		"free":   "<≈ Haiku 4.5 (бесплатная)",
	}
	for tier, expected := range want {
		if got := tierHeadings[tier]; got != expected {
			t.Errorf("tierHeadings[%q] = %q, want %q", tier, got, expected)
		}
	}
}

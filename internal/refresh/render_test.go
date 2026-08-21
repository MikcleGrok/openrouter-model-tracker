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
		Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 93.0, VariantMeasured: "openai/gpt-5.6-luna"},
		ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Rankable: true,
		Owner: "OpenAI (C)", OpenWeights: "нет", CopyrightGuardrail: "unknown", Note: "Независимая оценка (vals.ai).",
		Tokens10In: pricing.Tokens10(0.5), Tokens10Out: pricing.Tokens10(3), Tokens10Mixed: pricing.Tokens10(1.125),
		LongContextPriceLabel: "$1.00 / $4.00 от 272K+",
		LongContextInLabel:    "$1.00 от 272K+",
		LongContextOutLabel:   "$4.00 от 272K+",
	}
	sol = model.Model{
		Slug: "openai/gpt-5.6-sol", DisplayName: "GPT-5.6 Sol", Tier: "opus",
		InPerM: 5, OutPerM: 30, Context: 1000000,
		Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 96.2, VariantMeasured: "openai/gpt-5.6-sol"},
		ScoreLabel: "96.2%", QualityPriceLabel: "8.6", Rankable: true,
		Owner: "OpenAI (C)", OpenWeights: "нет", CopyrightGuardrail: "unknown", Note: "Оговорка METR сохраняется.",
		Tokens10In: pricing.Tokens10(5), Tokens10Out: pricing.Tokens10(30), Tokens10Mixed: pricing.Tokens10(11.25),
	}
	nemo = model.Model{
		Slug: "nvidia/nemotron-3-ultra-550b-a55b:free", DisplayName: "NVIDIA Nemotron 3 Ultra", Tier: "free",
		Context: 1000000, Free: true,
		Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70.4, VariantMeasured: "vendor-claimed"},
		ScoreLabel: "65–70.4% (только вендор)", QualityPriceLabel: "n/a (free)", Rankable: true,
		ClaudeRef: "<≈ Haiku 4.5 (бесплатная)", Owner: "NVIDIA", OpenWeights: "да, OpenMDW-1.1", CopyrightGuardrail: "unknown",
		Note: "550B/55B-active MoE.",
	}
	return
}

func goldenData() RenderData {
	luna, sol, nemo := goldenModels()
	return RenderData{
		UpdatedDate:    "2026-08-04",
		UpdatedNote:    "цены и оценки собраны автоматически",
		FavoritesIntro: "Один лучший вариант на каждый уровень качества Claude.",
		Favorites: []FavoriteRow{
			{TierLabel: "≈ Fable 5", Fallback: "нет достойного кандидата", Reason: "Ни одна проверенная модель независимо не подтверждает Fable-уровень."},
			{TierLabel: ">≈ Opus 5", Model: &luna, Reason: "Лучшее соотношение цена/качество."},
			{TierLabel: "↳ второй выбор", Model: &sol, Reason: "Ближе всего к Opus 5 по сырой оценке."},
		},
		ClaudePrices:  []notes.ClaudePrice{{Model: "Claude Opus 5", In: "$5", Out: "$25", Context: "1M", Note: "—"}},
		ClaudeNote:    "На OpenRouter цены Claude совпадают с прайсом Anthropic 1:1.",
		SafetyIntro:   "Рейтинг безопасности — оценка компании в целом, а не модели.",
		Companies:     []notes.Company{{Name: "OpenAI", Grade: "C (2.28)", Comment: "Лидирует в категории Risk Assessment"}},
		SaferAI:       "SaferAI Frontier Risk Management Tracker: OpenAI 34%.",
		OpenWeights:   "Полностью закрытые: всё OpenAI.",
		TiersIntro:    "Категории — по примерному уровню качества относительно Claude.",
		Tiers:         []TierSection{{Heading: ">≈ Opus 5", Rows: []model.Model{luna, sol}}},
		Tokens10Intro: "Смешанное соотношение 3:1 (вход:выход).",
		ClaudeTokens:  []notes.ClaudeTokens{{Model: "Claude Opus 5", In: "2.00", Out: "0.40", Mixed: "1.00"}},
		Caveats:       []string{"Цены на OpenRouter меняются часто.", "Бенчмарки сильно зависят от скаффолда."},
		FreeIntro:     "Модели с ценой $0/$0 — из каталога OpenRouter.",
		FreeModels:    []model.Model{nemo},
		FreeTerms:     "Для всех `:free`-моделей: rate-limit 20 запросов/мин.",
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

	if len(d.FreeModels) != 1 || d.FreeModels[0].Slug != nemo.Slug {
		t.Errorf("FreeModels = %+v, want the free tier in its own slice, out of Tiers", d.FreeModels)
	}
	if len(d.Caveats) != 2 || len(d.Companies) != 2 || len(d.ClaudePrices) != 2 {
		t.Errorf("static blocks not pulled from notes.yaml: caveats %d, companies %d, claude prices %d",
			len(d.Caveats), len(d.Companies), len(d.ClaudePrices))
	}
}

func TestRenderNormalizesLegacyMissingLabelsInMarkdownProvenance(t *testing.T) {
	var output bytes.Buffer
	data := RenderData{Tiers: []TierSection{{Heading: "Tier", Rows: []model.Model{{DisplayName: "Demo", Slug: "demo/model", Score: &model.ScoreInfo{Value: 1, Metric: "н/д", Uncertainty: "н/д"}, ScoreLabel: "1.0%", QualityPriceLabel: "n/a", Owner: "Demo (n/a)", OpenWeights: "n/a"}}}}}
	if err := Render(&output, data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "н/д") {
		t.Fatalf("rendered Markdown contains legacy missing label:\n%s", output.String())
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

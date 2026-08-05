package model

import (
	"path/filepath"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

func testNotes(t *testing.T) *notes.Notes {
	t.Helper()
	n, err := notes.Load(filepath.Join("..", "notes", "testdata", "notes.yaml"))
	if err != nil {
		t.Fatalf("notes.Load: %v", err)
	}
	return n
}

func testEntries() []modelmap.Entry {
	return []modelmap.Entry{
		{Slug: "openai/gpt-5.6-luna", Tier: "opus", Names: map[string]string{"vals": "openai/gpt-5.6-luna"}},
		{Slug: "minimax/minimax-m3", Tier: "sonnet", Names: map[string]string{}},
		{Slug: "deepseek/deepseek-v4-pro", Tier: "sonnet", Names: map[string]string{}},
		{Slug: "x-ai/grok-4.1-fast", Tier: "sonnet", Names: map[string]string{}},
		{Slug: "nvidia/nemotron-3-ultra-550b-a55b:free", Tier: "free", Names: map[string]string{}},
	}
}

func testPrices() map[string]sources.PriceInfo {
	return map[string]sources.PriceInfo{
		"openai/gpt-5.6-luna": {
			Slug: "openai/gpt-5.6-luna", InPerM: 0.5, OutPerM: 3, Context: 1000000, Found: true,
			HasOverride: true, OverrideMinTokens: 272000, OverrideInPerM: 1, OverrideOutPerM: 4,
		},
		"minimax/minimax-m3":                     {Slug: "minimax/minimax-m3", InPerM: 0.3, OutPerM: 1.2, Context: 1000000, Found: true},
		"deepseek/deepseek-v4-pro":               {Slug: "deepseek/deepseek-v4-pro", InPerM: 0.435, OutPerM: 0.87, Context: 1000000, Found: true},
		"x-ai/grok-4.1-fast":                     {Slug: "x-ai/grok-4.1-fast"}, // Found == false: slug пропал из каталога
		"nvidia/nemotron-3-ultra-550b-a55b:free": {Slug: "nvidia/nemotron-3-ultra-550b-a55b:free", Context: 1000000, Free: true, Found: true},
	}
}

func byslug(models []Model) map[string]Model {
	out := map[string]Model{}
	for _, m := range models {
		out[m.Slug] = m
	}
	return out
}

func TestMerge(t *testing.T) {
	scores := []sources.ScoreRow{
		{Slug: "openai/gpt-5.6-luna", Metric: "SWE-bench Verified", Value: 93.0, VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://www.vals.ai/benchmarks/swebench", Checked: "2026-08-03"},
	}
	got := Merge(testEntries(), testPrices(), scores, testNotes(t))

	if len(got) != 4 {
		t.Fatalf("got %d models, want 4 — a slug whose price lookup came back not-found must be dropped: %+v", len(got), got)
	}
	m := byslug(got)
	if _, ok := m["x-ai/grok-4.1-fast"]; ok {
		t.Error("a not-found slug leaked into the merged models")
	}

	luna := m["openai/gpt-5.6-luna"]
	if luna.DisplayName != "GPT-5.6 Luna" || luna.Owner != "OpenAI (C)" || luna.OpenWeights != "нет" {
		t.Errorf("luna prose = %+v, want it pulled from notes.yaml", luna)
	}
	if luna.Score == nil || luna.Score.Value != 93.0 {
		t.Fatalf("luna.Score = %+v, want the fetched row", luna.Score)
	}
	if luna.ScoreLabel != "93.0%" {
		t.Errorf("luna.ScoreLabel = %q, want %q", luna.ScoreLabel, "93.0%")
	}
	if luna.MixedPrice != 1.125 {
		t.Errorf("luna.MixedPrice = %v, want 1.125", luna.MixedPrice)
	}
	if luna.QualityPriceLabel != "82.7" {
		t.Errorf("luna.QualityPriceLabel = %q, want %q (93.0 / 1.125 = 82.67)", luna.QualityPriceLabel, "82.7")
	}
	if !luna.Rankable {
		t.Error("luna.Rankable = false, want true")
	}
	if luna.Tokens10In != 20 || luna.Tokens10Out != 10.0/3 {
		t.Errorf("luna tokens = in %v out %v, want 20 and 10/3", luna.Tokens10In, luna.Tokens10Out)
	}
	if luna.LongContextPriceLabel != "$1.00 / $4.00 от 272K+" {
		t.Errorf("luna.LongContextPriceLabel = %q, want %q", luna.LongContextPriceLabel, "$1.00 / $4.00 от 272K+")
	}
	if luna.LongContextInLabel != "$1.00 от 272K+" {
		t.Errorf("luna.LongContextInLabel = %q, want %q", luna.LongContextInLabel, "$1.00 от 272K+")
	}
	if luna.LongContextOutLabel != "$4.00 от 272K+" {
		t.Errorf("luna.LongContextOutLabel = %q, want %q", luna.LongContextOutLabel, "$4.00 от 272K+")
	}

	m3 := m["minimax/minimax-m3"]
	if m3.Score == nil || m3.Score.Value != 80.5 {
		t.Fatalf("m3.Score = %+v, want the notes.yaml override", m3.Score)
	}
	if m3.ScoreLabel != "80.5% (только вендор)" {
		t.Errorf("m3.ScoreLabel = %q, want the override's own label", m3.ScoreLabel)
	}
	if !m3.Rankable {
		t.Error("m3.Rankable = false, want true — the override declares rankable: true")
	}
	if m3.QualityPriceLabel != "153" {
		t.Errorf("m3.QualityPriceLabel = %q, want %q (80.5 / 0.525 = 153.3, and >= 100 prints as an integer)", m3.QualityPriceLabel, "153")
	}
	if m3.LongContextPriceLabel != "" || m3.LongContextInLabel != "" || m3.LongContextOutLabel != "" {
		t.Errorf("m3 long-context labels = %q / %q / %q, want all empty — the catalogue reported no override for this slug",
			m3.LongContextPriceLabel, m3.LongContextInLabel, m3.LongContextOutLabel)
	}

	pro := m["deepseek/deepseek-v4-pro"]
	if pro.Score != nil {
		t.Errorf("v4-pro.Score = %+v, want nil — no source and no override", pro.Score)
	}
	if pro.ScoreLabel != "н/д" {
		t.Errorf("v4-pro.ScoreLabel = %q, want %q", pro.ScoreLabel, "н/д")
	}
	if pro.QualityPriceLabel != "н/д (оценка не для этого варианта)" {
		t.Errorf("v4-pro.QualityPriceLabel = %q, want the per-model no_score_reason", pro.QualityPriceLabel)
	}
	if pro.Rankable {
		t.Error("v4-pro.Rankable = true, want false")
	}

	free := m["nvidia/nemotron-3-ultra-550b-a55b:free"]
	if !free.Free || free.QualityPriceLabel != "н/д (цена $0)" {
		t.Errorf("free = %+v, want Free with the $0 quality/price label", free)
	}
	if free.ClaudeRef != "≈ Haiku 4.5 (середина диапазона)" {
		t.Errorf("free.ClaudeRef = %q", free.ClaudeRef)
	}
}

func TestMergeTakesTheFirstSourceForASlug(t *testing.T) {
	scores := []sources.ScoreRow{
		{Slug: "openai/gpt-5.6-luna", Metric: "SWE-bench Verified", Value: 79.2, VariantMeasured: "OpenHands", SourceURL: "https://www.swebench.com/"},
		{Slug: "openai/gpt-5.6-luna", Metric: "SWE-bench Verified", Value: 93.0, VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://www.vals.ai/benchmarks/swebench"},
	}
	m := byslug(Merge(testEntries(), testPrices(), scores, testNotes(t)))
	luna := m["openai/gpt-5.6-luna"]
	if luna.Score == nil || luna.Score.Value != 79.2 {
		t.Fatalf("luna.Score = %+v, want 79.2 — the first row in slice order wins, never an average", luna.Score)
	}
}

// rankSet is a hand-computed ranking fixture.
//
//	sonnet: cheap  in 0.30 out 1.20 -> mixed 0.525, score 80.5 -> 153.33  (1st)
//	        mid    in 1.25 out 4.25 -> mixed 2.00,  score 77.4 ->  38.70  (2nd)
//	        dear   in 2.00 out 6.00 -> mixed 3.00,  score 86.6 ->  28.87  (3rd)
//	        noscore in 0.50 out 2.00 -> mixed 0.875, unrankable
//	        dearer-noscore in 5.00 out 5.00 -> mixed 5.00, unrankable
//	free:   two rows, ranked by score alone
func rankSet() []Model {
	mk := func(slug, tier string, in, out, score float64, rankable bool) Model {
		m := Model{Slug: slug, Tier: tier, InPerM: in, OutPerM: out, Rankable: rankable}
		m.MixedPrice = (3*in + out) / 4
		if rankable || score > 0 {
			m.Score = &ScoreInfo{Metric: "SWE-bench Verified", Value: score}
		}
		if rankable && m.MixedPrice > 0 {
			m.QualityPrice = score / m.MixedPrice
		}
		return m
	}
	return []Model{
		mk("s/dear", "sonnet", 2.00, 6.00, 86.6, true),
		mk("s/cheap", "sonnet", 0.30, 1.20, 80.5, true),
		mk("s/dearer-noscore", "sonnet", 5.00, 5.00, 0, false),
		mk("s/mid", "sonnet", 1.25, 4.25, 77.4, true),
		mk("s/noscore", "sonnet", 0.50, 2.00, 0, false),
		mk("o/only", "opus", 0.50, 3.00, 93.0, true),
		mk("f/lower", "free", 0, 0, 67.6, true),
		mk("f/higher", "free", 0, 0, 70.9, true),
	}
}

func slugsOf(models []Model) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.Slug)
	}
	return out
}

func equalSlugs(t *testing.T, what string, got []Model, want []string) {
	t.Helper()
	gs := slugsOf(got)
	if len(gs) != len(want) {
		t.Fatalf("%s = %v, want %v", what, gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, gs, want)
		}
	}
}

func TestRankFavorites(t *testing.T) {
	equalSlugs(t, "RankFavorites(sonnet)", RankFavorites(rankSet(), "sonnet"),
		[]string{"s/cheap", "s/mid", "s/dear"})

	equalSlugs(t, "RankFavorites(opus)", RankFavorites(rankSet(), "opus"),
		[]string{"o/only"})

	// Free models all cost $0, so quality/price is undefined: rank by score.
	equalSlugs(t, "RankFavorites(free)", RankFavorites(rankSet(), "free"),
		[]string{"f/higher", "f/lower"})

	equalSlugs(t, "RankFavorites(haiku)", RankFavorites(rankSet(), "haiku"), nil)
}

func TestRankFavoritesExcludesUnrankableRows(t *testing.T) {
	for _, m := range RankFavorites(rankSet(), "sonnet") {
		if !m.Rankable || m.Score == nil {
			t.Errorf("%s slipped into the ranking: Rankable=%v Score=%v", m.Slug, m.Rankable, m.Score)
		}
	}
}

func TestTierRowsPutsUnrankableRowsLastByPrice(t *testing.T) {
	equalSlugs(t, "TierRows(sonnet)", TierRows(rankSet(), "sonnet"),
		[]string{"s/cheap", "s/mid", "s/dear", "s/noscore", "s/dearer-noscore"})
}

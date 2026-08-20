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
	if luna.DisplayName != "GPT-5.6 Luna" || luna.Owner != "OpenAI (C)" || luna.OpenWeights != "нет" || luna.Copyright != notes.CopyrightCompliant {
		t.Errorf("luna prose = %+v, want it pulled from notes.yaml", luna)
	}
	if byslug(got)["minimax/minimax-m3"].Copyright != notes.CopyrightUnknown {
		t.Errorf("missing copyright = %q, want unknown", byslug(got)["minimax/minimax-m3"].Copyright)
	}
	if len(luna.TaskFit) != 3 || luna.TaskFit[0] != "implement" {
		t.Errorf("luna.TaskFit = %v, want propagated normalized metadata", luna.TaskFit)
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
	// The raw override numbers behind those frozen (unconditionally Russian)
	// labels must also be exposed, so a language-aware renderer can format
	// its own "from"/"от" clause instead of reading the Russian string.
	if !luna.HasLongContextOverride {
		t.Error("luna.HasLongContextOverride = false, want true — the catalogue reported an override for this slug")
	}
	if luna.LongContextOverrideInPerM != 1 || luna.LongContextOverrideOutPerM != 4 || luna.LongContextOverrideMinTokens != 272000 {
		t.Errorf("luna long-context override raw fields = in %v out %v minTokens %v, want 1 / 4 / 272000",
			luna.LongContextOverrideInPerM, luna.LongContextOverrideOutPerM, luna.LongContextOverrideMinTokens)
	}

	m3 := m["minimax/minimax-m3"]
	if m3.Score == nil || m3.Score.Value != 80.5 {
		t.Fatalf("m3.Score = %+v, want the notes.yaml override", m3.Score)
	}
	if m3.ScoreLabel != "80.5% (только вендор)" {
		t.Errorf("m3.ScoreLabel = %q, want the override's own label", m3.ScoreLabel)
	}
	if m3.Rankable {
		t.Error("m3.Rankable = true, want false — a manual observation is not exact-product quality")
	}
	if m3.QualityPriceLabel != "n/a (observation only)" {
		t.Errorf("m3.QualityPriceLabel = %q, want observation-only quality", m3.QualityPriceLabel)
	}
	if m3.LongContextPriceLabel != "" || m3.LongContextInLabel != "" || m3.LongContextOutLabel != "" {
		t.Errorf("m3 long-context labels = %q / %q / %q, want all empty — the catalogue reported no override for this slug",
			m3.LongContextPriceLabel, m3.LongContextInLabel, m3.LongContextOutLabel)
	}
	if m3.HasLongContextOverride {
		t.Error("m3.HasLongContextOverride = true, want false — the catalogue reported no override for this slug")
	}

	pro := m["deepseek/deepseek-v4-pro"]
	if pro.Score != nil {
		t.Errorf("v4-pro.Score = %+v, want nil — no source and no override", pro.Score)
	}
	if pro.ScoreLabel != "n/a" {
		t.Errorf("v4-pro.ScoreLabel = %q, want %q", pro.ScoreLabel, "n/a")
	}
	if pro.QualityPriceLabel != "n/a (variant mismatch)" {
		t.Errorf("v4-pro.QualityPriceLabel = %q, want the per-model no_score_reason", pro.QualityPriceLabel)
	}
	if pro.Rankable {
		t.Error("v4-pro.Rankable = true, want false")
	}

	free := m["nvidia/nemotron-3-ultra-550b-a55b:free"]
	if !free.Free || free.QualityPriceLabel != "n/a (free)" {
		t.Errorf("free = %+v, want Free with the $0 quality/price label", free)
	}
	if free.ClaudeRef != "<≈ Haiku 4.5 (середина диапазона)" {
		t.Errorf("free.ClaudeRef = %q", free.ClaudeRef)
	}
}

func TestProviderLabelUsesCatalogueValueOrNamespaceAlias(t *testing.T) {
	tests := []struct {
		slug, provider, want string
	}{
		{"qwen/qwen3.7-flash", "", "Qwen"},
		{"nvidia/nemotron-3-nano-30b-a3b", "", "NVIDIA"},
		{"poolside/laguna-xs-2.1", "", "Poolside"},
		{"google/gemma-4-31b-it", "", "Google"},
		{"openai/gpt-5-mini", "", "OpenAI"},
		{"openai/gpt-5-mini", "n/a", "OpenAI"},
		{"openai/gpt-5-mini", "n/a (catalog missing)", "OpenAI"},
		{"openai/gpt-5-mini", "н/д (каталог)", "OpenAI"},
		{"openai/gpt-5-mini", "_нужен обзор_ (каталог)", "OpenAI"},
		{"openai/gpt-5-mini", "n/a(каталог)", "OpenAI"},
		{"openai/gpt-5-mini", "n/d(каталог)", "OpenAI"},
		{"x-ai/grok-4.5", "xAI", "xAI"},
		{"mistralai/codestral", "", "Mistral AI"},
		{"demo/model", "Demo Vendor", "Demo Vendor"},
	}
	for _, test := range tests {
		if got := ProviderLabel(test.slug, test.provider); got != test.want {
			t.Errorf("ProviderLabel(%q, %q) = %q, want %q", test.slug, test.provider, got, test.want)
		}
	}
}

func TestIsPlaceholderRecognizesSupportedForms(t *testing.T) {
	for _, value := range []string{
		"_нужен обзор_", " _НУЖЕН ОБЗОР_ ", "_нужен обзор_ (нет данных)", "_нужен обзор_(нет данных)",
		"n/a", "N/A (catalog missing)", "n/a(catalog missing)", "n/d", "N/D (нет данных)",
		"н/д", "н/д (нет оценки)", "н/д(нет оценки)", "(n/a)", "(н/д (нет оценки))",
	} {
		if !IsPlaceholder(value) {
			t.Errorf("IsPlaceholder(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"OpenAI", "OpenAI (n/a)", "Catalog Provider"} {
		if IsPlaceholder(value) {
			t.Errorf("IsPlaceholder(%q) = true, want false", value)
		}
	}
}

func TestMergeWithArenaKeepsValidCatalogProvider(t *testing.T) {
	const slug = "catalog/model"
	entries := []modelmap.Entry{{Slug: slug, Tier: "sonnet"}}
	prices := map[string]sources.PriceInfo{slug: {Slug: slug, Provider: "Catalog Provider", Found: true}}
	arena := []sources.ScoreRow{{Slug: slug, Metric: sources.MetricArenaElo, Value: 1453, ConfiguredIdentity: slug, CanonicalID: slug, Provider: "Conflicting Arena Provider"}}

	models := MergeWithArena(entries, prices, nil, arena, testNotes(t))
	if len(models) != 1 {
		t.Fatalf("merged models = %d, want 1", len(models))
	}
	if models[0].Provider != "Catalog Provider" {
		t.Fatalf("provider = %q, want valid catalog provider", models[0].Provider)
	}
}

func TestBenchmarkIdentityGate(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "deepseek/deepseek-v4-flash", Tier: "haiku"},
		{Slug: "openai/gpt-5.6-luna", Tier: "opus"},
	}
	prices := map[string]sources.PriceInfo{
		"deepseek/deepseek-v4-flash": {Slug: "deepseek/deepseek-v4-flash", InPerM: 0.1, OutPerM: 0.4, Found: true},
		"openai/gpt-5.6-luna":        {Slug: "openai/gpt-5.6-luna", InPerM: 0.5, OutPerM: 3, Found: true},
	}
	scores := []sources.ScoreRow{
		{Slug: "deepseek/deepseek-v4-flash", Metric: sources.MetricSWEBenchVerified, Value: 88.8, Unit: "%", VariantMeasured: "deepseek-v4-flash-0731", SourceURL: "https://vals.ai"},
		{Slug: "openai/gpt-5.6-luna", Metric: sources.MetricSWEBenchVerified, Value: 93, Unit: "%", VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://vals.ai"},
	}
	models := byslug(Merge(entries, prices, scores, testNotes(t)))
	deepseek, luna := models["deepseek/deepseek-v4-flash"], models["openai/gpt-5.6-luna"]
	if deepseek.Score.IdentityStatus != IdentityVariantMismatch || deepseek.Rankable || deepseek.QualityPriceLabel != "n/a (variant mismatch)" {
		t.Fatalf("DeepSeek gate = %+v, want visible mismatch and non-rankable quality", deepseek)
	}
	if luna.Score.IdentityStatus != IdentityExact || !luna.Rankable || luna.QualityPrice == 0 {
		t.Fatalf("Luna gate = %+v, want exact rankable observation", luna)
	}
}

// TestMappedIdentityTrustsModelMapOverTextualEquality is the regression test
// for the identity-gate bug: a human-curated model-map.tsv mapping IS the
// identity assertion, and must be trusted even when the mapped key differs
// textually from the OpenRouter slug (namespace/spelling differences between
// the two sites are the normal case, not the exception).
func TestMappedIdentityTrustsModelMapOverTextualEquality(t *testing.T) {
	// moonshotai/kimi-k3 is a real production row: vals.ai lists the same
	// product under "kimi/kimi-k3", a different namespace than the
	// OpenRouter slug.
	entries := []modelmap.Entry{{Slug: "moonshotai/kimi-k3", Tier: "sonnet", Names: map[string]string{"vals": "kimi/kimi-k3"}}}
	prices := map[string]sources.PriceInfo{"moonshotai/kimi-k3": {Slug: "moonshotai/kimi-k3", InPerM: 1, OutPerM: 3, Found: true}}
	scores := []sources.ScoreRow{{
		Slug: "moonshotai/kimi-k3", SourceFamily: "vals", ConfiguredIdentity: "kimi/kimi-k3",
		Metric: sources.MetricSWEBenchVerified, Value: 71.4, Unit: "%", VariantMeasured: "kimi/kimi-k3",
		SourceURL: "https://www.vals.ai/benchmarks/swebench",
	}}
	kimi := Merge(entries, prices, scores, testNotes(t))[0]
	if kimi.Score == nil || kimi.Score.IdentityStatus != IdentityExact || !kimi.Rankable {
		t.Fatalf("kimi identity = %+v rankable=%v, want exact_product/rankable — model-map.tsv already maps kimi/kimi-k3 to this slug", kimi.Score, kimi.Rankable)
	}
}

// TestMappedIdentityCanBeFlaggedAsAGenuineVariant covers the human override:
// a mapped key that matched exactly can still be excluded when model-map.tsv
// marks it with "!variant" — the mapped vals.ai entry measured a real
// different checkpoint, not just a different spelling of the same product.
func TestMappedIdentityCanBeFlaggedAsAGenuineVariant(t *testing.T) {
	entries := []modelmap.Entry{{
		Slug:     "moonshotai/kimi-k3",
		Tier:     "sonnet",
		Names:    map[string]string{"vals": "kimi/kimi-k3"},
		Variants: map[string]bool{"vals": true},
	}}
	prices := map[string]sources.PriceInfo{"moonshotai/kimi-k3": {Slug: "moonshotai/kimi-k3", InPerM: 1, OutPerM: 3, Found: true}}
	scores := []sources.ScoreRow{{
		Slug: "moonshotai/kimi-k3", SourceFamily: "vals", ConfiguredIdentity: "kimi/kimi-k3",
		Metric: sources.MetricSWEBenchVerified, Value: 71.4, Unit: "%", VariantMeasured: "kimi/kimi-k3",
		SourceURL: "https://www.vals.ai/benchmarks/swebench",
	}}
	kimi := Merge(entries, prices, scores, testNotes(t))[0]
	if kimi.Score == nil || kimi.Score.IdentityStatus != IdentityVariantMismatch || kimi.Rankable {
		t.Fatalf("kimi identity = %+v rankable=%v, want variant_mismatch/non-rankable — the !variant marker must override the mapped trust", kimi.Score, kimi.Rankable)
	}
}

// TestUnmappedTextuallyDifferentKeyStillFails pins down the fallback path:
// with no model-map.tsv entry for this source at all, there is no mapping to
// trust, so a row whose key differs from the OpenRouter identifiers still
// fails the way it always has.
func TestUnmappedTextuallyDifferentKeyStillFails(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/model", Tier: "sonnet"}}
	prices := map[string]sources.PriceInfo{"a/model": {Slug: "a/model", InPerM: 1, OutPerM: 3, Found: true}}
	scores := []sources.ScoreRow{{Slug: "a/model", Metric: sources.MetricSWEBenchVerified, Value: 80, Unit: "%", VariantMeasured: "some/other-key"}}
	got := Merge(entries, prices, scores, testNotes(t))[0]
	if got.Score == nil || got.Score.IdentityStatus != IdentityVariantMismatch || got.Rankable {
		t.Fatalf("unmapped identity = %+v rankable=%v, want variant_mismatch/non-rankable", got.Score, got.Rankable)
	}
}

// TestSWEBenchMappedRowIsRankableDespiteScaffoldVariantName is the swebench.com
// half of the mapped-identity trust. A swebench.com row's VariantMeasured names
// the leaderboard submission (a scaffold, or the median wording), which can
// never equal the configured "Model: " tag — so a rule that demands that
// equality rejects every correctly mapped row from that source. Six production
// entries (deepseek/deepseek-v3.2, qwen/qwen3-coder, moonshotai/kimi-k2.5, the
// two llama-4 rows, openai/gpt-5-mini) have a swebench= mapping and no vals=
// fallback, so this is the difference between them ranking and not existing.
func TestSWEBenchMappedRowIsRankableDespiteScaffoldVariantName(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "deepseek/deepseek-v3.2", Tier: "haiku", Names: map[string]string{"swebench": "Model: deepseek-v3.2"}}}
	prices := map[string]sources.PriceInfo{"deepseek/deepseek-v3.2": {Slug: "deepseek/deepseek-v3.2", InPerM: 0.3, OutPerM: 1.2, Found: true}}
	scores := []sources.ScoreRow{{
		Slug: "deepseek/deepseek-v3.2", SourceFamily: "swebench", ConfiguredIdentity: "Model: deepseek-v3.2",
		Metric: sources.MetricSWEBenchVerified, Value: 62.4, Unit: "%",
		VariantMeasured: "OpenHands + DeepSeek V3.2", SourceURL: "https://www.swebench.com/",
		Scaffold: "single scaffold (OpenHands + DeepSeek V3.2)",
	}}
	got := Merge(entries, prices, scores, testNotes(t))[0]
	if got.Score == nil || got.Score.IdentityStatus != IdentityExact || !got.Rankable {
		t.Fatalf("swebench identity = %+v rankable=%v, want exact_product/rankable — the swebench= mapping is the identity assertion, and its VariantMeasured is a scaffold name by construction", got.Score, got.Rankable)
	}
	if got.QualityPrice == 0 {
		t.Errorf("QualityPrice = 0, want a ranked quality/price — a rankable row must reach the ranking, not just the label")
	}
}

// TestSWEBenchMappedRowStillHonoursTheVariantMarker keeps the swebench.com
// relaxation from swallowing the one override that must survive it.
func TestSWEBenchMappedRowStillHonoursTheVariantMarker(t *testing.T) {
	entries := []modelmap.Entry{{
		Slug: "deepseek/deepseek-v3.2", Tier: "haiku",
		Names:    map[string]string{"swebench": "Model: deepseek-v3.2"},
		Variants: map[string]bool{"swebench": true},
	}}
	prices := map[string]sources.PriceInfo{"deepseek/deepseek-v3.2": {Slug: "deepseek/deepseek-v3.2", InPerM: 0.3, OutPerM: 1.2, Found: true}}
	scores := []sources.ScoreRow{{
		Slug: "deepseek/deepseek-v3.2", SourceFamily: "swebench", ConfiguredIdentity: "Model: deepseek-v3.2",
		Metric: sources.MetricSWEBenchVerified, Value: 62.4, Unit: "%",
		VariantMeasured: "OpenHands + DeepSeek V3.2", SourceURL: "https://www.swebench.com/",
	}}
	got := Merge(entries, prices, scores, testNotes(t))[0]
	if got.Score == nil || got.Score.IdentityStatus != IdentityVariantMismatch || got.Rankable {
		t.Fatalf("flagged swebench identity = %+v rankable=%v, want variant_mismatch/non-rankable", got.Score, got.Rankable)
	}
}

// TestValsRowStillRequiresItsOwnKeyEcho pins the other side of the per-source
// split: vals.ai does echo the key it matched into VariantMeasured, so an
// inequality there is not a naming difference but a forged or stale row, and
// it must stay a mismatch even though the mapping exists.
func TestValsRowStillRequiresItsOwnKeyEcho(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "moonshotai/kimi-k3", Tier: "sonnet", Names: map[string]string{"vals": "kimi/kimi-k3"}}}
	prices := map[string]sources.PriceInfo{"moonshotai/kimi-k3": {Slug: "moonshotai/kimi-k3", InPerM: 1, OutPerM: 3, Found: true}}
	scores := []sources.ScoreRow{{
		Slug: "moonshotai/kimi-k3", SourceFamily: "vals", ConfiguredIdentity: "kimi/kimi-k3",
		Metric: sources.MetricSWEBenchVerified, Value: 71.4, Unit: "%", VariantMeasured: "kimi/kimi-k3-0219-preview",
		SourceURL: "https://www.vals.ai/benchmarks/swebench",
	}}
	got := Merge(entries, prices, scores, testNotes(t))[0]
	if got.Score == nil || got.Score.IdentityStatus != IdentityVariantMismatch || got.Rankable {
		t.Fatalf("vals identity = %+v rankable=%v, want variant_mismatch/non-rankable — a vals.ai row that measured a key other than the configured one is not the configured product", got.Score, got.Rankable)
	}
}

// TestFlaggedTopSourceFallsThroughToTheNextSource is the row-selection fix:
// model-map.tsv describes swebench.com as vals.ai's fallback, so a vals.ai row
// the human flagged "!variant" must not occupy the slot and leave the model
// unranked — the next source's usable row takes it.
func TestFlaggedTopSourceFallsThroughToTheNextSource(t *testing.T) {
	entries := []modelmap.Entry{{
		Slug: "moonshotai/kimi-k3", Tier: "sonnet",
		Names:    map[string]string{"vals": "kimi/kimi-k3-0219-preview", "swebench": "Model: kimi-k3"},
		Variants: map[string]bool{"vals": true},
	}}
	prices := map[string]sources.PriceInfo{"moonshotai/kimi-k3": {Slug: "moonshotai/kimi-k3", InPerM: 1, OutPerM: 3, Found: true}}
	// Concatenated in run()'s real priority order: vals.ai first.
	scores := []sources.ScoreRow{
		{
			Slug: "moonshotai/kimi-k3", SourceFamily: "vals", ConfiguredIdentity: "kimi/kimi-k3-0219-preview",
			Metric: sources.MetricSWEBenchVerified, Value: 71.4, Unit: "%", VariantMeasured: "kimi/kimi-k3-0219-preview",
			SourceURL: "https://www.vals.ai/benchmarks/swebench",
		},
		{
			Slug: "moonshotai/kimi-k3", SourceFamily: "swebench", ConfiguredIdentity: "Model: kimi-k3",
			Metric: sources.MetricSWEBenchVerified, Value: 68.2, Unit: "%", VariantMeasured: "OpenHands + Kimi K3",
			SourceURL: "https://www.swebench.com/",
		},
	}
	got := Merge(entries, prices, scores, testNotes(t))[0]
	if got.Score == nil || got.Score.IdentityStatus != IdentityExact || !got.Rankable {
		t.Fatalf("selected identity = %+v rankable=%v, want the swebench.com row, exact_product and rankable", got.Score, got.Rankable)
	}
	if got.Score.Value != 68.2 || got.Score.SourceURL != "https://www.swebench.com/" {
		t.Fatalf("selected row = %v from %q, want 68.2 from swebench.com — the flagged vals.ai row must not hold the slot", got.Score.Value, got.Score.SourceURL)
	}
}

// TestUnusableRowIsKeptWhenNoSourcePasses guards the other half of that
// selection rule: falling through is for finding a usable row, not for hiding
// the reason a model does not rank. With every source failing, the
// highest-priority row still shows, mismatch label and all.
func TestUnusableRowIsKeptWhenNoSourcePasses(t *testing.T) {
	entries := []modelmap.Entry{{
		Slug: "moonshotai/kimi-k3", Tier: "sonnet",
		Names:    map[string]string{"vals": "kimi/kimi-k3", "swebench": "Model: kimi-k3"},
		Variants: map[string]bool{"vals": true, "swebench": true},
	}}
	prices := map[string]sources.PriceInfo{"moonshotai/kimi-k3": {Slug: "moonshotai/kimi-k3", InPerM: 1, OutPerM: 3, Found: true}}
	scores := []sources.ScoreRow{
		{
			Slug: "moonshotai/kimi-k3", SourceFamily: "vals", ConfiguredIdentity: "kimi/kimi-k3",
			Metric: sources.MetricSWEBenchVerified, Value: 71.4, Unit: "%", VariantMeasured: "kimi/kimi-k3",
			SourceURL: "https://www.vals.ai/benchmarks/swebench",
		},
		{
			Slug: "moonshotai/kimi-k3", SourceFamily: "swebench", ConfiguredIdentity: "Model: kimi-k3",
			Metric: sources.MetricSWEBenchVerified, Value: 68.2, Unit: "%", VariantMeasured: "OpenHands + Kimi K3",
			SourceURL: "https://www.swebench.com/",
		},
	}
	got := Merge(entries, prices, scores, testNotes(t))[0]
	if got.Rankable || got.Score == nil || got.Score.Value != 71.4 {
		t.Fatalf("selected row = %+v rankable=%v, want the first (vals.ai) row kept and non-rankable", got.Score, got.Rankable)
	}
	if got.QualityPriceLabel != "n/a (variant mismatch)" {
		t.Errorf("QualityPriceLabel = %q, want %q — the reason must stay visible", got.QualityPriceLabel, "n/a (variant mismatch)")
	}
}

// TestProductionDeepSeekV4FlashRowStaysExcluded is the regression test for the
// verification-pass finding: the identity-gate fix built the "!variant" escape
// hatch but the one production row that actually needs it,
// deepseek/deepseek-v4-flash, was left unmarked — silently promoting vals.ai's
// 0731-checkpoint measurement into the ranking as the catalogue's
// 20260423 product. It loads the real model-map.tsv (not a hand-built
// fixture), confirms the row carries the marker, and then proves the marker
// still bites even against a vals.ai row that echoes the mapped key exactly —
// the one shape that would otherwise pass every other check and classify
// exact_product.
func TestProductionDeepSeekV4FlashRowStaysExcluded(t *testing.T) {
	entries, err := modelmap.Load(filepath.Join("..", "..", "model-map.tsv"))
	if err != nil {
		t.Fatalf("production model-map.tsv: %v", err)
	}
	var entry modelmap.Entry
	found := false
	for _, e := range entries {
		if e.Slug == "deepseek/deepseek-v4-flash" {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatal("production model-map.tsv has no deepseek/deepseek-v4-flash row")
	}
	mappedKey, hasVals := entry.Names["vals"]
	if !hasVals {
		t.Fatal("deepseek/deepseek-v4-flash has no vals= mapping in production model-map.tsv")
	}
	if !entry.Variants["vals"] {
		t.Fatalf("deepseek/deepseek-v4-flash vals mapping %q is not flagged !variant in production model-map.tsv — the 0731-vs-20260423 checkpoint mismatch would silently rank", mappedKey)
	}

	prices := map[string]sources.PriceInfo{
		"deepseek/deepseek-v4-flash": {
			Slug: "deepseek/deepseek-v4-flash", InPerM: 0.06, OutPerM: 0.12, Found: true,
			CanonicalSlug: "deepseek/deepseek-v4-flash-20260423",
		},
	}
	// Mirrors exactly what FetchValsSWEBench emits for a matched row: the
	// looked-up key echoed back as both ConfiguredIdentity and
	// VariantMeasured. Absent the !variant marker this shape passes every
	// other check and classifies exact_product — the marker is the only
	// thing standing between this row and a wrongly-ranked score.
	scores := []sources.ScoreRow{{
		Slug: "deepseek/deepseek-v4-flash", SourceFamily: "vals", ConfiguredIdentity: mappedKey,
		Metric: sources.MetricSWEBenchVerified, Value: 88.8, Unit: "%", VariantMeasured: mappedKey,
		SourceURL: "https://www.vals.ai/benchmarks/swebench",
	}}
	got := Merge([]modelmap.Entry{entry}, prices, scores, testNotes(t))[0]
	if got.Score == nil || got.Score.IdentityStatus != IdentityVariantMismatch || got.Rankable {
		t.Fatalf("deepseek/deepseek-v4-flash identity = %+v rankable=%v, want variant_mismatch/non-rankable with the production model-map.tsv row", got.Score, got.Rankable)
	}
}

func TestLegacyScoreIsNotPromotedByMissingProvenance(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "openai/gpt-5.6-luna", Tier: "opus"}}
	prices := map[string]sources.PriceInfo{"openai/gpt-5.6-luna": {Slug: "openai/gpt-5.6-luna", InPerM: 1, OutPerM: 1, Found: true}}
	scores := []sources.ScoreRow{{Slug: "openai/gpt-5.6-luna", Metric: sources.MetricSWEBenchVerified, Value: 93, VariantMeasured: "openai/gpt-5.6-luna", IdentityStatus: IdentityLegacyUnknown}}
	luna := Merge(entries, prices, scores, testNotes(t))[0]
	if luna.Rankable || luna.QualityPriceLabel != "n/a (legacy)" {
		t.Fatalf("legacy score = %+v, want unknown/non-rankable", luna)
	}
}

func TestEmptyIdentityIsExplicitlyNonRankableForBothSources(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/model", Tier: "sonnet"}}
	prices := map[string]sources.PriceInfo{"a/model": {Slug: "a/model", InPerM: 1, OutPerM: 1, Found: true}}
	models := MergeWithArena(entries, prices, []sources.ScoreRow{{Slug: "a/model", Metric: sources.MetricSWEBenchVerified, Value: 80}}, []sources.ScoreRow{{Slug: "a/model", Metric: sources.MetricArenaElo, Value: 1400}}, testNotes(t))
	if models[0].Score.IdentityStatus != IdentityMissing || models[0].Rankable {
		t.Fatalf("SWE empty identity = %+v, want missing_identity/non-rankable", models[0].Score)
	}
	if models[0].ArenaScore.IdentityStatus != IdentityMissing || models[0].ArenaRankable {
		t.Fatalf("Arena empty identity = %+v, want missing_identity/non-rankable", models[0].ArenaScore)
	}
}

func TestIdentityGateRecomputesForgedExactAndConflicts(t *testing.T) {
	tests := []struct {
		name  string
		price sources.PriceInfo
		row   sources.ScoreRow
	}{
		{name: "missing fields", price: sources.PriceInfo{Slug: "a/model", Found: true}, row: sources.ScoreRow{Slug: "a/model", Value: 80, IdentityStatus: IdentityExact}},
		{name: "canonical conflict", price: sources.PriceInfo{Slug: "a/model", CanonicalSlug: "a/model", Found: true}, row: sources.ScoreRow{Slug: "a/model", Value: 80, IdentityStatus: IdentityExact, CanonicalID: "a/other", VariantMeasured: "a/model"}},
		{name: "variant conflict", price: sources.PriceInfo{Slug: "a/model", Found: true}, row: sources.ScoreRow{Slug: "a/model", Value: 80, IdentityStatus: IdentityExact, VariantMeasured: "a/other"}},
		{name: "provider conflict", price: sources.PriceInfo{Slug: "a/model", Provider: "A", Found: true}, row: sources.ScoreRow{Slug: "a/model", Value: 80, IdentityStatus: IdentityExact, VariantMeasured: "a/model", Provider: "B"}},
		{name: "reasoning conflict", price: sources.PriceInfo{Slug: "a/model", Reasoning: "high", Found: true}, row: sources.ScoreRow{Slug: "a/model", Value: 80, IdentityStatus: IdentityExact, VariantMeasured: "a/model", Reasoning: "low"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Merge([]modelmap.Entry{{Slug: "a/model", Tier: "sonnet"}}, map[string]sources.PriceInfo{"a/model": tt.price}, []sources.ScoreRow{tt.row}, testNotes(t))[0]
			if got.Score.IdentityStatus == IdentityExact || got.Rankable {
				t.Fatalf("forged exact survived: score=%+v rankable=%v", got.Score, got.Rankable)
			}
		})
	}
	arena := MergeWithArena(
		[]modelmap.Entry{{Slug: "a/model", Tier: "sonnet", Names: map[string]string{"arena": "a-model-arena"}}},
		map[string]sources.PriceInfo{"a/model": {Slug: "a/model", InPerM: 1, OutPerM: 1, Found: true}},
		nil,
		[]sources.ScoreRow{{Slug: "a/model", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "a-model-arena", CanonicalID: "forged-other-key", IdentityStatus: IdentityExact, Metric: sources.MetricArenaElo, Value: 1400}},
		testNotes(t),
	)[0]
	if arena.ArenaScore.IdentityStatus == IdentityExact || arena.ArenaRankable {
		t.Fatalf("forged Arena exact survived: score=%+v rankable=%v", arena.ArenaScore, arena.ArenaRankable)
	}
}

func TestArenaNormalizationIgnoresInvalidScores(t *testing.T) {
	models := []Model{
		{Slug: "a/valid-low", ArenaScore: &ScoreInfo{Value: 100}, ArenaRankable: true},
		{Slug: "a/valid-high", ArenaScore: &ScoreInfo{Value: 200}, ArenaRankable: true},
		{Slug: "a/invalid", ArenaScore: &ScoreInfo{Value: 10000}, ArenaRankable: false},
	}
	fillArenaDerived(models)
	if models[0].ArenaNormalized != 0 || models[1].ArenaNormalized != 100 {
		t.Fatalf("invalid Arena score contaminated normalization: low=%v high=%v", models[0].ArenaNormalized, models[1].ArenaNormalized)
	}
}

func TestArenaIdentityUsesConfiguredArenaNamespace(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "openai/gpt-5.6-luna", Tier: "opus", Names: map[string]string{"arena": "gpt-5.6-luna-xhigh-text"}},
		{Slug: "openai/gpt-5.6-sol", Tier: "opus", Names: map[string]string{"arena": "gpt-5.6-sol-xhigh-text"}},
		{Slug: "deepseek/deepseek-v4-pro", Tier: "sonnet", Names: map[string]string{"arena": "deepseek-v4-pro-ch1-text"}},
	}
	prices := map[string]sources.PriceInfo{
		"openai/gpt-5.6-luna":      {Slug: "openai/gpt-5.6-luna", InPerM: 1, OutPerM: 3, Found: true},
		"openai/gpt-5.6-sol":       {Slug: "openai/gpt-5.6-sol", InPerM: 1, OutPerM: 3, Found: true},
		"deepseek/deepseek-v4-pro": {Slug: "deepseek/deepseek-v4-pro", InPerM: 1, OutPerM: 3, Found: true},
	}
	arena := []sources.ScoreRow{
		{Slug: "openai/gpt-5.6-luna", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "gpt-5.6-luna-xhigh-text", CanonicalID: "gpt-5.6-luna-xhigh-text", Metric: sources.MetricArenaElo, Value: 1500, VariantMeasured: "GPT-5.6 Luna", Unit: "Elo"},
		{Slug: "openai/gpt-5.6-sol", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "gpt-5.6-sol-xhigh-text", CanonicalID: "gpt-5.6-sol-xhigh-text", Metric: sources.MetricArenaElo, Value: 1400, VariantMeasured: "GPT-5.6 Sol", Unit: "Elo"},
		{Slug: "deepseek/deepseek-v4-pro", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "deepseek-v4-pro-ch1-text", CanonicalID: "unknown-arena-key", Metric: sources.MetricArenaElo, Value: 9999, VariantMeasured: "DeepSeek V4 Pro", Unit: "Elo"},
	}
	models := byslug(MergeWithArena(entries, prices, nil, arena, testNotes(t)))
	luna, sol, deepseek := models["openai/gpt-5.6-luna"], models["openai/gpt-5.6-sol"], models["deepseek/deepseek-v4-pro"]
	if !luna.ArenaRankable || luna.ArenaNormalized != 100 {
		t.Fatalf("configured Arena key was rejected: %+v", luna)
	}
	if !sol.ArenaRankable || sol.ArenaNormalized != 0 {
		t.Fatalf("configured Arena key was not ranked: %+v", sol)
	}
	if deepseek.ArenaRankable || deepseek.ArenaNormalized != 0 || deepseek.ArenaScore.IdentityStatus != IdentityVariantMismatch {
		t.Fatalf("unknown Arena key contaminated normalization: %+v", deepseek)
	}
	ambiguousEntries := []modelmap.Entry{
		{Slug: "a/base", Tier: "sonnet", Names: map[string]string{"arena": "shared-arena-key"}},
		{Slug: "a/base:free", Tier: "free", Names: map[string]string{"arena": "shared-arena-key"}},
	}
	ambiguousPrices := map[string]sources.PriceInfo{
		"a/base":      {Slug: "a/base", InPerM: 1, OutPerM: 1, Found: true},
		"a/base:free": {Slug: "a/base:free", Found: true, Free: true},
	}
	ambiguousRows := []sources.ScoreRow{
		{Slug: "a/base", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "shared-arena-key", IdentityAmbiguous: true, CanonicalID: "shared-arena-key", Metric: sources.MetricArenaElo, Value: 1500},
		{Slug: "a/base:free", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "shared-arena-key", IdentityAmbiguous: true, CanonicalID: "shared-arena-key", Metric: sources.MetricArenaElo, Value: 1500},
	}
	for slug, m := range byslug(MergeWithArena(ambiguousEntries, ambiguousPrices, nil, ambiguousRows, testNotes(t))) {
		if m.ArenaRankable || m.ArenaScore.IdentityStatus != IdentityMissing {
			t.Fatalf("ambiguous Arena mapping %q was promoted: %+v", slug, m)
		}
	}
}

func TestArenaExactMappingsProducePaidQualityPrice(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "deepseek/deepseek-v4-pro", Tier: "sonnet", Names: map[string]string{"arena": "deepseek-v4-pro-ch1-text"}},
		{Slug: "google/gemini-3.1-pro-preview", Tier: "sonnet", Names: map[string]string{"arena": "gemini-3.1-pro-preview"}},
		{Slug: "google/gemini-3.6-flash", Tier: "sonnet", Names: map[string]string{"arena": "gemini-3.6-flash"}},
	}
	prices := map[string]sources.PriceInfo{}
	arena := make([]sources.ScoreRow, 0, len(entries))
	for i, entry := range entries {
		prices[entry.Slug] = sources.PriceInfo{Slug: entry.Slug, InPerM: 1, OutPerM: 3, Found: true}
		arena = append(arena, sources.ScoreRow{Slug: entry.Slug, SourceFamily: ScoreSourceArena, ConfiguredIdentity: entry.Names["arena"], CanonicalID: entry.Names["arena"], Metric: sources.MetricArenaElo, Value: float64(1400 + i*50), Unit: "Elo"})
	}
	models := ForScoreSource(MergeWithArena(entries, prices, nil, arena, testNotes(t)), ScoreSourceArena)
	for _, got := range models {
		if !got.ArenaRankable || !got.HasQualityPrice || got.QualityPriceLabel == "n/a (no LMArena score)" {
			t.Fatalf("exact Arena mapping lost rankable Q/P for %s: %+v", got.Slug, got)
		}
	}
}

func TestMergePreservesArenaMetadataWithoutOverridingManualNotes(t *testing.T) {
	prices := testPrices()
	rows := []sources.ScoreRow{{Slug: "openai/gpt-5.6-luna", Metric: "LMArena Elo", Value: 1453, Provider: "OpenAI", License: "proprietary", ModelURL: "https://arena.ai/models/luna", MetadataSourceURL: "https://arena.ai/leaderboard/text"}}
	got := MergeWithArena(testEntries(), prices, nil, rows, testNotes(t))
	luna := byslug(got)["openai/gpt-5.6-luna"]
	if luna.Provider != "OpenAI" || luna.License != "proprietary" || luna.ModelURL != "https://arena.ai/models/luna" || luna.MetadataSourceURL != "https://arena.ai/leaderboard/text" {
		t.Fatalf("Arena metadata = %+v, want sourced provider/license/link", luna)
	}
	if luna.Owner != "OpenAI (C)" || luna.OpenWeights != "нет" || luna.ClaudeRef == "" {
		t.Fatalf("manual metadata was overridden: %+v", luna)
	}
}

// TestMergeCarriesCatalogueCreatedAndDescription checks the second hop of
// the catalogue-metadata pipeline. It goes through Merge (not
// MergeWithArena) on purpose: Merge is a thin wrapper, and this pins down
// that it keeps getting new catalogue fields for free.
func TestMergeCarriesCatalogueCreatedAndDescription(t *testing.T) {
	prices := testPrices()
	luna := prices["openai/gpt-5.6-luna"]
	luna.Created, luna.Description = 1786034890, "GPT-5.6 Luna is OpenAI's long-context flagship."
	prices["openai/gpt-5.6-luna"] = luna

	got := byslug(Merge(testEntries(), prices, nil, testNotes(t)))

	if got["openai/gpt-5.6-luna"].Created != 1786034890 {
		t.Errorf("luna.Created = %d, want the catalogue timestamp carried through the merge", got["openai/gpt-5.6-luna"].Created)
	}
	if got["openai/gpt-5.6-luna"].Description != "GPT-5.6 Luna is OpenAI's long-context flagship." {
		t.Errorf("luna.Description = %q, want the catalogue prose carried through the merge", got["openai/gpt-5.6-luna"].Description)
	}
	if m3 := got["minimax/minimax-m3"]; m3.Created != 0 || m3.Description != "" {
		t.Errorf("m3 = %+v, want zero catalogue metadata for an entry whose price info carries none", m3)
	}
}

// TestMergeCarriesTheCatalogueLinkIdentifiers checks the second hop of
// the link-identifier pipeline. It goes through Merge (not
// MergeWithArena) on purpose: Merge is a thin wrapper, and this pins down
// that it keeps getting new catalogue fields for free. The negative case
// matters as much as the positive one — an entry whose price info carries
// no identifiers must end up with empty strings rather than with anything
// reconstructed from its slug.
func TestMergeCarriesTheCatalogueLinkIdentifiers(t *testing.T) {
	prices := testPrices()
	luna := prices["openai/gpt-5.6-luna"]
	luna.CanonicalSlug, luna.HuggingFaceID = "openai/gpt-5.6-luna-20260804", "openai-community/gpt-5-6-luna"
	prices["openai/gpt-5.6-luna"] = luna

	got := byslug(Merge(testEntries(), prices, nil, testNotes(t)))

	if got["openai/gpt-5.6-luna"].CanonicalSlug != "openai/gpt-5.6-luna-20260804" {
		t.Errorf("luna.CanonicalSlug = %q, want the catalogue identifier carried through the merge", got["openai/gpt-5.6-luna"].CanonicalSlug)
	}
	if got["openai/gpt-5.6-luna"].HuggingFaceID != "openai-community/gpt-5-6-luna" {
		t.Errorf("luna.HuggingFaceID = %q, want the catalogue identifier carried through the merge", got["openai/gpt-5.6-luna"].HuggingFaceID)
	}
	if m3 := got["minimax/minimax-m3"]; m3.CanonicalSlug != "" || m3.HuggingFaceID != "" {
		t.Errorf("m3 = %+v, want empty link identifiers for an entry whose price info carries none — never a value derived from the slug", m3)
	}
}

// TestMergeTakesTheFirstSourceForASlug pins the priority rule when both
// candidate rows are usable: the first in slice order wins outright and the
// two numbers are never averaged. (Fall-through to a later source happens only
// when the earlier row fails identity classification — see
// TestFlaggedTopSourceFallsThroughToTheNextSource.) Both rows here name the
// OpenRouter slug itself, so identity is not what decides the outcome.
func TestMergeTakesTheFirstSourceForASlug(t *testing.T) {
	scores := []sources.ScoreRow{
		{Slug: "openai/gpt-5.6-luna", SourceFamily: "swebench", ConfiguredIdentity: "Model: gpt-5.6-luna", Metric: "SWE-bench Verified", Value: 79.2, VariantMeasured: "OpenHands + GPT-5.6 Luna", SourceURL: "https://www.swebench.com/"},
		{Slug: "openai/gpt-5.6-luna", SourceFamily: "vals", ConfiguredIdentity: "openai/gpt-5.6-luna", Metric: "SWE-bench Verified", Value: 93.0, VariantMeasured: "openai/gpt-5.6-luna", SourceURL: "https://www.vals.ai/benchmarks/swebench"},
	}
	m := byslug(Merge(testEntries(), testPrices(), scores, testNotes(t)))
	luna := m["openai/gpt-5.6-luna"]
	if luna.Score == nil || luna.Score.Value != 79.2 {
		t.Fatalf("luna.Score = %+v, want 79.2 — the first row in slice order wins, never an average", luna.Score)
	}
	if !luna.Rankable {
		t.Fatalf("luna.Rankable = false, want true — both candidate rows are correctly mapped, so priority alone decides")
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

func TestMergeWithArenaKeepsTheTwoSourcesApart(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "a/high", Tier: "sonnet", Names: map[string]string{"vals": "a/high", "arena": "a-high"}},
		{Slug: "a/low", Tier: "sonnet", Names: map[string]string{"arena": "a-low"}},
		{Slug: "a/none", Tier: "sonnet", Names: map[string]string{"vals": "a/none"}},
	}
	prices := map[string]sources.PriceInfo{
		"a/high": {Slug: "a/high", InPerM: 1, OutPerM: 3, Context: 1000, Found: true},
		"a/low":  {Slug: "a/low", InPerM: 1, OutPerM: 3, Context: 1000, Found: true},
		"a/none": {Slug: "a/none", InPerM: 1, OutPerM: 3, Context: 1000, Found: true},
	}
	scores := []sources.ScoreRow{{Slug: "a/high", Metric: sources.MetricSWEBenchVerified, Value: 70, VariantMeasured: "a/high"}}
	arena := []sources.ScoreRow{
		{Slug: "a/high", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "a-high", CanonicalID: "a-high", Metric: sources.MetricArenaElo, Value: 1500, VariantMeasured: "a/high"},
		{Slug: "a/low", SourceFamily: ScoreSourceArena, ConfiguredIdentity: "a-low", CanonicalID: "a-low", Metric: sources.MetricArenaElo, Value: 1300, VariantMeasured: "a/low"},
	}
	models := MergeWithArena(entries, prices, scores, arena, testNotes(t))
	byslug := map[string]Model{}
	for _, m := range models {
		byslug[m.Slug] = m
	}

	high := byslug["a/high"]
	if high.Score == nil || high.Score.Value != 70 || high.ScoreLabel != "70.0%" {
		t.Errorf("a/high SWE-bench cell = %+v / %q, want the untouched 70%%", high.Score, high.ScoreLabel)
	}
	if high.ArenaScore == nil || high.ArenaScore.Value != 1500 {
		t.Errorf("a/high ArenaScore = %+v, want the raw Elo 1500", high.ArenaScore)
	}
	if high.ArenaLabel != "1500 Elo" {
		t.Errorf("a/high ArenaLabel = %q, want %q", high.ArenaLabel, "1500 Elo")
	}
	if high.ArenaNormalized != 100 {
		t.Errorf("a/high ArenaNormalized = %v, want 100 (it is the set maximum)", high.ArenaNormalized)
	}

	low := byslug["a/low"]
	if low.Score != nil || low.ScoreLabel != "n/a" {
		t.Errorf("a/low SWE-bench cell = %+v / %q, want no number at all — an Arena Elo must never fill it", low.Score, low.ScoreLabel)
	}
	if low.ArenaNormalized != 0 || !low.ArenaRankable {
		t.Errorf("a/low arena = %v / rankable %v, want 0 and rankable", low.ArenaNormalized, low.ArenaRankable)
	}

	none := byslug["a/none"]
	if none.ArenaScore != nil || none.ArenaRankable {
		t.Errorf("a/none arena = %+v / rankable %v, want nothing", none.ArenaScore, none.ArenaRankable)
	}
	if none.ArenaLabel != "n/a" {
		t.Errorf("a/none ArenaLabel = %q, want %q", none.ArenaLabel, "н/д")
	}
	if none.ArenaQualityPriceLabel != "n/a (no LMArena score)" {
		t.Errorf("a/none ArenaQualityPriceLabel = %q, want the Arena-specific reason, not the SWE-bench one", none.ArenaQualityPriceLabel)
	}
}

func TestMergeStillWorksWithoutArena(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "a/high", Tier: "sonnet", Names: map[string]string{"vals": "a/high"}}}
	prices := map[string]sources.PriceInfo{"a/high": {Slug: "a/high", InPerM: 1, OutPerM: 3, Context: 1000, Found: true}}
	scores := []sources.ScoreRow{{Slug: "a/high", Metric: sources.MetricSWEBenchVerified, Value: 70, VariantMeasured: "a/high"}}
	models := Merge(entries, prices, scores, testNotes(t))
	if len(models) != 1 || models[0].ScoreLabel != "70.0%" || models[0].ArenaScore != nil {
		t.Errorf("Merge = %+v, want the pre-existing behaviour and an empty Arena column", models)
	}
}

func TestSourceFamilyRegistry(t *testing.T) {
	for id, want := range map[string]string{"swebench": ScoreSourceSWEBench, "vals": ScoreSourceSWEBench, "arena": ScoreSourceArena} {
		if got := SourceFamily[id]; got != want {
			t.Errorf("SourceFamily[%q] = %q, want %q", id, got, want)
		}
	}
	if got := SourceFamily["not-a-source"]; got != "" {
		t.Errorf("SourceFamily[unknown] = %q, want the empty string so unknown ids feed neither view", got)
	}
}

func TestForScoreSourceProjectsArenaAndHidesSWEBench(t *testing.T) {
	models := []Model{
		{
			Slug: "a/both", Tier: "sonnet", MixedPrice: 2,
			Score: &ScoreInfo{Metric: "SWE-bench Verified", Value: 70}, ScoreLabel: "70.0%", Rankable: true,
			QualityPrice: 35, QualityPriceLabel: "35",
			ArenaScore:      &ScoreInfo{Metric: "LMArena Elo", Value: 1500, Unit: "Elo", VariantMeasured: "a-both", SourceURL: "u", Checked: "2026-08-06", IdentityStatus: IdentityExact},
			ArenaNormalized: 100, ArenaLabel: "1500 Elo", ArenaRankable: true,
			ArenaQualityPrice: 50, ArenaQualityPriceLabel: "50",
		},
		{
			Slug: "a/swe-only", Tier: "sonnet", MixedPrice: 2,
			Score: &ScoreInfo{Metric: "SWE-bench Verified", Value: 60}, ScoreLabel: "60.0%", Rankable: true,
			QualityPrice: 30, QualityPriceLabel: "30",
			ArenaLabel: "n/a", ArenaQualityPriceLabel: "n/a (no LMArena score)",
		},
	}

	same := ForScoreSource(models, ScoreSourceSWEBench)
	if same[0].ScoreLabel != "70.0%" || same[1].ScoreLabel != "60.0%" {
		t.Errorf("the swebench view changed the rows: %+v", same)
	}

	arena := ForScoreSource(models, ScoreSourceArena)
	if arena[0].Score == nil || arena[0].Score.Value != 1500 {
		t.Errorf("a/both Score = %+v, want raw Elo 1500", arena[0].Score)
	}
	if arena[0].Score.Metric != "LMArena Elo" || arena[0].Score.Checked != "2026-08-06" || arena[0].Score.Unit != "Elo" {
		t.Errorf("a/both provenance = %+v, want the Arena metric and date", arena[0].Score)
	}
	if arena[0].ScoreLabel != "1500 Elo" || arena[0].RankingScore != 100 {
		t.Errorf("a/both ScoreLabel = %q, want the raw Elo, which is what a human should read", arena[0].ScoreLabel)
	}
	if arena[0].QualityPriceLabel != "50" || arena[0].QualityPrice != 50 {
		t.Errorf("a/both quality/price = %v / %q, want the Arena view's own numbers", arena[0].QualityPrice, arena[0].QualityPriceLabel)
	}
	if arena[1].Score != nil || arena[1].Rankable {
		t.Errorf("a/swe-only = %+v / rankable %v, want no number at all in the Arena view", arena[1].Score, arena[1].Rankable)
	}
	if arena[1].ScoreLabel != "n/a" {
		t.Errorf("a/swe-only ScoreLabel = %q, want н/д even though it has a real SWE-bench score", arena[1].ScoreLabel)
	}

	if models[0].ScoreLabel != "70.0%" || models[1].Score == nil {
		t.Errorf("ForScoreSource mutated its input: %+v", models)
	}
}

package notes

import (
	"path/filepath"
	"testing"
)

func load(t *testing.T) *Notes {
	t.Helper()
	n, err := Load(filepath.Join("testdata", "notes.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return n
}

func TestModelAccessors(t *testing.T) {
	n := load(t)

	if got := n.DisplayName("openai/gpt-5.6-luna"); got != "GPT-5.6 Luna" {
		t.Errorf("DisplayName = %q, want %q", got, "GPT-5.6 Luna")
	}
	if got := n.Owner("openai/gpt-5.6-luna"); got != "OpenAI (C)" {
		t.Errorf("Owner = %q, want %q", got, "OpenAI (C)")
	}
	if got := n.OpenWeights("openai/gpt-5.6-luna"); got != "нет" {
		t.Errorf("OpenWeights = %q, want %q", got, "нет")
	}
	if got := n.ModelNote("openai/gpt-5.6-luna"); got != "Оценка независимая (vals.ai)." {
		t.Errorf("ModelNote = %q", got)
	}
	if got := n.ClaudeRef("nvidia/nemotron-3-ultra-550b-a55b:free"); got != "≈ Haiku 4.5 и ниже (середина диапазона)" {
		t.Errorf("ClaudeRef = %q", got)
	}
}

func TestMissingKeysFallBackToNeedsReview(t *testing.T) {
	n := load(t)
	const unknown = "some/brand-new-model"

	if got := n.ModelNote(unknown); got != NeedsReview {
		t.Errorf("ModelNote(unknown) = %q, want %q", got, NeedsReview)
	}
	if got := n.Owner(unknown); got != NeedsReview {
		t.Errorf("Owner(unknown) = %q, want %q", got, NeedsReview)
	}
	if got := n.OpenWeights(unknown); got != NeedsReview {
		t.Errorf("OpenWeights(unknown) = %q, want %q", got, NeedsReview)
	}
	if got := n.ClaudeRef(unknown); got != NeedsReview {
		t.Errorf("ClaudeRef(unknown) = %q, want %q", got, NeedsReview)
	}
	if got := n.DisplayName(unknown); got != unknown {
		t.Errorf("DisplayName(unknown) = %q, want the slug itself so the table still renders", got)
	}
	if got := n.NoScoreReason(unknown); got != "н/д (нет оценки по SWE-bench Verified)" {
		t.Errorf("NoScoreReason(unknown) = %q, want the default reason", got)
	}
	if got := n.NoScoreReason("deepseek/deepseek-v4-pro"); got != "н/д (оценка не для этого варианта)" {
		t.Errorf("NoScoreReason(v4-pro) = %q, want the per-model override", got)
	}
}

func TestScoreOverride(t *testing.T) {
	n := load(t)

	ov, ok := n.ScoreOverride("minimax/minimax-m3")
	if !ok {
		t.Fatal("ScoreOverride(minimax/minimax-m3) not found, want it")
	}
	if ov.Label != "80.5% (только вендор)" || ov.Value != 80.5 || !ov.Rankable {
		t.Errorf("override = %+v, want label/value/rankable from the fixture", ov)
	}
	if ov.Source != "https://minimax.io/" {
		t.Errorf("override.Source = %q", ov.Source)
	}

	if _, ok := n.ScoreOverride("openai/gpt-5.6-luna"); ok {
		t.Error("ScoreOverride(luna) found, want none — its number comes from a live source")
	}
	if _, ok := n.ScoreOverride("deepseek/deepseek-v4-pro"); ok {
		t.Error("ScoreOverride(v4-pro) found, want none — the row is deliberately unscored")
	}
}

func TestStaticSections(t *testing.T) {
	n := load(t)

	if got := n.UpdatedNote(); got != "цены и оценки собраны автоматически" {
		t.Errorf("UpdatedNote = %q", got)
	}
	if got := n.Section("tiers_intro"); got != "Категории — по примерному уровню качества относительно Claude." {
		t.Errorf("Section(tiers_intro) = %q", got)
	}
	if got := n.Section("free_terms"); got != NeedsReview {
		t.Errorf("Section(free_terms) = %q, want %q — the fixture omits it", got, NeedsReview)
	}
	if got := n.FableVerdict(); got == NeedsReview {
		t.Error("FableVerdict fell back to NeedsReview, want the fixture text")
	}
	if got := n.ClaudeNote(); got == NeedsReview {
		t.Error("ClaudeNote fell back to NeedsReview, want the fixture text")
	}

	if prices := n.ClaudePrices(); len(prices) != 2 || prices[0].Model != "Claude Opus 5" || prices[1].Context != "200K" {
		t.Errorf("ClaudePrices = %+v, want the two fixture rows in order", prices)
	}
	if toks := n.ClaudeTokens(); len(toks) != 1 || toks[0].Mixed != "1.00" {
		t.Errorf("ClaudeTokens = %+v", toks)
	}
	if comp := n.Companies(); len(comp) != 2 || comp[0].Name != "Anthropic" || comp[1].Grade != "C (2.28)" {
		t.Errorf("Companies = %+v, want the two fixture rows in order", comp)
	}
	if cav := n.Caveats(); len(cav) != 2 || cav[1] != "Бенчмарки сильно зависят от скаффолда." {
		t.Errorf("Caveats = %+v", cav)
	}
}

func TestFavoriteReason(t *testing.T) {
	n := load(t)

	if got := n.FavoriteReason("opus", "openai/gpt-5.6-luna"); got != "Лучшее соотношение цена/качество в Opus-тире." {
		t.Errorf("FavoriteReason(opus, luna) = %q", got)
	}
	if got := n.FavoriteReason("opus", "openai/gpt-5.6-sol"); got != NeedsReview {
		t.Errorf("FavoriteReason(opus, sol) = %q, want %q", got, NeedsReview)
	}
	if got := n.FavoriteReason("haiku", "openai/gpt-5.6-luna"); got != NeedsReview {
		t.Errorf("FavoriteReason(haiku, luna) = %q, want %q — the reason is per tier AND slug", got, NeedsReview)
	}
}

func TestLoadMissingFileIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join("testdata", "no-such-notes.yaml")); err == nil {
		t.Fatal("Load returned nil error for a missing notes.yaml, want an error")
	}
}

func TestUpdatedNoteFallsBackToNeedsReview(t *testing.T) {
	n := &Notes{}
	if got := n.UpdatedNote(); got != NeedsReview {
		t.Errorf("UpdatedNote() on empty Notes = %q, want %q", got, NeedsReview)
	}
}

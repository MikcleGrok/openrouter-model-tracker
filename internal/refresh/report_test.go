package refresh

import (
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

func TestBuildReport(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "openai/gpt-5.6-luna", Tier: "opus"},
		{Slug: "openai/gpt-5.6-sol", Tier: "opus", Names: map[string]string{"vals": "openai/gpt-5.6-sol"}},
		{Slug: "x-ai/grok-4.1-fast", Tier: "sonnet"},
	}
	catalog := []string{
		"openai/gpt-5.6-luna",
		"openai/gpt-5.6-sol",
		"openai/gpt-5.7-nova",  // новый у уже отслеживаемого вендора -> кандидат
		"acme/brand-new-thing", // вендор, которого в карте нет вовсе -> шум, не кандидат
	}
	prices := map[string]sources.PriceInfo{
		"openai/gpt-5.6-luna": {Slug: "openai/gpt-5.6-luna", Found: true},
		"openai/gpt-5.6-sol":  {Slug: "openai/gpt-5.6-sol", Found: true},
		"x-ai/grok-4.1-fast":  {Slug: "x-ai/grok-4.1-fast"}, // Found == false
	}
	models := []model.Model{
		{Slug: "openai/gpt-5.6-luna", Note: "есть", Owner: "OpenAI (C)", OpenWeights: "нет",
			Score: &model.ScoreInfo{Value: 93}},
		{Slug: "openai/gpt-5.6-sol", Note: notes.NeedsReview, Owner: "OpenAI (C)", OpenWeights: "нет"},
	}

	r := BuildReport(entries, catalog, prices, true, models)

	if len(r.NewCandidates) != 1 || r.NewCandidates[0] != "openai/gpt-5.7-nova" {
		t.Errorf("NewCandidates = %v, want only the new model of an already tracked vendor", r.NewCandidates)
	}
	if len(r.Retired) != 1 || r.Retired[0] != "x-ai/grok-4.1-fast" {
		t.Errorf("Retired = %v, want the slug the catalogue no longer knows", r.Retired)
	}
	if len(r.NeedsReview) != 1 || r.NeedsReview[0] != "openai/gpt-5.6-sol" {
		t.Errorf("NeedsReview = %v, want the model whose prose is missing", r.NeedsReview)
	}
	if len(r.NoScore) != 1 || r.NoScore[0] != "openai/gpt-5.6-sol" {
		t.Errorf("NoScore = %v, want the model that ended the run with no number", r.NoScore)
	}
}

func TestBuildReportNoScoreOnlyFlagsModelsWithADeclaredSource(t *testing.T) {
	entries := []modelmap.Entry{
		// Declares a vals= source but ended the run with no score — the real
		// "check your model-map.tsv spelling" signal.
		{Slug: "openai/gpt-5.6-sol", Tier: "opus", Names: map[string]string{"vals": "openai/gpt-5.6-sol"}},
		// No source declared at all — its score, if any, comes entirely from a
		// manual notes.yaml override or is absent by design. Not a map bug.
		{Slug: "minimax/minimax-m3", Tier: "sonnet"},
	}
	models := []model.Model{
		{Slug: "openai/gpt-5.6-sol", Note: "есть", Owner: "OpenAI (C)", OpenWeights: "нет"},
		{Slug: "minimax/minimax-m3", Note: "есть", Owner: "MiniMax (н/д)", OpenWeights: "да"},
	}

	r := BuildReport(entries, nil, nil, true, models)
	if len(r.NoScore) != 1 || r.NoScore[0] != "openai/gpt-5.6-sol" {
		t.Errorf("NoScore = %v, want only the model with a declared source and no score", r.NoScore)
	}
}

func TestBuildReportReportsNothingWhenTheCatalogueIsUnavailable(t *testing.T) {
	entries := []modelmap.Entry{{Slug: "openai/gpt-5.6-luna", Tier: "opus"}}
	prices := map[string]sources.PriceInfo{
		"openai/gpt-5.6-luna": {Slug: "openai/gpt-5.6-luna", InPerM: 0.5, OutPerM: 3, Found: true},
	}
	models := []model.Model{{Slug: "openai/gpt-5.6-luna", Note: "есть", Owner: "OpenAI (C)", OpenWeights: "нет", Score: &model.ScoreInfo{Value: 93}}}

	r := BuildReport(entries, nil, prices, true, models)
	if len(r.NewCandidates) != 0 {
		t.Errorf("NewCandidates = %v, want none when the catalogue could not be fetched", r.NewCandidates)
	}
	if len(r.Retired) != 0 {
		t.Errorf("Retired = %v, want none — a failed lookup must never look like a retirement", r.Retired)
	}
}

func TestCatalogDeltaReportsAddedAndRemovedSlugs(t *testing.T) {
	added, removed := catalogDelta(
		[]string{"acme/old", "openai/kept"},
		[]string{"openai/kept", "openai/new"},
	)
	if len(added) != 1 || added[0] != "openai/new" {
		t.Errorf("added = %v, want [openai/new]", added)
	}
	if len(removed) != 1 || removed[0] != "acme/old" {
		t.Errorf("removed = %v, want [acme/old]", removed)
	}
}

func TestBuildReportRetiredKeysOffPricesOKNotCatalog(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "openai/gpt-5.6-luna", Tier: "opus"},
		{Slug: "x-ai/grok-4.1-fast", Tier: "sonnet"},
	}
	prices := map[string]sources.PriceInfo{
		"openai/gpt-5.6-luna": {Slug: "openai/gpt-5.6-luna", Found: true},
		"x-ai/grok-4.1-fast":  {Slug: "x-ai/grok-4.1-fast"}, // Found == false
	}
	models := []model.Model{
		{Slug: "openai/gpt-5.6-luna", Note: "есть", Owner: "OpenAI (C)", OpenWeights: "нет",
			Score: &model.ScoreInfo{Value: 93}},
	}

	// Catalogue failed (nil/empty) but prices succeeded: Retired must still be
	// reported, since it only needs prices, not the catalogue slug list.
	r := BuildReport(entries, nil, prices, true, models)
	if len(r.NewCandidates) != 0 {
		t.Errorf("NewCandidates = %v, want none when the catalogue could not be fetched", r.NewCandidates)
	}
	if len(r.Retired) != 1 || r.Retired[0] != "x-ai/grok-4.1-fast" {
		t.Errorf("Retired = %v, want the slug prices reported as gone, even with no catalogue", r.Retired)
	}

	// Prices failed (pricesOK == false): Retired must not be reported, even
	// with a catalogue and stale/fallback price entries present.
	r2 := BuildReport(entries, []string{"openai/gpt-5.6-luna", "x-ai/grok-4.1-fast"}, prices, false, models)
	if len(r2.Retired) != 0 {
		t.Errorf("Retired = %v, want none when prices did not succeed this run", r2.Retired)
	}
}

func TestBuildReportSeparatesTheTwoSourceFamilies(t *testing.T) {
	entries := []modelmap.Entry{
		{Slug: "a/arena-only-token", Tier: "sonnet", Names: map[string]string{"arena": "a-arena"}},
		{Slug: "a/swe-token", Tier: "sonnet", Names: map[string]string{"vals": "a/swe"}},
		{Slug: "a/both-tokens", Tier: "sonnet", Names: map[string]string{"vals": "a/both", "arena": "a-both"}},
		// Declares both tokens and got a real score back from both — the
		// negative case for ArenaOnly: having an Arena score is not enough,
		// it must ALSO be missing a real SWE-bench score.
		{Slug: "a/fully-scored", Tier: "sonnet", Names: map[string]string{"vals": "a/full", "arena": "a-full"}},
	}
	models := []model.Model{
		{Slug: "a/arena-only-token", ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1400}},
		{Slug: "a/swe-token"},
		{Slug: "a/both-tokens", Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70}},
		{Slug: "a/fully-scored",
			Score:      &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 80},
			ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1500}},
	}
	r := BuildReport(entries, nil, nil, true, models)

	if len(r.NoScore) != 1 || r.NoScore[0] != "a/swe-token" {
		t.Errorf("NoScore = %v, want only the slug that declared a SWE-bench source and got nothing", r.NoScore)
	}
	if len(r.NoArenaScore) != 1 || r.NoArenaScore[0] != "a/both-tokens" {
		t.Errorf("NoArenaScore = %v, want only the slug that declared arena= and got nothing", r.NoArenaScore)
	}
	if len(r.ArenaOnly) != 1 || r.ArenaOnly[0] != "a/arena-only-token" {
		t.Errorf("ArenaOnly = %v, want only the row whose sole quality signal is a crowd Elo — a/fully-scored has a real SWE-bench score too and must not appear here", r.ArenaOnly)
	}
}

func TestReportStringShowsTheArenaSections(t *testing.T) {
	out := Report{NoArenaScore: []string{"a/x"}, ArenaOnly: []string{"a/y"}}.String()
	if !strings.Contains(out, "a/x") || !strings.Contains(out, "arena=") {
		t.Errorf("report does not name the Arena mapping gap:\n%s", out)
	}
	if !strings.Contains(out, "a/y") || !strings.Contains(out, "только Arena") {
		t.Errorf("report does not show the Arena-only section:\n%s", out)
	}
}

func TestReportString(t *testing.T) {
	r := Report{
		NewCandidates:  []string{"openai/gpt-5.7-nova"},
		CatalogAdded:   []string{"openai/gpt-5.8-orbit"},
		CatalogRemoved: []string{"openai/gpt-5.5-old"},
		Retired:        []string{"x-ai/grok-4.1-fast"},
		NeedsReview:    []string{"openai/gpt-5.6-sol"},
		NoScore:        []string{"openai/gpt-5.6-sol"},
		Warnings:       []string{"vals: источник не отвечает"},
	}
	s := r.String()
	for _, want := range []string{"openai/gpt-5.7-nova", "openai/gpt-5.8-orbit", "openai/gpt-5.5-old", "x-ai/grok-4.1-fast", "openai/gpt-5.6-sol", "vals: источник не отвечает"} {
		if !strings.Contains(s, want) {
			t.Errorf("Report.String() does not mention %q:\n%s", want, s)
		}
	}

	if empty := (Report{}).String(); !strings.Contains(empty, "нечего") {
		t.Errorf("an empty report must say so explicitly, got:\n%s", empty)
	}
}

// TestReportStringPriceChangesStayRussian locks down that this report — read
// only by internal/refresh's Russian-language callers, never by the TUI's
// own language toggle — keeps rendering a long-context override change with
// the Russian preposition "от" regardless of pricehistory.Format's lang
// parameter existing at all: this call site must always pass "ru".
func TestReportStringPriceChangesStayRussian(t *testing.T) {
	r := Report{PriceChanges: []PriceChange{{
		Slug:     "openai/gpt-5.6-luna",
		Previous: pricehistory.Price{Found: true, InPerM: 0.5, OutPerM: 3, Context: 1000000},
		Current: pricehistory.Price{
			Found: true, InPerM: 0.5, OutPerM: 3, Context: 1000000,
			HasOverride: true, OverrideMinTokens: 272000, OverrideInPerM: 1, OverrideOutPerM: 4,
		},
	}}}
	s := r.String()
	if !strings.Contains(s, "openai/gpt-5.6-luna: $0.5/$3, 1000K → $0.5/$3, 1000K; long-context $1/$4 от 272K+") {
		t.Errorf("Report.String() price change did not render the Russian long-context clause:\n%s", s)
	}
	if strings.Contains(s, " from ") {
		t.Errorf("Report.String() leaked the English preposition into this always-Russian report:\n%s", s)
	}
}

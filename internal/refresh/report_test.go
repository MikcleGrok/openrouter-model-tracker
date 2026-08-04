package refresh

import (
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
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

func TestReportString(t *testing.T) {
	r := Report{
		NewCandidates: []string{"openai/gpt-5.7-nova"},
		Retired:       []string{"x-ai/grok-4.1-fast"},
		NeedsReview:   []string{"openai/gpt-5.6-sol"},
		NoScore:       []string{"openai/gpt-5.6-sol"},
		Warnings:      []string{"vals: источник не отвечает"},
	}
	s := r.String()
	for _, want := range []string{"openai/gpt-5.7-nova", "x-ai/grok-4.1-fast", "openai/gpt-5.6-sol", "vals: источник не отвечает"} {
		if !strings.Contains(s, want) {
			t.Errorf("Report.String() does not mention %q:\n%s", want, s)
		}
	}

	if empty := (Report{}).String(); !strings.Contains(empty, "нечего") {
		t.Errorf("an empty report must say so explicitly, got:\n%s", empty)
	}
}

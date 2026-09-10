package pricehistory

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

func TestHistoryRoundTripAndEmptyOrLegacyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache", "price-history.json")
	history := &History{}
	history.Add(time.Date(2026, 8, 4, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60)), map[string]sources.PriceInfo{
		"openai/model": {Slug: "openai/model", Found: true, InPerM: 0.5, OutPerM: 3, Context: 1000000, HasOverride: true, OverrideMinTokens: 500000, OverrideInPerM: 1, OverrideOutPerM: 4},
	})
	if err := history.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil || got.SchemaVersion != SchemaVersion || len(got.Observations) != 1 {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if got.Observations[0].ObservedAt.Location() != time.UTC || got.Observations[0].Prices["openai/model"].OverrideOutPerM != 4 {
		t.Fatalf("round trip lost normalized values: %+v", got.Observations[0])
	}
	emptyPath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	empty, err := Load(emptyPath)
	if err != nil || len(empty.Observations) != 0 {
		t.Fatalf("empty file = %+v, %v", empty, err)
	}
	legacyPath := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(legacyPath, []byte(`{"observations":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy, err := Load(legacyPath)
	if err != nil || legacy.SchemaVersion != SchemaVersion {
		t.Fatalf("legacy file = %+v, %v", legacy, err)
	}
}

func TestHistoryRetention(t *testing.T) {
	history := &History{}
	for i := 0; i < MaxObservations+4; i++ {
		history.Add(time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC), nil)
	}
	if len(history.Observations) != MaxObservations {
		t.Fatalf("observations = %d, want %d", len(history.Observations), MaxObservations)
	}
	if !history.Observations[0].ObservedAt.Equal(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("oldest observation = %s", history.Observations[0].ObservedAt)
	}
}

func TestHistoryStoresLiveScoresAndDerivedSWEQualityPrice(t *testing.T) {
	history := &History{}
	history.AddObservation(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC), map[string]sources.PriceInfo{
		"openai/model": {Found: true, InPerM: 0.5, OutPerM: 3},
	}, []sources.ScoreRow{{Slug: "openai/model", SourceFamily: "vals", Metric: sources.MetricSWEBenchVerified, Value: 90, Unit: "%", ConfiguredIdentity: "openai/model", IdentityStatus: "exact_product", SourceURL: "swe"}}, []sources.ScoreRow{{Slug: "openai/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1400, Unit: "Elo", IdentityStatus: "exact_product", SourceURL: "arena"}})
	if len(history.Observations) != 1 || len(history.Observations[0].Scores) != 2 {
		t.Fatalf("scores = %#v", history.Observations[0].Scores)
	}
	swe := history.Observations[0].Scores["openai/model\x00swebench"]
	if swe.SourceID != "vals" || swe.Provenance != "swe" || swe.QualityPrice == nil || swe.Formula == "" {
		t.Fatalf("SWE observation = %#v", swe)
	}
	arena := history.Observations[0].Scores["openai/model\x00arena"]
	if arena.Value != 1400 || arena.Unit != "Elo" || arena.QualityPrice != nil {
		t.Fatalf("Arena observation = %#v", arena)
	}
}

func TestHistoryQPGateRequiresCanonicalLiveSWEIdentity(t *testing.T) {
	history := &History{}
	prices := map[string]sources.PriceInfo{"demo/model": {Found: true, InPerM: 1, OutPerM: 3}}
	rows := []sources.ScoreRow{
		{Slug: "demo/model", SourceFamily: "vals", Metric: sources.MetricSWEBenchVerified, Value: 90, IdentityStatus: "exact_product", SourceURL: "vals"},
	}
	history.AddObservation(time.Now(), prices, rows, nil)
	point := history.Observations[0].Scores["demo/model\x00swebench"]
	if point.SourceID != "vals" || point.QualityPrice == nil {
		t.Fatalf("canonical vals point = %#v", point)
	}

	for _, status := range []string{"missing_identity", "legacy_unknown", "observation_only", "variant_mismatch", ""} {
		point := Score{}
		h := &History{}
		h.AddObservation(time.Now(), prices, []sources.ScoreRow{{Slug: "demo/model", SourceFamily: "vals", Metric: sources.MetricSWEBenchVerified, Value: 90, IdentityStatus: status}}, nil)
		point = h.Observations[0].Scores["demo/model\x00swebench"]
		if point.QualityPrice != nil {
			t.Errorf("status %q produced Q/P: %#v", status, point)
		}
	}
}

func TestHistoryArenaRequiresExactFiniteIdentity(t *testing.T) {
	for _, row := range []sources.ScoreRow{
		{Slug: "demo/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1400, IdentityStatus: "variant_mismatch"},
		{Slug: "demo/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1400, IdentityStatus: ""},
		{Slug: "demo/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1400, IdentityStatus: "observation_only"},
		{Slug: "demo/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: 1400, IdentityStatus: "exact_product"},
		{Slug: "demo/model", SourceFamily: "arena", Metric: sources.MetricArenaElo, Value: math.NaN(), IdentityStatus: "exact_product"},
	} {
		history := &History{}
		history.AddObservation(time.Now(), map[string]sources.PriceInfo{"demo/model": {Found: true}}, nil, []sources.ScoreRow{row})
		if row.IdentityStatus == "exact_product" && row.Value == 1400 {
			if len(history.Observations[0].Scores) != 1 {
				t.Fatalf("valid Arena row was dropped: %#v", history.Observations[0].Scores)
			}
		} else if len(history.Observations[0].Scores) != 0 {
			t.Errorf("invalid Arena row was stored: %#v", history.Observations[0].Scores)
		}
	}
}

// TestFormat guards the long-context override clause's preposition: it must
// follow the lang argument ("" = English "from", "ru" = Russian "от")
// instead of being hardcoded to Russian regardless of caller — the bug that
// let an unconditional Cyrillic "от" leak into English-mode output.
func TestFormat(t *testing.T) {
	base := Price{Found: true, InPerM: 0.5, OutPerM: 3, Context: 1000000}
	if got := Format(base, ""); got != "$0.5/$3, 1000K" {
		t.Errorf("Format(base, \"\") = %q, want %q", got, "$0.5/$3, 1000K")
	}
	if got := Format(base, "ru"); got != "$0.5/$3, 1000K" {
		t.Errorf("Format(base, \"ru\") = %q, want %q — no override, nothing to translate", got, "$0.5/$3, 1000K")
	}

	override := base
	override.HasOverride = true
	override.OverrideMinTokens = 272000
	override.OverrideInPerM = 1
	override.OverrideOutPerM = 4
	if got := Format(override, ""); got != "$0.5/$3, 1000K; long-context $1/$4 from 272K+" {
		t.Errorf("Format(override, \"\") = %q, want the English preposition %q", got, "from")
	}
	if got := Format(override, "ru"); got != "$0.5/$3, 1000K; long-context $1/$4 от 272K+" {
		t.Errorf("Format(override, \"ru\") = %q, want the Russian preposition %q", got, "от")
	}
}

func TestHistorySaveErrorDoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	history := &History{}
	if err := history.Save(filepath.Join(path, "not-a-file")); err == nil {
		t.Fatal("Save unexpectedly succeeded below a regular file")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "old" {
		t.Fatalf("existing file changed: %q, %v", body, err)
	}
}

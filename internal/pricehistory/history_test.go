package pricehistory

import (
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

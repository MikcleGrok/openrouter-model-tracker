package modelmap

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	got, err := Load(filepath.Join("testdata", "model-map.tsv"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3 (comments and blank lines must be skipped)", len(got))
	}

	sol := got[0]
	if sol.Slug != "openai/gpt-5.6-sol" {
		t.Errorf("entry[0].Slug = %q, want %q", sol.Slug, "openai/gpt-5.6-sol")
	}
	if sol.Tier != "opus" {
		t.Errorf("entry[0].Tier = %q, want %q", sol.Tier, "opus")
	}
	if sol.Names["vals"] != "openai/gpt-5.6-sol" {
		t.Errorf("entry[0].Names[vals] = %q, want %q", sol.Names["vals"], "openai/gpt-5.6-sol")
	}
	if sol.Names["swebench"] != "Model: gpt-5.6-sol" {
		t.Errorf("entry[0].Names[swebench] = %q, want %q (the space after the colon must survive)", sol.Names["swebench"], "Model: gpt-5.6-sol")
	}

	m3 := got[1]
	if m3.Slug != "minimax/minimax-m3" || m3.Tier != "sonnet" {
		t.Errorf("entry[1] = %+v, want minimax/minimax-m3 in tier sonnet", m3)
	}
	if len(m3.Names) != 0 {
		t.Errorf("entry[1].Names = %v, want empty (a row may legitimately track no benchmark source)", m3.Names)
	}

	free := got[2]
	if free.Tier != "free" {
		t.Errorf("entry[2].Tier = %q, want %q", free.Tier, "free")
	}
}

func TestLoadRejectsRowWithoutTier(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "no-tier.tsv"))
	if err == nil {
		t.Fatal("Load returned nil error for a row without tier=, want an error")
	}
	if !strings.Contains(err.Error(), "no-tier.tsv:1") {
		t.Errorf("error %q must name the offending file and line", err)
	}
	if !strings.Contains(err.Error(), "openai/gpt-5.6-sol") {
		t.Errorf("error %q must name the offending slug", err)
	}
}

func TestLoadMissingFileIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join("testdata", "does-not-exist.tsv")); err == nil {
		t.Fatal("Load returned nil error for a missing file, want an error")
	}
}

func TestSlugsAndNamesFor(t *testing.T) {
	entries, err := Load(filepath.Join("testdata", "model-map.tsv"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	slugs := Slugs(entries)
	if len(slugs) != 3 || slugs[0] != "openai/gpt-5.6-sol" {
		t.Errorf("Slugs = %v, want 3 slugs in file order", slugs)
	}

	vals := NamesFor(entries, "vals")
	if len(vals) != 1 || vals["openai/gpt-5.6-sol"] != "openai/gpt-5.6-sol" {
		t.Errorf("NamesFor(vals) = %v, want exactly the one row that declares a vals name", vals)
	}

	swe := NamesFor(entries, "swebench")
	if len(swe) != 2 {
		t.Errorf("NamesFor(swebench) = %v, want 2 rows", swe)
	}
	if swe["nvidia/nemotron-3-ultra-550b-a55b:free"] != "Model: nemotron-3-ultra" {
		t.Errorf("NamesFor(swebench) lost the free row: %v", swe)
	}
}

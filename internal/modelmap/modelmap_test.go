package modelmap

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
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

func TestTaskFitMetadataUsesProductionLoaderForBothForms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.yaml")
	content := "models:\n  nested/model:\n    task_fit: [test, implement, test]\ntask_fit:\n  top/model: [audit, implement]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := notes.Load(path)
	if err != nil {
		t.Fatalf("notes.Load: %v", err)
	}
	if got := parsed.TaskFit("nested/model"); strings.Join(got, ",") != "implement,test" {
		t.Errorf("nested task_fit = %v", got)
	}
	if got := parsed.TaskFit("top/model"); strings.Join(got, ",") != "implement,audit" {
		t.Errorf("top-level task_fit = %v", got)
	}
	if _, err := func() (*notes.Notes, error) {
		invalidPath := filepath.Join(t.TempDir(), "notes.yaml")
		if err := os.WriteFile(invalidPath, []byte("models:\n  invalid/model:\n    task_fit: [unknown]\n"), 0o644); err != nil {
			return nil, err
		}
		return notes.Load(invalidPath)
	}(); err == nil || !strings.Contains(err.Error(), "unknown keyword") {
		t.Fatalf("invalid task_fit error = %v, want production validation error", err)
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

func TestProductionTaskFitMetadataMatchesModelMap(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	entries, err := Load(filepath.Join(root, "model-map.tsv"))
	if err != nil {
		t.Fatalf("production model-map.tsv: %v", err)
	}
	parsedNotes, err := notes.Load(filepath.Join(root, "notes.yaml"))
	if err != nil {
		t.Fatalf("production notes.yaml: %v", err)
	}
	mapSlugs := make(map[string]bool, len(entries))
	for _, entry := range entries {
		mapSlugs[entry.Slug] = true
	}
	for slug := range mapSlugs {
		if values := parsedNotes.TaskFit(slug); values == nil && slug != "nvidia/nemotron-nano-12b-v2-vl:free" && slug != "nvidia/nemotron-3.5-content-safety:free" && slug != "inclusionai/ling-3.0-flash:free" && slug != "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free" {
			t.Errorf("task_fit is missing model-map slug %q", slug)
		}
	}
	if got := parsedNotes.TaskFit("openai/gpt-5.6-luna"); strings.Join(got, ",") != "implement,plan,research,debug,audit,refactor,test" {
		t.Errorf("production task_fit normalization = %v", got)
	}
}

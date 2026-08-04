package refresh

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadIgnorePatternsSkipsCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ignore-candidates.txt")
	content := "# comment\n\n*/*-preview\n  */*-2025*  \n# another comment\n*/*-distill-*\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	patterns, err := loadIgnorePatterns(path)
	if err != nil {
		t.Fatalf("loadIgnorePatterns: %v", err)
	}
	want := []string{"*/*-preview", "*/*-2025*", "*/*-distill-*"}
	if !reflect.DeepEqual(patterns, want) {
		t.Errorf("patterns = %v, want %v", patterns, want)
	}
}

func TestLoadIgnorePatternsMissingFileIsNilNil(t *testing.T) {
	patterns, err := loadIgnorePatterns(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("loadIgnorePatterns on a missing file must not error, got: %v", err)
	}
	if patterns != nil {
		t.Errorf("patterns = %v, want nil for a missing file", patterns)
	}
}

func TestFilterIgnoredDropsMatchingCandidates(t *testing.T) {
	candidates := []string{
		"openai/gpt-5.7-nova",
		"openai/gpt-5.7-nova-preview",
		"openai/gpt-4.9-2024-legacy",
		"anthropic/claude-mystery",
	}
	patterns := []string{"*/*-preview", "*/*-2024*"}

	got := filterIgnored(candidates, patterns)
	want := []string{"openai/gpt-5.7-nova", "anthropic/claude-mystery"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterIgnored = %v, want %v", got, want)
	}
}

func TestFilterIgnoredNoPatternsKeepsEverything(t *testing.T) {
	candidates := []string{"openai/gpt-5.7-nova", "anthropic/claude-mystery"}
	got := filterIgnored(candidates, nil)
	if !reflect.DeepEqual(got, candidates) {
		t.Errorf("filterIgnored = %v, want the input unchanged", got)
	}
}

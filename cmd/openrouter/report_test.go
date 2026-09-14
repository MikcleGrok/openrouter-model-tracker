package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportCommandWritesDefaultOutput(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "output.md")
	config := writeConfig(t, "data_dir: "+root+"\ndefault_output: "+out+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}

	output := executeCLI(t, "report", "--config", config)
	if !strings.Contains(output, "📄 Записано: "+out) {
		t.Fatalf("report output = %q, want a written-path line for %s", output, out)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	doc := string(body)
	if !strings.Contains(doc, "## Рейтинг моделей по цене и качеству") {
		t.Fatalf("report output document has no ranked-list section:\n%s", doc)
	}
	if !strings.Contains(doc, "## Приложение: происхождение оценок") {
		t.Fatalf("report output document has no provenance appendix:\n%s", doc)
	}
	if strings.Contains(doc, "\n  Provenance:") {
		t.Fatalf("report output document still has a table-breaking bare Provenance line:\n%s", doc)
	}
}

func TestReportCommandSortChangesRankedOrder(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "output.md")
	config := writeConfig(t, "data_dir: "+root+"\ndefault_output: "+out+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}

	executeCLI(t, "report", "--config", config)
	byQP, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	docQP := string(byQP)
	rankedQP := rankedSection(t, docQP)
	if idxLow, idxHigh := strings.Index(rankedQP, "Demo Low"), strings.Index(rankedQP, "Demo High"); idxLow == -1 || idxHigh == -1 || idxLow >= idxHigh {
		t.Fatalf("default --sort q/p ranked section = %q, want Demo Low (QP=10) before Demo High (QP=0.9)", rankedQP)
	}

	executeCLI(t, "report", "--config", config, "--sort", "name")
	byName, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	docName := string(byName)
	rankedName := rankedSection(t, docName)
	if idxHigh, idxLow := strings.Index(rankedName, "Demo High"), strings.Index(rankedName, "Demo Low"); idxHigh == -1 || idxLow == -1 || idxHigh >= idxLow {
		t.Fatalf("--sort name ranked section = %q, want Demo High before Demo Low (alphabetical)", rankedName)
	}
	if !strings.Contains(docName, "Sort: name") {
		t.Fatalf("--sort name generation line missing from document:\n%s", docName)
	}
}

// rankedSection extracts just the "Рейтинг" table's text, so a position
// comparison between two model names cannot accidentally match a mention in
// a different section (Favorites, tier tables, the appendix, ...).
func rankedSection(t *testing.T, doc string) string {
	t.Helper()
	start := strings.Index(doc, "## Рейтинг моделей по цене и качеству")
	if start == -1 {
		t.Fatalf("document has no ranked-list section:\n%s", doc)
	}
	rest := doc[start:]
	end := strings.Index(rest, "\n## ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

func TestReportCommandRejectsInvalidScoreSource(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\ndefault_output: "+filepath.Join(root, "output.md")+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	err := executeCLIError(t, "report", "--config", config, "--score-source=auto")
	if err == nil || !strings.Contains(err.Error(), "invalid --score-source") {
		t.Fatalf("error = %v, want invalid --score-source", err)
	}
}

func TestReportCommandFailsWithoutDefaultOutput(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	err := executeCLIError(t, "report", "--config", config)
	if err == nil || !strings.Contains(err.Error(), "нет пути вывода") {
		t.Fatalf("error = %v, want the existing \"no output path\" error", err)
	}
}

func TestReportCommandOpenWarnsAndStillSucceedsOnAFailingOpener(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "output.md")
	config := writeConfig(t, "data_dir: "+root+"\ndefault_output: "+out+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	// OPENROUTER_OPEN overrides the platform opener with an arbitrary binary
	// name (internal/open's own documented test lever) — a name guaranteed
	// not to exist makes internal/open.File fail exactly like a real "no
	// handler" or "binary not found" condition would, without needing a Go-
	// level test seam reachable from this package.
	t.Setenv("OPENROUTER_OPEN", "omt-report-test-nonexistent-opener-binary")

	output := executeCLI(t, "report", "--config", config, "--open")
	if !strings.Contains(output, "📄 Записано: "+out) {
		t.Fatalf("report --open output = %q, want the document to still be written", output)
	}
	if !strings.Contains(output, "⚠️") || !strings.Contains(output, "Не удалось открыть документ") {
		t.Fatalf("report --open output = %q, want a warning about the failed opener", output)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output file missing after --open failure: %v", err)
	}
}

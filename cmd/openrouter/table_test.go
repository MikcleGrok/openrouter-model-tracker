package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

func TestRenderTableUsesPlainTextAndTruncatesCells(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "a very long model name that should be shortened", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "**a long note** that should also be shortened | safely"}}, 120)
	if !strings.Contains(output, "Model") || !strings.Contains(output, "Context") || !strings.Contains(output, "Input $/M") || !strings.Contains(output, "Output $/M") || !strings.Contains(output, "Status") || !strings.Contains(output, "Q/P") || !strings.Contains(output, "Note") {
		t.Fatalf("headers missing from table:\n%s", output)
	}
	if strings.Contains(output, "#") || strings.Contains(output, "|---") || strings.Contains(output, "<table") {
		t.Fatalf("table contains markup:\n%s", output)
	}
	if strings.Contains(output, "**") || strings.Contains(output, "`") {
		t.Fatalf("table contains Markdown emphasis markers:\n%s", output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if len(line) > 120 {
			t.Errorf("line exceeds requested width: %d: %q", len(line), line)
		}
	}
	if !strings.Contains(output, "...") {
		t.Errorf("long cells were not truncated:\n%s", output)
	}
}

func TestRenderTableSeparatesStatusQualityPriceAndNote(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "paid", ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "review this"}, {DisplayName: "free", ScoreLabel: "н/д", QualityPriceLabel: "н/д (цена $0)"}, {DisplayName: "no-score", ScoreLabel: "н/д", QualityPriceLabel: "н/д (оценка не для этого варианта)", Note: notes.NeedsReview}}, 120)
	if !strings.Contains(output, "| 93.0%") || !strings.Contains(output, "| 82.7") || !strings.Contains(output, "| review this") {
		t.Fatalf("paid model cells are not separated:\n%s", output)
	}
	if !strings.Contains(output, "н/д (цена $0)") || !strings.Contains(output, "н/д (оценка") {
		t.Fatalf("free/no-score Q/P labels missing:\n%s", output)
	}
	if strings.Contains(output, "93.0%; review this") || strings.Contains(output, "н/д;") || strings.Contains(output, notes.NeedsReview) {
		t.Fatalf("status and note were combined or review marker was shown:\n%s", output)
	}
}

func TestRenderTableFitsNarrowWidth(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "a model", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", Note: "a note"}}, 40)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if width := len([]rune(line)); width > 40 {
			t.Errorf("line exceeds requested width: %d: %q", width, line)
		}
	}
}

func TestRenderTableFitsNarrowWidthWithCyrillic(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "модель", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "значение", Note: "заметка с длинным текстом"}}, 40)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if width := tableDisplayWidth(line); width > 40 {
			t.Errorf("line exceeds display width: %d: %q", width, line)
		}
		if utf8.RuneCountInString(line) > 40 {
			t.Errorf("line exceeds rune limit: %d: %q", utf8.RuneCountInString(line), line)
		}
	}
	if !utf8.ValidString(output) {
		t.Fatalf("table output is not valid UTF-8: %q", output)
	}
}

func TestTableDisplayWidthHandlesCombiningAndWideRunes(t *testing.T) {
	if got := tableDisplayWidth("е\u0301界🙂"); got != 5 {
		t.Errorf("tableDisplayWidth = %d, want 5", got)
	}
	if got := truncateTable("е\u0301界🙂", 4); got != "е\u0301..." {
		t.Errorf("truncateTable = %q, want %q", got, "е\u0301...")
	}
}

func TestTableDisplayWidthHandlesEmojiSequences(t *testing.T) {
	if got := tableDisplayWidth("👍🏽👩‍💻"); got != 4 {
		t.Errorf("tableDisplayWidth = %d, want 4", got)
	}
	if got := truncateTable("👍🏽👩‍💻", 2); got != "👍🏽" {
		t.Errorf("truncateTable skin tone sequence = %q, want %q", got, "👍🏽")
	}
	if got := truncateTable("👩‍💻model", 5); got != "👩‍💻..." {
		t.Errorf("truncateTable ZWJ sequence = %q, want %q", got, "👩‍💻...")
	}
}

func TestRenderTableNormalizesControlCharacters(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "model\nname", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "score\tvalue", Note: "note\r\nwith\tcontrol\x1btext"}}, 80)
	if strings.ContainsAny(output, "\r\t\x1b") {
		t.Fatalf("table contains control characters: %q", output)
	}
}

func TestTableWidthRejectsImpossibleTerminalWidth(t *testing.T) {
	t.Setenv("COLUMNS", "39")
	if _, err := tableWidth(); err == nil || !strings.Contains(err.Error(), "minimum is 40") {
		t.Fatalf("tableWidth error = %v, want minimum-width error", err)
	}
}

func TestTableCommandReadsLocalSnapshot(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	output := executeCLI(t, "table", "--config", config)
	if !strings.Contains(output, "Model") || !strings.Contains(output, "Input $/M") {
		t.Fatalf("table output = %q", output)
	}
}

func TestTableCommandFailsWithoutSnapshot(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	if err := removeTableSnapshot(root); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"table", "--config", config})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("error = %v, want missing snapshot error", err)
	}
}

func copyTableFixture(t *testing.T, root string) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/model\ttier=sonnet\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/model:\n    display: Demo Model\n    note: Local fixture\n"), 0o644); err != nil {
		return err
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{"demo/model": {InPerM: 1, OutPerM: 2, Context: 128000}}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644)
}

func removeTableSnapshot(root string) error {
	return os.Remove(filepath.Join(root, "cache", "last-run-snapshot.json"))
}

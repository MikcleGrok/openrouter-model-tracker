package main

import (
	"encoding/json"
	"io"
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
	output := renderTable([]model.Model{{DisplayName: "a very long model name that should be shortened", Slug: "vendor/model", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "**a long note** that should also be shortened | safely"}}, 120, false)
	if !strings.Contains(output, "Name") || !strings.Contains(output, "Slug") || !strings.Contains(output, "Context") || !strings.Contains(output, "Input $/M") || !strings.Contains(output, "Output $/M") || !strings.Contains(output, "Status") || !strings.Contains(output, "Q/P") || !strings.Contains(output, "Note") {
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
	output := renderTable([]model.Model{{DisplayName: "paid", ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "review this"}, {DisplayName: "free", ScoreLabel: "н/д", QualityPriceLabel: "н/д (цена $0)"}, {DisplayName: "no-score", ScoreLabel: "н/д", QualityPriceLabel: "н/д (оценка не для этого варианта)", Note: notes.NeedsReview}}, 120, false)
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
	output := renderTable([]model.Model{{DisplayName: "a model", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", Note: "a note"}}, 40, false)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if width := len([]rune(line)); width > 40 {
			t.Errorf("line exceeds requested width: %d: %q", width, line)
		}
	}
}

func TestRenderTableFitsNarrowWidthWithCyrillic(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "модель", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "значение", Note: "заметка с длинным текстом"}}, 40, false)
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
	output := renderTable([]model.Model{{DisplayName: "model\nname", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "score\tvalue", Note: "note\r\nwith\tcontrol\x1btext"}}, 80, false)
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
	if !strings.Contains(output, "Name") || !strings.Contains(output, "Slug") || !strings.Contains(output, "Input $/M") {
		t.Fatalf("table output = %q", output)
	}
}

func TestSortTableModelsUsesTypedValuesAndSlugTiebreaker(t *testing.T) {
	models := []model.Model{{Slug: "z", DisplayName: "Same", Context: 2, InPerM: 10, OutPerM: 1, Score: &model.ScoreInfo{Value: 8}, Rankable: true, QualityPrice: 3}, {Slug: "a", DisplayName: "Same", Context: 10, InPerM: 2, OutPerM: 4, Score: &model.ScoreInfo{Value: 8}, Rankable: true, QualityPrice: 1}}
	if err := sortTableModels(models, "context", false); err != nil || models[0].Slug != "z" {
		t.Fatalf("context sort = %+v, err=%v", models, err)
	}
	if err := sortTableModels(models, "input", true); err != nil || models[0].Slug != "z" {
		t.Fatalf("reverse input sort = %+v, err=%v", models, err)
	}
	if err := sortTableModels(models, "score", false); err != nil || models[0].Slug != "a" {
		t.Fatalf("score tie-breaker = %+v, err=%v", models, err)
	}
}

func TestSortTableModelsSupportsEverySortKey(t *testing.T) {
	base := []model.Model{
		{Slug: "z/model", DisplayName: "Alpha", Context: 10, InPerM: 2, OutPerM: 9, Score: &model.ScoreInfo{Value: 8}, Rankable: true, QualityPrice: 4},
		{Slug: "a/model", DisplayName: "Zulu", Context: 20, InPerM: 1, OutPerM: 3, Score: &model.ScoreInfo{Value: 9}, Rankable: true, QualityPrice: 2},
	}
	tests := []struct {
		key  string
		want string
	}{
		{key: "name", want: "z/model"},
		{key: "slug", want: "a/model"},
		{key: "context", want: "z/model"},
		{key: "input", want: "a/model"},
		{key: "output", want: "a/model"},
		{key: "score", want: "z/model"},
		{key: "q/p", want: "a/model"},
	}
	for _, test := range tests {
		models := append([]model.Model(nil), base...)
		if err := sortTableModels(models, test.key, false); err != nil || models[0].Slug != test.want {
			t.Errorf("%s sort = %+v, err=%v", test.key, models, err)
		}
	}
}

func TestSortTableModelsMissingNumericValuesGoLast(t *testing.T) {
	models := []model.Model{{Slug: "missing"}, {Slug: "scored", Score: &model.ScoreInfo{Value: 1}, Rankable: true, QualityPrice: 1}}
	if err := sortTableModels(models, "score", false); err != nil || models[1].Slug != "missing" {
		t.Fatalf("score missing placement = %+v, err=%v", models, err)
	}
	if err := sortTableModels(models, "q/p", false); err != nil || models[1].Slug != "missing" {
		t.Fatalf("q/p missing placement = %+v, err=%v", models, err)
	}
}

func TestSortTableModelsRejectsUnknownKey(t *testing.T) {
	if err := sortTableModels(nil, "bogus", false); err == nil || !strings.Contains(err.Error(), tableSortHelp) {
		t.Fatalf("error = %v, want allowed sort values", err)
	}
}

func TestTablePagerDecision(t *testing.T) {
	var output strings.Builder
	if tableShouldPage(&output, false) || tableShouldPage(&output, true) {
		t.Fatal("buffer output must never use pager")
	}
	device, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("/dev/null is unavailable: %v", err)
	}
	t.Cleanup(func() { device.Close() })
	if tableShouldPage(device, false) {
		t.Fatal("character device without a TTY must not use pager")
	}
	previous := tableIsTTY
	tableIsTTY = func(io.Writer) bool { return true }
	t.Cleanup(func() { tableIsTTY = previous })
	if !tableShouldPage(&output, false) {
		t.Fatal("TTY output should use pager")
	}
	if tableShouldPage(&output, true) {
		t.Fatal("no-pager must disable pager in TTY")
	}
}

func TestTablePagerReceivesFullIdentityFields(t *testing.T) {
	previousTTY := tableIsTTY
	previousPager := runTablePager
	t.Cleanup(func() {
		tableIsTTY = previousTTY
		runTablePager = previousPager
	})
	tableIsTTY = func(io.Writer) bool { return true }
	var paged, stdout, stderr strings.Builder
	runTablePager = func(output string, out, errOut io.Writer) error {
		paged.WriteString(output)
		_, _ = io.WriteString(out, "pager stdout")
		_, _ = io.WriteString(errOut, "pager stderr")
		return nil
	}
	models := []model.Model{
		{DisplayName: "A model name longer than the preferred column", Slug: "vendor/a-model-slug-longer-than-the-column"},
		{DisplayName: "Short name", Slug: "vendor/another-model-with-a-different-long-slug"},
	}
	shouldPage := tableShouldPage(&stdout, false)
	if !shouldPage {
		t.Fatal("TTY output should use pager")
	}
	if err := writeTableOutput(renderTable(models, 40, shouldPage), &stdout, &stderr, shouldPage); err != nil {
		t.Fatalf("writeTableOutput: %v", err)
	}
	if !strings.Contains(paged.String(), models[0].DisplayName) || !strings.Contains(paged.String(), models[0].Slug) {
		t.Fatalf("pager received truncated identity fields:\n%s", paged.String())
	}
	if !strings.Contains(paged.String(), models[1].DisplayName) || !strings.Contains(paged.String(), models[1].Slug) {
		t.Fatalf("pager received truncated second identity fields:\n%s", paged.String())
	}
	var rowSeparators []string
	for _, line := range strings.Split(strings.TrimSpace(paged.String()), "\n") {
		if strings.HasPrefix(line, "|") {
			rowSeparators = append(rowSeparators, line)
		}
	}
	if len(rowSeparators) != 3 {
		t.Fatalf("pager rows = %d, want header plus two data rows:\n%s", len(rowSeparators), paged.String())
	}
	for column := 0; column < len(rowSeparators[0]); column++ {
		if rowSeparators[0][column] != '|' {
			continue
		}
		for _, row := range rowSeparators[1:] {
			if column >= len(row) || row[column] != '|' {
				t.Fatalf("column separator moved at position %d:\n%s", column, paged.String())
			}
		}
	}
	if stdout.String() != "pager stdout" || stderr.String() != "pager stderr" {
		t.Fatalf("pager writers: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestNonPagerTableBoundsIdentityFields(t *testing.T) {
	rowModel := model.Model{DisplayName: "A model name longer than the preferred column", Slug: "vendor/a-model-slug-longer-than-the-column"}
	output := renderTable([]model.Model{rowModel}, 40, false)
	if strings.Contains(output, rowModel.DisplayName) || strings.Contains(output, rowModel.Slug) {
		t.Fatalf("non-pager table did not truncate identity fields:\n%s", output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if tableDisplayWidth(line) > 40 {
			t.Errorf("non-pager line exceeds requested width: %d: %q", tableDisplayWidth(line), line)
		}
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

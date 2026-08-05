package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

func TestRenderTableUsesPlainTextAndTruncatesCells(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "a very long model name that should be shortened", Slug: "vendor/model", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "**a long note** that should also be shortened | safely"}}, 120, false)
	if !strings.Contains(output, "Name") || strings.Contains(output, "| Slug") || !strings.Contains(output, "Context") || !strings.Contains(output, "Input $/M") || !strings.Contains(output, "Output $/M") || !strings.Contains(output, "Status") || !strings.Contains(output, "Q/P") || !strings.Contains(output, "Note") {
		t.Fatalf("headers missing from table:\n%s", output)
	}
	assertTableHeaders(t, output, []string{"Name", "Claude", "Status", "Q/P", "Context", "Input $/M", "Output $/M", "Note"})
	if strings.Contains(output, "#") || strings.Contains(output, "|---") || strings.Contains(output, "<table") {
		t.Fatalf("table contains markup:\n%s", output)
	}
	if strings.Contains(output, "**") || strings.Contains(output, "`") {
		t.Fatalf("table contains Markdown emphasis markers:\n%s", output)
	}
	if !strings.Contains(output, "...") {
		t.Errorf("long cells were not truncated:\n%s", output)
	}
}

func TestRenderTableKeepsFullNoteAtAnyRequestedWidth(t *testing.T) {
	note := "full note with enough detail to exceed the preferred column width"
	models := []model.Model{{DisplayName: "model", Note: note}}
	for _, width := range []int{120, 40} {
		output := renderTable(models, width, false)
		if !strings.Contains(output, note) {
			t.Errorf("full note at %d columns is missing or truncated:\n%s", width, output)
		}
		if got := tableColumnWidths(output)[7]; got < tableDisplayWidth(note) {
			t.Errorf("note column width at %d columns = %d, want >= %d", width, got, tableDisplayWidth(note))
		}
	}
}

func TestRenderTableUsesMaximumDisplayWidthForAllNotes(t *testing.T) {
	notes := []string{"short", "е\u0301界🙂", "the longest note is kept in full"}
	models := make([]model.Model, 0, len(notes))
	wantWidth := 0
	for _, note := range notes {
		models = append(models, model.Model{DisplayName: "model", Note: note})
		wantWidth = max(wantWidth, tableDisplayWidth(note))
	}

	output := renderTable(models, 40, false)
	if got := tableColumnWidths(output)[7]; got != wantWidth {
		t.Fatalf("note column width = %d, want maximum display width %d:\n%s", got, wantWidth, output)
	}
	for _, note := range notes {
		if !strings.Contains(output, note) {
			t.Errorf("full note %q is missing or truncated:\n%s", note, output)
		}
	}
}

func TestRenderTableDoesNotExpandEmptyNoteColumn(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "model"}}, 40, false)
	if got := tableColumnWidths(output)[7]; got > 21 {
		t.Fatalf("empty note column width = %d, want <= 21", got)
	}
}

func TestRenderTableUsesSlugAsTheSingleIdentityColumn(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "Display name", Slug: "vendor/a-very-long-model-slug-that-must-be-bounded"}}, 120, true)
	assertTableHeaders(t, output, []string{"Slug", "Claude", "Status", "Q/P", "Context", "Input $/M", "Output $/M", "Note"})
	if !strings.Contains(output, "vendor/a-very") || strings.Contains(output, "Display name") {
		t.Fatalf("slug identity mode output = %s", output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(line, "| vendor/") && tableDisplayWidth(strings.TrimSpace(strings.Split(line, "|")[1])) > maxTableIdentityWidth {
			t.Fatalf("slug identity exceeds %d columns: %q", maxTableIdentityWidth, line)
		}
	}
}

func TestRenderTableKeepsRegularIdentityColumnWidth(t *testing.T) {
	for _, showSlug := range []bool{false, true} {
		output := renderTable([]model.Model{{DisplayName: "Display name", Slug: "vendor/model"}}, 120, showSlug)
		width := firstTableColumnWidth(output)
		if width < 30 || width > maxTableIdentityWidth {
			t.Errorf("identity column width for showSlug=%v = %d, want 30..%d", showSlug, width, maxTableIdentityWidth)
		}
	}
}

func firstTableColumnWidth(output string) int {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "+") {
			separatorEnd := strings.IndexByte(line[1:], '+')
			if separatorEnd >= 0 {
				return separatorEnd - 2
			}
		}
	}
	return 0
}

func assertTableHeaders(t *testing.T, output string, want []string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "| ") || !strings.Contains(line, "| Name ") && !strings.Contains(line, "| Slug ") {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		got := make([]string, 0, len(parts))
		for _, part := range parts {
			got = append(got, strings.TrimSpace(part))
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("table headers = %v, want %v", got, want)
		}
		return
	}
	t.Fatalf("header row not found in output:\n%s", output)
}

func TestRenderTableSeparatesStatusQualityPriceAndNote(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "paid", ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "review this"}, {DisplayName: "free", ScoreLabel: "н/д", QualityPriceLabel: "н/д (цена $0)"}, {DisplayName: "no-score", ScoreLabel: "н/д", QualityPriceLabel: "н/д (оценка не для этого варианта)", Note: notes.NeedsReview}}, 120, false)
	if got := tableColumnWidths(output); got[7] < tableDisplayWidth("review this") {
		t.Fatalf("note column width = %d, want >= %d", got[7], tableDisplayWidth("review this"))
	}
	if !strings.Contains(output, "| 93.0%") || !strings.Contains(output, "| 82.7") || !strings.Contains(output, "| review this") {
		t.Fatalf("paid model cells are not separated:\n%s", output)
	}
	if !strings.Contains(output, "н/...") || strings.Contains(output, "н/д (цена $0)") || strings.Contains(output, "н/д (оценка") {
		t.Fatalf("free/no-score Q/P labels were not safely truncated:\n%s", output)
	}
	if strings.Contains(output, "93.0%; review this") || strings.Contains(output, "н/д;") || strings.Contains(output, notes.NeedsReview) {
		t.Fatalf("status and note were combined or review marker was shown:\n%s", output)
	}
}

func TestRenderTableKeepsQualityPriceWithinFiveColumns(t *testing.T) {
	models := []model.Model{{DisplayName: "paid", ScoreLabel: "93.0%", QualityPriceLabel: "436", Note: "status note"}, {DisplayName: "free", ScoreLabel: "н/д", QualityPriceLabel: "н/д (цена $0)"}, {DisplayName: "missing", ScoreLabel: "н/д", QualityPriceLabel: "н/д (оценка не для этого варианта)"}}
	for _, width := range []int{120, 40} {
		output := renderTable(models, width, false)
		if got := tableColumnWidths(output)[3]; got > 5 {
			t.Errorf("Q/P column width at %d columns = %d, want <= 5:\n%s", width, got, output)
		}
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if !strings.HasPrefix(line, "| ") || strings.Contains(line, "| Name ") {
				continue
			}
			cells := strings.Split(strings.Trim(line, "|"), "|")
			if len(cells) < 4 {
				t.Fatalf("malformed table row at %d columns: %q", width, line)
			}
			if got := tableDisplayWidth(strings.TrimSpace(cells[3])); got > 5 {
				t.Errorf("Q/P cell width at %d columns = %d, want <= 5: %q", width, got, line)
			}
		}
	}
}

func tableColumnWidths(output string) []int {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "+") {
			continue
		}
		parts := strings.Split(strings.Trim(line, "+"), "+")
		widths := make([]int, 0, len(parts))
		for _, part := range parts {
			widths = append(widths, len(part)-2)
		}
		return widths
	}
	return nil
}

func TestRenderTableFitsNarrowWidth(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "a model", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", Note: "a note"}}, 40, false)
	if !strings.Contains(output, "| Name ") || !strings.Contains(output, "| Cla ") || !strings.Contains(output, "| Q/P ") {
		t.Fatalf("minimum table does not preserve required headers:\n%s", output)
	}
}

func TestRenderTableUsesEightCompactColumnWidths(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "model"}}, 40, false)
	want := []int{4, 3, 1, 3, 1, 1, 1, 1}
	if got := tableColumnWidths(output); !reflect.DeepEqual(got, want) {
		t.Fatalf("compact table widths = %v, want %v:\n%s", got, want, output)
	}
	if strings.Count(strings.Split(output, "\n")[1], "|") != 9 {
		t.Fatalf("compact header does not have 8 columns:\n%s", output)
	}
}

func TestRenderTableCompactRowsFitRequestedWidth(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "model", ScoreLabel: "93.0%", QualityPriceLabel: "4.2"}}, 40, false)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if got := tableDisplayWidth(line); got > 40 {
			t.Errorf("compact table line width = %d, want <= 40: %q", got, line)
		}
	}
}

func TestRenderTableFitsNarrowWidthWithCyrillic(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "модель", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "значение", Note: "заметка с длинным текстом"}}, 40, false)
	if !utf8.ValidString(output) {
		t.Fatalf("table output is not valid UTF-8: %q", output)
	}
}

func TestRenderTableShowsManualClaudeEquivalent(t *testing.T) {
	models := []model.Model{
		{DisplayName: "opus", Tier: "opus"},
		{DisplayName: "sonnet", Tier: "sonnet"},
		{DisplayName: "haiku", Tier: "haiku"},
		{DisplayName: "free-ref", Tier: "free", ClaudeRef: "≈ Haiku 4.5 (середина диапазона)"},
		{DisplayName: "free-fallback", Tier: "free"},
		{DisplayName: "unknown", Tier: "other"},
	}
	output := renderTable(models, 40, false)
	for _, want := range []string{"≈ Opus 5", "≈ Sonnet 5", "≈ Haiku 4.5", "≈ Haiku 4.5 (середина диапазона)", "≈ Haiku 4.5 (бесплатная)", "н/д"} {
		if !strings.Contains(output, want) {
			t.Errorf("Claude equivalent %q missing from output:\n%s", want, output)
		}
	}
}

func TestRenderTableKeepsFullClaudeEquivalentAtAnyRequestedWidth(t *testing.T) {
	want := "≈ Haiku 4.5 (уточнение классификации)"
	for _, width := range []int{120, 40} {
		output := renderTable([]model.Model{{DisplayName: "model", Tier: "free", ClaudeRef: want}}, width, false)
		if !strings.Contains(output, want) {
			t.Errorf("full Claude equivalent at %d columns is missing or truncated:\n%s", width, output)
		}
		if got := tableColumnWidths(output)[1]; got < tableDisplayWidth(want) {
			t.Errorf("Claude column width at %d columns = %d, want >= %d", width, got, tableDisplayWidth(want))
		}
	}
}

func TestRenderTableKeepsNormalizedFullClaudeAndNoteAtAnyRequestedWidth(t *testing.T) {
	claudeRef := "**≈ Haiku 4.5** | уточнение\t界🙂"
	note := "**полная** заметка | с control\r\nи Unicode е\u0301界🙂"
	wantClaude := "≈ Haiku 4.5 / уточнение 界🙂"
	wantNote := "полная заметка / с control  и Unicode е\u0301界🙂"
	for _, width := range []int{120, 40} {
		output := renderTable([]model.Model{{DisplayName: "model", Tier: "free", ClaudeRef: claudeRef, Note: note}}, width, false)
		if !strings.Contains(output, wantClaude) || !strings.Contains(output, wantNote) {
			t.Errorf("normalized full fields at %d columns are missing or truncated:\n%s", width, output)
		}
		if got := tableColumnWidths(output); got[1] < tableDisplayWidth(wantClaude) || got[7] < tableDisplayWidth(wantNote) {
			t.Errorf("full field widths at %d columns = %v, want Claude >= %d and Note >= %d", width, got, tableDisplayWidth(wantClaude), tableDisplayWidth(wantNote))
		}
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

func TestTableDisplayWidthHandlesRegionalIndicatorPairs(t *testing.T) {
	if got := tableDisplayWidth("🇺🇸"); got != 2 {
		t.Errorf("tableDisplayWidth flag = %d, want 2", got)
	}
	if got := tableDisplayWidth("🇺"); got != 2 {
		t.Errorf("tableDisplayWidth lone regional indicator = %d, want 2", got)
	}
	if got := truncateTable("🇺🇸model", 2); got != "🇺🇸" {
		t.Errorf("truncateTable flag = %q, want %q", got, "🇺🇸")
	}
	if got := truncateTable("🇺🇸model", 5); got != "🇺🇸..." {
		t.Errorf("truncateTable flag with suffix = %q, want %q", got, "🇺🇸...")
	}
}

func TestRenderTableBoundsRegionalIndicatorIdentity(t *testing.T) {
	modelWithFlag := model.Model{DisplayName: "🇺🇸 model name that is longer than the identity column", Slug: "🇺🇸/model-that-is-longer-than-the-identity-column"}
	for _, showSlug := range []bool{false, true} {
		output := renderTable([]model.Model{modelWithFlag}, 40, showSlug)
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if !strings.HasPrefix(line, "| ") || strings.Contains(line, "| Name ") && showSlug || strings.Contains(line, "| Slug ") && !showSlug {
				continue
			}
			if strings.Contains(line, "🇺") && !strings.Contains(line, "🇺🇸") {
				t.Errorf("identity table split flag cluster: %q", line)
			}
			if strings.Count(line, "|") != 9 {
				t.Errorf("identity table separators = %d, want 9: %q", strings.Count(line, "|"), line)
			}
		}
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
	t.Setenv("COLUMNS", "40")
	output := executeCLI(t, "table", "--config", config)
	if !strings.Contains(output, "Name") || strings.Contains(output, "| Slug") || !strings.Contains(output, "Claude") {
		t.Fatalf("table output = %q", output)
	}
	if got := tableColumnWidths(output); len(got) != 8 {
		t.Fatalf("CLI table has %d columns, want 8:\n%s", len(got), output)
	}
}

func TestLimitTableModelsDefaultsToNoLimit(t *testing.T) {
	models := []model.Model{{Slug: "first"}, {Slug: "second"}}
	if got := limitTableModels(models, -1); !reflect.DeepEqual(got, models) {
		t.Fatalf("no limit = %v, want %v", got, models)
	}
}

func TestLimitTableModelsPicksFirstAfterQualityPriceSort(t *testing.T) {
	models := []model.Model{
		{Slug: "low", Score: &model.ScoreInfo{Value: 1}, Rankable: true, QualityPrice: 1},
		{Slug: "high", Score: &model.ScoreInfo{Value: 3}, Rankable: true, QualityPrice: 3},
	}
	if err := sortTableModels(models, "q/p", false); err != nil {
		t.Fatalf("sort: %v", err)
	}
	got := limitTableModels(models, 1)
	if len(got) != 1 || got[0].Slug != "high" {
		t.Fatalf("limited q/p result = %v, want [high]", got)
	}
}

func TestLimitTableModelsZeroRows(t *testing.T) {
	output := renderTable(limitTableModels([]model.Model{{Slug: "model"}}, 0), 120, false)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(line, "| ") && !strings.Contains(line, "| Name ") {
			t.Fatalf("zero limit rendered a data row: %q", line)
		}
	}
	if !strings.Contains(output, "| Name ") || strings.Count(output, "| Name ") != 1 {
		t.Fatalf("zero limit omitted or duplicated header:\n%s", output)
	}
}

func TestTableLimitRejectsNegativeAndInvalidValues(t *testing.T) {
	for _, value := range []string{"-1", "not-a-number", "9223372036854775808"} {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"table", "--limit", value})
		if err := cmd.Execute(); err == nil {
			t.Errorf("limit %q unexpectedly succeeded", value)
		} else if !strings.Contains(err.Error(), "limit") {
			t.Errorf("limit %q error = %v, want limit context", value, err)
		}
	}
}

func TestTableHelpIncludesSlugAlias(t *testing.T) {
	cmd := newRootCmd()
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"table", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("table help: %v", err)
	}
	if !strings.Contains(output.String(), "-s, --slug") && !strings.Contains(output.String(), "--slug") {
		t.Fatalf("table help does not contain slug alias:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "-n, --limit") || !strings.Contains(output.String(), "after sorting") {
		t.Fatalf("table help does not describe limit:\n%s", output.String())
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

func TestSortTableModelsDefaultsToDescendingQualityPrice(t *testing.T) {
	models := []model.Model{
		{Slug: "missing"},
		{Slug: "low", Score: &model.ScoreInfo{Value: 1}, Rankable: true, QualityPrice: 1},
		{Slug: "high", Score: &model.ScoreInfo{Value: 3}, Rankable: true, QualityPrice: 3},
	}
	if err := sortTableModels(models, "", false); err != nil {
		t.Fatalf("default sort error = %v", err)
	}
	if got := []string{models[0].Slug, models[1].Slug, models[2].Slug}; !reflect.DeepEqual(got, []string{"high", "low", "missing"}) {
		t.Fatalf("default sort = %v, want [high low missing]", got)
	}
	models = []model.Model{
		{Slug: "missing"},
		{Slug: "low", Score: &model.ScoreInfo{Value: 1}, Rankable: true, QualityPrice: 1},
		{Slug: "high", Score: &model.ScoreInfo{Value: 3}, Rankable: true, QualityPrice: 3},
	}
	if err := sortTableModels(models, "", true); err != nil {
		t.Fatalf("reverse default sort error = %v", err)
	}
	if got := []string{models[0].Slug, models[1].Slug, models[2].Slug}; !reflect.DeepEqual(got, []string{"low", "high", "missing"}) {
		t.Fatalf("reverse default sort = %v, want [low high missing]", got)
	}
}

func TestSortTableModelsExplicitSlugRemainsAscending(t *testing.T) {
	models := []model.Model{{Slug: "z"}, {Slug: "a"}}
	if err := sortTableModels(models, "slug", false); err != nil || models[0].Slug != "a" {
		t.Fatalf("explicit slug sort = %+v, err=%v", models, err)
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
		{key: "q/p", want: "z/model"},
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

func TestTablePagerBoundsIdentityFields(t *testing.T) {
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
	if strings.Contains(paged.String(), models[0].DisplayName) || strings.Contains(paged.String(), models[1].DisplayName) {
		t.Fatalf("pager preserved unbounded identity fields:\n%s", paged.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(paged.String()), "\n") {
		if strings.HasPrefix(line, "| ") {
			identity := strings.TrimSpace(strings.Split(line, "|")[1])
			if tableDisplayWidth(identity) > maxTableIdentityWidth {
				t.Fatalf("pager identity exceeds %d columns: %q", maxTableIdentityWidth, line)
			}
		}
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
	separatorColumns := tableSeparatorColumns(rowSeparators[0])
	for _, row := range rowSeparators[1:] {
		if got := tableSeparatorColumns(row); !reflect.DeepEqual(got, separatorColumns) {
			t.Fatalf("column separators moved from %v to %v:\n%s", separatorColumns, got, paged.String())
		}
	}
	if stdout.String() != "pager stdout" || stderr.String() != "pager stderr" {
		t.Fatalf("pager writers: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func tableSeparatorColumns(line string) []int {
	columns := make([]int, 0)
	for index := range line {
		if line[index] == '|' {
			columns = append(columns, tableDisplayWidth(line[:index]))
		}
	}
	return columns
}

func TestNonPagerTableBoundsIdentityFields(t *testing.T) {
	rowModel := model.Model{DisplayName: "A model name longer than the preferred column", Slug: "vendor/a-model-slug-longer-than-the-column"}
	output := renderTable([]model.Model{rowModel}, 40, false)
	if strings.Contains(output, rowModel.DisplayName) || strings.Contains(output, rowModel.Slug) {
		t.Fatalf("non-pager table did not truncate identity fields:\n%s", output)
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

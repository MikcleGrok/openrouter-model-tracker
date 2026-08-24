package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/tui/screen/output"
)

func TestTUIRuntimeViewGridAcrossDetailNavigationAndReopen(t *testing.T) {
	rows := []model.Model{
		{Slug: "first/model", DisplayName: "First model", Provider: "First", License: "Apache-2.0", Tier: "sonnet", ClaudeRef: "≈ Sonnet", TaskFit: []string{"implement", "audit"}, Context: 128000, InPerM: 0.5, OutPerM: 2, OpenWeights: "yes", CanonicalSlug: "first/model", MetadataSourceURL: "https://meta.example/first", Description: strings.Repeat("first long description ", 12), Note: "first note"},
		{Slug: "second/model", DisplayName: "Second model", Provider: "Second", License: "MIT", Tier: "haiku", ClaudeRef: "≈ Haiku", TaskFit: []string{"review"}, Context: 32768, InPerM: 0.1, OutPerM: 0.4, OpenWeights: "no", CanonicalSlug: "second/model", MetadataSourceURL: "https://meta.example/second", Description: "short description", Note: "short note", Score: &model.ScoreInfo{Value: 91.2, Metric: "SWE-bench Verified", Unit: "%", VariantMeasured: "second/model", SourceURL: "https://bench.example/second", Checked: "2026-08-20"}},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.sortKey = "name"
	m.rebuild()
	m = runtimeTUIUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	assertRuntimeDetailState(t, m, 0)
	expectedLast := runtimeExpectedLastOffset(runtimeExpectedDetailLines(rows[1]), m.width, m.height)
	for _, step := range []struct {
		key    string
		want   int
		marker string
	}{
		{"j", 1, "License:"},
		{"j", 2, "Tier:"},
		{"k", 1, "License:"},
		{"G", expectedLast, "Fit and notes"},
		{"g", 0, "Identity"},
	} {
		m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(step.key)})
		assertRuntimeDetailState(t, m, step.want)
		rows := assertRuntimeGrid(t, m.View(), m.width, m.height)
		selected, ok := m.detailRow()
		if !ok {
			t.Fatalf("after %q detail row disappeared", step.key)
		}
		expectedLines := runtimeExpectedDetailLines(selected)
		assertRuntimeExpectedPage(t, rows, expectedLines, m.detailOffset, m.width, m.height, step.key)
		if !containsPhysicalRow(rows, step.marker) {
			t.Fatalf("after %q no physical sentinel %q at offset %d: %#v", step.key, step.marker, m.detailOffset, rows)
		}
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.detailOffset != 1 {
		t.Fatalf("boundary below g: offset=%d, want 1", m.detailOffset)
	}
	m.detailOffset = expectedLast
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.detailOffset != expectedLast {
		t.Fatalf("down boundary: offset=%d, want %d", m.detailOffset, expectedLast)
	}
	detailRows := assertRuntimeGrid(t, m.View(), 80, 20)
	if len(detailRows) != 20 || m.width != 80 || m.height != 20 {
		t.Fatalf("initial frame dimensions: %dx%d rows=%d", m.width, m.height, len(detailRows))
	}
	m = runtimeTUIUpdate(t, m, tea.WindowSizeMsg{Width: 44, Height: 12})
	assertRuntimeDetailState(t, m, runtimeExpectedClampedOffset(expectedLast, runtimeExpectedDetailLines(rows[1]), m.width, m.height))
	detailRows = assertRuntimeGrid(t, m.View(), 44, 12)
	if m.width != 44 || m.height != 12 || len(detailRows) != 12 {
		t.Fatalf("shrunk frame dimensions: %dx%d rows=%d", m.width, m.height, len(detailRows))
	}
	if containsPhysicalRow(detailRows, "first long description first long description") {
		t.Fatalf("shrunk frame retained a stale wide row: %#v", detailRows)
	}
	m = runtimeTUIUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	assertRuntimeDetailState(t, m, runtimeExpectedClampedOffset(expectedLast, runtimeExpectedDetailLines(rows[1]), m.width, m.height))
	detailRows = assertRuntimeGrid(t, m.View(), 80, 20)
	if m.width != 80 || m.height != 20 || len(detailRows) != 20 {
		t.Fatalf("restored frame dimensions: %dx%d rows=%d", m.width, m.height, len(detailRows))
	}
	if containsPhysicalRow(detailRows, "Detail ") && containsPhysicalRow(detailRows, "first long description first long description") {
		t.Fatalf("restored frame mixed stale and current rows: %#v", detailRows)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEscape})
	if m.overlay != "" || m.detailOffset != 0 {
		t.Fatalf("Esc did not restore list state: overlay=%q offset=%d", m.overlay, m.detailOffset)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.cursor != 1 || m.selectedSlug != rows[1].Slug {
		t.Fatalf("list navigation after Esc: cursor=%d selected=%q", m.cursor, m.selectedSlug)
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.overlay != "detail" || m.detailOffset != 0 || m.detailRowSlug() != rows[1].Slug {
		t.Fatalf("reopen state: overlay=%q offset=%d slug=%q", m.overlay, m.detailOffset, m.detailRowSlug())
	}
	actual := assertRuntimeGrid(t, m.View(), m.width, m.height)
	expected := runtimeExpectedDetailFrameFromLines(runtimeExpectedDetailLines(rows[1]), m.width, m.height, m.detailOffset)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("reopened second model frame mismatch:\nactual:\n%s\nexpected:\n%s", strings.Join(actual, "\n"), strings.Join(expected, "\n"))
	}
	m = runtimeTUIUpdate(t, m, tea.KeyMsg{Type: tea.KeyEscape})
	if m.overlay != "" {
		t.Fatalf("second Esc left overlay=%q", m.overlay)
	}
}

func TestTUIRuntimePayloadPreservationAndTerminalSanitization(t *testing.T) {
	row := model.Model{Slug: "payload/model", DisplayName: "payload model", Owner: "OWNER_UNIQUE", Provider: "PROVIDER_UNIQUE_LONG", License: "LICENSE_UNIQUE_LONG", Tier: "TIER_UNIQUE_LONG", ClaudeRef: "REFERENCE_UNIQUE_LONG", TaskFit: []string{"TASK_UNIQUE_A", "TASK_UNIQUE_B"}, Context: 131072, InPerM: 1.25, OutPerM: 8.5, OpenWeights: "OPEN_WEIGHT_UNIQUE", CanonicalSlug: "CANONICAL_SLUG_UNIQUE", HuggingFaceID: "HUGGINGFACE_ID_UNIQUE", MetadataSourceURL: "https://source.example/UNIQUE_URL", ModelURL: "https://model.example/UNIQUE_URL", Created: 1785542400, Description: "DESCRIPTION_UNIQUE_LONG\\n" + strings.Repeat("wrapped ", 8), Note: "NOTE_UNIQUE_LONG"}
	row.HasLongContextOverride, row.LongContextOverrideInPerM, row.LongContextOverrideOutPerM, row.LongContextOverrideMinTokens = true, 2.5, 12.5, 256000
	row.LongContextPriceLabel, row.LongContextInLabel, row.LongContextOutLabel = "FROZEN_COMBINED_SENTINEL", "FROZEN_INPUT_SENTINEL", "FROZEN_OUTPUT_SENTINEL"
	row.Score = &model.ScoreInfo{Value: 91.2, Metric: "BENCHMARK_METRIC_UNIQUE", Unit: "BENCHMARK_UNIT_UNIQUE", SourceFamily: "swebench", ConfiguredIdentity: "CONFIGURED_IDENTITY_UNIQUE", IdentityAmbiguous: true, VariantMeasured: "BENCHMARK_VARIANT_UNIQUE", SourceURL: "https://benchmark.example/UNIQUE", Checked: "BENCHMARK_DATE_UNIQUE", IdentityStatus: "IDENTITY_STATUS_UNIQUE", Provenance: "PROVENANCE_UNIQUE", Stale: true, CanonicalID: "CANONICAL_ID_UNIQUE", ReleaseVariant: "RELEASE_VARIANT_UNIQUE", ModelVariant: "MODEL_VARIANT_UNIQUE", Reasoning: "REASONING_UNIQUE", Configuration: "CONFIGURATION_UNIQUE", Provider: "BENCHMARK_PROVIDER_UNIQUE", Uncertainty: "UNCERTAINTY_UNIQUE", SampleSize: "SAMPLE_SIZE_UNIQUE", Harness: "HARNESS_UNIQUE", Scaffold: "SCAFFOLD_UNIQUE"}
	row.ArenaScore = &model.ScoreInfo{Value: 1201, Metric: "ARENA_METRIC_UNIQUE", Unit: "ARENA_UNIT_UNIQUE", SourceFamily: "arena", ConfiguredIdentity: "ARENA_CONFIGURED_IDENTITY_UNIQUE", IdentityAmbiguous: true, VariantMeasured: "ARENA_VARIANT_UNIQUE", SourceURL: "https://arena.example/UNIQUE", Checked: "ARENA_DATE_UNIQUE", IdentityStatus: "ARENA_IDENTITY_STATUS_UNIQUE", Provenance: "ARENA_PROVENANCE_UNIQUE", CanonicalID: "ARENA_CANONICAL_ID_UNIQUE", ReleaseVariant: "ARENA_RELEASE_VARIANT_UNIQUE", ModelVariant: "ARENA_MODEL_VARIANT_UNIQUE", Reasoning: "ARENA_REASONING_UNIQUE", Configuration: "ARENA_CONFIGURATION_UNIQUE", Provider: "ARENA_PROVIDER_UNIQUE", Uncertainty: "ARENA_UNCERTAINTY_UNIQUE", SampleSize: "ARENA_SAMPLE_SIZE_UNIQUE", Harness: "ARENA_HARNESS_UNIQUE", Scaffold: "ARENA_SCAFFOLD_UNIQUE"}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.overlay, m.visible, m.cursor = "detail", []model.Model{row}, 0
	m.priceHistory = &pricehistory.History{Observations: []pricehistory.Observation{{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 1, OutPerM: 2, Context: row.Context}}}, {ObservedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 2, OutPerM: 4, Context: row.Context}}}, {ObservedAt: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 3, OutPerM: 6, Context: row.Context}}}, {ObservedAt: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), Prices: map[string]pricehistory.Price{row.Slug: {Found: true, InPerM: 4, OutPerM: 8, Context: row.Context}}}}}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	assertRuntimePayloadFixture(t, row)
	for _, viewport := range []struct{ width, height int }{{80, 18}, {44, 12}, {7, 7}, {3, 5}} {
		m.width, m.height, m.detailOffset = viewport.width, viewport.height, 0
		actual := assertRuntimeGrid(t, m.View(), m.width, m.height)
		expected := runtimeExpectedDetailFrame(row, m.width, m.height, m.detailOffset, now)
		if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
			t.Fatalf("payload frame mismatch at %dx%d expectedTotal=%d:\nactual:\n%s\nexpected:\n%s", m.width, m.height, len(runtimeExpectedPhysicalRows(runtimeExpectedPayloadLines(row, now), m.width)), strings.Join(actual, "\n"), strings.Join(expected, "\n"))
		}
	}
}

func TestTUIRuntimeNarrowProductionViewHasNoOOB(t *testing.T) {
	row := model.Model{Slug: "narrow/model", DisplayName: "Narrow", Provider: "Narrow provider", License: "MIT", Tier: "sonnet", ClaudeRef: "reference", Context: 128000, InPerM: 1, OutPerM: 2, OpenWeights: "no", CanonicalSlug: "narrow/model", Description: strings.Repeat("narrow payload ", 20), Note: strings.Repeat("note ", 10)}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, []model.Model{row})
	m.visible, m.cursor, m.overlay, m.width, m.height = []model.Model{row}, 0, "detail", 7, 5
	for _, size := range []struct{ width, height int }{{7, 5}, {3, 4}, {1, 1}} {
		m.width, m.height = size.width, size.height
		if _, err := runtimeGrid(m.View(), m.width, m.height); err != nil {
			t.Fatalf("production View OOB at %dx%d: %v", m.width, m.height, err)
		}
	}
}

func TestRuntimeTerminalEmulatorCSIAndDuplicateDetection(t *testing.T) {
	rows, err := runtimeGrid("abcde\x1b[2;3H\x1b[K\x1b[1;1H\x1b[KXYZ\x1b[2J", 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0] != "     " || rows[1] != "     " || rows[2] != "     " {
		t.Fatalf("CSI J/K did not erase the expected grid: %#v", rows)
	}
	rows, err = runtimeGrid("abc\x1b[2;2H\x1b[K", 5, 2)
	if err != nil || rows[0] != "abc  " || rows[1] != "     " {
		t.Fatalf("CSI K erase-to-end or cursor move failed: rows=%#v err=%v", rows, err)
	}
	if _, err := runtimeGrid("ab\x1b[1;1Hcd", 4, 2); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("cursor overwrite was not rejected: %v", err)
	}
}

func TestTUIRuntimeInjectedCursorOverwriteFails(t *testing.T) {
	if _, err := runtimeGrid("ab\x1b[1;1Hcd", 4, 2); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("overwrite fixture was not rejected: %v", err)
	}
}

func runtimeTUIUpdate(t *testing.T, m tuiModel, msg tea.Msg) tuiModel {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(tuiModel)
}

func assertRuntimeDetailState(t *testing.T, m tuiModel, wantOffset int) {
	t.Helper()
	if m.overlay != "detail" || m.detailOffset != wantOffset {
		t.Fatalf("detail state: overlay=%q offset=%d want=%d", m.overlay, m.detailOffset, wantOffset)
	}
	assertRuntimeGrid(t, m.View(), m.width, m.height)
	frame := actualRuntimeDetailFrame(m)
	if frame.Offset != wantOffset || frame.FooterLine < 0 || len(frame.Lines) != m.height {
		t.Fatalf("production detail frame: offset=%d footer=%d lines=%d", frame.Offset, frame.FooterLine, len(frame.Lines))
	}
}

func actualRuntimeDetailFrame(m tuiModel) output.DetailFrame {
	row, _ := m.detailRow()
	lines := m.detailLines(row)
	return output.Detail(output.DetailData{Width: m.width, Height: m.height, Offset: m.detailOffset, Lines: lines, Regions: output.RegionsFromLines(lines), FooterFunc: func(offset, end, total int) string {
		return fmt.Sprintf("Detail %d-%d/%d · ↑↓ scroll · Esc close", offset+1, end, total)
	}})
}

func assertRuntimeExpectedPage(t *testing.T, actual []string, logical []string, offset, width, height int, action string) {
	t.Helper()
	physical := runtimeExpectedPhysicalRows(logical, width)
	bodyHeight := max(1, height-2)
	maxOffset := max(0, len(physical)-bodyHeight)
	pageOffset := min(max(0, offset), maxOffset)
	end := min(len(physical), pageOffset+bodyHeight)
	expected := append([]string(nil), physical[pageOffset:end]...)
	if len(expected) > 0 {
		expected = append(expected, "")
	}
	expected = append(expected, fmt.Sprintf("Detail %d-%d/%d · ↑↓ scroll · Esc close", pageOffset+1, end, len(physical)))
	for len(expected) < height {
		expected = append(expected, "")
	}
	if len(expected) > height {
		expected = expected[:height]
	}
	for i := range expected {
		expected[i] = runtimeVisibleRow(expected[i], width)
	}
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("page mismatch after %q offset=%d:\nactual:\n%s\nexpected:\n%s", action, pageOffset, strings.Join(actual, "\n"), strings.Join(expected, "\n"))
	}
}

func runtimeVisibleRow(value string, width int) string {
	value = runtimeOraclePlain(value)
	value = ansi.Truncate(value, width, "")
	var cells strings.Builder
	used := 0
	for _, r := range value {
		cellWidth := ansi.StringWidth(string(r))
		for i := 0; i < cellWidth; i++ {
			cells.WriteRune(r)
		}
		used += cellWidth
	}
	return cells.String() + strings.Repeat(" ", max(0, width-used))
}

func runtimeExpectedDetailLines(row model.Model) []string {
	license := row.License
	if license == "" {
		license = row.OpenWeights
	}
	canonical := row.CanonicalSlug
	if canonical == "" {
		canonical = row.Slug
	}
	lines := []string{row.DisplayName + " (" + row.Slug + ")", "", "-- Identity --", "Manufacturer: ❔ " + row.Provider, "Provider: " + row.Provider, "License: " + license, "Tier: " + row.Tier, "Claude reference: " + row.ClaudeRef, "Task fit: " + strings.Join(row.TaskFit, " + "), "", "-- Pricing --", fmt.Sprintf("Context: %dK tokens", (row.Context+500)/1000), fmt.Sprintf("Input: $%.2f per M tokens", row.InPerM), fmt.Sprintf("Output: $%.2f per M tokens", row.OutPerM), "Open weights: " + row.OpenWeights, "", "-- Benchmarks --", "SWE-bench Verified score (percent):"}
	if row.Score == nil {
		lines = append(lines, "  Value: n/a", "  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=n/a")
	} else {
		lines = append(lines, "  Value: n/a", "  Variant measured: "+row.Score.VariantMeasured, "  Metric: "+row.Score.Metric, "  Unit: "+row.Score.Unit, "  Identity status: n/a", "  Source: "+row.Score.SourceURL, "  Checked: "+row.Score.Checked, "  Provenance: raw=91.2; metric="+row.Score.Metric+"; unit="+row.Score.Unit+"; variant="+row.Score.VariantMeasured+"; identity=n/a; checked="+row.Score.Checked+"; source="+row.Score.SourceURL+"; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=n/a")
	}
	return append(lines, "", "LMArena score (Elo rating):", "  Value: n/a", "  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=n/a", "", "-- Provenance and metadata --", "Release date: n/a", "OpenRouter page: https://openrouter.ai/"+canonical, "Metadata source: "+row.MetadataSourceURL, "Description:", "  "+row.Description, "", "-- Fit and notes --", "Note:", "  "+row.Note)
}

func runtimeExpectedDetailFrame(row model.Model, width, height, offset int, now time.Time) []string {
	return runtimeExpectedDetailFrameFromLines(runtimeExpectedPayloadLines(row, now), width, height, offset)
}

func runtimeExpectedDetailFrameFromLines(lines []string, width, height, offset int) []string {
	physical := runtimeExpectedPhysicalRows(lines, width)
	bodyHeight := max(1, height-2)
	pageOffset := min(max(0, offset), max(0, len(physical)-bodyHeight))
	end := min(len(physical), pageOffset+bodyHeight)
	rows := append([]string(nil), physical[pageOffset:end]...)
	if len(rows) > 0 {
		rows = append(rows, "")
	}
	rows = append(rows, fmt.Sprintf("Detail %d-%d/%d · ↑↓ scroll · Esc close", pageOffset+1, end, len(physical)))
	for len(rows) < height {
		rows = append(rows, "")
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	for i := range rows {
		rows[i] = runtimeVisibleRow(rows[i], width)
	}
	return rows
}

func runtimeExpectedPayloadLines(row model.Model, now time.Time) []string {
	date := time.Unix(row.Created, 0).UTC().Format("2006-01-02")
	age := runtimeExpectedAge(time.Unix(row.Created, 0).UTC(), now)
	return []string{
		row.DisplayName + " (" + row.Slug + ")", "", "-- Identity --",
		"Manufacturer: ❔ PROVIDER_UNIQUE_LONG", "Provider: " + row.Provider, "License: " + row.License, "Tier: " + row.Tier, "Claude reference: " + row.ClaudeRef, "Task fit: " + strings.Join(row.TaskFit, " + "),
		"", "-- Pricing --", fmt.Sprintf("Context: %dK tokens", (row.Context+500)/1000), "Input: $1.25 per M tokens", "Output: $8.50 per M tokens", "Long context: $2.50 / $12.50 from 256K+", "  input: $2.50 from 256K+", "  output: $12.50 from 256K+", "Price history:", "  2026-08-02: $1/$2, 131K -> $2/$4, 131K", "  2026-08-03: $2/$4, 131K -> $3/$6, 131K", "  2026-08-04: $3/$6, 131K -> $4/$8, 131K", "Open weights: " + row.OpenWeights,
		"", "-- Benchmarks --", "SWE-bench Verified score (percent):", "  Value: n/a", "  Stale: value taken from a previous snapshot", "  Variant measured: BENCHMARK_VARIANT_UNIQUE", "  Metric: BENCHMARK_METRIC_UNIQUE", "  Unit: BENCHMARK_UNIT_UNIQUE", "  Identity status: IDENTITY_STATUS_UNIQUE", "  Source: https://benchmark.example/UNIQUE", "  Checked: BENCHMARK_DATE_UNIQUE", "  Provenance: raw=91.2; metric=BENCHMARK_METRIC_UNIQUE; unit=BENCHMARK_UNIT_UNIQUE; variant=BENCHMARK_VARIANT_UNIQUE; identity=IDENTITY_STATUS_UNIQUE; checked=BENCHMARK_DATE_UNIQUE; source=https://benchmark.example/UNIQUE; uncertainty=UNCERTAINTY_UNIQUE; sample=SAMPLE_SIZE_UNIQUE; harness=HARNESS_UNIQUE; scaffold=SCAFFOLD_UNIQUE; provider=BENCHMARK_PROVIDER_UNIQUE; configuration=CONFIGURATION_UNIQUE; configured_identity=CONFIGURED_IDENTITY_UNIQUE; canonical_id=CANONICAL_ID_UNIQUE; release_variant=RELEASE_VARIANT_UNIQUE; model_variant=MODEL_VARIANT_UNIQUE; reasoning=REASONING_UNIQUE; provenance=PROVENANCE_UNIQUE", "", "LMArena score (Elo rating):", "  Value: n/a", "  Variant measured: ARENA_VARIANT_UNIQUE", "  Metric: ARENA_METRIC_UNIQUE", "  Unit: ARENA_UNIT_UNIQUE", "  Identity status: ARENA_IDENTITY_STATUS_UNIQUE", "  Source: https://arena.example/UNIQUE", "  Checked: ARENA_DATE_UNIQUE", "  Provenance: raw=1201; metric=ARENA_METRIC_UNIQUE; unit=ARENA_UNIT_UNIQUE; variant=ARENA_VARIANT_UNIQUE; identity=ARENA_IDENTITY_STATUS_UNIQUE; checked=ARENA_DATE_UNIQUE; source=https://arena.example/UNIQUE; uncertainty=ARENA_UNCERTAINTY_UNIQUE; sample=ARENA_SAMPLE_SIZE_UNIQUE; harness=ARENA_HARNESS_UNIQUE; scaffold=ARENA_SCAFFOLD_UNIQUE; provider=ARENA_PROVIDER_UNIQUE; configuration=ARENA_CONFIGURATION_UNIQUE; configured_identity=ARENA_CONFIGURED_IDENTITY_UNIQUE; canonical_id=ARENA_CANONICAL_ID_UNIQUE; release_variant=ARENA_RELEASE_VARIANT_UNIQUE; model_variant=ARENA_MODEL_VARIANT_UNIQUE; reasoning=ARENA_REASONING_UNIQUE; provenance=ARENA_PROVENANCE_UNIQUE",
		"", "-- Provenance and metadata --", fmt.Sprintf("Release date: %s (%s); catalogue entry creation date, release date unknown", date, age), "OpenRouter page: https://openrouter.ai/" + row.CanonicalSlug, "Model page: " + row.ModelURL, "Metadata source: " + row.MetadataSourceURL, "HuggingFace repository: https://huggingface.co/" + row.HuggingFaceID, "Description:", "  " + strings.TrimSpace(row.Description), "", "-- Fit and notes --", "Note:", "  " + strings.TrimSpace(row.Note),
	}
}

// Manufacturer, score blocks and history are derived while Model becomes a
// DetailDTO; they do not exist as corresponding Model fields. Keep their
// expected values as an independent fixture so this oracle checks the DTO
// projection instead of reproducing it through production helpers.
func assertRuntimePayloadFixture(t *testing.T, row model.Model) {
	t.Helper()
	if row.LongContextPriceLabel != "FROZEN_COMBINED_SENTINEL" || row.LongContextInLabel != "FROZEN_INPUT_SENTINEL" || row.LongContextOutLabel != "FROZEN_OUTPUT_SENTINEL" {
		t.Fatalf("long-context model fixture changed: combined=%q input=%q output=%q", row.LongContextPriceLabel, row.LongContextInLabel, row.LongContextOutLabel)
	}
}

func runtimeExpectedAge(published time.Time, now time.Time) string {
	days := int(now.UTC().Sub(published).Hours() / 24)
	if days < 0 {
		return "future date"
	}
	if days == 0 {
		return "today"
	}
	if days < 31 {
		return fmt.Sprintf("%d day%s ago", days, map[bool]string{true: "", false: "s"}[days == 1])
	}
	if days < 365 {
		months := days / 30
		return fmt.Sprintf("%d month%s ago", months, map[bool]string{true: "", false: "s"}[months == 1])
	}
	years := days / 365
	return fmt.Sprintf("%d year%s ago", years, map[bool]string{true: "", false: "s"}[years == 1])
}

func assertRuntimeGrid(t *testing.T, stream string, width, height int) []string {
	t.Helper()
	rows, err := runtimeGrid(stream, width, height)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func runtimeGrid(stream string, width, height int) ([]string, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid terminal size %dx%d", width, height)
	}
	e := runtimeTerminal{cells: make([][]rune, height), writes: make([][]bool, height), width: width, height: height}
	for y := range e.cells {
		e.cells[y] = []rune(strings.Repeat(" ", width))
		e.writes[y] = make([]bool, width)
	}
	for i := 0; i < len(stream); {
		if stream[i] == '\x1b' {
			next, err := e.escape(stream, i)
			if err != nil {
				return nil, err
			}
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(stream[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("invalid UTF-8 at byte %d", i)
		}
		switch r {
		case '\n':
			e.y++
			e.x = 0
		case '\r':
			e.x = 0
		case '\t':
			e.x = min(e.width, (e.x/8+1)*8)
		default:
			w := ansi.StringWidth(string(r))
			if w > 0 {
				if err := e.write(r, w); err != nil {
					return nil, err
				}
			}
		}
		i += size
	}
	if e.osc8Open {
		return nil, fmt.Errorf("unbalanced OSC8 hyperlink")
	}
	if e.y < 0 || e.y >= height || e.x < 0 || e.x > width {
		return nil, fmt.Errorf("final cursor (%d,%d) outside %dx%d", e.x, e.y, width, height)
	}
	rows := make([]string, height)
	for i := range e.cells {
		rows[i] = string(e.cells[i])
	}
	return rows, nil
}

type runtimeTerminal struct {
	cells         [][]rune
	writes        [][]bool
	width, height int
	x, y          int
	osc8Open      bool
}

func (e *runtimeTerminal) write(r rune, width int) error {
	if e.y < 0 || e.y >= e.height || e.x < 0 || e.x+width > e.width {
		return fmt.Errorf("write (%d,%d) width %d outside %dx%d", e.x, e.y, width, e.width, e.height)
	}
	for offset := 0; offset < width; offset++ {
		if e.writes[e.y][e.x+offset] {
			return fmt.Errorf("duplicate write at (%d,%d)", e.x+offset, e.y)
		}
		e.writes[e.y][e.x+offset] = true
		e.cells[e.y][e.x+offset] = r
	}
	e.x += width
	return nil
}

func (e *runtimeTerminal) escape(stream string, start int) (int, error) {
	if start+1 >= len(stream) {
		return len(stream), fmt.Errorf("incomplete escape at byte %d", start)
	}
	if stream[start+1] == ']' {
		i := start + 2
		for i < len(stream) && stream[i] != '\a' && !(stream[i] == '\x1b' && i+1 < len(stream) && stream[i+1] == '\\') {
			i++
		}
		if i >= len(stream) {
			return len(stream), fmt.Errorf("incomplete OSC at byte %d", start)
		}
		payload := stream[start+2 : i]
		if strings.HasPrefix(payload, "8;") {
			e.osc8Open = !strings.HasPrefix(payload, "8;;")
		}
		if stream[i] == '\a' {
			return i + 1, nil
		}
		return i + 2, nil
	}
	if stream[start+1] != '[' {
		return min(len(stream), start+2), nil
	}
	i := start + 2
	for i < len(stream) && (stream[i] < 0x40 || stream[i] > 0x7e) {
		i++
	}
	if i >= len(stream) {
		return len(stream), fmt.Errorf("incomplete CSI at byte %d", start)
	}
	command := stream[i]
	params := parseRuntimeCSIParams(stream[start+2 : i])
	if err := e.csi(command, params); err != nil {
		return i + 1, err
	}
	return i + 1, nil
}

func parseRuntimeCSIParams(raw string) []int {
	parts := strings.Split(strings.TrimPrefix(raw, "?"), ";")
	params := make([]int, len(parts))
	for i, part := range parts {
		if part == "" {
			params[i] = 0
			continue
		}
		for _, r := range part {
			if !unicode.IsDigit(r) {
				params[i] = 1
				break
			}
			params[i] = params[i]*10 + int(r-'0')
		}
	}
	return params
}

func (e *runtimeTerminal) csi(command byte, p []int) error {
	n := func(i int) int {
		if i < len(p) {
			return max(1, p[i])
		}
		return 1
	}
	mode := 0
	if len(p) > 0 {
		mode = p[0]
	}
	switch command {
	case 'A':
		e.y -= n(0)
	case 'B':
		e.y += n(0)
	case 'C':
		e.x += n(0)
	case 'D':
		e.x -= n(0)
	case 'G':
		e.x = n(0) - 1
	case 'd':
		e.y = n(0) - 1
	case 'H', 'f':
		e.y, e.x = n(0)-1, n(1)-1
	case 'J':
		e.eraseDisplay(mode)
	case 'K':
		e.eraseLine(mode)
	case 'm':
	default:
		return fmt.Errorf("unsupported CSI %q", command)
	}
	if e.x < 0 || e.x > e.width || e.y < 0 || e.y >= e.height {
		return fmt.Errorf("cursor (%d,%d) outside %dx%d after CSI %q", e.x, e.y, e.width, e.height, command)
	}
	return nil
}

func (e *runtimeTerminal) eraseDisplay(mode int) {
	for y := range e.cells {
		for x := range e.cells[y] {
			if mode == 2 || mode == 0 && (y > e.y || y == e.y && x >= e.x) || mode == 1 && (y < e.y || y == e.y && x <= e.x) {
				e.cells[y][x] = ' '
				e.writes[y][x] = false
			}
		}
	}
}

func (e *runtimeTerminal) eraseLine(mode int) {
	start, end := 0, e.width
	if mode == 0 {
		start = e.x
	}
	if mode == 1 {
		end = e.x + 1
	}
	for x := start; x < end && x < e.width; x++ {
		e.cells[e.y][x] = ' '
		e.writes[e.y][x] = false
	}
}

func containsPhysicalRow(rows []string, marker string) bool {
	for _, row := range rows {
		if strings.Contains(row, marker) {
			return true
		}
	}
	return false
}

func runtimeExpectedLastOffset(lines []string, width, height int) int {
	physical := runtimeExpectedPhysicalRows(lines, width)
	return max(0, len(physical)-max(1, height-2))
}

func runtimeExpectedClampedOffset(previous int, lines []string, width, height int) int {
	return min(previous, runtimeExpectedLastOffset(lines, width, height))
}

func runtimeExpectedPhysicalRows(lines []string, width int) []string {
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, paragraph := range strings.Split(strings.ReplaceAll(line, `\n`, "\n"), "\n") {
			paragraph = runtimeOraclePlain(paragraph)
			if paragraph == "" {
				rows = append(rows, "")
				continue
			}
			indent := ""
			if strings.HasPrefix(paragraph, "  ") {
				indent = "  "
			}
			words := strings.Fields(strings.TrimLeft(paragraph, " "))
			current := indent
			for _, word := range words {
				if ansi.StringWidth(word) > max(1, width-ansi.StringWidth(indent)) {
					if current != indent {
						rows = append(rows, current)
						current = indent
					}
					chunks := runtimeSplitCellWord(word, max(1, width-ansi.StringWidth(indent)))
					for _, chunk := range chunks[:len(chunks)-1] {
						rows = append(rows, indent+chunk)
					}
					word = chunks[len(chunks)-1]
				}
				candidate := indent + word
				if current != indent {
					candidate = current + " " + word
				}
				if ansi.StringWidth(candidate) > width && current != indent {
					rows = append(rows, current)
					current = indent + word
				} else {
					current = candidate
				}
			}
			rows = append(rows, current)
		}
	}
	return rows
}

func runtimeSplitCellWord(word string, width int) []string {
	chunks := []string{""}
	used := 0
	for _, r := range word {
		cellWidth := ansi.StringWidth(string(r))
		if used > 0 && used+cellWidth > width {
			chunks = append(chunks, "")
			used = 0
		}
		chunks[len(chunks)-1] += string(r)
		used += cellWidth
	}
	return chunks
}

func runtimeOraclePlain(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			out.WriteByte(' ')
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func (m tuiModel) detailRowSlug() string {
	row, ok := m.detailRow()
	if !ok {
		return ""
	}
	return row.Slug
}

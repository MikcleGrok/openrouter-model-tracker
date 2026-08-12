package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/spf13/pflag"
)

func TestRenderTableUsesPlainTextAndTruncatesCells(t *testing.T) {
	output := renderTable([]model.Model{{DisplayName: "a very long model name that should be shortened", Slug: "vendor/model", Context: 128000, InPerM: 1.25, OutPerM: 2.5, ScoreLabel: "93.0%", QualityPriceLabel: "82.7", Note: "**a long note** that should also be shortened | safely"}}, 120, false)
	if !strings.Contains(output, "Name") || strings.Contains(output, "| Slug") || !strings.Contains(output, "SWE %") || !strings.Contains(output, "In $/M") || !strings.Contains(output, "Out $/M") || !strings.Contains(output, "Note") {
		t.Fatalf("headers missing from table:\n%s", output)
	}
	wide := renderTable([]model.Model{{DisplayName: "model"}}, 220, false)
	assertTableHeaders(t, wide, []string{"Name", "Claude", "SWE %", "Q/P score/$M", "Context tok", "In $/M", "Out $/M", "Note"})
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

func TestManufacturerBadgeMappingNormalizesCaseAndWhitespace(t *testing.T) {
	for _, test := range []struct{ name, badge string }{
		{" OpenAI  Labs ", "🌀"}, {"ANTHROPIC", "🔶"}, {"Google DeepMind", "🌐"},
		{"Meta AI", "Ⓜ️"}, {"DeepSeek", "🐋"}, {"Qwen", "🌸"}, {"Mistral AI", "🌪️"},
		{"xAI", "🚀"}, {"  ", "❔"}, {"Unknown vendor", "❔"},
	} {
		if got := manufacturerBadge(test.name); got != test.badge {
			t.Errorf("manufacturerBadge(%q) = %q, want %q", test.name, got, test.badge)
		}
	}
}

func TestManufacturerBadgeIconsAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, name := range []string{"OpenAI", "Anthropic", "Google", "Meta", "DeepSeek", "Qwen", "Mistral", "xAI"} {
		icon := manufacturerBadge(name)
		if previous, ok := seen[icon]; ok {
			t.Fatalf("manufacturer icon %q is shared by %s and %s", icon, previous, name)
		}
		seen[icon] = name
	}
	got := manufacturerBadge("Unknown vendor")
	if owner, ok := seen[got]; ok {
		t.Fatalf("generic manufacturer icon %q is also used by %s", got, owner)
	}
}

func TestManufacturerBadgeIconsUseTerminalWidth(t *testing.T) {
	for _, name := range []string{"OpenAI", "Anthropic", "Google", "Meta", "DeepSeek", "Qwen", "Mistral", "xAI", "Unknown"} {
		icon := manufacturerBadge(name)
		want := testIconContract(manufacturerBadge(name)).displayWidth
		if got := tableDisplayWidth(icon); got != want {
			t.Errorf("manufacturerBadge(%q) = %q has terminal width %d, want %d", name, icon, got, want)
		}
	}
	if got := truncateTable("🌀 OpenAI", 2); got != "🌀" {
		t.Fatalf("truncateTable split or dropped the OpenAI icon: %q", got)
	}
}

func TestModelIdentityUsesOneVisibleSpaceAfterConfiguredEmojiIcons(t *testing.T) {
	for _, test := range []struct {
		name, icon string
	}{
		{"Meta", "Ⓜ️"}, {"OpenAI", "🌀"}, {"Qwen", "🌸"},
	} {
		t.Run(test.name, func(t *testing.T) {
			icons := config.IconConfig{Manufacturers: map[string]string{strings.ToLower(test.name): test.icon}, Unknown: "❔"}
			row := model.Model{DisplayName: test.name + " Muse Spark 1.1", Owner: test.name}
			gap := 1
			wantManufacturer := testIconContract(test.icon).slot + strings.Repeat(" ", gap) + test.name
			wantIdentity := wantManufacturer + " " + row.DisplayName
			if got := manufacturerDisplayWithIcons(row, icons); got != wantManufacturer {
				t.Fatalf("manufacturer formatter = %q, want %q", got, wantManufacturer)
			}
			if got := modelIdentityWithIcons(row, icons); got != wantIdentity {
				t.Fatalf("identity formatter = %q, want %q", got, wantIdentity)
			}
			if got := tableDisplayWidth(test.icon); got != testIconContract(test.icon).displayWidth {
				t.Fatalf("icon display width = %d, want %d for %q", got, testIconContract(test.icon).displayWidth, test.icon)
			}
			if got := tableDisplayWidth(wantManufacturer); got != testIconContract(test.icon).slotWidth+gap+len(test.name) {
				t.Fatalf("manufacturer display width = %d, want %d", got, testIconContract(test.icon).slotWidth+gap+len(test.name))
			}
			if got := tuiCellWithIcons(row, colName, false, scoreSourceDefault, icons); got != wantIdentity {
				t.Fatalf("TUI identity = %q, want %q", got, wantIdentity)
			}
			if got := renderTableModeWithIconsAndNameWidth([]model.Model{row}, 180, false, "short", scoreSourceDefault, icons, 40); !strings.Contains(got, wantIdentity) {
				t.Fatalf("CLI identity missing %q:\n%s", wantIdentity, got)
			}
			for _, line := range strings.Split(strings.TrimSuffix(renderTableModeWithIconsAndNameWidth([]model.Model{row}, 40, false, "short", scoreSourceDefault, icons, 40), "\n"), "\n") {
				if got := tableDisplayWidth(line); got > 42 {
					t.Fatalf("narrow CLI line width = %d, want <= 42: %q", got, line)
				}
			}
		})
	}
}

func TestIconGapRenderedBytesAndDisplayPositions(t *testing.T) {
	for _, test := range []struct {
		name, icon string
	}{
		{"Meta", "Ⓜ️"}, {"OpenAI", "🌀"}, {"Qwen", "🌸"}, {"Custom", "🛠️"},
	} {
		t.Run(test.name, func(t *testing.T) {
			icons := config.IconConfig{Manufacturers: map[string]string{strings.ToLower(test.name): test.icon}, Unknown: "❔"}
			row := model.Model{DisplayName: test.name + " Muse Spark 1.1", Owner: test.name}
			gap := 1
			want := testIconContract(test.icon).slot + strings.Repeat(" ", gap) + test.name + " " + row.DisplayName
			got := modelIdentityWithIcons(row, icons)
			t.Logf("rendered identity: %q bytes=% x width=%d", got, []byte(got), tableDisplayWidth(got))
			if got != want {
				t.Fatalf("identity bytes = % x, want % x", []byte(got), []byte(want))
			}
			byteGap := strings.Index(got, " ")
			if byteGap < 0 || tableDisplayWidth(got[:byteGap]) != tableDisplayWidth(test.icon) || got[byteGap:byteGap+1] != " " {
				t.Fatalf("identity icon gap at byte %d, display position %d: %q", byteGap, tableDisplayWidth(got[:byteGap]), got)
			}
			line := renderTableModeWithIconsAndNameWidth([]model.Model{row}, 180, false, "short", scoreSourceDefault, icons, 40)
			if !strings.Contains(line, want) {
				t.Fatalf("CLI render missing %q: %q", want, line)
			}
			tui := tuiModel{width: 120, nameWidth: 40}
			rendered := tui.renderTUILine([]tuiColumn{colName}, []string{tuiCellWithIcons(row, colName, false, scoreSourceDefault, icons)}, false)
			t.Logf("rendered CLI=%q TUI=%q", line, rendered)
			marker := testIconContract(test.icon).slot + strings.Repeat(" ", gap) + test.name
			index := strings.Index(rendered, marker)
			if index < 0 || tableDisplayWidth(rendered[:index+len(testIconContract(test.icon).slot)]) != testIconContract(test.icon).slotWidth+2 {
				t.Fatalf("TUI gap display position = %d, want %d: %q", tableDisplayWidth(rendered[:index+len(testIconContract(test.icon).slot)]), testIconContract(test.icon).slotWidth+2, rendered)
			}
			if tableDisplayWidth(rendered[:index+len(marker)]) != 4+gap+tableDisplayWidth(test.name) {
				t.Fatalf("TUI manufacturer position = %d, want %d: %q", tableDisplayWidth(rendered[:index+len(marker)]), 4+gap+tableDisplayWidth(test.name), rendered)
			}
		})
	}
}

func TestTableRenderersKeepGraphemeAwareColumnBoundaries(t *testing.T) {
	icons := config.IconConfig{Manufacturers: map[string]string{
		"meta": "Ⓜ️", "mistral": "🌪️", "openai": "🌀", "qwen": "🌸",
		"google": "🌐", "unknown": "❔", "xai": "🚀", "deepseek": "🐋",
	}, Unknown: "❔"}
	rows := []model.Model{
		{DisplayName: "Meta Model", Owner: "Meta"}, {DisplayName: "Mistral Model", Owner: "Mistral"},
		{DisplayName: "OpenAI Model", Owner: "OpenAI"}, {DisplayName: "Qwen Model", Owner: "Qwen"},
		{DisplayName: "Google Model", Owner: "Google"}, {DisplayName: "Unknown Model", Owner: "Unknown"},
		{DisplayName: "xAI Model", Owner: "xAI"}, {DisplayName: "DeepSeek Model", Owner: "DeepSeek"},
	}
	for _, gap := range []int{0, 1, 3} {
		for _, width := range []int{120, 40} {
			t.Run(fmt.Sprintf("cli/gap-%d/width-%d", gap, width), func(t *testing.T) {
				output := renderTableModeWithIconsAndNameWidthAndGap(rows, width, false, "notes", scoreSourceDefault, icons, 40, gap)
				lines := nonEmptyTableLines(output)
				if len(lines) != len(rows)+4 {
					t.Fatalf("CLI lines = %d, want header, %d rows, and 3 separators:\n%s", len(lines), len(rows), output)
				}
				wantColumns := testCLISeparatorColumns(width)
				for _, line := range lines {
					columns := tablePipeColumns(line)
					if len(columns) != 9 {
						t.Fatalf("CLI separator count = %d, want 9: %q", len(columns), line)
					}
					if !reflect.DeepEqual(columns, wantColumns) {
						t.Fatalf("CLI separator columns drifted: got %v, want %v: %q", columns, wantColumns, line)
					}
					if columns[0] != 0 || columns[len(columns)-1] != width-1 {
						t.Fatalf("CLI separator bounds = %v, want first 0 and last %d: %q", columns, width-1, line)
					}
					if tableDisplayWidth(ansi.Strip(line)) > width {
						t.Fatalf("CLI line exceeds configured width %d: %d: %q", width, tableDisplayWidth(ansi.Strip(line)), line)
					}
				}
			})
		}
	}
}

func TestManufacturerIconSlotHasOneConfiguredGapAndStableNameStart(t *testing.T) {
	manufacturers := []struct {
		name, icon string
	}{
		{"Meta", "Ⓜ️"}, {"Mistral", "🌪️"}, {"OpenAI", "🌀"}, {"Qwen", "🌸"},
		{"Google", "🌐"}, {"Unknown", "❔"}, {"xAI", "🚀"}, {"DeepSeek", "🐋"},
	}
	icons := config.IconConfig{Manufacturers: map[string]string{}, Unknown: "❔"}
	for _, manufacturer := range manufacturers {
		icons.Manufacturers[strings.ToLower(manufacturer.name)] = manufacturer.icon
	}
	for _, gap := range []int{0, 1, 3} {
		for _, manufacturer := range manufacturers {
			t.Run(fmt.Sprintf("%s/gap-%d", manufacturer.name, gap), func(t *testing.T) {
				row := model.Model{DisplayName: manufacturer.name + " Model", Owner: manufacturer.name}
				identity := modelIdentityWithIconsAndGap(row, icons, gap)
				nameStart := strings.Index(identity, manufacturer.name)
				if nameStart < 0 {
					t.Fatalf("manufacturer name missing from identity %q", identity)
				}
				if got := tableDisplayWidth(identity[:nameStart]); got != testIconContract(manufacturer.icon).slotWidth+gap {
					t.Fatalf("manufacturer start column = %d, want %d: %q", got, testIconContract(manufacturer.icon).slotWidth+gap, identity)
				}
				wantPrefix := testIconContract(manufacturer.icon).slot + strings.Repeat(" ", gap)
				if !strings.HasPrefix(identity, wantPrefix) {
					t.Fatalf("identity prefix = %q, want %q: %q", identity[:nameStart], wantPrefix, identity)
				}
			})
		}
	}
}

func TestTUIFormatterMatchesCLIIdentityAndKeepsColumnBoundaries(t *testing.T) {
	icons := config.IconConfig{Manufacturers: map[string]string{
		"meta": "Ⓜ️", "mistral": "🌪️", "openai": "🌀", "qwen": "🌸",
		"google": "🌐", "unknown": "❔", "xai": "🚀", "deepseek": "🐋",
	}, Unknown: "❔"}
	columns := []tuiColumn{colName, colClaude, colStatus, colQuality, colContext, colInput, colOutput, colNote}
	row := model.Model{DisplayName: "Meta Model", Owner: "Meta", ScoreLabel: "90%"}
	for _, gap := range []int{0, 1, 3} {
		for _, width := range []int{120, 40} {
			t.Run(fmt.Sprintf("gap-%d/width-%d", gap, width), func(t *testing.T) {
				m := tuiModel{width: width, nameWidth: 40, iconGap: gap, icons: icons, scoreSource: scoreSourceDefault}
				values := make([]string, len(columns))
				for i, column := range columns {
					values[i] = tuiCellWithIconsAndGap(row, column, false, scoreSourceDefault, icons, gap)
				}
				header := m.renderTUILine(columns, nil, false)
				line := m.renderTUILine(columns, values, false)
				wantColumns := testTUISeparatorColumns(width, len(columns))
				if got := tablePipeColumns(header); !reflect.DeepEqual(got, wantColumns) {
					t.Fatalf("TUI header separator columns = %v, want %v: %q", got, wantColumns, header)
				}
				if got := tablePipeColumns(line); !reflect.DeepEqual(got, wantColumns) {
					t.Fatalf("TUI row separator columns = %v, want %v: %q", got, wantColumns, line)
				}
				if tableDisplayWidth(ansi.Strip(header)) > width || tableDisplayWidth(ansi.Strip(line)) > width {
					t.Fatalf("TUI line exceeds configured width %d: header=%d row=%d", width, tableDisplayWidth(ansi.Strip(header)), tableDisplayWidth(ansi.Strip(line)))
				}
				wantIdentity := modelIdentityWithIconsAndGap(row, icons, gap)
				if values[0] != wantIdentity || values[0] != strings.TrimSpace(cliIdentityCell(rowsForIdentity(row), icons, gap)) {
					t.Fatalf("CLI/TUI identity mismatch: CLI=%q TUI=%q want=%q", cliIdentityCell(rowsForIdentity(row), icons, gap), values[0], wantIdentity)
				}
			})
		}
	}
}

func rowsForIdentity(row model.Model) []model.Model {
	return []model.Model{row}
}

func cliIdentityCell(rows []model.Model, icons config.IconConfig, gap int) string {
	output := renderTableModeWithIconsAndNameWidthAndGap(rows, 120, false, "notes", scoreSourceDefault, icons, 40, gap)
	for _, line := range nonEmptyTableLines(output) {
		if strings.HasPrefix(line, "| ") && !strings.Contains(line, "| Name ") {
			return strings.TrimSpace(strings.Split(line, "|")[1])
		}
	}
	return ""
}

type testIconLayout struct {
	slot         string
	bytes        []byte
	slotWidth    int
	displayWidth int
}

var testIconLayouts = map[string]testIconLayout{
	"Ⓜ️": {slot: "Ⓜ️", bytes: []byte{0xE2, 0x93, 0x82, 0xEF, 0xB8, 0x8F}, slotWidth: 2, displayWidth: 2},
	"🌪️": {slot: "🌪️", bytes: []byte{0xF0, 0x9F, 0x8C, 0xAA, 0xEF, 0xB8, 0x8F}, slotWidth: 2, displayWidth: 2},
	"🌀":  {slot: "🌀", bytes: []byte{0xF0, 0x9F, 0x8C, 0x80}, slotWidth: 2, displayWidth: 2},
	"🌸":  {slot: "🌸", bytes: []byte{0xF0, 0x9F, 0x8C, 0xB8}, slotWidth: 2, displayWidth: 2},
	"🐋":  {slot: "🐋", bytes: []byte{0xF0, 0x9F, 0x90, 0x8B}, slotWidth: 2, displayWidth: 2},
	"❔":  {slot: "❔", bytes: []byte{0xE2, 0x9D, 0x94}, slotWidth: 2, displayWidth: 2},
	"🔶":  {slot: "🔶", bytes: []byte{0xF0, 0x9F, 0x94, 0xB6}, slotWidth: 2, displayWidth: 2},
	"🌐":  {slot: "🌐", bytes: []byte{0xF0, 0x9F, 0x8C, 0x90}, slotWidth: 2, displayWidth: 2},
	"🚀":  {slot: "🚀", bytes: []byte{0xF0, 0x9F, 0x9A, 0x80}, slotWidth: 2, displayWidth: 2},
	"🛠️": {slot: "🛠️", bytes: []byte{0xF0, 0x9F, 0x9B, 0xA0, 0xEF, 0xB8, 0x8F}, slotWidth: 2, displayWidth: 2},
	"x":  {slot: "x ", bytes: []byte{'x', ' '}, slotWidth: 2, displayWidth: 1},
}

func testIconContract(icon string) testIconLayout {
	contract, ok := testIconLayouts[icon]
	if !ok {
		panic("missing independent icon contract for " + icon)
	}
	return contract
}

func testCLISeparatorColumns(width int) []int {
	want, ok := map[int][]int{
		120: {0, 38, 47, 58, 73, 87, 99, 112, 119},
		40:  {0, 7, 13, 17, 23, 27, 31, 35, 39},
	}[width]
	if !ok {
		panic(fmt.Sprintf("missing independent CLI geometry contract for width %d", width))
	}
	return want
}

func nonEmptyTableLines(output string) []string {
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func tablePipeColumns(line string) []int {
	line = ansi.Strip(line)
	columns := []int{}
	for index, r := range line {
		if r == '|' || r == '+' {
			columns = append(columns, tableDisplayWidth(line[:index]))
		}
	}
	return columns
}

func TestConfiguredIconGapIsSharedByCLIAndTUI(t *testing.T) {
	for _, gap := range []int{1, 3} {
		for _, test := range []struct {
			name, icon string
		}{
			{"Meta", "Ⓜ️"}, {"OpenAI", "🌀"}, {"Qwen", "🌸"}, {"Custom", "🛠️"},
		} {
			t.Run(fmt.Sprintf("%s/gap-%d", test.name, gap), func(t *testing.T) {
				icons := config.IconConfig{Manufacturers: map[string]string{strings.ToLower(test.name): test.icon}, Unknown: "❔"}
				row := model.Model{DisplayName: test.name + " Model", Owner: test.name}
				want := testIconContract(test.icon).slot + strings.Repeat(" ", gap) + test.name + " " + row.DisplayName
				if got := modelIdentityWithIconsAndGap(row, icons, gap); got != want {
					t.Fatalf("identity = %q, want %q", got, want)
				}
				if got := tuiCellWithIconsAndGap(row, colName, false, scoreSourceDefault, icons, gap); got != want {
					t.Fatalf("TUI identity = %q, want %q", got, want)
				}
				cli := renderTableModeWithIconsAndNameWidthAndGap([]model.Model{row}, 180, false, "short", scoreSourceDefault, icons, 40, gap)
				if !strings.Contains(cli, want) {
					t.Fatalf("CLI output missing %q:\n%s", want, cli)
				}
				narrow := renderTableModeWithIconsAndNameWidthAndGap([]model.Model{row}, 40, false, "short", scoreSourceDefault, icons, 40, gap)
				for _, line := range strings.Split(strings.TrimSuffix(narrow, "\n"), "\n") {
					if tableDisplayWidth(line) > 42 {
						t.Fatalf("narrow CLI line width = %d: %q", tableDisplayWidth(line), line)
					}
				}
			})
		}
	}
}

func TestDefaultAndCustomIconGapsRenderExactVendorBoundaries(t *testing.T) {
	icons := config.DefaultIconConfig()
	rows := []struct {
		name, icon string
	}{
		{"Meta", "Ⓜ️"}, {"Mistral", "🌪️"}, {"OpenAI", "🌀"}, {"Google", "🌐"},
	}
	for _, rowData := range rows {
		row := model.Model{DisplayName: rowData.name + " Model", Owner: rowData.name}
		gap := config.TableConfig{IconGap: config.DefaultIconGap}.EffectiveIconGapFor(rowData.name)
		want := testIconContract(rowData.icon).slot + strings.Repeat(" ", gap) + rowData.name + " " + row.DisplayName
		if got := modelIdentityWithIcons(row, icons); got != want {
			t.Errorf("%s bytes = % x, want % x", rowData.name, []byte(got), []byte(want))
		}
		if got := tableDisplayWidth(strings.Split(want, rowData.name)[0]); got != 2+gap {
			t.Errorf("%s boundary width = %d, want %d", rowData.name, got, 2+gap)
		}
	}
	custom := config.IconGaps{"meta": 0, "mistral": 3}.WithDefaults()
	for _, rowData := range rows[:2] {
		row := model.Model{DisplayName: rowData.name + " Model", Owner: rowData.name}
		wantGap := 0
		if rowData.name == "Mistral" {
			wantGap = 3
		}
		want := testIconContract(rowData.icon).slot + strings.Repeat(" ", wantGap) + rowData.name + " " + row.DisplayName
		if got := modelIdentityWithIconsAndGaps(row, icons, custom, 1); got != want {
			t.Errorf("custom %s = %q, want %q", rowData.name, got, want)
		}
	}
}

func TestManufacturerNamesShareFixedIconSlotAcrossRenderers(t *testing.T) {
	rows := []struct {
		name, icon string
	}{
		{"Meta", "Ⓜ️"}, {"Mistral", "🌪️"}, {"OpenAI", "🌀"}, {"Qwen", "🌸"}, {"Google", "🌐"},
	}
	icons := config.IconConfig{Manufacturers: map[string]string{"meta": "Ⓜ️", "mistral": "🌪️", "openai": "🌀", "qwen": "🌸", "google": "🌐", "custom": "x"}, Unknown: "❔"}
	for _, gap := range []int{0, 3} {
		positions := make(map[string]int, len(rows))
		for _, rowData := range rows {
			row := model.Model{DisplayName: rowData.name + " Model", Owner: rowData.name}
			got := modelIdentityWithIconsAndGap(row, icons, gap)
			want := testIconContract(rowData.icon).slot + strings.Repeat(" ", gap) + rowData.name + " " + row.DisplayName
			if got != want {
				t.Fatalf("gap=%d %s bytes = % x, want %x", gap, rowData.name, []byte(got), []byte(want))
			}
			nameStart := strings.Index(got, rowData.name)
			positions[rowData.name] = tableDisplayWidth(got[:nameStart])
			if positions[rowData.name] != testIconContract(rowData.icon).slotWidth+gap {
				t.Fatalf("gap=%d %s name starts at column %d, want %d", gap, rowData.name, positions[rowData.name], testIconContract(rowData.icon).slotWidth+gap)
			}
			if tui := tuiCellWithIconsAndGap(row, colName, false, scoreSourceDefault, icons, gap); tui != got {
				t.Fatalf("gap=%d %s TUI bytes = % x, want %x", gap, rowData.name, []byte(tui), []byte(got))
			}
			if detail := tuiDetailLinesWithHistoryAndIconsAndGap(row, scoreSourceDefault, 80, time.Unix(0, 0), nil, icons, gap); !strings.Contains(strings.Join(detail, "\n"), "Производитель: "+got[:strings.Index(got, " "+row.DisplayName)]) {
				t.Fatalf("gap=%d %s detail does not contain %q", gap, rowData.name, got)
			}
		}
		custom := model.Model{DisplayName: "Custom Model", Owner: "Custom"}
		got := modelIdentityWithIconsAndGap(custom, icons, gap)
		want := testIconContract("x").slot + strings.Repeat(" ", gap) + "Custom Custom Model"
		if got != want || tableDisplayWidth(got[:strings.Index(got, "Custom")]) != testIconContract("x").slotWidth+gap {
			t.Fatalf("gap=%d custom bytes/position = % x/%d, want %x/%d", gap, []byte(got), tableDisplayWidth(got[:strings.Index(got, "Custom")]), []byte(want), testIconContract("x").slotWidth+gap)
		}
	}
}

func TestModelIdentityNormalizesBoundaryWhitespaceToOneTerminalGap(t *testing.T) {
	icons := config.IconConfig{Manufacturers: map[string]string{"meta": "Ⓜ️"}, Unknown: "❔"}
	row := model.Model{DisplayName: "  Meta Muse Spark 1.1", Owner: " Meta "}
	want := "Ⓜ️ Meta Meta Muse Spark 1.1"
	if got := modelIdentityWithIcons(row, icons); got != want {
		t.Fatalf("normalized identity = %q bytes=% x, want %q bytes=% x", got, []byte(got), want, []byte(want))
	}
	if got := manufacturerDisplayWithIcons(row, icons); got != "Ⓜ️ Meta" {
		t.Fatalf("normalized manufacturer = %q, want %q", got, "Ⓜ️ Meta")
	}
	if got := joinTerminalWords("Ⓜ️  ", "  Meta", 1); got != "Ⓜ️ Meta" {
		t.Fatalf("joinTerminalWords = %q, want %q", got, "Ⓜ️ Meta")
	}
}

func TestManufacturerDisplayKeepsUnknownTextAndPrefersArenaOrganization(t *testing.T) {
	row := model.Model{DisplayName: "Demo", Owner: "Owner", ArenaScore: &model.ScoreInfo{Provider: " OpenAI ", IdentityStatus: model.IdentityExact}}
	if got := manufacturerDisplay(row); got != "🌀 OpenAI" {
		t.Fatalf("manufacturerDisplay = %q, want Arena organization with badge", got)
	}
	row.ArenaScore.Provider = ""
	row.Owner = "Mystery Vendor"
	if got := manufacturerDisplay(row); got != "❔ Mystery Vendor" {
		t.Fatalf("unknown manufacturer display = %q, want badge and original text", got)
	}
}

func TestManufacturerDisplaySkipsNeedsReviewOwnerAndUsesProviderNamespace(t *testing.T) {
	for _, test := range []struct{ slug, want string }{
		{"qwen/qwen3.7-flash", "🌸 Qwen"},
		{"nvidia/nemotron-3-nano-30b-a3b", "❔ NVIDIA"},
		{"poolside/laguna-xs-2.1", "❔ Poolside"},
		{"google/gemma-4-31b-it", "🌐 Google"},
		{"openai/gpt-5-mini", "🌀 OpenAI"},
	} {
		t.Run(test.slug, func(t *testing.T) {
			row := model.Model{Slug: test.slug, DisplayName: "Model", Owner: notes.NeedsReview}
			if got := manufacturerDisplay(row); got != test.want {
				t.Fatalf("manufacturerDisplay = %q, want %q", got, test.want)
			}
			identity := modelIdentityWithIcons(row, config.DefaultIconConfig())
			if strings.Contains(identity, notes.NeedsReview) || !strings.Contains(identity, test.want) {
				t.Fatalf("model identity = %q, want provider and no placeholder", identity)
			}
			if got := tuiCellWithIcons(row, colName, false, scoreSourceDefault, config.DefaultIconConfig()); got != identity {
				t.Fatalf("TUI identity = %q, want CLI identity %q", got, identity)
			}
		})
	}
}

func TestManufacturerDisplayPrefersCatalogProviderAndFiltersPlaceholders(t *testing.T) {
	row := model.Model{Slug: "catalog/model", DisplayName: "Model", Provider: "Catalog Provider", Owner: notes.NeedsReview, ArenaScore: &model.ScoreInfo{Provider: "Conflicting Arena Provider", IdentityStatus: model.IdentityExact}}
	want := "❔ Catalog Provider"
	if got := manufacturerDisplay(row); got != want {
		t.Fatalf("manufacturerDisplay = %q, want catalog provider before Arena", got)
	}
	if got := tuiCellWithIcons(row, colName, false, scoreSourceDefault, config.DefaultIconConfig()); got != want+" Model" {
		t.Fatalf("TUI identity = %q, want CLI identity %q", got, want+" Model")
	}

	for _, placeholder := range []string{
		"_нужен обзор_", "_нужен обзор_ (нет данных)", "_нужен обзор_(нет данных)",
		"n/a", "n/a (нет данных)", "n/a(нет данных)", "n/d", "n/d (нет данных)",
		"н/д", "н/д (нет данных)", "н/д(нет данных)", "(n/a)", "(н/д (нет данных))",
	} {
		t.Run(placeholder, func(t *testing.T) {
			row := model.Model{Slug: "openai/model", DisplayName: "Model", Provider: placeholder, Owner: placeholder, ArenaScore: &model.ScoreInfo{Provider: placeholder, IdentityStatus: model.IdentityExact}}
			want := "🌀 OpenAI Model"
			if got := manufacturerDisplay(row); got != "🌀 OpenAI" {
				t.Fatalf("manufacturerDisplay = %q, want namespace fallback %q", got, want)
			}
			if got := tuiCellWithIcons(row, colName, false, scoreSourceDefault, config.DefaultIconConfig()); got != want {
				t.Fatalf("TUI identity = %q, want CLI identity %q", got, want)
			}
		})
	}

	row = model.Model{Slug: "openai/model", DisplayName: "Model", Owner: "n/a", ArenaScore: &model.ScoreInfo{Provider: "Arena Provider", IdentityStatus: model.IdentityExact}}
	if got := manufacturerDisplay(row); got != "❔ Arena Provider" {
		t.Fatalf("manufacturerDisplay = %q, want valid Arena fallback", got)
	}
}

func TestManufacturerDisplayRejectsUnverifiedArenaOrganization(t *testing.T) {
	for _, status := range []string{model.IdentityVariantMismatch, model.IdentityMissing, model.IdentityLegacyUnknown, model.IdentityObservationOnly, ""} {
		t.Run(status, func(t *testing.T) {
			row := model.Model{DisplayName: "Demo", Owner: "Owner", ArenaScore: &model.ScoreInfo{Provider: "Wrong Arena Organization", IdentityStatus: status}}
			if got := manufacturerDisplay(row); got != "❔ Owner" {
				t.Fatalf("manufacturerDisplay with Arena status %q = %q, want Owner fallback", status, got)
			}
			row.Owner = ""
			row.Provider = "Provider"
			if got := manufacturerDisplay(row); got != "❔ Provider" {
				t.Fatalf("manufacturerDisplay with Arena status %q and empty Owner = %q, want catalogue Provider fallback", status, got)
			}
		})
	}
}

func TestManufacturerBadgeUsesConfiguredMapping(t *testing.T) {
	icons := config.IconConfig{Manufacturers: map[string]string{"openai": "🧩", "custom": "🛠️"}, Unknown: "❓"}
	if got := manufacturerBadgeWithIcons("OpenAI Labs", icons); got != "🧩" {
		t.Fatalf("configured OpenAI icon = %q", got)
	}
	if got := manufacturerBadgeWithIcons("Custom Vendor", icons); got != "🛠️" {
		t.Fatalf("configured custom icon = %q", got)
	}
	if got := manufacturerBadgeWithIcons("Unknown Vendor", icons); got != "❓" {
		t.Fatalf("configured unknown icon = %q", got)
	}
}

func TestRenderTableUsesConfiguredNameWidth(t *testing.T) {
	row := model.Model{DisplayName: "A deliberately long model name", Owner: "OpenAI"}
	output := renderTableModeWithIconsAndNameWidth([]model.Model{row}, 120, false, "short", scoreSourceDefault, config.DefaultIconConfig(), 24)
	if got := firstTableColumnWidth(output); got != 24 {
		t.Fatalf("configured Name column width = %d, want 24:\n%s", got, output)
	}
}

func TestRenderTableConfiguredNameWidthIsBoundedByViewport(t *testing.T) {
	output := renderTableModeWithIconsAndNameWidth([]model.Model{{DisplayName: "model"}}, 40, false, "short", scoreSourceDefault, config.DefaultIconConfig(), 100)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if got := tableDisplayWidth(line); got > 42 {
			t.Fatalf("configured Name column exceeded the table border budget: %d: %q", got, line)
		}
	}
}

func TestConfiguredIconsPreserveRenderingWidthAndArenaFallback(t *testing.T) {
	icons := config.IconConfig{Manufacturers: map[string]string{"openai": "🧩"}, Unknown: "❔"}
	row := model.Model{DisplayName: "GPT", Owner: "Owner", ArenaScore: &model.ScoreInfo{Provider: " OpenAI ", IdentityStatus: model.IdentityExact}}
	if got := manufacturerDisplayWithIcons(row, icons); got != "🧩 OpenAI" {
		t.Fatalf("Arena manufacturer display = %q", got)
	}
	if got := tuiCellWithIcons(row, colName, true, scoreSourceDefault, icons); got != "🧩 OpenAI GPT" {
		t.Fatalf("configured TUI identity = %q", got)
	}
	if got := renderTableModeWithIcons([]model.Model{row}, 120, false, "short", scoreSourceDefault, icons); !strings.Contains(got, "🧩 OpenAI GPT") {
		t.Fatalf("configured CLI table identity missing: %s", got)
	}
}

func TestTUITableCellShowsManufacturerBadgeWithoutLosingModelName(t *testing.T) {
	got := tuiCell(model.Model{DisplayName: "GPT", Owner: "OpenAI"}, colName, true, scoreSourceDefault)
	if got != "🌀 OpenAI GPT" {
		t.Fatalf("TUI identity = %q, want badge and model name", got)
	}
}

func TestRenderTableModeShowsManufacturerBadgeAndStaysWithinWidth(t *testing.T) {
	row := model.Model{DisplayName: "GPT-5.6 Luna", Owner: "OpenAI", ScoreLabel: "93.0%", QualityPriceLabel: "82.7"}
	wide := renderTableMode([]model.Model{row}, 120, false, "short", scoreSourceDefault)
	if !strings.Contains(wide, "🌀 OpenAI GPT-5.6 Luna") {
		t.Fatalf("CLI table lost manufacturer badge or model name:\n%s", wide)
	}
	narrow := renderTableMode([]model.Model{row}, 40, false, "short", scoreSourceDefault)
	for _, line := range strings.Split(strings.TrimSuffix(narrow, "\n"), "\n") {
		if tableDisplayWidth(line) > 42 {
			t.Fatalf("narrow CLI table line exceeds the existing border budget: %d > 42: %q", tableDisplayWidth(line), line)
		}
	}
	if got, want := tableDisplayWidth(manufacturerDisplay(row)), 2+1+6; got != want {
		t.Fatalf("manufacturer display width = %d, want terminal width %d", got, want)
	}
	if got := tableDisplayWidth(manufacturerDisplay(row)); got == len(manufacturerDisplay(row)) {
		t.Fatalf("manufacturer display width unexpectedly uses byte length: width=%d bytes=%d", got, len(manufacturerDisplay(row)))
	}
	tuiRow := tuiCell(row, colName, true, scoreSourceDefault)
	if tableDisplayWidth(truncateTable(tuiRow, 12)) > 12 {
		t.Fatalf("narrow TUI identity exceeds width: %d: %q", tableDisplayWidth(truncateTable(tuiRow, 12)), tuiRow)
	}
}

func TestFilterTableModelsRejectsUnknownTier(t *testing.T) {
	_, err := filterTableModels(nil, []string{"tier:unknown"})
	if err == nil || !strings.Contains(err.Error(), `unknown tier "unknown"`) || !strings.Contains(err.Error(), "opus, sonnet, haiku, free") {
		t.Fatalf("unknown tier error = %v, want the allowed tier values", err)
	}
}

func TestFilterTableModelsAcceptsTierCaseInsensitively(t *testing.T) {
	models := []model.Model{{Slug: "sonnet", Tier: "sonnet"}, {Slug: "free", Tier: "free"}}
	filtered, err := filterTableModels(models, []string{"tier:SONNET"})
	if err != nil || len(filtered) != 1 || filtered[0].Slug != "sonnet" {
		t.Fatalf("case-insensitive tier filter = %+v, error %v", filtered, err)
	}
}

func TestFilterTableModelsQualityPriceAndAvailabilityPredicates(t *testing.T) {
	models := []model.Model{{Slug: "paid", Free: false, HasQualityPrice: true, QualityPrice: 4}, {Slug: "free", Free: true, HasQualityPrice: false}, {Slug: "invalid", Free: false, HasQualityPrice: false}}
	got, err := filterTableModels(models, []string{"has-q/p", "availability:paid"})
	if err != nil || len(got) != 1 || got[0].Slug != "paid" {
		t.Fatalf("filtered models = %+v, error %v", got, err)
	}
	got, err = filterTableModels(models, []string{"availability:free"})
	if err != nil || len(got) != 1 || got[0].Slug != "free" {
		t.Fatalf("free models = %+v, error %v", got, err)
	}
}

func TestFilterTableModelsRejectsInvalidAvailability(t *testing.T) {
	if _, err := filterTableModels(nil, []string{"availability:discount"}); err == nil {
		t.Fatal("invalid availability unexpectedly accepted")
	}
}

func TestRenderTableTaskFitShortAndLong(t *testing.T) {
	row := model.Model{DisplayName: "model", TaskFit: []string{"implement", "test"}}
	short := renderTableMode([]model.Model{row}, 120, false, "short", scoreSourceDefault)
	long := renderTableMode([]model.Model{row}, 120, false, "long", scoreSourceDefault)
	narrowLong := renderTableMode([]model.Model{row}, 40, false, "long", scoreSourceDefault)
	if !strings.Contains(short, "Task fit") || !strings.Contains(short, "IT") || strings.Contains(short, "I+T") {
		t.Fatalf("short task fit = %s", short)
	}
	if !strings.Contains(long, "implement + test") {
		t.Fatalf("long task fit = %s", long)
	}
	if strings.Contains(long, "IT") {
		t.Fatalf("long task fit used short formatting = %s", long)
	}
	if !strings.Contains(narrowLong, "implement + test") {
		t.Fatalf("long task fit at 40 columns was truncated:\n%s", narrowLong)
	}
}

func TestRenderTableTaskFitCanonicalShortHasNoPluses(t *testing.T) {
	row := model.Model{DisplayName: "model", TaskFit: []string{"implement", "debug", "refactor", "test"}}
	short := renderTableMode([]model.Model{row}, 120, false, "short", scoreSourceDefault)
	if !strings.Contains(short, "IDFT") || strings.Contains(short, "I+D+F+T") {
		t.Fatalf("canonical short task fit = %s", short)
	}
}

func TestRenderTableTaskFitMissingIsNA(t *testing.T) {
	output := renderTableMode([]model.Model{{DisplayName: "model"}}, 120, false, "short", scoreSourceDefault)
	if !strings.Contains(output, "Task fit") || !strings.Contains(output, "n/a") {
		t.Fatalf("missing task fit = %s", output)
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
	assertTableHeaders(t, output, []string{"Slug", "Claude", "SWE %", "Q/P score/$M", "Context tok", "In $/M", "Out $/M", "Note"})
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
	if !strings.Contains(output, "н/д (цена...") || !strings.Contains(output, "н/д (оцен...") || strings.Contains(output, "н/д (цена $0)") || strings.Contains(output, "н/д (оценка") {
		t.Fatalf("free/no-score Q/P labels were not safely truncated:\n%s", output)
	}
	if strings.Contains(output, "93.0%; review this") || strings.Contains(output, "н/д;") || strings.Contains(output, notes.NeedsReview) {
		t.Fatalf("status and note were combined or review marker was shown:\n%s", output)
	}
}

func TestRenderTableKeepsQualityPriceWithinReadableColumn(t *testing.T) {
	models := []model.Model{{DisplayName: "paid", ScoreLabel: "93.0%", QualityPriceLabel: "436", Note: "status note"}, {DisplayName: "free", ScoreLabel: "н/д", QualityPriceLabel: "н/д (цена $0)"}, {DisplayName: "missing", ScoreLabel: "н/д", QualityPriceLabel: "н/д (оценка не для этого варианта)"}}
	for _, width := range []int{120, 40} {
		output := renderTable(models, width, false)
		if got := tableColumnWidths(output)[3]; got > 12 {
			t.Errorf("Q/P column width at %d columns = %d, want <= 12:\n%s", width, got, output)
		}
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if !strings.HasPrefix(line, "| ") || strings.Contains(line, "| Name ") {
				continue
			}
			cells := strings.Split(strings.Trim(line, "|"), "|")
			if len(cells) < 4 {
				t.Fatalf("malformed table row at %d columns: %q", width, line)
			}
			if got := tableDisplayWidth(strings.TrimSpace(cells[3])); got > 12 {
				t.Errorf("Q/P cell width at %d columns = %d, want <= 12: %q", width, got, line)
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
		{DisplayName: "haiku-high", Tier: "haiku", Score: &model.ScoreInfo{Value: 70}, Rankable: true, ClaudeRef: "**<≈ Haiku 4.5** | бесплатная, середина диапазона, уточнение漢字"},
		{DisplayName: "haiku-mid", Tier: "haiku", Score: &model.ScoreInfo{Value: 60}, Rankable: true},
		{DisplayName: "haiku-low", Tier: "haiku", Score: &model.ScoreInfo{Value: 59.9}, Rankable: true},
		{DisplayName: "haiku-fallback", Tier: "haiku"},
		{DisplayName: "free-high", Tier: "free", Score: &model.ScoreInfo{Value: 70}, Rankable: true},
		{DisplayName: "free-mid", Tier: "free", Score: &model.ScoreInfo{Value: 60}, Rankable: true},
		{DisplayName: "free-low", Tier: "free", Score: &model.ScoreInfo{Value: 59.9}, Rankable: true},
		{DisplayName: "free-fallback", Tier: "free"},
		{DisplayName: "unknown", Tier: "other"},
	}
	output := renderTable(models, 120, false)
	wantClaude := map[string]string{
		"opus":           ">≈ Opus 5",
		"sonnet":         "≈ Sonnet 5",
		"haiku-high":     "≈ Haiku 4.5",
		"haiku-mid":      "<≈ Haiku 4.5",
		"haiku-low":      "<<≈ Haiku 4.5",
		"haiku-fallback": "≈ Haiku 4.5",
		"free-high":      "≈ Haiku 4.5",
		"free-mid":       "<≈ Haiku 4.5",
		"free-low":       "<<≈ Haiku 4.5",
		"free-fallback":  "<≈ Haiku 4.5",
		"unknown":        "n/a",
	}
	for slug, want := range wantClaude {
		if got := tableRowCell(t, output, slug, 1); got != want {
			t.Errorf("Claude equivalent for %q = %q, want %q:\n%s", slug, got, want, output)
		}
	}
	for _, forbidden := range []string{"ClaudeRef", "бесплатная", "середина диапазона", "уточнение"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("forbidden Claude text %q present:\n%s", forbidden, output)
		}
	}
}

func tableRowCell(t *testing.T, output, identity string, column int) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.Contains(line, "| Name ") || strings.Contains(line, "| Slug ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 8 {
			t.Fatalf("table row has %d columns, want 8: %q", len(cells), line)
		}
		cellIdentity := strings.TrimSpace(cells[0])
		if len(cells) <= column || (cellIdentity != identity && !strings.HasSuffix(cellIdentity, " "+identity)) {
			continue
		}
		return strings.TrimSpace(cells[column])
	}
	return ""
}

func TestRenderTableKeepsClaudeLabelAtAnyRequestedWidth(t *testing.T) {
	want := "<<≈ Haiku 4.5"
	for _, width := range []int{120, 40} {
		output := renderTable([]model.Model{{DisplayName: "model", Tier: "free", Score: &model.ScoreInfo{Value: 59}, Rankable: true}}, width, false)
		if !strings.Contains(output, want) {
			t.Errorf("full Claude equivalent at %d columns is missing or truncated:\n%s", width, output)
		}
		if got := tableColumnWidths(output)[1]; got < tableDisplayWidth(want) {
			t.Errorf("Claude column width at %d columns = %d, want >= %d", width, got, tableDisplayWidth(want))
		}
	}
}

func TestRenderTableKeepsNormalizedFullClaudeAndNoteAtAnyRequestedWidth(t *testing.T) {
	note := "**полная** заметка | с control\r\nи Unicode е\u0301界🙂"
	claudeRef := "__<≈ Haiku 4.5__ | бесплатная, середина диапазона, уточнение漢字"
	wantClaude := ">≈ Opus 5"
	wantNote := "полная заметка / с control  и Unicode е\u0301界🙂"
	for _, width := range []int{120, 40} {
		output := renderTable([]model.Model{{DisplayName: "model", Tier: "opus", ClaudeRef: claudeRef, Note: note}}, width, false)
		if strings.Count(output, wantClaude) != 1 || !strings.Contains(output, wantNote) {
			t.Errorf("normalized full fields at %d columns are missing or truncated:\n%s", width, output)
		}
		for _, forbidden := range []string{"бесплатная", "середина диапазона", "уточнение", "漢字"} {
			if strings.Contains(output, forbidden) {
				t.Errorf("ClaudeRef text %q leaked into table at %d columns:\n%s", forbidden, width, output)
			}
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

func TestTableDisplayWidthOnlyPromotesEmojiCapableBasesForVS16(t *testing.T) {
	for _, value := range []string{"A\ufe0f", "1\ufe0f"} {
		if got := tableDisplayWidth(value); got != 1 {
			t.Errorf("tableDisplayWidth(%q) = %d, want 1", value, got)
		}
	}
	for _, name := range []string{"OpenAI", "Anthropic", "Google", "Meta", "DeepSeek", "Qwen", "Mistral", "xAI", "Unknown"} {
		icon := manufacturerBadge(name)
		if got := tableDisplayWidth(icon); got != 2 {
			t.Errorf("manufacturerBadge(%q) = %q has terminal width %d, want 2", name, icon, got)
		}
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
	t.Setenv("COLUMNS", "120")
	output := executeCLI(t, "table", "--config", config)
	if !strings.Contains(output, "Name") || strings.Contains(output, "| Slug") || !strings.Contains(output, "Claude") {
		t.Fatalf("table output = %q", output)
	}
	if got := tableColumnWidths(output); len(got) != 8 {
		t.Fatalf("CLI table has %d columns, want 8:\n%s", len(got), output)
	}
}

func TestTableCommandUsesFormulaFromConfig(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\nranking:\n  mixed_utility:\n    price:\n      input_weight: 1\n      output_weight: 1\n    tier_factors:\n      sonnet: 1\n      haiku: 2\n    formula:\n      op: sub\n      args:\n        - var: score\n        - op: mul\n          args:\n            - var: tier_factor\n            - var: price_mix\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")
	output := executeCLI(t, "table", "--config", config, "--no-pager", "--slug", "-n", "1")
	if got := tableFirstCell(output, 0); got != "demo/low" {
		t.Fatalf("configured formula first table slug = %q, want demo/low:\n%s", got, output)
	}
}

func TestTableCommandRejectsRuntimeFormulaError(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\nranking:\n  mixed_utility:\n    formula:\n      op: div\n      args:\n        - const: 1\n        - const: 0\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	err := executeCLIError(t, "table", "--config", config, "--no-pager")
	if err == nil || !strings.Contains(err.Error(), `cannot rank model "demo/high"`) || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("table error = %v, want propagated formula runtime error", err)
	}
}

func TestTableCommandTaskFitModesThroughCLI(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "default short", args: []string{"table", "--config", config}, want: []string{"Task fit", "IT"}},
		{name: "explicit short", args: []string{"table", "--config", config, "--task-fit=short"}, want: []string{"Task fit", "IT"}},
		{name: "long", args: []string{"table", "--config", config, "--task-fit=long"}, want: []string{"Task fit", "implement + test"}},
		{name: "legacy notes", args: []string{"table", "--config", config, "--notes"}, want: []string{"Note", "Local fixture"}},
		{name: "long tier ranking", args: []string{"table", "--config", config, "--ranking=tier-priority"}, want: []string{"Ranking: tier-priority"}},
		{name: "long mixed ranking", args: []string{"table", "--config", config, "--ranking=mixed-utility"}, want: []string{"Ranking: mixed-utility"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := executeCLI(t, tt.args...)
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("output does not contain %q:\n%s", want, output)
				}
			}
		})
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "notes and long conflict", args: []string{"table", "--config", config, "--notes", "--task-fit=long"}, want: "--notes cannot be combined"},
		{name: "invalid value", args: []string{"table", "--config", config, "--task-fit=invalid"}, want: "invalid --task-fit"},
		{name: "empty value", args: []string{"table", "--config", config, "--task-fit="}, want: "invalid --task-fit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := executeCLIError(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}

	output := executeCLI(t, "table", "--config", config, "--task-fit=long")
	if !strings.Contains(output, "n/a") {
		t.Fatalf("missing task fit did not render n/a:\n%s", output)
	}
}

func TestTableCLIOptionsOverrideConfigOperationalValues(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\ntable:\n  task_fit: long\n  ranking: tier-priority\n  limit: 1\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")
	fromConfig := executeCLI(t, "table", "--config", config, "--no-pager")
	if !strings.Contains(fromConfig, "Ranking: tier-priority") || !strings.Contains(fromConfig, "implement + test") || len(tableDataRows(fromConfig)) != 1 {
		t.Fatalf("config operational values were not applied:\n%s", fromConfig)
	}
	fromCLI := executeCLI(t, "table", "--config", config, "--no-pager", "--ranking=legacy", "--task-fit=short", "--limit=0")
	if strings.Contains(fromCLI, "Ranking: tier-priority") || !strings.Contains(fromCLI, "Task fit") || strings.Contains(fromCLI, "implement + test") || len(tableDataRows(fromCLI)) < 2 {
		t.Fatalf("CLI operational values did not override config:\n%s", fromCLI)
	}
}

func tableDataRows(output string) []string {
	var rows []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.Contains(line, "| Name ") && !strings.Contains(line, "| Slug ") {
			rows = append(rows, line)
		}
	}
	return rows
}

func TestTableCommandLongTaskFitAtMinimumWidth(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/high:\n    display: Demo High\n    task_fit: [implement, plan, research, debug, audit, refactor, test]\n  demo/low:\n    display: Demo Low\n  demo/missing:\n    display: Demo Missing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "40")
	output := executeCLI(t, "table", "--config", config, "--no-pager", "--task-fit=long", "--sort", "quality", "-n", "1")
	longText := "implement + plan + research + debug + audit + refactor + test"
	if !strings.Contains(output, longText) {
		t.Fatalf("minimum-width CLI output lost full task-fit text:\n%s", output)
	}
	widths := tableColumnWidths(output)
	if len(widths) != 8 || widths[7] < tableDisplayWidth(longText) {
		t.Fatalf("task-fit column widths = %v, want final column >= %d:\n%s", widths, tableDisplayWidth(longText), output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(line, "Ranking:") || strings.HasPrefix(line, "Sort:") || strings.TrimSpace(line) == "" {
			continue
		}
		if got := tableDisplayWidth(line); got < 40 {
			t.Errorf("CLI table line width = %d, want at least 40: %q", got, line)
		}
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

func TestLimitTableModelsZeroMeansUnlimited(t *testing.T) {
	models := []model.Model{{Slug: "model"}}
	if got := limitTableModels(models, 0); !reflect.DeepEqual(got, models) {
		t.Fatalf("zero limit = %v, want unlimited result %v", got, models)
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

func TestTableHelpIncludesUpdatedAliases(t *testing.T) {
	cmd := newRootCmd()
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"table", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("table help: %v", err)
	}
	if !strings.Contains(output.String(), "-S, --slug") && !strings.Contains(output.String(), "--slug") {
		t.Fatalf("table help does not contain slug alias:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "-s, --sort") || !strings.Contains(output.String(), "-R, --reverse") || !strings.Contains(output.String(), "quality") {
		t.Fatalf("table help does not contain sort/reverse aliases and quality:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "-n, --limit") || !strings.Contains(output.String(), "after sorting") {
		t.Fatalf("table help does not describe limit:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "standalone -N is shorthand for -n N") || !strings.Contains(output.String(), "0 means unlimited") {
		t.Fatalf("table help does not describe numeric shorthand and zero semantics:\n%s", output.String())
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
	if err := sortTableModels(models, "quality", false); err != nil || models[0].Slug != "a" {
		t.Fatalf("quality tie-breaker = %+v, err=%v", models, err)
	}
}

func TestSortTableModelsDefaultUsesMixedUtility(t *testing.T) {
	models := []model.Model{
		{Slug: "missing"},
		{Slug: "deepseek", Tier: "haiku", Score: &model.ScoreInfo{Value: 90}, Rankable: true, QualityPrice: 90},
		{Slug: "opus", Tier: "opus", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 1},
	}
	if err := sortTableModels(models, "", false); err != nil {
		t.Fatalf("default sort error = %v", err)
	}
	if got := []string{models[0].Slug, models[1].Slug, models[2].Slug}; !reflect.DeepEqual(got, []string{"deepseek", "opus", "missing"}) {
		t.Fatalf("default sort = %v, want [deepseek opus missing]", got)
	}
	if err := sortTableModelsWithRanking(models, "", false, rankingLegacy); err != nil {
		t.Fatalf("explicit legacy sort error = %v", err)
	}
	if models[0].Slug != "deepseek" {
		t.Fatalf("explicit legacy first model = %q, want deepseek", models[0].Slug)
	}
}

func TestRankingModesKeepTierPriorityAndQualityDominanceSeparate(t *testing.T) {
	rows := []model.Model{
		{Slug: "deepseek", Tier: "haiku", Score: &model.ScoreInfo{Value: 90}, Rankable: true, QualityPrice: 90},
		{Slug: "luna", Tier: "opus", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 1},
	}
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingTier); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "luna" {
		t.Fatalf("tier ranking = %q, want luna first", rows[0].Slug)
	}
	rows[0], rows[1] = rows[1], rows[0]
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingMixed); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "deepseek" {
		t.Fatalf("mixed ranking = %q, want deepseek first by tier-adjusted utility", rows[0].Slug)
	}
}

func TestMixedUtilityRanksRealModelsByGlobalTierAdjustedUtility(t *testing.T) {
	rows := []model.Model{
		{Slug: "openai/gpt-5.6-luna", DisplayName: "GPT-5.6 Luna", Tier: "opus", Score: &model.ScoreInfo{Value: 93.0}, Rankable: true, InPerM: 0.10, OutPerM: 0.60},
		{Slug: "minimax/minimax-m3", DisplayName: "MiniMax M3", Tier: "sonnet", Score: &model.ScoreInfo{Value: 80.5}, Rankable: true, InPerM: 0.30, OutPerM: 1.20},
		{Slug: "moonshotai/kimi-k3", DisplayName: "Kimi K3", Tier: "sonnet", Score: &model.ScoreInfo{Value: 93.4}, Rankable: true, InPerM: 3.00, OutPerM: 15.00},
		{Slug: "x-ai/grok-4.5", DisplayName: "Grok 4.5", Tier: "sonnet", Score: &model.ScoreInfo{Value: 86.6}, Rankable: true, InPerM: 2.00, OutPerM: 6.00},
		{Slug: "openai/gpt-5.6-sol", DisplayName: "GPT-5.6 Sol", Tier: "opus", Score: &model.ScoreInfo{Value: 96.2}, Rankable: true, InPerM: 5.00, OutPerM: 30.00},
		{Slug: "deepseek/deepseek-v4-flash", DisplayName: "DeepSeek V4 Flash", Tier: "haiku", Score: &model.ScoreInfo{Value: 76.35}, Rankable: true, InPerM: 0.09, OutPerM: 0.18},
	}
	for i := range rows {
		rows[i].MixedPrice = pricing.MixedPrice(rows[i].InPerM, rows[i].OutPerM)
		rows[i].QualityPrice = pricing.QualityPrice(rows[i].Score.Value, rows[i].MixedPrice)
	}
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingMixed); err != nil {
		t.Fatalf("mixed ranking: %v", err)
	}
	got := make([]string, len(rows))
	for i := range rows {
		got[i] = rows[i].DisplayName
	}
	want := []string{"GPT-5.6 Luna", "MiniMax M3", "Kimi K3", "Grok 4.5", "GPT-5.6 Sol", "DeepSeek V4 Flash"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed ranking = %v, want %v", got, want)
	}
}

func TestMixedUtilityUsesStrongPriceWeight(t *testing.T) {
	rows := []model.Model{
		{Slug: "minimax/minimax-m3", DisplayName: "MiniMax M3", Tier: "sonnet", Score: &model.ScoreInfo{Value: 70}, Rankable: true, QualityPrice: 100},
		{Slug: "moonshotai/kimi-k3", DisplayName: "Kimi K3", Tier: "sonnet", Score: &model.ScoreInfo{Value: 75}, Rankable: true, QualityPrice: 50},
		{Slug: "x-ai/grok-4.5", DisplayName: "Grok 4.5", Tier: "sonnet", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 25},
	}
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingMixed); err != nil {
		t.Fatalf("mixed ranking: %v", err)
	}
	if got, want := []string{rows[0].DisplayName, rows[1].DisplayName, rows[2].DisplayName}, []string{"MiniMax M3", "Kimi K3", "Grok 4.5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed ranking within Sonnet = %v, want %v", got, want)
	}
}

func TestMixedUtilityRanksLunaByComputedUtility(t *testing.T) {
	rows := []model.Model{
		{Slug: "openai/gpt-5.6-luna", DisplayName: "GPT-5.6 Luna", Tier: "opus", Score: &model.ScoreInfo{Value: 93}, Rankable: true, InPerM: 0.10, OutPerM: 0.60},
		{Slug: "openai/gpt-5.6-sol", DisplayName: "GPT-5.6 Sol", Tier: "opus", Score: &model.ScoreInfo{Value: 96.2}, Rankable: true, InPerM: 5.00, OutPerM: 30.00},
	}
	for i := range rows {
		rows[i].MixedPrice = pricing.MixedPrice(rows[i].InPerM, rows[i].OutPerM)
		rows[i].QualityPrice = pricing.QualityPrice(rows[i].Score.Value, rows[i].MixedPrice)
	}
	if got, want := rows[0].QualityPrice, 413.33; math.Abs(got-want) > 0.01 {
		t.Fatalf("Luna Q/P = %v, want approximately %v", got, want)
	}
	if got, want := rows[1].QualityPrice, 8.55; math.Abs(got-want) > 0.01 {
		t.Fatalf("Sol Q/P = %v, want approximately %v", got, want)
	}
	if rows[1].QualityPrice >= rows[0].QualityPrice {
		t.Fatalf("Sol Q/P = %v, want less than Luna Q/P = %v", rows[1].QualityPrice, rows[0].QualityPrice)
	}
	lunaUtility := rows[0].Score.Value + config.DefaultMixedUtilityPriceWeight*math.Log1p(rows[0].QualityPrice)
	solUtility := rows[1].Score.Value + config.DefaultMixedUtilityPriceWeight*math.Log1p(rows[1].QualityPrice)
	if lunaUtility <= solUtility {
		t.Fatalf("test data does not make Luna utility higher: Luna=%v Sol=%v", lunaUtility, solUtility)
	}
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingMixed); err != nil {
		t.Fatalf("mixed ranking: %v", err)
	}
	if got, want := rows[0].Slug, "openai/gpt-5.6-luna"; got != want {
		t.Fatalf("mixed ranking first slug = %q, want %q by utility", got, want)
	}
}

func TestMixedUtilityUsesConfiguredPriceWeight(t *testing.T) {
	rows := []model.Model{
		{Slug: "quality", Tier: "sonnet", Score: &model.ScoreInfo{Value: 90}, Rankable: true, QualityPrice: 1},
		{Slug: "value", Tier: "sonnet", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 100},
	}
	if err := sortTableModelsWithRankingAndWeight(rows, "utility", false, rankingMixed, 0); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "quality" {
		t.Fatalf("zero-weight ranking first slug = %q, want quality", rows[0].Slug)
	}
	if err := sortTableModelsWithRankingAndWeight(rows, "utility", false, rankingMixed, 10); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "value" {
		t.Fatalf("custom-weight ranking first slug = %q, want value", rows[0].Slug)
	}
}

func TestTierPriorityKeepsTierAheadOfMixedUtility(t *testing.T) {
	rows := []model.Model{
		{Slug: "haiku/cheap", Tier: "haiku", Score: &model.ScoreInfo{Value: 95}, Rankable: true, QualityPrice: 100},
		{Slug: "opus/expensive", Tier: "opus", Score: &model.ScoreInfo{Value: 60}, Rankable: true, QualityPrice: 1},
	}
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingTier); err != nil {
		t.Fatalf("tier-priority ranking: %v", err)
	}
	if got, want := rows[0].Slug, "opus/expensive"; got != want {
		t.Fatalf("tier-priority first model = %q, want %q", got, want)
	}
}

func TestRankingLongNamesMatchShortNames(t *testing.T) {
	rows := []model.Model{
		{Slug: "deepseek", Tier: "haiku", Score: &model.ScoreInfo{Value: 90}, Rankable: true, QualityPrice: 90},
		{Slug: "luna", Tier: "opus", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 1},
	}
	for _, test := range []struct {
		short, long string
	}{
		{short: rankingTier, long: "tier-priority"},
		{short: rankingMixed, long: "mixed-utility"},
	} {
		shortRows := append([]model.Model(nil), rows...)
		longRows := append([]model.Model(nil), rows...)
		if err := sortTableModelsWithRanking(shortRows, "utility", false, test.short); err != nil {
			t.Fatalf("short ranking %q: %v", test.short, err)
		}
		if err := sortTableModelsWithRanking(longRows, "utility", false, test.long); err != nil {
			t.Fatalf("long ranking %q: %v", test.long, err)
		}
		if got, want := []string{longRows[0].Slug, longRows[1].Slug}, []string{shortRows[0].Slug, shortRows[1].Slug}; !reflect.DeepEqual(got, want) {
			t.Errorf("ranking %q = %v, want %v", test.long, got, want)
		}
		if got := rankingLabel(test.long); got != test.long {
			t.Errorf("rankingLabel(%q) = %q, want %q", test.long, got, test.long)
		}
	}
}

func TestRankingDoesNotUseTaskFit(t *testing.T) {
	rows := []model.Model{
		{Slug: "z", Tier: "haiku", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 2, TaskFit: []string{"implement"}},
		{Slug: "a", Tier: "haiku", Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 2, TaskFit: []string{"test"}},
	}
	if err := sortTableModelsWithRanking(rows, "utility", false, rankingTier); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "a" {
		t.Fatalf("task fit affected tie-break: %+v", rows)
	}
}

func TestSortTableModelsExplicitSlugRemainsAscending(t *testing.T) {
	models := []model.Model{{Slug: "z"}, {Slug: "a"}}
	if err := sortTableModels(models, "slug", false); err != nil || models[0].Slug != "a" {
		t.Fatalf("explicit slug sort = %+v, err=%v", models, err)
	}
}

func TestSortTableModelsSupportsShortAliases(t *testing.T) {
	models := []model.Model{
		{Slug: "expensive", MixedPrice: 4, Score: &model.ScoreInfo{Value: 90}, Rankable: true, QualityPrice: 22.5},
		{Slug: "cheap", MixedPrice: 1, Score: &model.ScoreInfo{Value: 80}, Rankable: true, QualityPrice: 80},
	}
	for _, test := range []struct {
		key, want string
	}{
		{key: "Q", want: "expensive"},
		{key: "P", want: "cheap"},
		{key: "QP", want: "cheap"},
	} {
		copy := append([]model.Model(nil), models...)
		ranking := rankingDefault
		if test.key == "QP" {
			ranking = rankingLegacy
		}
		if err := sortTableModelsWithRanking(copy, test.key, false, ranking); err != nil || copy[0].Slug != test.want {
			t.Errorf("sort %q = %+v, err=%v; first slug = %q, want %q", test.key, copy, err, copy[0].Slug, test.want)
		}
	}
}

func TestFilterTableModelsUsesANDSemantics(t *testing.T) {
	models := []model.Model{
		{Slug: "match", Tier: "sonnet", Context: 128000, InPerM: 1, OutPerM: 2, Score: &model.ScoreInfo{Value: 90}, Rankable: true},
		{Slug: "wrong-tier", Tier: "opus", Context: 128000, InPerM: 1, OutPerM: 2, Score: &model.ScoreInfo{Value: 90}, Rankable: true},
		{Slug: "wrong-score", Tier: "sonnet", Context: 128000, InPerM: 1, OutPerM: 2, Score: &model.ScoreInfo{Value: 80}, Rankable: true},
	}
	filtered, err := filterTableModels(models, []string{"paid", "tier:sonnet", "quality>=90", "context>=100000", "input<=1", "output<=2", "scored"})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Slug != "match" {
		t.Fatalf("filtered = %+v, want only match", filtered)
	}
}

func TestFilterTableModelsAcceptsQualityPercentAndFraction(t *testing.T) {
	models := []model.Model{
		{Slug: "high", Score: &model.ScoreInfo{Value: 80}, Rankable: true},
		{Slug: "low", Score: &model.ScoreInfo{Value: 79.9}, Rankable: true},
	}
	for _, filter := range []string{"quality>=80", "quality>=0.8"} {
		filtered, err := filterTableModels(models, []string{filter})
		if err != nil || len(filtered) != 1 || filtered[0].Slug != "high" {
			t.Errorf("quality filter %q = %+v, err=%v; want high only", filter, filtered, err)
		}
	}
}

func TestFilterTableModelsRejectsQualityOutsideDocumentedBounds(t *testing.T) {
	for _, filter := range []string{"quality>=-0.1", "quality>=100.1", "quality>=101"} {
		if _, err := filterTableModels(nil, []string{filter}); err == nil || !strings.Contains(err.Error(), "between 0 and 100") {
			t.Errorf("quality filter %q error = %v, want a clear bounds error", filter, err)
		}
	}
}

func TestFilterTableModelsQualityOneMeansOneHundredPercent(t *testing.T) {
	models := []model.Model{
		{Slug: "full", Score: &model.ScoreInfo{Value: 100}, Rankable: true},
		{Slug: "partial", Score: &model.ScoreInfo{Value: 99.9}, Rankable: true},
	}
	filtered, err := filterTableModels(models, []string{"quality>=1"})
	if err != nil || len(filtered) != 1 || filtered[0].Slug != "full" {
		t.Fatalf("quality>=1 = %+v, err=%v; want full only", filtered, err)
	}
}

func TestFilterTableModelsSplitsRepeatedCommaSeparatedFilters(t *testing.T) {
	models := []model.Model{
		{Slug: "match", Tier: "sonnet", Free: false, Score: &model.ScoreInfo{Value: 80}, Rankable: true},
		{Slug: "wrong-tier", Tier: "opus", Free: false, Score: &model.ScoreInfo{Value: 80}, Rankable: true},
		{Slug: "wrong-quality", Tier: "sonnet", Free: false, Score: &model.ScoreInfo{Value: 79}, Rankable: true},
	}
	filtered, err := filterTableModels(models, []string{"paid,quality>=80", "tier:sonnet"})
	if err != nil || len(filtered) != 1 || filtered[0].Slug != "match" {
		t.Fatalf("comma/repeated filters = %+v, err=%v; want match only", filtered, err)
	}
}

func TestTableCommandSplitsRepeatedCommaSeparatedFilters(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")
	output := executeCLI(t, "table", "--config", config, "--no-pager", "--slug", "--filter", "paid,quality>=80", "--filter", "tier:sonnet")
	if !strings.Contains(output, "demo/high") || strings.Contains(output, "demo/low") || strings.Contains(output, "demo/missing") {
		t.Fatalf("CLI comma/repeated filters output = %s; want demo/high only", output)
	}
}

func TestTableCommandQualityFilterUsesActiveScoreSource(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyScoreSourceFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")
	swe := executeCLI(t, "table", "--config", config, "--no-pager", "--slug", "--filter", "quality>=0.6")
	if !strings.Contains(swe, "demo/swe") || !strings.Contains(swe, "demo/both") || strings.Contains(swe, "demo/arena") {
		t.Fatalf("SWE-bench quality filter output = %s; want swe and both", swe)
	}
	arena := executeCLI(t, "table", "--config", config, "--no-pager", "--slug", "--score-source=arena", "--filter", "quality>=0.8")
	if !strings.Contains(arena, "demo/both") || strings.Contains(arena, "demo/swe") || strings.Contains(arena, "demo/arena") {
		t.Fatalf("Arena quality filter output = %s; want both only", arena)
	}
}

func TestFilterTableModelsRejectsMalformedAndUnknownFilters(t *testing.T) {
	for _, filter := range []string{"tier:", "quality>=bad", "quality>=NaN", "quality>=+Inf", "quality>=-Inf", "input<=NaN", "output<=+Inf", "context>=bad", "input<=bad", "unknown"} {
		if _, err := filterTableModels(nil, []string{filter}); err == nil || !strings.Contains(err.Error(), "filter") {
			t.Errorf("filter %q error = %v, want filter error", filter, err)
		}
	}
}

func TestFilterTableModelsAcceptsSignedFiniteNonQualityThresholds(t *testing.T) {
	models := []model.Model{{Slug: "negative", Context: -1, InPerM: -1, OutPerM: -1}}
	if filtered, err := filterTableModels(models, []string{"context>=-1", "input<=-1", "output<=-1"}); err != nil || len(filtered) != 1 {
		t.Fatalf("signed finite filters = %+v, err=%v; want one matching model", filtered, err)
	}
}

func TestTableLimitShorthandParsesNegativeLookingNumbers(t *testing.T) {
	flags := pflag.NewFlagSet("table", pflag.ContinueOnError)
	limit := -1
	flags.IntVarP(&limit, "limit", "n", -1, "")
	for _, test := range []struct {
		arg  string
		want int
	}{
		{arg: "-1", want: 1},
		{arg: "-20", want: 20},
		{arg: "-0", want: 0},
	} {
		flags := pflag.NewFlagSet("table", pflag.ContinueOnError)
		limit := -1
		flags.IntVarP(&limit, "limit", "n", -1, "")
		if err := parseTableArgs([]string{test.arg}, flags); err != nil {
			t.Fatalf("shorthand %q parse: %v", test.arg, err)
		}
		if limit != test.want {
			t.Errorf("shorthand %q limit = %d, want %d", test.arg, limit, test.want)
		}
	}
}

func TestTableLimitShorthandDoesNotRewriteFlagValues(t *testing.T) {
	for _, args := range [][]string{{"--limit", "-1"}, {"--sort", "-1"}, {"-s", "-1"}, {"--filter", "-1"}, {"-f", "-1"}} {
		flags := pflag.NewFlagSet("table", pflag.ContinueOnError)
		limit, sortKey := -1, ""
		filters := []string(nil)
		flags.IntVarP(&limit, "limit", "n", -1, "")
		flags.StringVarP(&sortKey, "sort", "s", "q/p", "")
		flags.StringArrayVarP(&filters, "filter", "f", nil, "")
		if err := parseTableArgs(args, flags); err != nil {
			t.Fatalf("flag value %v parse: %v", args, err)
		}
		if args[0] == "--limit" && limit != -1 || (args[0] == "--sort" || args[0] == "-s") && sortKey != "-1" || (args[0] == "--filter" || args[0] == "-f") && !reflect.DeepEqual(filters, []string{"-1"}) {
			t.Errorf("flag value %v was rewritten: limit=%d sort=%q filter=%v", args, limit, sortKey, filters)
		}
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
		{key: "quality", want: "a/model"},
		{key: "q/p", want: "a/model"},
	}
	for _, test := range tests {
		models := append([]model.Model(nil), base...)
		ranking := rankingDefault
		if test.key == "q/p" {
			ranking = rankingLegacy
		}
		if err := sortTableModelsWithRanking(models, test.key, false, ranking); err != nil || models[0].Slug != test.want {
			t.Errorf("%s sort = %+v, err=%v", test.key, models, err)
		}
	}
}

func TestSortTableModelsMissingNumericValuesGoLast(t *testing.T) {
	models := []model.Model{{Slug: "missing"}, {Slug: "scored", Score: &model.ScoreInfo{Value: 1}, Rankable: true, QualityPrice: 1}}
	if err := sortTableModels(models, "quality", false); err != nil || models[1].Slug != "missing" {
		t.Fatalf("quality missing placement = %+v, err=%v", models, err)
	}
	if err := sortTableModels(models, "q/p", false); err != nil || models[1].Slug != "missing" {
		t.Fatalf("q/p missing placement = %+v, err=%v", models, err)
	}
}

func TestSortTableModelsQualityBoundariesAndMissingValues(t *testing.T) {
	newModels := func() []model.Model {
		return []model.Model{
			{Slug: "missing"},
			{Slug: "not-rankable", Score: &model.ScoreInfo{Value: 99}, Rankable: false},
			{Slug: "below", Score: &model.ScoreInfo{Value: 59.9}, Rankable: true},
			{Slug: "boundary-low", Score: &model.ScoreInfo{Value: 60}, Rankable: true},
			{Slug: "boundary-high", Score: &model.ScoreInfo{Value: 70}, Rankable: true},
		}
	}

	models := newModels()
	if err := sortTableModels(models, "quality", false); err != nil {
		t.Fatalf("descending quality sort: %v", err)
	}
	if got := []string{models[0].Slug, models[1].Slug, models[2].Slug, models[3].Slug, models[4].Slug}; !reflect.DeepEqual(got, []string{"boundary-high", "boundary-low", "below", "missing", "not-rankable"}) {
		t.Fatalf("descending quality sort = %v, want rankable boundaries first and missing values last", got)
	}

	models = newModels()
	if err := sortTableModels(models, "quality", true); err != nil {
		t.Fatalf("ascending quality sort: %v", err)
	}
	if got := []string{models[0].Slug, models[1].Slug, models[2].Slug, models[3].Slug, models[4].Slug}; !reflect.DeepEqual(got, []string{"below", "boundary-low", "boundary-high", "missing", "not-rankable"}) {
		t.Fatalf("ascending quality sort = %v, want rankable boundaries first and missing values last", got)
	}
}

func TestTableCLIQualitySortAliasesAndFlags(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	tests := []struct {
		name      string
		args      []string
		firstSlug string
	}{
		{name: "long quality", args: []string{"--sort", "quality"}, firstSlug: "demo/high"},
		{name: "q alias", args: []string{"--sort", "q"}, firstSlug: "demo/high"},
		{name: "short Q alias", args: []string{"-s", "Q"}, firstSlug: "demo/high"},
		{name: "reverse quality", args: []string{"--sort", "quality", "-R", "-n", "1"}, firstSlug: "demo/low"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"table", "--config", config, "--no-pager", "-S"}, test.args...)
			output := executeCLI(t, args...)
			if got := tableFirstCell(output, 0); got != test.firstSlug {
				t.Fatalf("first table slug = %q, want %q:\n%s", got, test.firstSlug, output)
			}
			if test.name == "reverse quality" && !strings.Contains(output, "| Slug ") {
				t.Fatalf("-S did not select the Slug column:\n%s", output)
			}
			if test.name == "reverse quality" && strings.Count(output, "| demo/") != 1 {
				t.Fatalf("-n 1 did not leave one model row:\n%s", output)
			}
		})
	}
}

func TestTableCLIDefaultSortUsesMixedUtility(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyTableFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	output := executeCLI(t, "table", "--config", config, "--no-pager", "-S")
	if got := tableFirstCell(output, 0); got != "demo/high" {
		t.Fatalf("default mixed-utility first table slug = %q, want demo/high:\n%s", got, output)
	}

	output = executeCLI(t, "table", "--config", config, "--no-pager", "-S", "--ranking=legacy")
	if got := tableFirstCell(output, 0); got != "demo/low" {
		t.Fatalf("explicit legacy first table slug = %q, want demo/low:\n%s", got, output)
	}
}

func tableFirstCell(output string, column int) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.Contains(line, "| Slug ") || strings.Contains(line, "| Name ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) > column {
			return strings.TrimSpace(cells[column])
		}
	}
	return ""
}

func TestSortTableModelsRejectsUnknownKey(t *testing.T) {
	if err := sortTableModels(nil, "bogus", false); err == nil || !strings.Contains(err.Error(), tableSortHelp) {
		t.Fatalf("error = %v, want allowed sort values", err)
	}
}

func TestSortQPIsIndependentOfRankingMode(t *testing.T) {
	rows := []model.Model{
		{Slug: "xiaomi/mimo", Tier: "sonnet", Score: &model.ScoreInfo{Value: 90}, Rankable: true, InPerM: 0.3, OutPerM: 1.4529411764705883},
		{Slug: "other/thirty-three", Tier: "sonnet", Score: &model.ScoreInfo{Value: 99}, Rankable: true, InPerM: 2.25, OutPerM: 5.25},
		{Slug: "other/eight", Tier: "sonnet", Score: &model.ScoreInfo{Value: 100}, Rankable: true, InPerM: 10, OutPerM: 20},
		{Slug: "free/model", Tier: "free", Free: true, Score: &model.ScoreInfo{Value: 100}, Rankable: true},
		{Slug: "other/missing"},
		{Slug: "other/unrankable", Tier: "sonnet", InPerM: 1, OutPerM: 3},
	}
	if err := sortTableModelsWithRanking(rows, "q/p", false, rankingMixed); err != nil {
		t.Fatal(err)
	}
	if got := []string{rows[0].Slug, rows[1].Slug, rows[2].Slug}; !reflect.DeepEqual(got, []string{"xiaomi/mimo", "other/thirty-three", "other/eight"}) {
		t.Fatalf("q/p sort = %v, want utility-per-price descending with missing last", got)
	}
	for _, row := range rows[3:] {
		if row.Score != nil && row.Rankable && !row.Free {
			t.Fatalf("unrankable/missing-last row became rankable: %+v", row)
		}
	}
	if math.Abs(rows[0].QualityPrice-153) > 0.2 {
		t.Fatalf("Xiaomi raw quality-per-price = %v, want approximately 153 from benchmark quality only", rows[0].QualityPrice)
	}
	if err := sortTableModelsWithRanking(rows, "q/p", true, rankingTier); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "other/eight" || rows[1].Slug != "other/thirty-three" || rows[2].Slug != "xiaomi/mimo" {
		t.Fatalf("reverse q/p sort = %+v, want ascending rankable values and missing last", rows)
	}
}

func TestCanonicalQPUsesProjectedSWEBenchOrArenaScore(t *testing.T) {
	rows := []model.Model{
		{Slug: "swebench/model", Tier: "sonnet", Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 80}, Rankable: true, InPerM: 1, OutPerM: 3},
		{Slug: "arena/model", Tier: "sonnet", Score: &model.ScoreInfo{Metric: "LMArena Elo normalized", Value: 40}, Rankable: true, InPerM: 1, OutPerM: 3},
	}
	if err := sortTableModelsWithRanking(rows, "q/p", false, rankingMixed); err != nil {
		t.Fatal(err)
	}
	if rows[0].Slug != "swebench/model" || rows[0].QualityPrice <= rows[1].QualityPrice {
		t.Fatalf("source-aware Q/P = %+v, want the projected higher score first", rows)
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
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/high\ttier=sonnet\ndemo/low\ttier=haiku\ndemo/missing\ttier=free\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/high:\n    display: Demo High\n    note: Local fixture\n    task_fit: [implement, test]\n    claude_ref: '**<≈ Haiku 4.5** | бесплатная, середина диапазона, уточнение漢字'\n  demo/low:\n    display: Demo Low\n  demo/missing:\n    display: Demo Missing\n"), 0o644); err != nil {
		return err
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{
		"demo/high":    {InPerM: 100, OutPerM: 100, Context: 128000, Score: &model.ScoreInfo{Value: 90, Unit: "%", VariantMeasured: "demo/high", IdentityStatus: model.IdentityExact}},
		"demo/low":     {InPerM: 1, OutPerM: 1, Context: 128000, Score: &model.ScoreInfo{Value: 10, Unit: "%", VariantMeasured: "demo/low", IdentityStatus: model.IdentityExact}},
		"demo/missing": {InPerM: 1, OutPerM: 1, Context: 128000},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644)
}

func removeTableSnapshot(root string) error {
	return os.Remove(filepath.Join(root, "cache", "last-run-snapshot.json"))
}

func copyScoreSourceFixture(t *testing.T, root string) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/swe\ttier=sonnet\tvals=demo/swe\ndemo/arena\ttier=sonnet\tarena=demo-arena\ndemo/both\ttier=sonnet\tvals=demo/both\tarena=demo-both\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/swe:\n    display: Demo SWE\n  demo/arena:\n    display: Demo Arena\n  demo/both:\n    display: Demo Both\n"), 0o644); err != nil {
		return err
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{
		"demo/swe":   {InPerM: 1, OutPerM: 3, Context: 128000, Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 70, Unit: "%", VariantMeasured: "demo/swe", IdentityStatus: model.IdentityExact}},
		"demo/arena": {InPerM: 1, OutPerM: 3, Context: 128000, ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1300, SourceFamily: model.ScoreSourceArena, ConfiguredIdentity: "demo-arena", CanonicalID: "demo-arena", VariantMeasured: "demo/arena", Unit: "Elo", IdentityStatus: model.IdentityExact}},
		"demo/both":  {InPerM: 1, OutPerM: 3, Context: 128000, Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 60, Unit: "%", VariantMeasured: "demo/both", IdentityStatus: model.IdentityExact}, ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1500, SourceFamily: model.ScoreSourceArena, ConfiguredIdentity: "demo-both", CanonicalID: "demo-both", VariantMeasured: "demo/both", Unit: "Elo", IdentityStatus: model.IdentityExact}},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644)
}

func TestTableScoreSourceSwitchesTheWholeView(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyScoreSourceFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	swe := executeCLI(t, "table", "--config", config)
	if !strings.Contains(swe, "70.0%") || !strings.Contains(swe, "60.0%") {
		t.Errorf("default view lost its SWE-bench numbers:\n%s", swe)
	}
	if strings.Contains(swe, "Elo") {
		t.Errorf("default view leaked an Arena number:\n%s", swe)
	}

	arena := executeCLI(t, "table", "--config", config, "--score-source=arena")
	if !strings.Contains(arena, "1300 Elo") || !strings.Contains(arena, "1500 Elo") {
		t.Errorf("arena view lost its Elo numbers:\n%s", arena)
	}
	if strings.Contains(arena, "70.0%") || strings.Contains(arena, "60.0%") {
		t.Errorf("arena view leaked a SWE-bench number:\n%s", arena)
	}
	if !strings.Contains(arena, "Score source: arena") {
		t.Errorf("arena view does not say which scale it is on:\n%s", arena)
	}
}

func TestTableScoreSourceRejectsUnknownValues(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyScoreSourceFixture(t, root); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"auto", "lmarena", ""} {
		err := executeCLIError(t, "table", "--config", config, "--score-source="+value)
		if err == nil || !strings.Contains(err.Error(), "invalid --score-source") {
			t.Errorf("--score-source=%q error = %v, want a rejection; there is deliberately no auto mode", value, err)
		}
	}
}

func TestTableScoreSourceRanksByTheActiveSource(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyScoreSourceFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	arena := executeCLI(t, "table", "--config", config, "--score-source=arena", "--slug")
	both := strings.Index(arena, "demo/both")
	only := strings.Index(arena, "demo/arena")
	swe := strings.Index(arena, "demo/swe")
	if both < 0 || only < 0 || swe < 0 {
		t.Fatalf("a tracked model is missing from the arena view:\n%s", arena)
	}
	if !(both < only) {
		t.Errorf("demo/both (1500 Elo) must outrank demo/arena (1300 Elo):\n%s", arena)
	}
	if !(only < swe) {
		t.Errorf("a row with no Arena number must sink below the ranked ones:\n%s", arena)
	}
}

// copyScoreSourceClaudeFixture is a two-model, haiku-tier-only fixture for
// exercising ClaudeEquivalent's percentage thresholds specifically: one row
// has only a SWE-bench number, the other only an Arena number, so the two
// modes disagree about which row is "scored" and which threshold branch (if
// any) may run.
func copyScoreSourceClaudeFixture(t *testing.T, root string) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/haiku-swe\ttier=haiku\tvals=demo/haiku-swe\ndemo/haiku-arena\ttier=haiku\tarena=demo-haiku-arena\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/haiku-swe:\n    display: Demo Haiku SWE\n  demo/haiku-arena:\n    display: Demo Haiku Arena\n"), 0o644); err != nil {
		return err
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{
		"demo/haiku-swe":   {InPerM: 1, OutPerM: 3, Context: 128000, Score: &model.ScoreInfo{Metric: "SWE-bench Verified", Value: 75}},
		"demo/haiku-arena": {InPerM: 1, OutPerM: 3, Context: 128000, ArenaScore: &model.ScoreInfo{Metric: "LMArena Elo", Value: 1400, SourceFamily: model.ScoreSourceArena, ConfiguredIdentity: "demo-haiku-arena", CanonicalID: "demo-haiku-arena", VariantMeasured: "demo-haiku-arena"}},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644)
}

// TestLoadLocalModelsForSourceRestoresCatalogueMetadata closes the loop
// the TUI actually runs: it never reads a live model.Model, it re-derives
// every row from the on-disk snapshot. A field that survives NewSnapshot
// but is not rebuilt here reaches the detail screen as a permanent н/д.
func TestLoadLocalModelsForSourceRestoresCatalogueMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/dated\ttier=sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/dated:\n    display: Demo Dated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := refresh.Snapshot{Models: map[string]refresh.SnapshotEntry{
		"demo/dated": {
			InPerM: 1, OutPerM: 3, Context: 128000, Created: 1786034890, Description: "Demo prose.",
			CanonicalSlug: "demo/dated-20260804", HuggingFaceID: "demo-labs/Dated",
		},
	}}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cache", "last-run-snapshot.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	models, err := loadLocalModelsForSource(root, scoreSourceDefault)
	if err != nil {
		t.Fatalf("loadLocalModelsForSource: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1: %+v", len(models), models)
	}
	if models[0].Created != 1786034890 || models[0].Description != "Demo prose." {
		t.Errorf("row = %+v, want Created/Description rebuilt from the snapshot entry", models[0])
	}
	if models[0].CanonicalSlug != "demo/dated-20260804" || models[0].HuggingFaceID != "demo-labs/Dated" {
		t.Errorf("row = %+v, want the link identifiers rebuilt from the snapshot entry: this is the path the TUI actually runs, and a field missing here is a permanent н/д on the detail screen", models[0])
	}
}

// TestTableScoreSourceClaudeColumnNeverBlendsScales guards against exactly
// the bug a review caught live: ClaudeEquivalent's haiku/free thresholds
// (>=70, >=60) are calibrated on SWE-bench Verified percentage points. After
// projection through model.ForScoreSource, an arena-mode Score.Value holds a
// min-max-normalized Arena position instead, so applying those thresholds to
// it silently reads an Elo rank as if it were a SWE-bench score — the exact
// cross-scale blending --score-source exists to prevent. A row with a real
// SWE-bench score and no Arena data must not claim a SWE-bench-calibrated
// Claude tier once the view has moved to Arena: its Status cell already says
// н/д, and the Claude cell must agree rather than contradict it. A row that
// does have real Arena data fares no better: there is no established mapping
// from a normalized Arena position onto a Claude tier, so the threshold
// logic must not run for it either in arena mode.
func TestTableScoreSourceClaudeColumnNeverBlendsScales(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyScoreSourceClaudeFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	swe := executeCLI(t, "table", "--config", config, "--slug")
	if got := tableRowCell(t, swe, "demo/haiku-swe", 1); got != "≈ Haiku 4.5" {
		t.Errorf("swebench-mode Claude cell for demo/haiku-swe (score 75 >= 70) = %q, want %q:\n%s", got, "≈ Haiku 4.5", swe)
	}

	arena := executeCLI(t, "table", "--config", config, "--score-source=arena", "--slug")
	if got := tableRowCell(t, arena, "demo/haiku-swe", 2); got != "n/a" {
		t.Fatalf("test setup: demo/haiku-swe Status in arena mode = %q, want н/д (it has no Arena number):\n%s", got, arena)
	}
	if got := tableRowCell(t, arena, "demo/haiku-swe", 1); got != "n/a" {
		t.Errorf("arena-mode Claude cell for demo/haiku-swe = %q, want %q; it must not claim a SWE-bench-calibrated tier next to a н/д Status:\n%s", got, "н/д", arena)
	}
	if got := tableRowCell(t, arena, "demo/haiku-arena", 2); got != "1400 Elo" {
		t.Fatalf("test setup: demo/haiku-arena Status in arena mode = %q, want 1400 Elo:\n%s", got, arena)
	}
	if got := tableRowCell(t, arena, "demo/haiku-arena", 1); got != "n/a" {
		t.Errorf("arena-mode Claude cell for demo/haiku-arena = %q, want %q; there is no established Elo-to-Claude-tier mapping:\n%s", got, "н/д", arena)
	}
}

// TestTableScoreSourceFilterScoredTracksTheActiveSource pins down that
// --filter scored (and, by the same mechanism, quality>=N) reads whichever
// source is currently projected onto Score/Rankable, so its meaning silently
// changes with --score-source: "has a SWE-bench number" in swebench mode,
// "has an Arena number" in arena mode. This is existing, already-correct
// behavior (ForScoreSource runs before filterTableModels); the test exists so
// a future refactor cannot regress it unnoticed.
func TestTableScoreSourceFilterScoredTracksTheActiveSource(t *testing.T) {
	root := t.TempDir()
	config := writeConfig(t, "data_dir: "+root+"\n")
	if err := copyScoreSourceFixture(t, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLUMNS", "120")

	arena := executeCLI(t, "table", "--config", config, "--score-source=arena", "--filter", "scored", "--slug")
	if strings.Contains(arena, "demo/swe") {
		t.Errorf("arena --filter scored kept demo/swe, which has no Arena number:\n%s", arena)
	}
	if !strings.Contains(arena, "demo/arena") || !strings.Contains(arena, "demo/both") {
		t.Errorf("arena --filter scored dropped a row that does have an Arena number:\n%s", arena)
	}

	swe := executeCLI(t, "table", "--config", config, "--filter", "scored", "--slug")
	if strings.Contains(swe, "demo/arena") {
		t.Errorf("swebench --filter scored kept demo/arena, which has no SWE-bench number:\n%s", swe)
	}
	if !strings.Contains(swe, "demo/swe") || !strings.Contains(swe, "demo/both") {
		t.Errorf("swebench --filter scored dropped a row that does have a SWE-bench number:\n%s", swe)
	}
}

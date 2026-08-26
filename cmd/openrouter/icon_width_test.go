package main

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
)

// The three default manufacturer icons below are bare "circled Latin letter"
// glyphs from the Enclosed Alphanumerics block. They carry no
// variation-selector-16 and no Emoji_Presentation property, so every Unicode
// width table — mattn/go-runewidth and charmbracelet/x/ansi alike — reports
// width 1 for them. Real terminals (macOS Terminal.app, iTerm2) have no glyph
// for them in the primary monospace face and fall back to an emoji font, which
// paints them across two cells.
//
// That divergence is not a Unicode question a library can answer correctly; it
// is observed terminal behaviour, so the width oracle needs an explicit
// override for exactly these runes. Without it the table pads one cell too
// many after the icon (misaligned columns, the user-visible bug) and the TUI
// lays out rows one cell wider than the terminal can hold.
var bareCircledIcons = []struct {
	name, icon string
}{
	{"z.ai", "Ⓩ"},   // Ⓩ
	{"nvidia", "Ⓝ"}, // Ⓝ
	{"xiaomi", "ⓧ"}, // ⓧ
}

func TestBareCircledManufacturerIconsMeasureTwoCells(t *testing.T) {
	for _, tc := range bareCircledIcons {
		t.Run(tc.name, func(t *testing.T) {
			if got := tableDisplayWidth(tc.icon); got != 2 {
				t.Fatalf("tableDisplayWidth(%q) = %d, want 2: real terminals render this glyph through emoji font fallback across two cells", tc.icon, got)
			}
		})
	}
}

// TestManufacturerIconSlotWidthIsUniformAcrossUnicodeCategories is the
// end-to-end alignment guard the reported bug asks for: whatever Unicode
// category an icon belongs to, the identity cell must reach the manufacturer
// name at the same column. It deliberately mixes the three categories that
// exercise three different code paths in the width oracle:
//
//   - a plain 2-wide emoji, which every width table already gets right;
//   - a VS16-forced emoji ("Ⓜ️"), where go-runewidth said 1 and ansi says 2 —
//     the case commit eb5885a fixed by switching oracles;
//   - a bare circled letter, where both libraries say 1 and only real
//     terminals say 2 — the case this test was written for.
func TestManufacturerIconPrefixWidthIsUniformAcrossUnicodeCategories(t *testing.T) {
	icons := []struct{ name, icon string }{
		{"plain-emoji", "\U0001f310"},             // 🌐
		{"vs16-emoji", "Ⓜ️"},                      // Ⓜ️
		{"bare-circled-zai", "Ⓩ"},                 // Ⓩ
		{"bare-circled-nvidia", "Ⓝ"},              // Ⓝ
		{"bare-circled-xiaomi", "ⓧ"},              // ⓧ
		{"cjk", "界"},                              // 界
		{"ascii", "x"},                            //
		{"zwj-sequence", "\U0001f469‍\U0001f4bb"}, // 👩‍💻
	}
	for _, gap := range []int{0, 1, 3} {
		for _, tc := range icons {
			t.Run(tc.name+"/gap-"+strconv.Itoa(gap), func(t *testing.T) {
				cfg := config.IconConfig{Manufacturers: map[string]string{"acme": tc.icon}, Unknown: "?"}
				row := model.Model{DisplayName: "Acme Model", Owner: "Acme"}
				identity := modelIdentityWithIconsAndGap(row, cfg, gap)
				index := strings.Index(identity, "Acme")
				if index < 0 {
					t.Fatalf("identity %q lost the manufacturer name", identity)
				}
				if got, want := tableDisplayWidth(identity[:index]), manufacturerIconSlotWidth+gap; got != want {
					t.Fatalf("prefix width before the manufacturer name = %d, want %d (icon %q, identity %q)", got, want, tc.icon, identity)
				}
			})
		}
	}
}

// iconLineWidth/iconLineHeight are the synthetic terminal geometry used by
// TestRendererTreatsBareCircledIconLineAsFullWidth below.
const (
	iconLineWidth  = 40
	iconLineHeight = 6
)

// bareCircledIconFullLine builds a line whose real, on-screen width is exactly
// w cells: one bare circled icon (two cells on a real terminal) plus w-2 dots.
func bareCircledIconFullLine(w int) string {
	return "Ⓩ" + strings.Repeat(".", w-2)
}

// bareCircledIconModel paints nothing but full-width icon lines, so the
// renderer's own erase-line decision for such a line becomes observable in the
// raw bytes it writes.
type bareCircledIconModel struct{ w, h int }

func (m bareCircledIconModel) Init() tea.Cmd { return nil }

func (m bareCircledIconModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.w, m.h = size.Width, size.Height
	}
	return m, nil
}

func (m bareCircledIconModel) View() string {
	lines := make([]string, m.h)
	for i := range lines {
		lines[i] = bareCircledIconFullLine(m.w)
	}
	return strings.Join(lines, "\n")
}

// TestRendererTreatsBareCircledIconLineAsFullWidth polices the second half of
// the fix, in the patched renderer
// (internal/thirdparty/bubbletea-patched/standard_renderer.go).
//
// flush() appends ansi.EraseLineRight to a painted line only when it believes
// the line is narrower than the terminal — because on a line that fills the
// terminal the cursor sits in the last cell with wrap pending, and an erase
// there wipes the character just painted instead of clearing empty space.
//
// A line that is genuinely full width but *measured* one cell short is exactly
// that hazard: the renderer erases the last cell of every such row. Correcting
// the layout oracle alone would create it, since the layout would then produce
// truly full-width rows for these icons while the renderer still measured them
// short. So the renderer must share the corrected belief.
func TestRendererTreatsBareCircledIconLineAsFullWidth(t *testing.T) {
	rp := startRuntimeProgramForModel(t, bareCircledIconModel{w: iconLineWidth, h: iconLineHeight}, iconLineWidth, iconLineHeight)
	writes := drainRuntimeWritesUntilQuiet(rp, resizeStampQuietWindow)
	rp.Quit(t, resizeStampSyncTimeout)

	line := bareCircledIconFullLine(iconLineWidth)
	painted := false
	for _, write := range writes {
		if strings.Contains(write, line) {
			painted = true
		}
		if strings.Contains(write, line+"\x1b[K") {
			t.Fatalf("renderer appended erase-line-right to a full-width line: it measured %q as narrower than the %d-column terminal, so it would wipe the last painted cell", line, iconLineWidth)
		}
	}
	if !painted {
		t.Fatalf("renderer never painted the icon line; drained %d writes: %q", len(writes), writes)
	}
}

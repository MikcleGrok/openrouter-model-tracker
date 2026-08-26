package main

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
)

// variationSelector16 forces emoji presentation on a base character that would
// otherwise be drawn as text. An icon carrying it has already settled the
// question this file is about, so it is exempt from the go-runewidth half of
// the agreement check below.
const variationSelector16 = '\ufe0f'

// defaultIconValues is every icon this program ships as a default: one per
// manufacturer plus the unknown-manufacturer fallback.
func defaultIconValues() []string {
	icons := config.DefaultIconConfig()
	values := make([]string, 0, len(icons.Manufacturers)+1)
	for _, icon := range icons.Manufacturers {
		values = append(values, icon)
	}
	return append(values, icons.Unknown)
}

func hasVariationSelector16(value string) bool {
	return strings.ContainsRune(value, variationSelector16)
}

// TestDefaultManufacturerIconsAreUnambiguouslyTwoCellsWide is the invariant
// every default icon has to satisfy, and it is the whole reason the icons for
// z.ai, NVIDIA and Xiaomi changed.
//
// Those three used to be bare circled Latin letters from the Enclosed
// Alphanumerics block ("Ⓩ", "Ⓝ", "ⓧ"). They carry neither
// variation-selector-16 nor the Emoji_Presentation property, so East_Asian_Width
// calls them Ambiguous and every width table reports 1 — while macOS
// Terminal.app and iTerm2, which have no glyph for them in the primary
// monospace face, fall back to an emoji font and paint them across two cells.
// A terminal following the Unicode tables paints one. There is no width the
// program can believe that is right on both, so the icons themselves had to go.
//
// The replacements, like every other default icon, are unambiguous:
//
//   - ansi.StringWidth — the oracle the CLI table, the TUI layout and the
//     patched renderer all measure with — must say 2, so the icon fills the
//     manufacturer slot on its own with no compensating padding;
//   - mattn/go-runewidth must agree, unless the icon settles its presentation
//     with an explicit variation-selector-16 (the "Ⓜ️" case commit eb5885a
//     dealt with). Two independent tables agreeing on a bare glyph is what
//     rules out the font-fallback divergence above.
func TestDefaultManufacturerIconsAreUnambiguouslyTwoCellsWide(t *testing.T) {
	for _, icon := range defaultIconValues() {
		t.Run(icon, func(t *testing.T) {
			if got := ansi.StringWidth(icon); got != manufacturerIconSlotWidth {
				t.Errorf("ansi.StringWidth(%q) = %d, want %d: a default icon must fill the manufacturer slot on its own", icon, got, manufacturerIconSlotWidth)
			}
			if got := tableDisplayWidth(icon); got != manufacturerIconSlotWidth {
				t.Errorf("tableDisplayWidth(%q) = %d, want %d", icon, got, manufacturerIconSlotWidth)
			}
			if hasVariationSelector16(icon) {
				return
			}
			if got := runewidth.StringWidth(icon); got != manufacturerIconSlotWidth {
				t.Errorf("runewidth.StringWidth(%q) = %d, want %d: a bare icon that any width table calls narrow has no Emoji_Presentation to fall back on, so whether a terminal paints it across one cell or two comes down to the font stack — that cannot be a default", icon, got, manufacturerIconSlotWidth)
			}
		})
	}
}

// TestDefaultIconRowsAlignWithAPlainWideEmojiRow is the regression guard for
// the reported bug: column separators drifting by one cell on the rows of
// certain manufacturers.
//
// The reference icon is a plain Emoji_Presentation glyph that every width
// table and every terminal has always agreed measures two cells. Every default
// icon must produce byte-identical layout around it: the identity prefix is
// exactly the icon followed by the configured gap and nothing else, and it
// reaches the manufacturer name at the same column as the reference row.
//
// The "and nothing else" half is the part that matters. An icon the tables
// call one cell wide still lines up under manufacturerIconSlot, because the
// slot pads it with a compensating space — self-consistent on paper, and
// exactly one cell wrong on a terminal that paints the glyph at two.
func TestDefaultIconRowsAlignWithAPlainWideEmojiRow(t *testing.T) {
	const referenceIcon = "🌀"
	row := model.Model{DisplayName: "Acme Model", Owner: "Acme"}
	identityFor := func(icon string, gap int) string {
		cfg := config.IconConfig{Manufacturers: map[string]string{"acme": icon}, Unknown: "?"}
		return modelIdentityWithIconsAndGap(row, cfg, gap)
	}
	prefixOf := func(t *testing.T, identity string) string {
		t.Helper()
		index := strings.Index(identity, "Acme")
		if index < 0 {
			t.Fatalf("identity %q lost the manufacturer name", identity)
		}
		return identity[:index]
	}
	for _, icon := range defaultIconValues() {
		for _, gap := range []int{0, 1, 3} {
			t.Run(icon+"/gap-"+strconv.Itoa(gap), func(t *testing.T) {
				prefix := prefixOf(t, identityFor(icon, gap))
				if want := icon + strings.Repeat(" ", gap); prefix != want {
					t.Fatalf("identity prefix bytes % x, want % x: the icon slot padded the icon, which only aligns on a terminal that agrees the icon is narrow", []byte(prefix), []byte(want))
				}
				reference := prefixOf(t, identityFor(referenceIcon, gap))
				if got, want := ansi.StringWidth(prefix), ansi.StringWidth(reference); got != want {
					t.Fatalf("manufacturer name starts at column %d for icon %q, but at column %d for the reference icon %q", got, icon, want, referenceIcon)
				}
			})
		}
	}
}

// iconLineWidth/iconLineHeight are the synthetic terminal geometry used by
// TestRendererTreatsADefaultIconLineAsFullWidth below.
const (
	iconLineWidth  = 40
	iconLineHeight = 6
)

// iconFullLine builds a line out of nothing but repetitions of one default
// manufacturer icon, wide enough to fill a w-column terminal exactly.
func iconFullLine(icon string, w int) string {
	return strings.Repeat(icon, w/manufacturerIconSlotWidth)
}

// defaultIconLineModel paints nothing but full-width icon lines, so the
// renderer's own erase-line decision for such a line becomes observable in the
// raw bytes it writes.
type defaultIconLineModel struct {
	icon string
	w, h int
}

func (m defaultIconLineModel) Init() tea.Cmd { return nil }

func (m defaultIconLineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.w, m.h = size.Width, size.Height
	}
	return m, nil
}

func (m defaultIconLineModel) View() string {
	lines := make([]string, m.h)
	for i := range lines {
		lines[i] = iconFullLine(m.icon, m.w)
	}
	return strings.Join(lines, "\n")
}

// TestRendererTreatsADefaultIconLineAsFullWidth carries the fix through to the
// patched renderer (internal/thirdparty/bubbletea-patched/standard_renderer.go),
// end to end.
//
// flush() appends ansi.EraseLineRight to a painted line only when it believes
// the line is narrower than the terminal — because on a line that fills the
// terminal the cursor sits in the last cell with wrap pending, and an erase
// there wipes the character just painted instead of clearing empty space.
//
// A row of half-a-terminal's worth of manufacturer icons therefore has to be a
// full-width row and be measured as one. With an icon whose width tables and
// terminal disagree it is neither reliably: the line is short by ansi's count
// and full on screen, so the renderer erases the last painted cell and leaves a
// stale glyph behind on the next frame.
func TestRendererTreatsADefaultIconLineAsFullWidth(t *testing.T) {
	icon := config.DefaultIconConfig().Manufacturers["z.ai"]
	line := iconFullLine(icon, iconLineWidth)
	if got := ansi.StringWidth(line); got != iconLineWidth {
		t.Fatalf("%d copies of the z.ai icon %q measure %d cells, want a full %d-column line: the icon is not %d cells wide", iconLineWidth/manufacturerIconSlotWidth, icon, got, iconLineWidth, manufacturerIconSlotWidth)
	}

	rp := startRuntimeProgramForModel(t, defaultIconLineModel{icon: icon, w: iconLineWidth, h: iconLineHeight}, iconLineWidth, iconLineHeight)
	writes := drainRuntimeWritesUntilQuiet(rp, resizeStampQuietWindow)
	rp.Quit(t, resizeStampSyncTimeout)

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

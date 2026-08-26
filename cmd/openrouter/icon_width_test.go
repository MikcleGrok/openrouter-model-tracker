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
// otherwise be drawn as text; variationSelector15 forces the opposite. Either
// one in a default icon means the codepoint underneath does not carry emoji
// presentation on its own \u2014 which is exactly the property this file requires,
// so both are disqualifying rather than exempting.
const (
	variationSelector15 = '\ufe0e'
	variationSelector16 = '\ufe0f'
)

// modernPictographFloor is the first codepoint of the Miscellaneous Symbols and
// Pictographs block. Every emoji added from that block upward is
// Emoji_Presentation=Yes and East_Asian_Width=Wide by default, with no
// variation selector needed to say so \u2014 which is why every default icon at or
// above it has never been reported as misaligned, and every icon that ever was
// reported sat below it.
const modernPictographFloor = 0x1F300

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

func hasVariationSelector(value string) bool {
	return strings.ContainsRune(value, variationSelector16) || strings.ContainsRune(value, variationSelector15)
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
//   - mattn/go-runewidth must agree, with no exemption. An earlier revision of
//     this test let an icon off the go-runewidth hook when it carried an
//     explicit variation-selector-16, on the theory that the selector settles
//     the presentation question. It does not settle it on a real terminal, and
//     that exemption is precisely what let "Ⓜ️" (meta), "♟️" (minimax) and
//     "🌪️" (mistral) keep shipping until users reported all three misaligned
//     in both iTerm2 and Terminal.app. Two independent tables agreeing on the
//     bare glyph is the only thing that rules out the font-fallback divergence
//     above.
func TestDefaultManufacturerIconsAreUnambiguouslyTwoCellsWide(t *testing.T) {
	for _, icon := range defaultIconValues() {
		t.Run(icon, func(t *testing.T) {
			if got := ansi.StringWidth(icon); got != manufacturerIconSlotWidth {
				t.Errorf("ansi.StringWidth(%q) = %d, want %d: a default icon must fill the manufacturer slot on its own", icon, got, manufacturerIconSlotWidth)
			}
			if got := tableDisplayWidth(icon); got != manufacturerIconSlotWidth {
				t.Errorf("tableDisplayWidth(%q) = %d, want %d", icon, got, manufacturerIconSlotWidth)
			}
			if got := runewidth.StringWidth(icon); got != manufacturerIconSlotWidth {
				t.Errorf("runewidth.StringWidth(%q) = %d, want %d: a bare icon that any width table calls narrow has no Emoji_Presentation to fall back on, so whether a terminal paints it across one cell or two comes down to the font stack — that cannot be a default", icon, got, manufacturerIconSlotWidth)
			}
		})
	}
}

// TestDefaultManufacturerIconsAreSingleModernPictographs is the structural
// invariant that closes this bug class instead of one more glyph in it.
//
// Three separate releases fixed misaligned manufacturer icons one at a time —
// "Ⓩ"/"Ⓝ"/"ⓧ" in v1.16.1, then "Ⓜ️"/"♟️"/"🌪️" right after — and each time the
// fix was a width measurement the program had to believe. It never held,
// because the divergence is not in any width table: it is in the terminal's
// font stack, which this program cannot measure and does not get to choose.
//
// The property that actually holds is a property of the codepoint. An icon is
// safe when all three are true, and every icon ever reported broken failed at
// least one:
//
//	(a) exactly one rune, so no joined or combined sequence can be split, or
//	    measured, or rendered differently by different shapers. All six
//	    reported icons were two runes, base plus U+FE0F;
//	(b) at or above U+1F300, the pictograph range where Emoji_Presentation=Yes
//	    and East_Asian_Width=Wide normally come with the codepoint itself —
//	    Ⓜ (U+24C2), ♟ (U+265F), Ⓩ (U+24CF), Ⓝ (U+24C3) and ⓧ (U+24E7) all sit
//	    below it;
//	(c) no variation selector, because needing one to request emoji
//	    presentation is the same statement as the codepoint not carrying it —
//	    and a terminal is free to ignore the request, which is exactly what
//	    happened.
//
// The floor is necessary but not sufficient on its own: a handful of
// sub-blocks above it are Emoji_Presentation=No anyway, U+1F321..U+1F32C among
// them, which is where mistral's old 🌪 (U+1F32A) came from. Stripping its
// U+FE0F would satisfy all three clauses here and still measure one cell.
// That case is caught by the width-agreement test above, which now requires
// both tables to say two with no exemption — the two tests are complementary
// and neither is redundant.
//
// Checked here against the manufacturer map only. The unknown-manufacturer
// fallback "❔" (U+2754) sits below the floor and is covered separately below.
func TestDefaultManufacturerIconsAreSingleModernPictographs(t *testing.T) {
	for name, icon := range config.DefaultIconConfig().Manufacturers {
		t.Run(name, func(t *testing.T) {
			runes := []rune(icon)
			if len(runes) != 1 {
				t.Fatalf("icon %q for %q is %d runes (%U), want exactly 1: a multi-rune icon is a presentation request, not a glyph, and every terminal answers it differently", icon, name, len(runes), runes)
			}
			if hasVariationSelector(icon) {
				t.Errorf("icon %q for %q carries a variation selector: the codepoint underneath has no emoji presentation of its own, so its rendered width is the font stack's decision rather than Unicode's", icon, name)
			}
			if runes[0] < modernPictographFloor {
				t.Errorf("icon %q for %q is U+%04X, below the U+%04X pictograph floor: outside that range Emoji_Presentation and East_Asian_Width are not guaranteed, and a terminal falling back to an emoji font paints the glyph two cells wide while every width table reports one", icon, name, runes[0], modernPictographFloor)
			}
		})
	}
}

// TestDefaultUnknownIconIsUnambiguouslyWide covers the one default icon that
// sits below the pictograph floor and is still safe there.
//
// "❔" is U+2754 WHITE QUESTION MARK ORNAMENT, one of the handful of pre-U+1F300
// codepoints that carry Emoji_Presentation=Yes *and* East_Asian_Width=Wide
// outright. It needs no variation selector, and both width tables call it two
// cells with no disagreement to resolve — so the reason the floor exists does
// not apply to it. This test states that explicitly rather than leaving it as
// an untested hole in the invariant above.
func TestDefaultUnknownIconIsUnambiguouslyWide(t *testing.T) {
	icon := config.DefaultIconConfig().Unknown
	if runes := []rune(icon); len(runes) != 1 {
		t.Fatalf("unknown icon %q is %d runes (%U), want exactly 1", icon, len(runes), runes)
	}
	if hasVariationSelector(icon) {
		t.Errorf("unknown icon %q carries a variation selector", icon)
	}
	if got := ansi.StringWidth(icon); got != manufacturerIconSlotWidth {
		t.Errorf("ansi.StringWidth(%q) = %d, want %d", icon, got, manufacturerIconSlotWidth)
	}
	if got := runewidth.StringWidth(icon); got != manufacturerIconSlotWidth {
		t.Errorf("runewidth.StringWidth(%q) = %d, want %d: a sub-floor icon earns its exemption only by both tables agreeing it is wide", icon, got, manufacturerIconSlotWidth)
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

// TestReportedManufacturerIconRowsAlignWithAPlainWideEmojiRow pins the three
// manufacturers users actually reported — Meta's icon touching its name with no
// gap, MiniMax's rendering as an unrecognizable thin glyph, Mistral's alongside
// them — to the reference row, by name rather than by map iteration.
//
// TestDefaultIconRowsAlignWithAPlainWideEmojiRow above already sweeps the whole
// map, but it names nothing: if one of these three regresses, that test reports
// a subtest keyed by the broken glyph and nothing about which manufacturer it
// belongs to. This one names them, so the next failure points straight at the
// reported bug instead of at a codepoint to go look up.
func TestReportedManufacturerIconRowsAlignWithAPlainWideEmojiRow(t *testing.T) {
	const referenceIcon = "🌀"
	icons := config.DefaultIconConfig()
	rowFor := func(icon string) string {
		cfg := config.IconConfig{Manufacturers: map[string]string{"acme": icon}, Unknown: "?"}
		return modelIdentityWithIconsAndGap(model.Model{DisplayName: "Acme Model", Owner: "Acme"}, cfg, 1)
	}
	reference := rowFor(referenceIcon)
	for _, name := range []string{"meta", "minimax", "mistral"} {
		t.Run(name, func(t *testing.T) {
			icon := icons.Manufacturers[name]
			if icon == "" {
				t.Fatalf("no default icon for %q", name)
			}
			got := rowFor(icon)
			want := strings.Replace(reference, referenceIcon, icon, 1)
			if got != want {
				t.Fatalf("%s row bytes % x, want % x: the icon slot compensated for the icon instead of the icon filling it", name, []byte(got), []byte(want))
			}
			if got, want := len([]byte(icon)), 4; got != want {
				t.Fatalf("%s icon %q is %d bytes, want %d: a single unadorned pictograph, not a base plus a presentation selector", name, icon, got, want)
			}
		})
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

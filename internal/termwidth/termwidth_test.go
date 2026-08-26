package termwidth

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestStringOverridesOnlyTheVerifiedFontFallbackRunes(t *testing.T) {
	for _, test := range []struct {
		name, value string
		want        int
	}{
		{"ascii", "abc", 3},
		{"empty", "", 0},
		{"cjk", "界", 2},
		{"plain-emoji", "\U0001f310", 2},             // 🌐
		{"vs16-emoji", "Ⓜ️", 2},                      // Ⓜ️ — ansi already gets this right
		{"zwj-sequence", "\U0001f469‍\U0001f4bb", 2}, // 👩‍💻
		{"flag", "\U0001f1fa\U0001f1f8", 2},          // 🇺🇸
		{"circled-z", "Ⓩ", 2},                        // overridden
		{"circled-n", "Ⓝ", 2},                        // overridden
		{"circled-x", "ⓧ", 2},                        // overridden
		{"circled-in-a-line", "Ⓩ z.ai", 7},           // 2 + " z.ai"
		{"two-overridden-runes", "ⓧⓍ", 3},            // only ⓧ is on the list; Ⓧ is not
		{"styled", "\x1b[31mⓝ\x1b[0m", 1},            // ⓝ is not on the list, and SGR costs nothing
		{"styled-overridden", "\x1b[31mⓧ\x1b[0m", 2}, // SGR still costs nothing
		{"other-circled-letter-untouched", "Ⓐ", 1},   // no evidence it renders wide
		{"enclosed-digit-untouched", "①", 1},         // ditto
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := String(test.value); got != test.want {
				t.Fatalf("String(%q) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

// An explicit variation selector settles the presentation without consulting
// the font-fallback chain, so ansi's own answer stands and the override must
// not fire on top of it.
func TestStringLeavesExplicitVariationSelectorsToAnsi(t *testing.T) {
	for _, value := range []string{"Ⓩ️", "Ⓩ︎", "Ⓝ️", "Ⓝ︎", "ⓧ️", "ⓧ︎"} {
		if got, want := String(value), ansi.StringWidth(value); got != want {
			t.Fatalf("String(%q) = %d, want ansi.StringWidth = %d", value, got, want)
		}
	}
}

func TestTruncateNeverExceedsTheRequestedCellCount(t *testing.T) {
	for _, test := range []struct {
		name, value string
		width       int
		want        string
	}{
		{"fits", "Ⓩ z.ai", 10, "Ⓩ z.ai"},
		{"exact", "Ⓩ z.ai", 7, "Ⓩ z.ai"},
		{"one-short", "Ⓩ z.ai", 6, "Ⓩ z.a"},
		{"icon-only", "Ⓩ....", 2, "Ⓩ"},
		{"below-icon", "Ⓩ....", 1, ""},
		{"zero", "Ⓩ", 0, ""},
		{"ascii", "abcdef", 3, "abc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Truncate(test.value, test.width)
			if got != test.want {
				t.Fatalf("Truncate(%q, %d) = %q, want %q", test.value, test.width, got, test.want)
			}
			if width := String(got); width > test.width {
				t.Fatalf("Truncate(%q, %d) = %q is %d cells wide", test.value, test.width, got, width)
			}
		})
	}
}

// The override must survive repetition, since a wrapped detail row can carry
// several icons and each one costs the width tables a cell.
func TestStringScalesWithRepeatedOverriddenRunes(t *testing.T) {
	for count := 1; count <= 5; count++ {
		value := strings.Repeat("Ⓩ", count)
		if got, want := String(value), 2*count; got != want {
			t.Fatalf("String(%q) = %d, want %d", value, got, want)
		}
	}
}

// Package termwidth is the single width oracle for everything this program
// paints: how many terminal cells a string actually occupies on screen.
//
// It is charmbracelet/x/ansi's StringWidth — the same uniseg-based measure the
// TUI renderer uses — plus an explicit override for the handful of runes where
// real terminals disagree with every Unicode width table.
//
// Why an override is unavoidable. A terminal's cell width for a rune is not
// decided by Unicode alone: when the primary monospace face has no glyph for
// it, the terminal substitutes another font, and macOS Terminal.app and iTerm2
// both fall back to an emoji face for the bare "circled Latin letter" glyphs in
// the Enclosed Alphanumerics block. Those characters have neither
// variation-selector-16 nor the Emoji_Presentation property, so East_Asian_Width
// says Ambiguous and every width table — mattn/go-runewidth and
// charmbracelet/x/ansi alike — reports 1. The terminal paints 2. No library can
// be "fixed" here, because the answer is a property of the font stack, not of
// the character.
//
// So the list is deliberately narrow: only runes actually observed rendering
// wide, never a whole Unicode block. A rune that carries an explicit variation
// selector is left to ansi, which already handles both presentations.
package termwidth

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const (
	variationSelector15 = '︎' // text presentation
	variationSelector16 = '️' // emoji presentation
)

// fontFallbackWide holds the runes real terminals paint across two cells while
// every Unicode width table reports one. Each is a default manufacturer icon
// (internal/config.defaultManufacturerIcons) verified to render wide in
// Terminal.app and iTerm2; adding to this list requires the same verification,
// not a guess from the character's Unicode block.
var fontFallbackWide = map[rune]struct{}{
	'Ⓩ': {}, // Ⓩ CIRCLED LATIN CAPITAL LETTER Z — z.ai
	'Ⓝ': {}, // Ⓝ CIRCLED LATIN CAPITAL LETTER N — nvidia
	'ⓧ': {}, // ⓧ CIRCLED LATIN SMALL LETTER X — xiaomi
}

// String reports how many terminal cells value occupies. ANSI escape sequences
// contribute nothing, exactly as with ansi.StringWidth.
func String(value string) int {
	return ansi.StringWidth(value) + fontFallbackExtraCells(value)
}

// Truncate cuts value down to at most width cells as String measures them.
// Styling is preserved, because the cut itself is still made by ansi.Truncate;
// only the acceptance test is this package's. The loop steps down at most once
// per overridden rune in the kept prefix, so it converges immediately for the
// overwhelmingly common case of a line holding none.
func Truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if String(value) <= width {
		return value
	}
	for limit := width; limit > 0; limit-- {
		cut := ansi.Truncate(value, limit, "")
		if String(cut) <= width {
			return cut
		}
	}
	return ""
}

// fontFallbackExtraCells counts the cells the width tables miss. Escape
// sequences need no special handling: they are ASCII-only, so none of the
// overridden runes can appear inside one.
func fontFallbackExtraCells(value string) int {
	extra := 0
	for index := 0; index < len(value); {
		r, size := utf8.DecodeRuneInString(value[index:])
		index += size
		if _, ok := fontFallbackWide[r]; !ok {
			continue
		}
		// An explicit variation selector takes the choice away from the font
		// stack and hands it to a rule ansi already implements.
		if next, _ := utf8.DecodeRuneInString(value[index:]); next == variationSelector15 || next == variationSelector16 {
			continue
		}
		extra++
	}
	return extra
}

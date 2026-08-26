package tea

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// This file is part of the local patch set; it does not exist upstream. See
// README.md, "painted_width.go — measuring a line the way the terminal paints
// it", for the rationale.
//
// It is a deliberate, self-contained copy of the host repository's
// internal/termwidth package. This directory is its own Go module (see
// go.mod), reached only through a replace directive, so it cannot import
// internal/termwidth without inverting that dependency. The two lists must
// stay in sync; internal/termwidth is the authoritative one.

const (
	fallbackVariationSelector15 = '︎' // text presentation
	fallbackVariationSelector16 = '️' // emoji presentation
)

// fontFallbackWide holds runes that real terminals paint across two cells
// while every Unicode width table reports one, because the primary monospace
// face has no glyph for them and the terminal substitutes an emoji font.
var fontFallbackWide = map[rune]struct{}{
	'Ⓩ': {}, // Ⓩ CIRCLED LATIN CAPITAL LETTER Z
	'Ⓝ': {}, // Ⓝ CIRCLED LATIN CAPITAL LETTER N
	'ⓧ': {}, // ⓧ CIRCLED LATIN SMALL LETTER X
}

// paintedLineWidth reports how many cells a line occupies once painted, which
// is what flush() must compare against r.width before deciding whether an
// erase-line-right is safe to append.
func paintedLineWidth(line string) int {
	width := ansi.StringWidth(line)
	for index := 0; index < len(line); {
		r, size := utf8.DecodeRuneInString(line[index:])
		index += size
		if _, ok := fontFallbackWide[r]; !ok {
			continue
		}
		if next, _ := utf8.DecodeRuneInString(line[index:]); next == fallbackVariationSelector15 || next == fallbackVariationSelector16 {
			continue
		}
		width++
	}
	return width
}

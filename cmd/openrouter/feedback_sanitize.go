package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// feedbackReviewMaxRunes and feedbackReviewMaxLines are the hard caps the
// task brief's sanitization contract requires (.superpowers/sdd/plan/task-6-brief.md
// section 8.3): after normalization the result never exceeds these many
// runes or lines, truncated deterministically at a rune/line boundary.
//
// feedbackReviewMaxRunes MUST equal internal/feedback.ReviewMaxLength, the
// domain's real, server-enforced limit — it is kept as a separate constant
// (not an import) rather than referencing that package directly, matching
// feedback.go's feedbackSkillKeys convention that cmd/openrouter (the TUI
// binary) does not gain a dependency on the feedback server's domain
// package for a handful of small constants. An earlier, larger value here
// (4096) let the TUI advertise and accept reviews the server would then
// reject with an inexplicable 400 that a retry could never fix (the review
// draft is kept for retry, so the rejection loop was unrecoverable without
// manually shortening the text). TestFeedbackReviewMaxRunesMatchesDomainLimit
// pins these two numbers together so they cannot silently drift apart again.
const (
	feedbackReviewMaxRunes = 2000
	feedbackReviewMaxLines = 200
)

// c1CSI, c1OSC and c1DCS are the single-rune C1 (8-bit) forms of the CSI,
// OSC and DCS introducers, equivalent to the 7-bit ESC+'[' / ESC+']' /
// ESC+'P' sequences. c1ST is the C1 String Terminator, equivalent to the
// 7-bit ESC+'\\'.
const (
	c1CSI = 0x9b
	c1OSC = 0x9d
	c1DCS = 0x90
	c1ST  = 0x9c
	cBEL  = 0x07
	cESC  = 0x1b
)

// sanitizeFeedbackReviewText applies the exact terminal-safe sanitization
// contract from the task brief to review text before it is ever rendered in
// the TUI (or sent to the server on save). It runs unconditionally on every
// review string, including one shaped like a prompt-injection payload: this
// function only ever inspects raw bytes/runes, never the text's meaning, and
// the result is display text only — it is never turned into a prompt,
// system/developer instruction, or log/debug output (brief 8.2's "текст
// отзыва не... писать в debug log", 8.3's "raw review не получают" signal/
// prompt access).
//
// Steps, in order:
//  1. Invalid UTF-8 byte sequences are replaced with U+FFFD, one per invalid
//     byte (matching utf8.DecodeRuneInString's own error granularity).
//  2. CRLF and lone CR are normalized to LF; LF is the only line break kept.
//  3. 7-bit and C1 CSI/OSC/DCS sequences are removed in full, including their
//     terminator. An unterminated CSI/OSC/DCS (or a lone ESC) that reaches
//     end of input without its terminator/final byte drops its introducer
//     *and* everything after it to EOF — there is no partial sequence left
//     dangling in the output.
//  4. Every other C1 control, Unicode Cc, Cf (format/bidi/joiner/BOM) and any
//     line separator other than LF is dropped.
//  5. Each TAB becomes a single ASCII space.
//  6. The result is capped to at most feedbackReviewMaxRunes runes, then to
//     at most feedbackReviewMaxLines lines, both cuts made from the tail so
//     the result never splits a rune or a line halfway.
func sanitizeFeedbackReviewText(raw string) string {
	valid := feedbackReplaceInvalidUTF8(raw)
	valid = strings.ReplaceAll(valid, "\r\n", "\n")
	valid = strings.ReplaceAll(valid, "\r", "\n")
	stripped := feedbackStripControlSequences(valid)
	return feedbackCapReviewText(stripped)
}

// feedbackReplaceInvalidUTF8 returns s with every invalid byte replaced by
// U+FFFD, leaving already-valid text (including a legitimately-encoded
// U+FFFD) untouched.
func feedbackReplaceInvalidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			b.WriteRune(utf8.RuneError)
			i++
			continue
		}
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

// feedbackStripControlSequences removes escape sequences and other
// control/format characters from already-valid, CRLF-normalized text. See
// sanitizeFeedbackReviewText's doc comment for the exact contract.
func feedbackStripControlSequences(s string) string {
	runes := []rune(s)
	out := make([]rune, 0, len(runes))
	n := len(runes)
	for i := 0; i < n; i++ {
		r := runes[i]
		switch {
		case r == '\n':
			out = append(out, r)
		case r == '\t':
			out = append(out, ' ')
		case r == cESC:
			if i+1 >= n {
				// A lone ESC at EOF: unterminated, drop it and (nothing)
				// remaining.
				return string(out)
			}
			switch runes[i+1] {
			case '[':
				end, ok := feedbackScanFinalByte(runes, i+2)
				if !ok {
					return string(out)
				}
				i = end - 1
			case ']':
				end, ok := feedbackScanST(runes, i+2, true)
				if !ok {
					return string(out)
				}
				i = end - 1
			case 'P':
				end, ok := feedbackScanST(runes, i+2, false)
				if !ok {
					return string(out)
				}
				i = end - 1
			default:
				// A short, otherwise-unrecognized 7-bit escape: drop the
				// introducer and the one rune it applies to, defensively,
				// rather than let either reach the terminal.
				i++
			}
		case r == c1CSI:
			end, ok := feedbackScanFinalByte(runes, i+1)
			if !ok {
				return string(out)
			}
			i = end - 1
		case r == c1OSC:
			end, ok := feedbackScanST(runes, i+1, true)
			if !ok {
				return string(out)
			}
			i = end - 1
		case r == c1DCS:
			end, ok := feedbackScanST(runes, i+1, false)
			if !ok {
				return string(out)
			}
			i = end - 1
		case r >= 0x80 && r <= 0x9f:
			// Any other C1 control (including a bare ST with no matching
			// OSC/DCS, NEL, etc.): drop.
		case r < 0x20 || r == 0x7f:
			// Remaining C0 controls and DEL (TAB/LF/ESC already handled
			// above): drop.
		case unicode.Is(unicode.Cf, r):
			// Bidi controls, joiners, BOM, soft hyphen, and every other
			// Unicode format character: drop.
		case r == ' ' || r == ' ':
			// Unicode line/paragraph separator: not a line break we keep
			// (only LF is), so drop rather than convert.
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// feedbackScanFinalByte scans a CSI body starting at start (just past its
// introducer) for its final byte, a rune in 0x40-0x7e. It returns the index
// just past that byte and true, or (len(runes), false) if EOF is reached
// first — an unterminated CSI.
func feedbackScanFinalByte(runes []rune, start int) (endExclusive int, ok bool) {
	for i := start; i < len(runes); i++ {
		if runes[i] >= 0x40 && runes[i] <= 0x7e {
			return i + 1, true
		}
	}
	return len(runes), false
}

// feedbackScanST scans an OSC/DCS body starting at start (just past its
// introducer) for its terminator: the C1 String Terminator (0x9c), the
// 7-bit ST (ESC \\), or — for OSC only, when allowBEL is true — a bare BEL.
// It returns the index just past the terminator and true, or (len(runes),
// false) if EOF is reached first — an unterminated OSC/DCS.
func feedbackScanST(runes []rune, start int, allowBEL bool) (endExclusive int, ok bool) {
	for i := start; i < len(runes); i++ {
		switch {
		case allowBEL && runes[i] == cBEL:
			return i + 1, true
		case runes[i] == c1ST:
			return i + 1, true
		case runes[i] == cESC && i+1 < len(runes) && runes[i+1] == '\\':
			return i + 2, true
		}
	}
	return len(runes), false
}

// feedbackCapReviewText truncates s to at most feedbackReviewMaxRunes runes
// and then at most feedbackReviewMaxLines lines, always dropping the tail so
// the cut never splits a rune or lands mid-line.
func feedbackCapReviewText(s string) string {
	runes := []rune(s)
	if len(runes) > feedbackReviewMaxRunes {
		runes = runes[:feedbackReviewMaxRunes]
	}
	lines := strings.Split(string(runes), "\n")
	if len(lines) > feedbackReviewMaxLines {
		lines = lines[:feedbackReviewMaxLines]
	}
	return strings.Join(lines, "\n")
}

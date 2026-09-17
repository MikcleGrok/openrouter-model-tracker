package sqlite

import (
	"testing"
	"time"
)

// TestFormatTimeIsLexicographicallySortable pins the exact bug this layout
// exists to fix: under time.RFC3339Nano (trailing-zero-stripping), a
// whole-second timestamp with no fractional part sorts lexicographically
// AFTER a later sub-second timestamp, because 'Z' (0x5A) sorts after '.'
// (0x2E). This test proves formatTime's fixed-width layout does not have
// that gap: for these two times, byte-wise string order must agree with
// chronological order.
func TestFormatTimeIsLexicographicallySortable(t *testing.T) {
	wholeSecond := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	laterSubSecond := wholeSecond.Add(500 * time.Millisecond)

	a := formatTime(wholeSecond)
	b := formatTime(laterSubSecond)

	if len(a) != len(b) {
		t.Fatalf("formatTime results have different lengths: %q (%d) vs %q (%d), want a fixed-width layout", a, len(a), b, len(b))
	}
	if !(a < b) {
		t.Fatalf("formatTime(%v) = %q, formatTime(%v) = %q: want the earlier time's string to sort strictly before the later one's", wholeSecond, a, laterSubSecond, b)
	}
}

// TestFormatTimeParseTimeRoundTrip proves the fixed-width layout still
// round-trips a time.Time exactly, whole-second and sub-second alike —
// the property the old RFC3339Nano layout also had, and that this fix must
// not lose.
func TestFormatTimeParseTimeRoundTrip(t *testing.T) {
	for _, want := range []time.Time{
		time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 12, 0, 0, 500000000, time.UTC),
		time.Date(2026, 1, 1, 12, 0, 0, 1, time.UTC),
		time.Date(2026, 6, 15, 23, 59, 59, 999999999, time.UTC),
	} {
		text := formatTime(want)
		got, err := parseTime(text)
		if err != nil {
			t.Fatalf("parseTime(%q): %v", text, err)
		}
		if !got.Equal(want) {
			t.Fatalf("round trip of %v via %q = %v, want exact match", want, text, got)
		}
	}
}

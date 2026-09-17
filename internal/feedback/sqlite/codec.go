package sqlite

import (
	"fmt"
	"sort"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// timeLayout is the text encoding used for every TEXT timestamp column
// (identities.created_at/last_seen_at, model_feedback.created_at/updated_at,
// privacy_cleanup_jobs.created_at/updated_at, schema_migrations.applied_at).
// It round-trips a Go time.Time exactly (to the nanosecond) through a plain
// string column, and formatTime always normalizes to UTC first so two
// servers in different local zones never disagree on stored order.
//
// This is a fixed-width layout (always 9 fractional digits), not
// time.RFC3339Nano, deliberately: RFC3339Nano strips trailing zeros from the
// fractional second, so a whole-second timestamp (no fractional part at
// all) can sort lexicographically AFTER a sub-second timestamp from the
// same or even a later real moment — e.g. "...T12:00:00Z" (whole second)
// compares greater than "...T12:00:00.5Z" (500ms later) because 'Z' (0x5A)
// sorts after '.' (0x2E) in a byte-wise comparison. That broke
// repository.go's LastUpdatedAt, which does MAX(updated_at) as a plain SQL
// string aggregate over this column: it could pick the wrong ("earlier" by
// clock time but lexicographically "larger") row. A fixed-width layout with
// always-present fractional digits has no such gap: every value is the same
// length and the same character position always carries the same
// significance, so byte-wise order and chronological order agree.
//
// This is the exact same layout string backup.go's own filename timestamp
// already uses (its "timestamp" local var), for consistency between the two
// places this package needs a lexicographically sortable time encoding.
const timeLayout = "20060102T150405.000000000Z"

// formatTime renders t as this package's canonical TEXT timestamp encoding.
func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

// parseTime parses a TEXT timestamp column value written by formatTime.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlite: parse timestamp %q: %w", s, err)
	}
	return t, nil
}

// identityBytes renders identity as the BLOB value stored in
// identities.id and every identity_id column that references it: the
// identity's raw bytes, unchanged. This package does not interpret
// IdentityID's contents (repository.go's own doc comment: canonical wire
// format belongs to a later layer) — it only needs a stable, reversible
// byte encoding to store and compare it.
func identityBytes(identity feedback.IdentityID) []byte {
	return []byte(identity)
}

// identityFromBytes reverses identityBytes.
func identityFromBytes(b []byte) feedback.IdentityID {
	return feedback.IdentityID(b)
}

// sortSkillsCanonical sorts skills in place into the fixed AllowedSkills()
// order, matching the order internal/feedback's own fakeRepository already
// produces for Aggregate.Skills (repository_fake_test.go) so a caller sees
// the same deterministic order regardless of which Repository implementation
// is behind Service. A skill key that is somehow not in AllowedSkills() (it
// should never reach storage — ValidateSkillRatings rejects it before a
// Service call ever reaches this package) sorts after every recognized key,
// rather than panicking or being dropped.
func sortSkillsCanonical(skills []feedback.SkillRating) {
	order := make(map[string]int, len(feedback.AllowedSkills()))
	for i, key := range feedback.AllowedSkills() {
		order[key] = i
	}
	rank := func(key string) int {
		if r, ok := order[key]; ok {
			return r
		}
		return len(order)
	}
	sort.SliceStable(skills, func(i, j int) bool {
		return rank(skills[i].Key) < rank(skills[j].Key)
	})
}

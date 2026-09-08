#!/usr/bin/env bash
# Negative-case regression test for scripts/check-onboarding-record.sh,
# modelled on scripts/sign_flags_test.sh and
# scripts/provenance_profile_test.sh: mutate a disposable copy of README.md
# and assert the gate refuses each broken shape. The script under test takes
# the record path as its first argument specifically so this test never has
# to touch the repository's real README.md.
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd -P)
GATE="$SCRIPT_DIR/check-onboarding-record.sh"

TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openrouter-onboarding-test.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

fixture="$TEST_ROOT/README.md"

# Sanity: the real record MUST pass today, or this test's negative cases
# would be trivially "passing" for the wrong reason (a gate that always
# fails also refuses a stale-date fixture).
bash "$GATE" "$ROOT/README.md" 90 >/dev/null || {
  printf '%s\n' 'PRECONDITION FAILED: the real README.md onboarding record does not currently pass the gate' >&2
  exit 1
}

assert_blocked() {
  local label="$1"
  local expect="$2"
  local out
  if out="$(bash "$GATE" "$fixture" 90 2>&1)"; then
    printf 'FAIL (%s): gate unexpectedly passed:\n%s\n' "$label" "$out" >&2
    exit 1
  fi
  printf '%s\n' "$out" | grep -Fq "$expect" || {
    printf 'FAIL (%s): expected message containing %q, got:\n%s\n' "$label" "$expect" "$out" >&2
    exit 1
  }
  printf 'ok - %s\n' "$label"
}

# Case 1: stale `last reviewed` (older than the 90-day default window).
cp "$ROOT/README.md" "$fixture"
sed -i.bak 's/| `last reviewed` | 2026-09-08 |/| `last reviewed` | 2026-01-01 |/' "$fixture"
assert_blocked "stale last reviewed" "older than the 90-day review window"

# Case 2: a required field is missing entirely.
cp "$ROOT/README.md" "$fixture"
sed -i.bak '/| `owners` |/d' "$fixture"
assert_blocked "missing required field" 'missing required field: `owners`'

# Case 3: a required field is present but empty.
cp "$ROOT/README.md" "$fixture"
sed -i.bak 's/| `SCA owner` | maintainer |/| `SCA owner` |  |/' "$fixture"
assert_blocked "empty field value" 'field `SCA owner` has an empty value'

# Case 4: duplicated `## Onboarding record` heading.
cp "$ROOT/README.md" "$fixture"
printf '\n## Onboarding record\n\nDuplicate section.\n' >>"$fixture"
assert_blocked "duplicate heading" 'expected exactly one'

# Case 5: `last reviewed` is not a real/parseable date.
cp "$ROOT/README.md" "$fixture"
sed -i.bak 's/| `last reviewed` | 2026-09-08 |/| `last reviewed` | not-a-date |/' "$fixture"
assert_blocked "unparseable last reviewed" 'not ISO-8601 YYYY-MM-DD'

# Case 6: `last reviewed` is in the future.
cp "$ROOT/README.md" "$fixture"
future="$(date -u -d '+1 day' +%Y-%m-%d 2>/dev/null || date -u -v+1d +%Y-%m-%d)"
sed -i.bak "s/| \`last reviewed\` | 2026-09-08 |/| \`last reviewed\` | $future |/" "$fixture"
assert_blocked "future last reviewed" 'is in the future'

# Case 7: `review trigger/profile state` names a deadline past last-reviewed + 90 days.
cp "$ROOT/README.md" "$fixture"
sed -i.bak 's/не позднее 2026-12-07/не позднее 2027-06-01/' "$fixture"
assert_blocked "deadline beyond review window" 'later than last reviewed'

printf '%s\n' 'check-onboarding-record negative-case tests passed'

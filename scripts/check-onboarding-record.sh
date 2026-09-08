#!/usr/bin/env bash
# Onboarding-record freshness/completeness gate (guide-tools README.md,
# "make check or make release-check MUST machine-checkably verify the
# mandatory fields and this freshness rule" + 00-overview.md, "MUST persist
# the record in the canonical README section"). Companion to
# scripts/check-sca-freshness.sh: same style (`set -euo pipefail`, the same
# `fail()` shape with a BLOCKED: line plus a HINT:, the same portable
# `to_epoch()`, and the same "artifact path first argument, threshold second"
# calling convention), but it checks the onboarding record itself rather than
# SCA evidence.
#
# Freshness rule (guide-tools README.md): the record is stale if any of
# profile, channel, version source or trust boundary changed since `last
# reviewed`, or the default 90-day window elapsed. This script cannot see
# code/profile changes, so it enforces the mechanically checkable half: the
# 90-day window, and that any explicit review deadline named in `review
# trigger/profile state` does not itself exceed that window.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
record="${1:-$root/README.md}"
cadence_days="${2:-90}"

fail() {
  printf 'BLOCKED: %s\n' "$1" >&2
  printf 'HINT: update the "## Onboarding record" section in %s.\n' "$record" >&2
  exit 1
}

test -s "$record" || fail "$record does not exist or is empty"

heading_count="$(grep -c '^## Onboarding record$' "$record" || true)"
test "$heading_count" -eq 1 || fail "expected exactly one '## Onboarding record' heading in $record, found $heading_count"

# Section body: everything after the heading line up to (not including) the
# next '## ' heading, or EOF if the record is the last section.
section="$(awk '
  /^## Onboarding record$/ { found=1; next }
  found && /^## / { exit }
  found { print }
' "$record")"
test -n "$section" || fail "the '## Onboarding record' section in $record has no body"

REQUIRED_FIELDS=(
  "project type"
  "profiles"
  "OS/ARCH"
  "modes"
  "channels"
  "version source"
  "Makefile targets"
  "Docker toolchain image"
  "Docker runtime image"
  "Docker runtime base image"
  "host-only exceptions"
  "shared-location scoping"
  "owners"
  "SCA cadence"
  "SCA owner"
  "remediation deadline"
  "N/A controls/rationale"
  "last reviewed"
  "review trigger/profile state"
)

last_reviewed_value=""
review_trigger_value=""

for field in "${REQUIRED_FIELDS[@]}"; do
  prefix="| \`$field\` |"
  line="$(printf '%s\n' "$section" | grep -F -- "$prefix" | head -n1 || true)"
  test -n "$line" || fail "onboarding record is missing required field: \`$field\`"
  value="${line#*"$prefix"}"
  value="${value%|}"
  value="$(printf '%s' "$value" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  test -n "$value" || fail "onboarding record field \`$field\` has an empty value"
  case "$field" in
    "last reviewed") last_reviewed_value="$value" ;;
    "review trigger/profile state") review_trigger_value="$value" ;;
  esac
done

to_epoch() {
  date -u -d "$1" +%s 2>/dev/null && return 0
  date -u -j -f '%Y-%m-%d' "$1" +%s 2>/dev/null && return 0
  return 1
}

[[ "$last_reviewed_value" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || fail "onboarding record field \`last reviewed\` is not ISO-8601 YYYY-MM-DD: $last_reviewed_value"
last_epoch="$(to_epoch "$last_reviewed_value")" || fail "onboarding record field \`last reviewed\` is not a real calendar date: $last_reviewed_value"

now_epoch="$(date -u +%s)"
age_seconds=$((now_epoch - last_epoch))
if ((age_seconds < 0)); then
  fail "onboarding record field \`last reviewed\` is in the future: $last_reviewed_value"
fi
age_days=$((age_seconds / 86400))
window_seconds=$((cadence_days * 86400))
if ((age_seconds > window_seconds)); then
  fail "onboarding record is $age_days day(s) old since last reviewed ($last_reviewed_value), older than the ${cadence_days}-day review window"
fi

# If `review trigger/profile state` names an explicit review deadline, that
# deadline MUST NOT itself exceed last-reviewed + cadence_days -- a record
# that promises a review date past its own staleness window is inconsistent
# with the rule it is supposed to satisfy.
deadline_dates="$(printf '%s' "$review_trigger_value" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' || true)"
if [[ -n "$deadline_dates" ]]; then
  max_epoch=$((last_epoch + window_seconds))
  while IFS= read -r deadline; do
    [[ -n "$deadline" ]] || continue
    deadline_epoch="$(to_epoch "$deadline")" || fail "onboarding record field \`review trigger/profile state\` names an unparseable deadline: $deadline"
    if ((deadline_epoch > max_epoch)); then
      fail "onboarding record field \`review trigger/profile state\` names a deadline ($deadline) later than last reviewed ($last_reviewed_value) + ${cadence_days} days"
    fi
  done <<<"$deadline_dates"
fi

printf 'PASS: onboarding record is complete (%d fields) and fresh (%s day(s) old, window: %s days)\n' "${#REQUIRED_FIELDS[@]}" "$age_days" "$cadence_days"

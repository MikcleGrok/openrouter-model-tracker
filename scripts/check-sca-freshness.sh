#!/usr/bin/env bash
# SCA-freshness staleness gate (guide-tools 08-security-and-reliability.md,
# "Gate по свежести SCA-evidence"). It does not run any scanner itself and
# does not replace `make dependency-check`: it only decides whether the
# *last* full dependency-check run is still trustworthy enough for the
# cadence this project committed to in the onboarding record (README.md,
# `SCA cadence`) -- not older than the cadence window, generated for the
# exact go.mod/go.sum state currently on disk, and clean (or at least not a
# scanner/runtime error or an unresolved finding).
#
# This is the canonical embedding point the guide names for the periodic SCA
# cadence requirement in "Зависимости и supply chain": `make check` runs it
# on every ordinary developer cycle (earlier and more often than
# `make release-check`), so a lapsed scan is caught at the next normal
# invocation instead of surfacing only at release time.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
evidence="${1:-$root/.release/dependency-evidence.json}"
cadence_days="${2:-30}"

fail() {
  printf 'BLOCKED: %s\n' "$1" >&2
  printf 'HINT: run `make dependency-check` from %s to refresh the evidence.\n' "$root" >&2
  exit 1
}

command -v jq >/dev/null 2>&1 || fail "jq is required to read $evidence"

if [[ ! -s "$evidence" ]]; then
  fail "no dependency-check evidence at $evidence (missing evidence is treated the same as stale evidence, never as a first-run pass)"
fi

jq -e . "$evidence" >/dev/null 2>&1 || fail "$evidence is not valid JSON (unreadable evidence blocks the same as stale evidence)"

scan_status="$(jq -r '.scan_status // empty' "$evidence")"
generated_at="$(jq -r '.generated_at // empty' "$evidence")"
recorded_digest="$(jq -r '.input_digest // empty' "$evidence")"
test -n "$scan_status" || fail "$evidence has no scan_status field"
test -n "$generated_at" || fail "$evidence has no generated_at field"
test -n "$recorded_digest" || fail "$evidence has no input_digest field"

# Recompute the exact input digest `make dependency-check` records, to catch
# evidence left over from a different go.mod/go.sum state (a dependency bump
# since the last scan, or evidence copied from another checkout). This MUST
# match the formula in the Makefile's dependency-check target byte for byte:
# relative "go.mod go.sum" filenames (as the Makefile does after `cd
# $(ROOT)`; an absolute path changes the hashed text) written to a real file
# before the second shasum pass, since a `$(...)` capture would strip the
# trailing newline the Makefile's `>`-redirected file keeps, changing the
# digest of otherwise identical content.
tmp_checksums="$(mktemp)"
trap 'rm -f "$tmp_checksums"' EXIT
(cd "$root" && shasum -a 256 go.mod go.sum) > "$tmp_checksums"
actual_digest="$(shasum -a 256 "$tmp_checksums")"
actual_digest="${actual_digest%% *}"
if [[ "$actual_digest" != "$recorded_digest" ]]; then
  fail "$evidence was generated for a different go.mod/go.sum state (recorded input_digest=$recorded_digest, current=$actual_digest)"
fi

# No exceptions registry exists yet (guide-tools "Evidence и изменяемые
# базы"), so any non-clean scan_status blocks unconditionally -- there is no
# exception to check a finding against.
if [[ "$scan_status" != clean ]]; then
  fail "dependency evidence scan_status=$scan_status (want clean); see $evidence and its native scanner outputs"
fi

to_epoch() {
  date -u -d "$1" +%s 2>/dev/null && return 0
  date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$1" +%s 2>/dev/null && return 0
  return 1
}

generated_epoch="$(to_epoch "$generated_at")" || fail "$evidence has an unparseable generated_at: $generated_at"
now_epoch="$(date -u +%s)"
age_seconds=$((now_epoch - generated_epoch))
if ((age_seconds < 0)); then age_seconds=0; fi
age_days=$((age_seconds / 86400))
window_seconds=$((cadence_days * 86400))

if ((age_seconds > window_seconds)); then
  fail "dependency evidence is $age_days day(s) old, older than the ${cadence_days}-day SCA cadence window (generated_at=$generated_at, evidence=$evidence)"
fi

printf 'PASS: dependency evidence is %s day(s) old (window: %s days, status: %s, evidence: %s)\n' "$age_days" "$cadence_days" "$scan_status" "$evidence"

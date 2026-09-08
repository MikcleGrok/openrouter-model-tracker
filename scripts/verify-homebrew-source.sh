#!/usr/bin/env bash
# Homebrew asset-channel source-provenance verifier (guide-tools
# 11-distribution-verifier.md, "Acceptance criteria для source provenance" —
# added in the 2026-09-03 delta, CHANGELOG.md). Verifies the CANONICAL,
# published Homebrew tap (`mikclegrok/tools/openrouter-model-tracker`) is
# genuinely what is installed and reachable on the maintainer's machine --
# not the local disposable dev-smoke tap `local/tap/openrouter`
# (scripts/verify-distribution.sh already covers that, a different, still
# valid purpose).
#
# This is a maintainer-machine gate, not a `make check`/CI gate: it needs
# `brew` and a real local Homebrew install, both explicitly out of scope for
# `make check` (Makefile: "check ... needs no network and does not build or
# persist a binary"). See `make homebrew-source-check`.
#
# All six points below are the guide's own acceptance criteria; violating
# any one is a hard BLOCKED/non-zero, never N/A, skipped, or a warning
# (11-distribution-verifier.md: "Нарушение любого пункта -- failed/non-zero,
# а не N/A, skipped или warning").
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
evidence_dir="$root/.release"
evidence_file="$evidence_dir/homebrew-source-evidence.json"

# 1. Exact fully-qualified formula, taken from the install matrix (README.md
# "Установка" / onboarding record `channels`) -- a short name is never
# accepted anywhere in this script; every brew invocation below spells it
# out in full.
HOMEBREW_CANONICAL_TAP="mikclegrok/tools"
HOMEBREW_CANONICAL_SHORT_NAME="openrouter-model-tracker"
HOMEBREW_CANONICAL_FORMULA="$HOMEBREW_CANONICAL_TAP/$HOMEBREW_CANONICAL_SHORT_NAME"
# The formula's own `install` recipe (Formula/openrouter-model-tracker.rb,
# `mikclegrok/tools`) installs the built binary under the formula's own
# short name, with `omt` as a plain install_symlink alias -- NOT under
# `openrouter`, unlike this checkout's own `make install` (README.md's
# "Установка" section documents and corrects this; do not assume they match
# without re-reading that formula, since the two are maintained in
# different repositories with no shared source of truth).
HOMEBREW_CANONICAL_BINARY="$HOMEBREW_CANONICAL_SHORT_NAME"
HOMEBREW_CANONICAL_ALIAS="omt"

expected_version=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      expected_version="${2:-}"
      shift 2
      ;;
    *)
      printf 'BLOCKED: unknown argument: %s\n' "$1" >&2
      exit 2
      ;;
  esac
done
test -n "$expected_version" || { printf 'BLOCKED: --version is required (the exact release version to expect installed)\n' >&2; exit 2; }

fail() {
  printf 'BLOCKED: %s\n' "$1" >&2
  exit 1
}

command -v brew >/dev/null 2>&1 || fail "brew is required and was not found -- this is a HOST-ONLY exception (see README.md onboarding record, 'host-only exceptions'); this check is never skipped, it is a hard fail without brew, exactly like 'make sbom' without syft"

# --- 2. Tap collision -------------------------------------------------------
# No other tapped/searchable formula may resolve the same short name; a
# collision blocks installation outright, it is not merely noted.
search_output="$(brew search "$HOMEBREW_CANONICAL_SHORT_NAME" 2>/dev/null || true)"
matching_full_names="$(printf '%s\n' "$search_output" | grep -E "(^|/)${HOMEBREW_CANONICAL_SHORT_NAME}\$" || true)"
match_count="$(printf '%s\n' "$matching_full_names" | grep -c . || true)"
test "$match_count" -ge 1 || fail "brew search found no formula named $HOMEBREW_CANONICAL_SHORT_NAME at all -- cannot verify source provenance for a formula that cannot be found"
printf '%s\n' "$matching_full_names" | grep -Fxq "$HOMEBREW_CANONICAL_FORMULA" || fail "brew search results for $HOMEBREW_CANONICAL_SHORT_NAME do not include the canonical $HOMEBREW_CANONICAL_FORMULA: $matching_full_names"
if [[ "$match_count" -gt 1 ]]; then
  fail "tap collision: more than one formula named $HOMEBREW_CANONICAL_SHORT_NAME is resolvable across connected taps ($matching_full_names) -- a short-name install could silently resolve to the wrong source"
fi

# --- 3. Installed receipt ---------------------------------------------------
receipt_list="$(brew list --formula --full-name 2>/dev/null || true)"
printf '%s\n' "$receipt_list" | grep -Fxq "$HOMEBREW_CANONICAL_FORMULA" || fail "brew list --formula --full-name does not include $HOMEBREW_CANONICAL_FORMULA -- it is not actually installed as the canonical tap/formula"

# --- 4. Cross-check ----------------------------------------------------------
info_json="$(brew info --json=v2 "$HOMEBREW_CANONICAL_FORMULA" 2>/dev/null)" || fail "brew info --json=v2 $HOMEBREW_CANONICAL_FORMULA failed"
command -v jq >/dev/null 2>&1 || fail "jq is required to parse brew info --json=v2 output"
printf '%s' "$info_json" | jq -e . >/dev/null 2>&1 || fail "brew info --json=v2 $HOMEBREW_CANONICAL_FORMULA returned unparseable JSON"

info_full_name="$(printf '%s' "$info_json" | jq -r '.formulae[0].full_name // empty')"
test "$info_full_name" = "$HOMEBREW_CANONICAL_FORMULA" || fail "brew info --json=v2 reports full_name=$info_full_name, expected $HOMEBREW_CANONICAL_FORMULA"

installed_version="$(printf '%s' "$info_json" | jq -r '.formulae[0].installed[0].version // empty')"
test -n "$installed_version" || fail "brew info --json=v2 reports no installed version for $HOMEBREW_CANONICAL_FORMULA -- it is resolvable but not actually installed"
test "$installed_version" = "$expected_version" || fail "brew-reported installed version ($installed_version) does not match the expected release version ($expected_version)"

prefix_path="$(brew --prefix "$HOMEBREW_CANONICAL_FORMULA" 2>/dev/null)" || fail "brew --prefix $HOMEBREW_CANONICAL_FORMULA failed"
test -n "$prefix_path" || fail "brew --prefix $HOMEBREW_CANONICAL_FORMULA returned an empty path"
resolved_keg="$(realpath "$prefix_path" 2>/dev/null)" || fail "could not resolve brew --prefix's own path ($prefix_path) with realpath"

binary_on_path="$(command -v "$HOMEBREW_CANONICAL_BINARY" 2>/dev/null || true)"
test -n "$binary_on_path" || fail "command -v $HOMEBREW_CANONICAL_BINARY found nothing on PATH"
resolved_binary="$(realpath "$binary_on_path" 2>/dev/null)" || fail "could not resolve $binary_on_path with realpath"
case "$resolved_binary" in
  "$resolved_keg"/*) : ;;
  *) fail "resolved binary ($resolved_binary) does not belong to the resolved keg for $HOMEBREW_CANONICAL_FORMULA ($resolved_keg) -- per 15-local-and-github-installation.md this is checked by comparing resolved paths, never by requiring 'command -v' itself to already point inside 'brew --prefix'" ;;
esac

alias_on_path="$(command -v "$HOMEBREW_CANONICAL_ALIAS" 2>/dev/null || true)"
test -n "$alias_on_path" || fail "command -v $HOMEBREW_CANONICAL_ALIAS found nothing on PATH"
resolved_alias="$(realpath "$alias_on_path" 2>/dev/null)" || fail "could not resolve $alias_on_path with realpath"
test "$resolved_alias" = "$resolved_binary" || fail "$HOMEBREW_CANONICAL_ALIAS ($resolved_alias) and $HOMEBREW_CANONICAL_BINARY ($resolved_binary) do not resolve to the same executable"

cli_version_output="$("$binary_on_path" version 2>/dev/null)" || fail "$binary_on_path version failed to run"
test "$cli_version_output" = "openrouter $expected_version" || fail "$binary_on_path version printed '$cli_version_output', want 'openrouter $expected_version'"

# --- 5. Disposable-tap cleanup ----------------------------------------------
# This script installs nothing and creates no tap of its own -- it only
# inspects state that already exists -- so there is no disposable tap for
# it to clean up after itself. It stays defensive anyway: it uses no
# temporary files, and it does not touch `local/homebrew-tap` or
# `local/tap/openrouter` (the disposable dev-smoke tap `make
# check-homebrew-formula`/`make homebrew-reinstall` use), which is a
# separate, deliberately-persistent local tool and out of scope here.

# --- 6. Evidence bundle ------------------------------------------------------
mkdir -p "$evidence_dir"
checked_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
jq -n \
  --arg tap "$HOMEBREW_CANONICAL_TAP" \
  --arg formula "$HOMEBREW_CANONICAL_FORMULA" \
  --arg receipt "$HOMEBREW_CANONICAL_FORMULA" \
  --arg binary_path "$resolved_binary" \
  --arg version "$installed_version" \
  --arg command "brew search $HOMEBREW_CANONICAL_SHORT_NAME; brew list --formula --full-name; brew info --json=v2 $HOMEBREW_CANONICAL_FORMULA; brew --prefix $HOMEBREW_CANONICAL_FORMULA; command -v $HOMEBREW_CANONICAL_BINARY; command -v $HOMEBREW_CANONICAL_ALIAS; realpath <each>; $HOMEBREW_CANONICAL_BINARY version" \
  --arg result "pass" \
  --arg checked_at "$checked_at" \
  '{tap: $tap, formula: $formula, receipt: $receipt, binary_path: $binary_path, version: $version, command: $command, result: $result, checked_at: $checked_at}' \
  >"$evidence_file"

printf 'PASS: %s is genuinely installed and reachable (version %s, binary %s); evidence written to %s\n' "$HOMEBREW_CANONICAL_FORMULA" "$installed_version" "$resolved_binary" "$evidence_file"

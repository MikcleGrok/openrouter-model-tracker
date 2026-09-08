#!/usr/bin/env bash
# Negative-case regression test for scripts/check-tag-protection.sh, modelled
# on scripts/provenance_profile_test.sh and scripts/sign_flags_test.sh: a
# disposable stub `gh` on PATH stands in for the real GitHub API/CLI, so
# every failure path (no ruleset, disabled enforcement, missing rule,
# unauthenticated, wrong repository) is exercised deterministically, with no
# real repository mutation and no network access. The one live/positive
# check -- that the real, currently-configured GitHub ruleset actually
# passes -- is left to `make tag-protection-check` itself, run separately
# against the real API.
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd -P)
GATE="$SCRIPT_DIR/check-tag-protection.sh"

REAL_REMOTE="$(git -C "$ROOT" remote get-url origin | sed -E 's#^git@github\.com:##; s#^https://github\.com/##; s#\.git$##')"

TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openrouter-tag-protection-test.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

fake_bin="$TEST_ROOT/bin"
mkdir -p "$fake_bin"
evidence_file="$ROOT/.release/tag-protection-evidence.json"

# write_stub_gh writes a fake `gh` that always authenticates successfully and
# answers `api repos/<repo>/rulesets`, `.../rulesets/<id>` and
# `.../tags/protection` from the JSON fixtures given as arguments (each may
# be omitted, in which case that endpoint 404s the way `gh api` does for a
# genuinely missing resource -- exit 1, nothing on stdout).
write_stub_gh() {
  local rulesets_json="$1" detail_json="${2:-}" legacy_json="${3:-}"
  cat >"$fake_bin/gh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
case "\$*" in
  "auth status")
    exit 0
    ;;
  api\ */rulesets)
    cat <<'JSON'
$rulesets_json
JSON
    ;;
  api\ */rulesets/*)
$( if [[ -n "$detail_json" ]]; then
  printf '    cat <<'"'"'JSON'"'"'\n%s\nJSON\n' "$detail_json"
else
  printf '    exit 1\n'
fi )
    ;;
  api\ */tags/protection)
$( if [[ -n "$legacy_json" ]]; then
  printf '    cat <<'"'"'JSON'"'"'\n%s\nJSON\n' "$legacy_json"
else
  printf '    exit 1\n'
fi )
    ;;
  *)
    printf 'unexpected gh invocation: %s\n' "\$*" >&2
    exit 99
    ;;
esac
EOF
  chmod 0755 "$fake_bin/gh"
}

run_gate() {
  rm -f "$evidence_file"
  PATH="$fake_bin:$PATH" GITHUB_REPOSITORY="${TEST_REPO:-$REAL_REMOTE}" bash "$GATE"
}

assert_blocked() {
  local label="$1" expect="$2"
  local out
  if out="$(run_gate 2>&1)"; then
    printf 'FAIL (%s): gate unexpectedly passed:\n%s\n' "$label" "$out" >&2
    exit 1
  fi
  printf '%s\n' "$out" | grep -Fq "$expect" || {
    printf 'FAIL (%s): expected message containing %q, got:\n%s\n' "$label" "$expect" "$out" >&2
    exit 1
  }
  test ! -s "$evidence_file" || {
    printf 'FAIL (%s): evidence file was written despite a blocked result\n' "$label" >&2
    exit 1
  }
  printf 'ok - %s\n' "$label"
}

# Case 1: no rulesets at all, no legacy record -- BLOCKED.
write_stub_gh '[]'
assert_blocked "no rulesets" "no active repository ruleset protects"

# Case 2: a tag ruleset exists but enforcement is "disabled" -- BLOCKED.
write_stub_gh \
  '[{"id":1,"target":"tag"}]' \
  '{"id":1,"enforcement":"disabled","conditions":{"ref_name":{"include":["refs/tags/v*"]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"}],"bypass_actors":[]}'
assert_blocked "disabled enforcement" "no active repository ruleset protects"

# Case 3: active ruleset, correct pattern, but missing non_fast_forward.
write_stub_gh \
  '[{"id":1,"target":"tag"}]' \
  '{"id":1,"enforcement":"active","conditions":{"ref_name":{"include":["refs/tags/v*"]}},"rules":[{"type":"deletion"}],"bypass_actors":[]}'
assert_blocked "missing non_fast_forward rule" "no active repository ruleset protects"

# Case 4: active ruleset, correct pattern, but missing deletion.
write_stub_gh \
  '[{"id":1,"target":"tag"}]' \
  '{"id":1,"enforcement":"active","conditions":{"ref_name":{"include":["refs/tags/v*"]}},"rules":[{"type":"non_fast_forward"}],"bypass_actors":[]}'
assert_blocked "missing deletion rule" "no active repository ruleset protects"

# Case 5: active ruleset with both rules, but it protects a different ref
# pattern (e.g. branches) and never matches refs/tags/v*.
write_stub_gh \
  '[{"id":1,"target":"tag"}]' \
  '{"id":1,"enforcement":"active","conditions":{"ref_name":{"include":["refs/tags/legacy-*"]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"}],"bypass_actors":[]}'
assert_blocked "wrong ref pattern" "no active repository ruleset protects"

# Case 6: canonical remote does not match GITHUB_REPOSITORY -- BLOCKED before
# any gh API call at all (no fixtures needed).
write_stub_gh '[]'
TEST_REPO="someone-else/unrelated-repo"
assert_blocked "repository mismatch" "does not match GITHUB_REPOSITORY"
unset TEST_REPO

# Case 7: gh reports as unauthenticated.
cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "auth status") exit 1 ;;
  *) exit 99 ;;
esac
EOF
chmod 0755 "$fake_bin/gh"
assert_blocked "gh not authenticated" "gh is not authenticated"

# Case 8 (positive control): active ruleset, correct pattern, both rules --
# MUST pass and MUST write evidence with the expected fields.
write_stub_gh \
  '[{"id":42,"target":"tag"}]' \
  '{"id":42,"enforcement":"active","conditions":{"ref_name":{"include":["refs/tags/v*"]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"}],"bypass_actors":[]}'
out="$(run_gate)"
printf '%s\n' "$out" | grep -Fq "PASS" || {
  printf 'FAIL (positive control): expected PASS, got:\n%s\n' "$out" >&2
  exit 1
}
test -s "$evidence_file" || {
  printf 'FAIL (positive control): evidence file was not written\n' >&2
  exit 1
}
command -v jq >/dev/null 2>&1 && {
  test "$(jq -r '.forbids_deletion' "$evidence_file")" = "true" || { printf 'FAIL: forbids_deletion is not true\n' >&2; exit 1; }
  test "$(jq -r '.forbids_force_update' "$evidence_file")" = "true" || { printf 'FAIL: forbids_force_update is not true\n' >&2; exit 1; }
  test "$(jq -r '.tag_pattern' "$evidence_file")" = "refs/tags/v*" || { printf 'FAIL: tag_pattern mismatch\n' >&2; exit 1; }
  test "$(jq -r '.result' "$evidence_file")" = "pass" || { printf 'FAIL: result is not pass\n' >&2; exit 1; }
}
rm -f "$evidence_file"
printf 'ok - %s\n' "positive control"

printf '%s\n' 'check-tag-protection negative-case tests passed'

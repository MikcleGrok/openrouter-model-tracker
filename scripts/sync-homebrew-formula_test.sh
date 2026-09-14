#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

SCRIPT="$ROOT/scripts/sync-homebrew-formula.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# scripts/sync-homebrew-formula.sh resolves its exact release tag the same
# way winget-manifest.sh/scoop-manifest.sh do when no TAG is supplied on the
# command line (git describe --tags --exact-match against HEAD), so this
# test tags the current commit itself instead of hardcoding a tag from
# whichever commit the test was first written against -- same pattern as
# scripts/winget-manifest_test.sh and scripts/provenance_profile_test.sh. A
# fictitious version distinct from those siblings' own v0.0.0/v0.0.9 avoids
# any chance of collision if several of these tests happen to run around the
# same time.
test_tag=v0.0.8
test_version=0.0.8

fixture_dir=""
cleanup() {
  rm -rf "$fixture_dir"
  git tag -d "$test_tag" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Register the trap (above) before creating the tag: if anything between tag
# creation and trap setup were to fail (e.g. mktemp -d), the tag would leak
# into the real repo with nothing left to clean it up.
git tag "$test_tag" HEAD

fixture_dir="$(mktemp -d)"
formula_path="$fixture_dir/Taps/local/homebrew-tap/Formula/openrouter-devtap.rb"
legacy_path="$fixture_dir/Taps/local/homebrew-tap/Formula/openrouter.rb"

run_sync() {
  FORMULA_PATH="$formula_path" "$SCRIPT" "$@"
}

# --- sync into a missing path creates it (mkdir -p the formula dir too) ---
test ! -e "$formula_path" || fail 'fixture setup: formula_path must not pre-exist'
sync_create_out="$fixture_dir/sync-create.out"
run_sync "$test_tag" > "$sync_create_out" 2>&1 \
  || { cat "$sync_create_out" >&2; fail 'sync into a missing path failed'; }
test -f "$formula_path" || fail 'sync into a missing path did not create the formula file'
grep -Fq 'created' "$sync_create_out" \
  || fail 'sync into a missing path did not report "created"'

printf '%s\n' 'sync creates a missing formula: OK'

# --- rendered content shape ---------------------------------------------
grep -Fq 'class OpenrouterDevtap' "$formula_path" \
  || fail 'rendered formula missing "class OpenrouterDevtap"'
grep -Fq 'keg_only' "$formula_path" \
  || fail 'rendered formula missing "keg_only"'
grep -Fq 'bin/"openrouter-devtap"' "$formula_path" \
  || fail 'rendered formula missing bin/"openrouter-devtap"'

url_lines="$(grep -c '^[[:space:]]*url "' "$formula_path" || true)"
test "$url_lines" -eq 1 || fail "rendered formula must contain exactly one url line (found $url_lines)"
version_lines="$(grep -c '^[[:space:]]*version "' "$formula_path" || true)"
test "$version_lines" -eq 1 || fail "rendered formula must contain exactly one version line (found $version_lines)"
awk '/^[[:space:]]*url "/ { url_line=NR } /^[[:space:]]*version "/ { version_line=NR } END { exit !(version_line == url_line + 1) }' "$formula_path" \
  || fail 'rendered formula version line must immediately follow the url line'

# The single most likely regression this test exists to catch: the shell
# mangling Ruby's #{version} string interpolation into something else while
# the heredoc is rendered. It must survive completely literal.
grep -Fq 'assert_equal "openrouter #{version}\n"' "$formula_path" \
  || fail 'rendered formula test must assert the formula version dynamically (literal #{version} did not survive)'

if grep -Fq 'install_symlink' "$formula_path"; then fail 'rendered formula must not install_symlink (would collide with the published channel)'; fi
if grep -Fq 'bin.install' "$formula_path"; then fail 'rendered formula must not use bin.install'; fi
if grep -Fq '"omt"' "$formula_path"; then fail 'rendered formula must not reference the omt alias'; fi
# "openrouter-model-tracker" legitimately appears in the formula's prose
# `desc` field (naming what this is a disposable dev-tap build OF) -- the
# plan's own literal template puts it there. What must never happen is that
# name being used as an *install target*: the binary path handed to `-o` or
# to bin.install/install_symlink.
if grep -Eq '(-o"?,? *|bin\.install |install_symlink )"?bin/"?openrouter-model-tracker' "$formula_path"; then
  fail 'rendered formula must not install a binary named openrouter-model-tracker (that name belongs to the published formula)'
fi

printf '%s\n' 'rendered formula shape (class/keg_only/binary/url+version/literal #{version}/no symlink/no omt): OK'

# --- tag/revision/version actually got substituted -----------------------
expected_revision="$(git rev-list -n 1 "$test_tag^{commit}")"
grep -Fq "tag: \"$test_tag\"" "$formula_path" || fail 'rendered formula does not contain the resolved tag'
grep -Fq "revision: \"$expected_revision\"" "$formula_path" || fail 'rendered formula does not contain the resolved revision'
grep -Fqx "  version \"$test_version\"" "$formula_path" || fail 'rendered formula does not contain the resolved version'

printf '%s\n' 'tag/revision/version substitution: OK'

# --- --check passes right after sync -------------------------------------
run_sync --check "$test_tag" > "$fixture_dir/check-ok.out" 2>&1 \
  || { cat "$fixture_dir/check-ok.out" >&2; fail '--check failed immediately after a sync'; }

printf '%s\n' '--check passes after sync: OK'

# --- a one-byte mutation makes --check fail with BLOCKED ------------------
printf 'x' >> "$formula_path"
check_stale_out="$fixture_dir/check-stale.out"
if run_sync --check "$test_tag" > "$check_stale_out" 2>&1; then
  cat "$check_stale_out" >&2
  fail '--check unexpectedly passed against a mutated formula'
fi
grep -Fq 'BLOCKED: formula is stale for' "$check_stale_out" \
  || fail '--check against a mutated formula did not print the expected "formula is stale for" BLOCKED: message'

printf '%s\n' '--check fails BLOCKED on a one-byte mutation: OK'

# --- re-sync restores byte-identity, and a second sync is a no-op --------
resync_out="$fixture_dir/resync.out"
run_sync "$test_tag" > "$resync_out" 2>&1 \
  || { cat "$resync_out" >&2; fail 're-sync after mutation failed'; }
run_sync --check "$test_tag" > "$fixture_dir/check-after-resync.out" 2>&1 \
  || { cat "$fixture_dir/check-after-resync.out" >&2; fail '--check failed after re-sync restored byte-identity'; }

noop_out="$fixture_dir/sync-noop.out"
run_sync "$test_tag" > "$noop_out" 2>&1 \
  || { cat "$noop_out" >&2; fail 'second sync (already in sync) failed'; }
grep -Fqi 'already' "$noop_out" \
  || fail 'second sync did not report the formula as already synchronized (no-op)'

printf '%s\n' 're-sync restores byte-identity, second sync is a no-op: OK'

# --- legacy-collision latch: both sync and --check refuse -----------------
: > "$legacy_path"

latch_sync_out="$fixture_dir/latch-sync.out"
if run_sync "$test_tag" > "$latch_sync_out" 2>&1; then
  cat "$latch_sync_out" >&2
  fail 'sync unexpectedly succeeded while a legacy Formula/openrouter.rb exists'
fi
grep -Fq 'BLOCKED: legacy colliding formula still exists:' "$latch_sync_out" \
  || fail 'legacy-latch sync failure did not print the expected "legacy colliding formula still exists" BLOCKED: message'

latch_check_out="$fixture_dir/latch-check.out"
if run_sync --check "$test_tag" > "$latch_check_out" 2>&1; then
  cat "$latch_check_out" >&2
  fail '--check unexpectedly succeeded while a legacy Formula/openrouter.rb exists'
fi
grep -Fq 'BLOCKED: legacy colliding formula still exists:' "$latch_check_out" \
  || fail 'legacy-latch --check failure did not print the expected "legacy colliding formula still exists" BLOCKED: message'

rm -f "$legacy_path"

printf '%s\n' 'legacy-collision latch blocks both sync and --check: OK'

# --- --check against a wholly missing file fails (check mode never
#     creates one) ----------------------------------------------------------
rm -f "$formula_path"
missing_check_out="$fixture_dir/check-missing.out"
if run_sync --check "$test_tag" > "$missing_check_out" 2>&1; then
  cat "$missing_check_out" >&2
  fail '--check unexpectedly succeeded against a missing formula file'
fi
grep -Fq 'BLOCKED: formula not found:' "$missing_check_out" \
  || fail '--check against a missing formula did not print the expected "formula not found" BLOCKED: message'
test ! -e "$formula_path" \
  || fail '--check must never create the formula file, but it now exists'

printf '%s\n' '--check against a missing file fails without creating it: OK'

# --- --print renders to stdout and never touches the filesystem -----------
# formula_path was just removed by the --check-against-missing-file block
# above, so its continued absence after --print is proof --print wrote
# nothing to disk.
test ! -e "$formula_path" || fail 'fixture setup: formula_path must not pre-exist before the --print test'
print_out="$fixture_dir/print.out"
run_sync --print "$test_tag" > "$print_out" 2>&1 \
  || { cat "$print_out" >&2; fail '--print failed'; }
grep -Fq 'class OpenrouterDevtap' "$print_out" \
  || fail '--print did not emit the rendered formula to stdout'
grep -Fq "tag: \"$test_tag\"" "$print_out" \
  || fail '--print output does not contain the resolved tag'
test ! -e "$formula_path" \
  || fail '--print must never create the formula file, but it now exists'

printf '%s\n' '--print renders to stdout without touching the filesystem: OK'

# --- --help exits 0 and does not require any evidence ----------------------
"$SCRIPT" --help > /dev/null || fail '--help must exit 0'

printf '%s\n' 'sync-homebrew-formula.sh offline checks passed'

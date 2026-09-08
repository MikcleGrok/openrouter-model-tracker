#!/usr/bin/env bash
# Negative-case regression test for scripts/verify-homebrew-source.sh,
# modelled on scripts/provenance_profile_test.sh and
# scripts/sign_flags_test.sh: a disposable stub `brew` plus a fake installed
# "keg" under a temp root stand in for a real Homebrew installation, so
# every failure path (tap collision, missing receipt, version mismatch,
# foreign keg, mismatched alias, wrong CLI version string, formula not
# found at all) is exercised deterministically without touching the real
# Homebrew installation or the network. `realpath` and `jq` are used for
# real -- only `brew` and the installed binaries are faked.
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
GATE="$SCRIPT_DIR/verify-homebrew-source.sh"

FORMULA="mikclegrok/tools/openrouter-model-tracker"
SHORT_NAME="openrouter-model-tracker"
VERSION="1.16.6"

TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/openrouter-homebrew-source-test.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

fake_bin="$TEST_ROOT/fakebin"
mkdir -p "$fake_bin"

# build_good_fixture lays out a realistic keg + opt symlink + bin symlinks,
# matching what was verified live against the real machine: bin/<short-name>
# is the real file, bin/omt is a plain symlink to it, and
# $TEST_ROOT/bin/{<short-name>,omt} symlink into the keg the same way
# /opt/homebrew/bin does. Returns nothing; sets globals via the paths below.
KEG=""
OPT_PREFIX=""
BIN_DIR=""
build_good_fixture() {
  rm -rf "$TEST_ROOT/Cellar" "$TEST_ROOT/opt" "$TEST_ROOT/bin"
  KEG="$TEST_ROOT/Cellar/$SHORT_NAME/$VERSION"
  OPT_PREFIX="$TEST_ROOT/opt/$SHORT_NAME"
  BIN_DIR="$TEST_ROOT/bin"
  mkdir -p "$KEG/bin" "$TEST_ROOT/opt" "$BIN_DIR"
  cat >"$KEG/bin/$SHORT_NAME" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  version) printf 'openrouter %s\n' "$OMT_TEST_VERSION" ;;
  *) exit 1 ;;
esac
EOF
  chmod 0755 "$KEG/bin/$SHORT_NAME"
  ln -sf "$SHORT_NAME" "$KEG/bin/omt"
  ln -sf "../Cellar/$SHORT_NAME/$VERSION" "$OPT_PREFIX"
  ln -sf "../Cellar/$SHORT_NAME/$VERSION/bin/$SHORT_NAME" "$BIN_DIR/$SHORT_NAME"
  ln -sf "../Cellar/$SHORT_NAME/$VERSION/bin/omt" "$BIN_DIR/omt"
}

# write_stub_brew writes a fake `brew` whose three canned answers
# (search/list/info) and --prefix path are all overridable, so each test
# case only has to override what it's testing.
write_stub_brew() {
  local search_out="$1" list_out="$2" info_json="$3" prefix_out="$4"
  cat >"$fake_bin/brew" <<EOF
#!/usr/bin/env bash
set -euo pipefail
case "\$*" in
  "search $SHORT_NAME")
    printf '%s\n' '$search_out'
    ;;
  "list --formula --full-name")
    printf '%s\n' '$list_out'
    ;;
  "info --json=v2 $FORMULA")
    cat <<'JSON'
$info_json
JSON
    ;;
  "--prefix $FORMULA")
    printf '%s\n' '$prefix_out'
    ;;
  *)
    printf 'unexpected brew invocation: %s\n' "\$*" >&2
    exit 99
    ;;
esac
EOF
  chmod 0755 "$fake_bin/brew"
}

good_info_json() {
  local version="$1"
  printf '{"formulae":[{"full_name":"%s","installed":[{"version":"%s"}]}]}' "$FORMULA" "$version"
}

run_gate() {
  local reported_version="${OMT_TEST_VERSION:-$VERSION}"
  rm -f "$SCRIPT_DIR/../.release/homebrew-source-evidence.json"
  PATH="$BIN_DIR:$fake_bin:$PATH" OMT_TEST_VERSION="$reported_version" bash "$GATE" --version "$VERSION"
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
  printf 'ok - %s\n' "$label"
}

# Case 1: brew search finds nothing for the short name at all.
build_good_fixture
write_stub_brew "" "$FORMULA" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
assert_blocked "formula not found by search" "brew search found no formula named"

# Case 2: tap collision -- two different full names share the short name.
build_good_fixture
write_stub_brew "$(printf '%s\n%s' "$FORMULA" "someone-else/tap/$SHORT_NAME")" "$FORMULA" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
assert_blocked "tap collision" "tap collision"

# Case 3: not actually installed -- missing from the receipt list.
build_good_fixture
write_stub_brew "$FORMULA" "some/other/formula" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
assert_blocked "missing receipt" "does not include $FORMULA"

# Case 4: version mismatch between brew's own report and the expected release.
build_good_fixture
write_stub_brew "$FORMULA" "$FORMULA" "$(good_info_json "9.9.9")" "$OPT_PREFIX"
assert_blocked "brew-reported version mismatch" "does not match the expected release version"

# Case 5: foreign keg -- command -v resolves to a binary outside the
# resolved brew --prefix keg (a decoy earlier on PATH).
build_good_fixture
decoy_dir="$TEST_ROOT/decoy"
mkdir -p "$decoy_dir"
cat >"$decoy_dir/$SHORT_NAME" <<EOF
#!/usr/bin/env bash
case "\$1" in version) printf 'openrouter %s\n' "$VERSION" ;; *) exit 1 ;; esac
EOF
chmod 0755 "$decoy_dir/$SHORT_NAME"
ln -sf "$decoy_dir/$SHORT_NAME" "$decoy_dir/omt"
write_stub_brew "$FORMULA" "$FORMULA" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
out="$(PATH="$decoy_dir:$BIN_DIR:$fake_bin:$PATH" OMT_TEST_VERSION="$VERSION" bash "$GATE" --version "$VERSION" 2>&1)" && {
  printf 'FAIL (foreign keg): gate unexpectedly passed:\n%s\n' "$out" >&2
  exit 1
}
printf '%s\n' "$out" | grep -Fq "does not belong to the resolved keg" || {
  printf 'FAIL (foreign keg): expected keg-mismatch message, got:\n%s\n' "$out" >&2
  exit 1
}
printf 'ok - %s\n' "foreign keg on PATH"

# Case 6: omt alias resolves somewhere else entirely (stale/foreign symlink).
build_good_fixture
rm -f "$BIN_DIR/omt"
foreign_target="$TEST_ROOT/foreign-omt"
cat >"$foreign_target" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod 0755 "$foreign_target"
ln -sf "$foreign_target" "$BIN_DIR/omt"
write_stub_brew "$FORMULA" "$FORMULA" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
assert_blocked "omt alias mismatch" "do not resolve to the same executable"

# Case 7: the binary runs but prints a version string that does not match
# (e.g. a build that forgot to embed the release version).
build_good_fixture
write_stub_brew "$FORMULA" "$FORMULA" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
out="$(PATH="$BIN_DIR:$fake_bin:$PATH" OMT_TEST_VERSION="0.0.0-dev" bash "$GATE" --version "$VERSION" 2>&1)" && {
  printf 'FAIL (CLI version mismatch): gate unexpectedly passed:\n%s\n' "$out" >&2
  exit 1
}
printf '%s\n' "$out" | grep -Fq "version printed" || {
  printf 'FAIL (CLI version mismatch): expected a version-string mismatch message, got:\n%s\n' "$out" >&2
  exit 1
}
printf 'ok - %s\n' "CLI-reported version mismatch"

# Case 8 (positive control): a fully consistent fixture MUST pass and MUST
# write evidence with the expected fields.
build_good_fixture
write_stub_brew "$FORMULA" "$FORMULA" "$(good_info_json "$VERSION")" "$OPT_PREFIX"
out="$(run_gate)"
printf '%s\n' "$out" | grep -Fq "PASS" || {
  printf 'FAIL (positive control): expected PASS, got:\n%s\n' "$out" >&2
  exit 1
}
evidence_file="$SCRIPT_DIR/../.release/homebrew-source-evidence.json"
test -s "$evidence_file" || {
  printf 'FAIL (positive control): evidence file was not written\n' >&2
  exit 1
}
test "$(jq -r '.formula' "$evidence_file")" = "$FORMULA" || { printf 'FAIL: evidence formula mismatch\n' >&2; exit 1; }
test "$(jq -r '.version' "$evidence_file")" = "$VERSION" || { printf 'FAIL: evidence version mismatch\n' >&2; exit 1; }
test "$(jq -r '.result' "$evidence_file")" = "pass" || { printf 'FAIL: evidence result is not pass\n' >&2; exit 1; }
rm -f "$evidence_file"
printf 'ok - %s\n' "positive control"

printf '%s\n' 'verify-homebrew-source negative-case tests passed'

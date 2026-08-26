#!/usr/bin/env bash
set -euo pipefail

ROOT=$(unset CDPATH; cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/openrouter-install-test.XXXXXX")
trap 'rm -rf "$TEST_ROOT"' EXIT
PREFIX="$TEST_ROOT/prefix with spaces"
BINDIR="$PREFIX/custom bin"
OUTSIDE="$TEST_ROOT/outside"
VERSION=9.9.9
INSTALLER="$ROOT/scripts/install.sh"
BUILD_ARTIFACT="$ROOT/bin/openrouter"
GO_BIN=${GO:-go}

if test -e "$BUILD_ARTIFACT"; then
  artifact_before=$(shasum -a 256 "$BUILD_ARTIFACT")
  artifact_existed=yes
else
  artifact_before=
  artifact_existed=no
fi

test_failure() {
  expected_status=$1
  shift
  set +e
  "$@"
  status=$?
  set -e
  test "$status" -eq "$expected_status" || { printf 'unexpected exit status: got %s, expected %s\n' "$status" "$expected_status" >&2; exit 1; }
}

make -C "$ROOT" install GO="$GO_BIN" PREFIX="$PREFIX" BINDIR="$BINDIR" VERSION="$VERSION" TARGET=./cmd/openrouter
test -x "$BINDIR/openrouter"
test "$(stat -c '%a' "$BINDIR/openrouter" 2>/dev/null || stat -f '%Lp' "$BINDIR/openrouter")" = 755
test "$("$BINDIR/openrouter" --version)" = "openrouter version $VERSION"
MARKER="$BINDIR/openrouter.openrouter-owner"
test -f "$MARKER"
test "$(stat -c '%a' "$MARKER" 2>/dev/null || stat -f '%Lp' "$MARKER")" = 600
test "$(sed -n '1p' "$MARKER")" = openrouter-installer-marker-v1
test "$(sed -n '2p' "$MARKER")" = identifier=openrouter-model-tracker
test "$(sed -n '3p' "$MARKER")" = "destination=$BINDIR/openrouter"
test "$(sed -n '4p' "$MARKER")" = "version=$VERSION"

test_failure 1 "$INSTALLER" install "$BINDIR/openrouter" relative/openrouter "$VERSION"
test_failure 1 "$INSTALLER" install "$BINDIR/openrouter" "" "$VERSION"
test_failure 2 make -C "$ROOT" install GO="$GO_BIN" PREFIX=relative-prefix BINDIR="$BINDIR" VERSION="$VERSION"
test_failure 2 make -C "$ROOT" install GO="$GO_BIN" PREFIX= BINDIR="$BINDIR" VERSION="$VERSION"
test_failure 2 make -C "$ROOT" install GO="$GO_BIN" PREFIX="$PREFIX" BINDIR=relative-bin VERSION="$VERSION"
test_failure 2 make -C "$ROOT" install GO="$GO_BIN" PREFIX="$PREFIX" BINDIR= VERSION="$VERSION"
test "$("$BINDIR/openrouter" version)" = "openrouter $VERSION"
"$BINDIR/openrouter" --help >/dev/null

make -C "$ROOT" upgrade GO="$GO_BIN" PREFIX="$PREFIX" BINDIR="$BINDIR" VERSION="$VERSION"
make -C "$ROOT" reinstall GO="$GO_BIN" PREFIX="$PREFIX" BINDIR="$BINDIR" VERSION="$VERSION"
test "$("$BINDIR/openrouter" --version)" = "openrouter version $VERSION"
test -f "$MARKER"

printf '%s\n' unmarked > "$BINDIR/unmarked"
test_failure 0 "$INSTALLER" uninstall "$BINDIR/unmarked"
test -f "$BINDIR/unmarked"
cp "$MARKER" "$TEST_ROOT/marker-before-mismatch"
printf '%s\n' openrouter-installer-marker-v1 identifier=openrouter-model-tracker "destination=$BINDIR/other" "version=$VERSION" > "$MARKER"
chmod 600 "$MARKER"
cp "$MARKER" "$TEST_ROOT/marker-mismatch"
test_failure 1 "$INSTALLER" uninstall "$BINDIR/openrouter"
cmp -s "$TEST_ROOT/marker-mismatch" "$MARKER"
cp "$TEST_ROOT/marker-before-mismatch" "$MARKER"

binary_before_marker_version=$(shasum -a 256 "$BINDIR/openrouter")
printf '%s\n' openrouter-installer-marker-v1 identifier=openrouter-model-tracker "destination=$BINDIR/openrouter" 'version=1.0.0' > "$MARKER"
chmod 600 "$MARKER"
test_failure 1 "$INSTALLER" uninstall "$BINDIR/openrouter"
test "$(shasum -a 256 "$BINDIR/openrouter")" = "$binary_before_marker_version"
test -f "$MARKER"
cp "$TEST_ROOT/marker-before-mismatch" "$MARKER"

printf '%s\n' openrouter-installer-marker-v1 identifier=openrouter-model-tracker "destination=$BINDIR/openrouter" > "$MARKER"
chmod 600 "$MARKER"
test_failure 1 "$INSTALLER" uninstall "$BINDIR/openrouter"
test "$(shasum -a 256 "$BINDIR/openrouter")" = "$binary_before_marker_version"
test -f "$MARKER"
cp "$TEST_ROOT/marker-before-mismatch" "$MARKER"

binary_before=$(shasum -a 256 "$BINDIR/openrouter")
marker_before=$(shasum -a 256 "$MARKER")
test_failure 1 "$INSTALLER" install "$TEST_ROOT/missing" "$BINDIR/openrouter" "$VERSION"
test "$(shasum -a 256 "$BINDIR/openrouter")" = "$binary_before"
test "$(shasum -a 256 "$MARKER")" = "$marker_before"

mkdir -p "$OUTSIDE"
printf '%s\n' outside > "$OUTSIDE/marker"
ln -s "$OUTSIDE" "$PREFIX/escape"
test_failure 1 "$INSTALLER" install "$BINDIR/openrouter" "$PREFIX/escape/openrouter" "$VERSION"
test -f "$OUTSIDE/marker"
test ! -e "$OUTSIDE/openrouter"

ln -s "$BINDIR/openrouter" "$BINDIR/install-link"
test_failure 1 "$INSTALLER" install "$BINDIR/openrouter" "$BINDIR/install-link" "$VERSION"
test_failure 1 "$INSTALLER" uninstall "$BINDIR/install-link"
test -L "$BINDIR/install-link"

printf '%s\n' '#!/bin/sh' 'printf "%s\\n" "openrouter version 1.0.0"' > "$TEST_ROOT/wrong"
chmod 0755 "$TEST_ROOT/wrong"
printf '%s\n' existing > "$BINDIR/wrong"
test_failure 1 "$INSTALLER" install "$TEST_ROOT/wrong" "$BINDIR/wrong" "$VERSION"
test "$(cat "$BINDIR/wrong")" = existing
test ! -e "$BINDIR/wrong.tmp"

cat > "$TEST_ROOT/invalid-source-preserve" <<'EOF'
#!/bin/sh
case "$1" in
  --version) printf '%s\n' 'openrouter version 1.0.0' ;;
  version) printf '%s\n' 'openrouter 1.0.0' ;;
  --help) exit 0 ;;
  *) exit 0 ;;
esac
EOF
chmod 0755 "$TEST_ROOT/invalid-source-preserve"
printf '%s\n' preserved > "$BINDIR/invalid-source-preserve"
test_failure 1 "$INSTALLER" install "$TEST_ROOT/invalid-source-preserve" "$BINDIR/invalid-source-preserve" "$VERSION"
test "$(cat "$BINDIR/invalid-source-preserve")" = preserved

printf '%s\n' '#!/bin/sh' > "$TEST_ROOT/non-executable"
test_failure 1 "$INSTALLER" install "$TEST_ROOT/non-executable" "$BINDIR/non-executable" "$VERSION"
test_failure 1 "$INSTALLER" install "$TEST_ROOT/missing" "$BINDIR/missing" "$VERSION"
mkdir -p "$BINDIR/directory"
test_failure 1 "$INSTALLER" install "$BINDIR/openrouter" "$BINDIR/directory" "$VERSION"
test_failure 2 "$INSTALLER" install "$BINDIR/openrouter" "$BINDIR/invalid"

PERMISSION_TARGET="$TEST_ROOT/no-write"
mkdir -p "$PERMISSION_TARGET"
chmod 0555 "$PERMISSION_TARGET"
test_failure 1 "$INSTALLER" install "$BINDIR/openrouter" "$PERMISSION_TARGET/openrouter" "$VERSION"
chmod 0755 "$PERMISSION_TARGET"
test ! -e "$PERMISSION_TARGET/openrouter"

UNINSTALL_PARENT="$TEST_ROOT/uninstall-parent"
mkdir -p "$OUTSIDE/uninstall"
printf '%s\n' protected > "$OUTSIDE/uninstall/marker"
ln -s "$OUTSIDE/uninstall" "$UNINSTALL_PARENT"
test_failure 1 "$INSTALLER" uninstall "$UNINSTALL_PARENT/openrouter"
test -f "$OUTSIDE/uninstall/marker"
test ! -e "$OUTSIDE/uninstall/openrouter"

PREFIX_ONLY="$TEST_ROOT/prefix-only"
EXPLICIT_BINDIR="$TEST_ROOT/explicit bin"
make -C "$ROOT" install GO="$GO_BIN" PREFIX="$PREFIX_ONLY" BINDIR="$EXPLICIT_BINDIR" VERSION="$VERSION"
test -x "$EXPLICIT_BINDIR/openrouter"
test ! -e "$PREFIX_ONLY/bin/openrouter"

CONCURRENT_TARGET="$TEST_ROOT/concurrent-target"
mkdir -p "$CONCURRENT_TARGET"
BASE_BINARY="$BINDIR/openrouter"
BASE_BINARY_QUOTED=$(printf '%q' "$BASE_BINARY")
mkdir "$CONCURRENT_TARGET/.openrouter-install.lock"
test_failure 1 env OPENROUTER_INSTALL_LOCK_TIMEOUT_SECONDS=0 "$INSTALLER" install "$BASE_BINARY" "$CONCURRENT_TARGET/openrouter" "$VERSION"
rmdir "$CONCURRENT_TARGET/.openrouter-install.lock"
pids=()
for i in $(seq 1 48); do
  concurrent_version="9.9.$i"
  concurrent_source="$TEST_ROOT/source-$i"
  # The generated wrapper intentionally contains an expanded test version.
  # shellcheck disable=SC2016
  printf '%s\n' '#!/usr/bin/env bash' 'case "${1:-}" in' "  --version) printf '%s\\n' \"openrouter version $concurrent_version\" ;;" "  version) printf '%s\\n' \"openrouter $concurrent_version\" ;;" '  --help) exit 0 ;;' "  *) exec $BASE_BINARY_QUOTED \"\$@\" ;;" 'esac' > "$concurrent_source"
  chmod 0755 "$concurrent_source"
  "$INSTALLER" install "$concurrent_source" "$CONCURRENT_TARGET/openrouter" "$concurrent_version" &
  pids+=("$!")
done
for pid in "${pids[@]}"; do
  wait "$pid"
done
test -x "$CONCURRENT_TARGET/openrouter"
final_concurrent_version=$("$CONCURRENT_TARGET/openrouter" --version)
case "$final_concurrent_version" in
  'openrouter version 9.9.'[1-9]|'openrouter version 9.9.'[1-4][0-9]) ;;
  *) printf '%s\n' "unexpected concurrent final version: $final_concurrent_version" >&2; exit 1 ;;
esac
final_short_version=$("$CONCURRENT_TARGET/openrouter" version)
case "$final_short_version" in
  'openrouter 9.9.'[1-9]|'openrouter 9.9.'[1-4][0-9]) ;;
  *) printf '%s\n' "unexpected concurrent short version: $final_short_version" >&2; exit 1 ;;
esac
"$CONCURRENT_TARGET/openrouter" --help >/dev/null
test -f "$CONCURRENT_TARGET/openrouter.openrouter-owner"

LOCK_TARGET="$TEST_ROOT/lock-replacement"
mkdir -p "$LOCK_TARGET"
cat > "$TEST_ROOT/slow-source" <<'EOF'
#!/bin/sh
case "${1:-}" in
  --version) sleep 1; printf '%s\n' 'openrouter version 9.9.9' ;;
  version) printf '%s\n' 'openrouter 9.9.9' ;;
  --help) exit 0 ;;
esac
EOF
chmod 0755 "$TEST_ROOT/slow-source"
"$INSTALLER" install "$TEST_ROOT/slow-source" "$LOCK_TARGET/openrouter" 9.9.9 &
slow_pid=$!
while test ! -f "$LOCK_TARGET/.openrouter-install.lock/owner"; do sleep 0.01; done
replacement_attempts=0
while ! test -e "$LOCK_TARGET"/.openrouter.owner.tmp.*; do
  replacement_attempts=$((replacement_attempts + 1))
  test "$replacement_attempts" -le 500 || { printf '%s\n' 'timed out waiting for installer marker staging' >&2; exit 1; }
  sleep 0.01
done
printf '%s\n' foreign-owner > "$LOCK_TARGET/.openrouter-install.lock/owner"
wait "$slow_pid" || slow_status=$?
test "${slow_status:-0}" -eq 0
test "$("$LOCK_TARGET/openrouter" --version)" = 'openrouter version 9.9.9'
test "$("$LOCK_TARGET/openrouter" version)" = 'openrouter 9.9.9'
"$LOCK_TARGET/openrouter" --help >/dev/null
test -d "$LOCK_TARGET/.openrouter-install.lock"
test "$(cat "$LOCK_TARGET/.openrouter-install.lock/owner")" = foreign-owner
rm -rf "$LOCK_TARGET/.openrouter-install.lock"

test_failure 2 make -C "$ROOT" install GO="$GO_BIN" PREFIX="$PREFIX" BINDIR="$PREFIX/escape/bin" VERSION="$VERSION"
test -f "$OUTSIDE/marker"
test ! -e "$OUTSIDE/bin/openrouter"

make -C "$ROOT" uninstall GO="$GO_BIN" PREFIX="$PREFIX" BINDIR="$BINDIR"
test ! -e "$BINDIR/openrouter"
make -C "$ROOT" uninstall GO="$GO_BIN" PREFIX="$PREFIX" BINDIR="$BINDIR"
test ! -e "$BINDIR/openrouter"

if test "$artifact_existed" = yes; then
  test "$(shasum -a 256 "$BUILD_ARTIFACT")" = "$artifact_before"
else
  test ! -e "$BUILD_ARTIFACT"
fi
printf '%s\n' 'install smoke: Makefile install/upgrade/reinstall, symlink rejection, validation, precedence, spaces, TARGET, preservation, and idempotent uninstall passed'

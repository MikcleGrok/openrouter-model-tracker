#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
FORMULA_PATH="${FORMULA_PATH:-}"
MODE=sync
TAG=""

usage() {
  printf '%s\n' "Usage: $0 [--check] [TAG]"
  printf '%s\n' 'TAG defaults to the exact tag checked out in the project repository.'
  printf '%s\n' 'FORMULA_PATH overrides the local Homebrew formula path.'
}

while test "$#" -gt 0; do
  case "$1" in
    --check)
      MODE=check
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --*)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
    *)
      test -z "$TAG" || { printf '%s\n' 'Only one tag may be provided.' >&2; exit 2; }
      TAG="$1"
      ;;
  esac
  shift
done

if test -z "$TAG"; then
  TAG="$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || true)"
fi
test -n "$TAG" || { printf '%s\n' 'An exact release tag is required.' >&2; exit 1; }
case "$TAG" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) printf '%s\n' "Tag must be a vMAJOR.MINOR.PATCH tag: $TAG" >&2; exit 1 ;;
esac

REVISION="$(git -C "$ROOT" rev-list -n 1 "$TAG^{commit}")"
test -n "$REVISION" || { printf '%s\n' "Cannot resolve immutable revision for $TAG." >&2; exit 1; }
VERSION="${TAG#v}"

if test -z "$FORMULA_PATH"; then
  BREW_ROOT="$(brew --repository)"
  FORMULA_PATH="$BREW_ROOT/Library/Taps/local/homebrew-tap/Formula/openrouter.rb"
fi
test -f "$FORMULA_PATH" || { printf '%s\n' "Formula not found: $FORMULA_PATH" >&2; exit 1; }

EXPECTED_URL="  url \"file://$ROOT\", using: :git, tag: \"$TAG\", revision: \"$REVISION\""
EXPECTED_VERSION="  version \"$VERSION\""
ACTUAL_URL="$(awk '/^[[:space:]]*url \"/ { print; count++ } END { if (count != 1) exit 2 }' "$FORMULA_PATH")" || {
  printf '%s\n' 'Formula must contain exactly one url declaration.' >&2
  exit 1
}
ACTUAL_VERSION="$(awk '/^[[:space:]]*version \"/ { print; count++ } END { if (count > 1) exit 2 }' "$FORMULA_PATH")" || {
  printf '%s\n' 'Formula must contain at most one version declaration.' >&2
  exit 1
}
awk '/assert_equal "openrouter #\{version\}\\n"/ { found=1 } END { exit !found }' "$FORMULA_PATH" || {
  printf '%s\n' 'Formula test must assert the formula version dynamically.' >&2
  exit 1
}

if test "$MODE" = check; then
  test "$ACTUAL_URL" = "$EXPECTED_URL" && test "$ACTUAL_VERSION" = "$EXPECTED_VERSION" || {
    printf '%s\n' "Formula is stale for $TAG:"
    printf 'expected: %s\nactual:   %s\n' "$EXPECTED_URL" "$ACTUAL_URL"
    printf 'expected: %s\nactual:   %s\n' "$EXPECTED_VERSION" "${ACTUAL_VERSION:-missing}"
    exit 1
  }
  printf '%s\n' "Homebrew formula is synchronized: tag=$TAG revision=$REVISION version=$VERSION"
  exit 0
fi

TMP="$(mktemp "${FORMULA_PATH}.tmp.XXXXXX")"
trap 'rm -f "$TMP"' EXIT
awk -v url="$EXPECTED_URL" -v ver="$EXPECTED_VERSION" '
  /^[[:space:]]*url "/ {
    print url
    if (!seen_version) { print ver; seen_version = 1 }
    next
  }
  /^[[:space:]]*version "/ {
    if (!seen_version) { print ver; seen_version = 1 }
    next
  }
  { print }
' "$FORMULA_PATH" > "$TMP"
cmp -s "$FORMULA_PATH" "$TMP" || mv "$TMP" "$FORMULA_PATH"
trap - EXIT
rm -f "$TMP"
printf '%s\n' "Homebrew formula synchronized: tag=$TAG revision=$REVISION version=$VERSION"

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
FORMULA_PATH="${FORMULA_PATH:-}"
MODE=sync
TAG=""

usage() {
  printf '%s\n' "Usage: $0 [--check|--print] [TAG]"
  printf '%s\n' 'TAG defaults to the exact tag checked out in the project repository.'
  printf '%s\n' 'FORMULA_PATH overrides the local Homebrew dev-tap formula path.'
  printf '%s\n' '--check verifies the formula is byte-identical to the rendered template and never creates it.'
  printf '%s\n' '--print renders the formula to stdout without touching the filesystem.'
}

while test "$#" -gt 0; do
  case "$1" in
    --check)
      MODE=check
      ;;
    --print)
      MODE=print
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
  FORMULA_PATH="$BREW_ROOT/Library/Taps/local/homebrew-tap/Formula/openrouter-devtap.rb"
fi

# render_formula is the single in-repo source of truth for the disposable
# local dev-tap formula: the whole file is rendered here, every time, so the
# live formula in the external tap checkout can never drift from this
# template (the old script only patched two lines and let everything else --
# class name, binary name, symlinks, test block -- rot in place).
#
# The heredoc below uses a *quoted* delimiter ('FORMULA_TEMPLATE'), so the
# shell performs no expansion of any kind inside it: Ruby's own string
# interpolation (`#{version}`) survives completely literal, and the four
# dynamic values (root/tag/revision/version) are spliced in afterwards via
# plain bash parameter substitution against unambiguous __TOKEN__ markers.
render_formula() {
  local root="$1" tag="$2" revision="$3" version="$4"
  local template
  template="$(cat <<'FORMULA_TEMPLATE'
# GENERATED FILE -- do not edit by hand.
# Source of truth: render_formula() in scripts/sync-homebrew-formula.sh, in
# the project repository this dev-tap build was cut from. Run
# `make sync-homebrew-formula` there to regenerate; `./scripts/sync-homebrew-formula.sh --check`
# verifies byte-identity.
class OpenrouterDevtap < Formula
  desc "Disposable local dev-tap build of openrouter-model-tracker"
  homepage "https://openrouter.ai/"
  url "file://__ROOT__", using: :git, tag: "__TAG__", revision: "__REVISION__"
  version "__VERSION__"
  license "MIT"

  keg_only "it is a disposable local dev-tap build; linking it would collide with the published formula"

  depends_on "go" => :build

  def install
    system "go", "build", "-trimpath", "-ldflags", "-s -w -X main.version=#{version}", "-o", bin/"openrouter-devtap", "./cmd/openrouter"
    generate_completions_from_executable(bin/"openrouter-devtap", shell_parameter_format: :cobra, shells: [:bash])
  end

  test do
    assert_equal "openrouter #{version}\n", shell_output("#{bin}/openrouter-devtap version")
    assert_equal "openrouter version #{version}\n", shell_output("#{bin}/openrouter-devtap --version")
    system bin/"openrouter-devtap", "--help"
    assert_match "__start_openrouter", (bash_completion/"openrouter-devtap").read
  end
end
FORMULA_TEMPLATE
)"
  template="${template//__ROOT__/$root}"
  template="${template//__TAG__/$tag}"
  template="${template//__REVISION__/$revision}"
  template="${template//__VERSION__/$version}"
  printf '%s\n' "$template"
}

# self_guard checks the rendered text itself, before it is ever written to
# disk or compared against a live formula. It is the anti-regression net for
# the single most likely mistake in this script: the shell silently mangling
# Ruby's `#{version}` interpolation into something byte-different from what
# ships (verified live: an unquoted heredoc leaves `#{version}` untouched
# since it has no leading `$`, but the quoted-heredoc + token-substitution
# design here removes the hazard by construction rather than by luck).
self_guard() {
  local content_file="$1"
  local url_count version_count

  url_count="$(grep -c '^[[:space:]]*url "' "$content_file" || true)"
  test "$url_count" -eq 1 || { printf '%s\n' "BLOCKED: rendered formula must contain exactly one url line (found $url_count)." >&2; exit 1; }

  version_count="$(grep -c '^[[:space:]]*version "' "$content_file" || true)"
  test "$version_count" -eq 1 || { printf '%s\n' "BLOCKED: rendered formula must contain exactly one version line (found $version_count)." >&2; exit 1; }

  awk '/^[[:space:]]*url "/ { url_line=NR } /^[[:space:]]*version "/ { version_line=NR } END { exit !(version_line == url_line + 1) }' "$content_file" \
    || { printf '%s\n' 'BLOCKED: rendered formula version line must immediately follow the url line.' >&2; exit 1; }

  grep -Fq 'assert_equal "openrouter #{version}\n"' "$content_file" \
    || { printf '%s\n' 'BLOCKED: rendered formula test must assert the formula version dynamically.' >&2; exit 1; }

  grep -Fq 'keg_only' "$content_file" \
    || { printf '%s\n' 'BLOCKED: rendered formula must be keg_only.' >&2; exit 1; }

  if grep -Fq 'install_symlink' "$content_file"; then
    printf '%s\n' 'BLOCKED: rendered formula must not install a symlink (would collide with the published channel).' >&2
    exit 1
  fi
  if grep -Fq 'bin.install' "$content_file"; then
    printf '%s\n' 'BLOCKED: rendered formula must not use bin.install.' >&2
    exit 1
  fi
  if grep -Fq '"omt"' "$content_file"; then
    printf '%s\n' 'BLOCKED: rendered formula must not reference the omt alias.' >&2
    exit 1
  fi
}

# legacy_latch refuses to run (sync or check) while the old, collision-shaped
# formula still exists next to the new one, so the one-time migration off it
# cannot be silently skipped.
legacy_latch() {
  local legacy_path
  legacy_path="$(dirname -- "$FORMULA_PATH")/openrouter.rb"
  if test -e "$legacy_path"; then
    printf '%s\n' "BLOCKED: legacy colliding formula still exists: $legacy_path" >&2
    printf '%s\n' 'DETAIL: confirm it is not installed (brew list --versions openrouter), then in the tap checkout: git rm Formula/openrouter.rb && git commit; re-run this script afterwards.' >&2
    exit 1
  fi
}

TMP_RENDER="$(mktemp)"
ATOMIC_TMP=""
cleanup() {
  rm -f "$TMP_RENDER"
  test -z "$ATOMIC_TMP" || rm -f "$ATOMIC_TMP"
}
trap cleanup EXIT

render_formula "$ROOT" "$TAG" "$REVISION" "$VERSION" > "$TMP_RENDER"
self_guard "$TMP_RENDER"

case "$MODE" in
  print)
    cat "$TMP_RENDER"
    exit 0
    ;;
  check)
    legacy_latch
    test -f "$FORMULA_PATH" || {
      printf '%s\n' "BLOCKED: formula not found: $FORMULA_PATH" >&2
      printf '%s\n' 'DETAIL: run make sync-homebrew-formula to create it (--check never creates one).' >&2
      exit 1
    }
    if cmp -s "$TMP_RENDER" "$FORMULA_PATH"; then
      printf '%s\n' "Homebrew formula is synchronized: tag=$TAG revision=$REVISION version=$VERSION"
      exit 0
    fi
    printf '%s\n' "BLOCKED: formula is stale for $TAG: $FORMULA_PATH" >&2
    diff -u "$FORMULA_PATH" "$TMP_RENDER" >&2 || true
    printf '%s\n' 'DETAIL: run make sync-homebrew-formula to regenerate it.' >&2
    exit 1
    ;;
  sync)
    legacy_latch
    mkdir -p -- "$(dirname -- "$FORMULA_PATH")"
    was_present=0
    if test -f "$FORMULA_PATH"; then
      was_present=1
      if cmp -s "$TMP_RENDER" "$FORMULA_PATH"; then
        printf '%s\n' "Homebrew formula already synchronized: tag=$TAG revision=$REVISION version=$VERSION"
        exit 0
      fi
    fi
    ATOMIC_TMP="$(mktemp "${FORMULA_PATH}.tmp.XXXXXX")"
    cp "$TMP_RENDER" "$ATOMIC_TMP"
    mv "$ATOMIC_TMP" "$FORMULA_PATH"
    ATOMIC_TMP=""
    if test "$was_present" -eq 1; then
      printf '%s\n' "Homebrew formula synchronized: tag=$TAG revision=$REVISION version=$VERSION"
    else
      printf '%s\n' "Homebrew formula created: tag=$TAG revision=$REVISION version=$VERSION"
    fi
    ;;
esac

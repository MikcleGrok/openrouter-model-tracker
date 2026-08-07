#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
guide_tools_root=${GUIDE_TOOLS_ROOT:-$(CDPATH= cd -- "$root/../guide-tools" && pwd)}
if test -n "${FORMULA_PATH:-}"; then formula=$FORMULA_PATH
elif command -v brew >/dev/null 2>&1; then formula="$(brew --repository)/Library/Taps/local/homebrew-tap/Formula/openrouter.rb"
else printf '%s\n' 'ERROR: brew is unavailable; set FORMULA_PATH explicitly.' >&2; exit 2; fi
safe_args=()
while test "$#" -gt 0; do
  case "$1" in
    --tag|--version|--format|--output|--installed-package|--installed-version|--brew-test) test "$#" -ge 2 || { printf '%s\n' "$1 requires a value" >&2; exit 2; }; safe_args+=("$1" "$2"); shift 2;;
    --check|--help|-h) safe_args+=("$1"); shift;;
    *) printf 'ERROR: unsupported wrapper argument: %s\n' "$1" >&2; exit 2;;
  esac
done
exec "$guide_tools_root/bin/guide-distribution-verify" "${safe_args[@]}" --profile formula --root "$root" --formula "$formula" --source-url "file://$root"

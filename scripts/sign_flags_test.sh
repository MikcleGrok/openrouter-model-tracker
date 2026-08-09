#!/usr/bin/env bash
set -euo pipefail
unset CDPATH

# Regression gate: every `cosign sign-blob` / `cosign attest-blob` call in
# this repo MUST carry --tlog-upload=false. Without that flag, cosign
# uploads the signing event to the public Rekor transparency log by
# default -- leaking the artifact digest, this repo's path, and the build
# timestamp, which is exactly the outcome the static-keypair signing design
# was chosen to avoid over keyless/Fulcio signing. See
# ~/projects/tools/guide-tools/.task/go-guide-compliance/signing-design.md
# §8 ("Фазирование и риски").
#
# scripts/verify-provenance.sh's --insecure-ignore-tlog is the
# *verification*-side flag for a related but different concern; it does not
# substitute for --tlog-upload=false on the signing side checked here.

ROOT="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"

sources=("$ROOT/Makefile")
for f in "$ROOT"/scripts/*.sh; do
  case "$f" in
    *_test.sh) ;;
    *) [[ -f "$f" ]] && sources+=("$f") ;;
  esac
done
for f in "$ROOT"/.github/workflows/*.yml; do
  [[ -f "$f" ]] && sources+=("$f")
done

matches="$(grep -n -E 'cosign (sign-blob|attest-blob) --' "${sources[@]}" || true)"

if [[ -z "$matches" ]]; then
  printf 'sign-flags gate found no cosign sign-blob/attest-blob invocations under %s -- test would be vacuous, investigate before trusting it\n' "$ROOT" >&2
  exit 1
fi

fail=0
while IFS= read -r line; do
  case "$line" in
    *--tlog-upload=false*) ;;
    *)
      printf 'MISSING --tlog-upload=false: %s\n' "$line" >&2
      fail=1
      ;;
  esac
done <<<"$matches"

if [[ "$fail" -ne 0 ]]; then
  printf '%s\n' 'BLOCKED: a cosign sign-blob/attest-blob invocation is missing --tlog-upload=false -- this would upload the signing event to the public Rekor transparency log' >&2
  exit 1
fi

printf 'sign-flags gate passed (%s invocation(s) verified)\n' "$(printf '%s\n' "$matches" | grep -c .)"

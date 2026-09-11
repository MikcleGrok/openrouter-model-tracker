#!/usr/bin/env bash
set -euo pipefail

ROOT=$(unset CDPATH; cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
BINARY=${1:?binary path is required}
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/openrouter-completion-test.XXXXXX")
trap 'rm -rf "$TEST_ROOT"' EXIT

ln -s "$BINARY" "$TEST_ROOT/openrouter"
ln -s "$BINARY" "$TEST_ROOT/omt"
export PATH="$TEST_ROOT:$PATH"

# Cobra's generated fallback uses this helper when bash-completion is absent.
_get_comp_words_by_ref() {
  cur=${COMP_WORDS[COMP_CWORD]}
  prev=${COMP_WORDS[COMP_CWORD-1]:-}
  words=("${COMP_WORDS[@]}")
  cword=$COMP_CWORD
}

source <(openrouter completion bash)
case "$(complete -p openrouter)" in
  'complete -o default -F __start_openrouter openrouter'|'complete -o default -o nospace -F __start_openrouter openrouter') ;;
  *) printf 'unexpected openrouter registration: %s\n' "$(complete -p openrouter)" >&2; exit 1 ;;
esac
case "$(complete -p omt)" in
  'complete -o default -F __start_openrouter omt'|'complete -o default -o nospace -F __start_openrouter omt') ;;
  *) printf 'unexpected omt registration: %s\n' "$(complete -p omt)" >&2; exit 1 ;;
esac
compopt() { :; }
COMP_TYPE=9
COLUMNS=120

check_candidates() {
  local command=$1
  shift
  local candidates
  COMP_WORDS=("$command" "")
  COMP_CWORD=1
  COMPREPLY=()
  __start_openrouter
  candidates=" ${COMPREPLY[*]} "
  for expected in "$@"; do
    [[ "$candidates" == *" $expected "* ]] || { printf 'missing %s for %s: %s\n' "$expected" "$command" "$candidates" >&2; exit 1; }
  done
}

check_candidates openrouter refresh update up
check_candidates omt refresh update up

COMP_WORDS=(openrouter --)
COMP_CWORD=1
COMPREPLY=()
__start_openrouter
candidates=" ${COMPREPLY[*]} "
for expected in --config --data-dir --help --version; do
  [[ "$candidates" == *" $expected "* ]] || { printf 'missing %s for root: %s\n' "$expected" "$candidates" >&2; exit 1; }
done

check_flag_candidates() {
  local command=$1
  local subcommand=$2
  local candidates
  COMP_WORDS=("$command" "$subcommand" --)
  COMP_CWORD=2
  COMPREPLY=()
  __start_openrouter
  candidates=" ${COMPREPLY[*]} "
  for expected in --force --dry-run --output --config --data-dir --help; do
    [[ "$candidates" == *" $expected "* ]] || { printf 'missing %s for %s %s: %s\n' "$expected" "$command" "$subcommand" "$candidates" >&2; exit 1; }
  done
}

for command in openrouter omt; do
  for subcommand in refresh update up; do
    check_flag_candidates "$command" "$subcommand"
  done
done

fish_output=$("$BINARY" completion fish)
require_fish_fragment() {
  local fragment=$1
  case "$fish_output" in
    *"$fragment"*) ;;
    *) printf 'fish completion is missing %s\n' "$fragment" >&2; exit 1 ;;
  esac
}
require_fish_fragment 'function __openrouter_requires_order_preservation'
require_fish_fragment 'function __openrouter_prepare_completions'
require_fish_fragment "complete -c omt -n 'not __openrouter_requires_order_preservation && __openrouter_prepare_completions'"
require_fish_fragment "complete -k -c omt -n '__openrouter_requires_order_preservation && __openrouter_prepare_completions'"
if command -v fish >/dev/null 2>&1; then
  fish -n <(printf '%s\n' "$fish_output")
else
  printf '%s\n' 'Fish unavailable; deterministic generated-output syntax/branch check passed.'
fi
printf '%s\n' 'Bash completion aliases and refresh/update/up flags passed.'

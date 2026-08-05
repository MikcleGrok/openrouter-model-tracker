#!/usr/bin/env bash

set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf '%s\n' 'Error: scripts/init.sh is supported on macOS only.' >&2
  exit 1
fi

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd -P)

CONFIG=${OPENROUTER_CONFIG:-"$HOME/.config/openrouter/config.yaml"}
DATA_DIR=${OPENROUTER_DATA_DIR:-"$ROOT"}
OUTPUT=${OPENROUTER_OUTPUT:-"$ROOT/docs/openrouter-model-comparison.md"}
OPEN_COMMAND=${OPENROUTER_OPEN:-open}

printf '%s\n' "Project root: $ROOT"
printf '%s\n' 'Building bin/openrouter through the Makefile...'
make -C "$ROOT" build

printf '%s\n' "Initializing config and cache (no network access): $CONFIG"
"$ROOT/bin/openrouter" init --config "$CONFIG" --data-dir "$DATA_DIR"

printf '%s\n' "Refreshing report from live sources: $OUTPUT"
"$ROOT/bin/openrouter" refresh --config "$CONFIG" --data-dir "$DATA_DIR" --output "$OUTPUT"

if [[ "${OPENROUTER_OPEN:-open}" == "0" ]]; then
  printf '%s\n' 'Skipping report opening because OPENROUTER_OPEN=0.'
else
  printf '%s\n' "Opening report: $OUTPUT"
  "$OPEN_COMMAND" "$OUTPUT"
fi

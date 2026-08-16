#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd -P)
DATE=/bin/date
CONFIG=${OPENROUTER_CONFIG:-"$HOME/.config/openrouter/config.yaml"}
DATA_DIR=${OPENROUTER_DATA_DIR:-"$ROOT"}
OUTPUT=${OPENROUTER_OUTPUT:-"$ROOT/docs/openrouter-model-comparison.md"}
BIN=${OPENROUTER_BIN:-"$ROOT/bin/openrouter"}

if [[ "${1:-}" == "install" ]]; then
  cron_line="0 * * * * $SCRIPT_DIR/cron-refresh.sh >> $HOME/Library/Logs/openrouter-refresh.log 2>&1"
  current_crontab=$(crontab -l 2>/dev/null || true)
  if printf '%s\n' "$current_crontab" | grep -Fqx "$cron_line"; then
    printf '%s\n' "openrouter: cron entry already installed"
    exit 0
  fi
  { printf '%s\n' "$current_crontab"; printf '%s\n' "$cron_line"; } | crontab -
  printf '%s\n' "openrouter: installed hourly cron entry"
  exit 0
fi

cutoff_day=$($DATE -v-6H +%F)
snapshot="$DATA_DIR/model-snapshot.json"
updated_day=""
if [[ -f "$snapshot" ]]; then
  updated_at=$(awk -F'"' '/"updated_at"/ {print $4; exit}' "$snapshot")
  if [[ -n "${updated_at:-}" ]]; then
    updated_day=$($DATE -j -f '%Y-%m-%dT%H:%M:%SZ' "$updated_at" +%F 2>/dev/null || true)
  else
    updated_day=$(awk -F'"' '/"fetched_at"/ {print $4; exit}' "$snapshot")
  fi
fi

if [[ "$updated_day" == "$cutoff_day" ]]; then
  printf '%s\n' "openrouter: data is current for $cutoff_day; skipping refresh"
  exit 0
fi

printf '%s\n' "openrouter: refreshing data for $cutoff_day"
"$BIN" refresh --config "$CONFIG" --data-dir "$DATA_DIR" --output "$OUTPUT"

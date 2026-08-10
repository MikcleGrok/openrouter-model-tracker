#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd -P)
LABEL=${OPENROUTER_LAUNCHD_LABEL:-com.openrouter.model-tracker.refresh}
PLIST_PATH=${OPENROUTER_LAUNCHD_PLIST:-"$HOME/Library/LaunchAgents/$LABEL.plist"}
CONFIG=${OPENROUTER_CONFIG:-"$HOME/.config/openrouter/config.yaml"}
DATA_DIR=${OPENROUTER_DATA_DIR:-"$ROOT"}
OUTPUT=${OPENROUTER_OUTPUT:-"$ROOT/docs/openrouter-model-comparison.md"}
BIN=${OPENROUTER_BIN:-"$ROOT/bin/openrouter"}
LOG_DIR=${OPENROUTER_LAUNCHD_LOG_DIR:-"$HOME/Library/Logs"}
DRY_RUN=${OPENROUTER_LAUNCHD_DRY_RUN:-0}

usage() {
  printf '%s\n' "Usage: $0 install|uninstall|status|run|start|validate"
}

xml_escape() {
  local value=$1
  value=${value//&/&amp;}
  value=${value//</&lt;}
  value=${value//>/&gt;}
  value=${value//\"/&quot;}
  value=${value//\'/&apos;}
  printf '%s' "$value"
}

write_plist() {
  local script_path cron_path config data_dir output bin stdout stderr
  script_path=$(xml_escape "$SCRIPT_DIR/launchd-refresh.sh")
  cron_path=$(xml_escape "$SCRIPT_DIR/cron-refresh.sh")
  config=$(xml_escape "$CONFIG")
  data_dir=$(xml_escape "$DATA_DIR")
  output=$(xml_escape "$OUTPUT")
  bin=$(xml_escape "$BIN")
  stdout=$(xml_escape "$LOG_DIR/openrouter-refresh.log")
  stderr=$(xml_escape "$LOG_DIR/openrouter-refresh.log")
  mkdir -p "$(dirname -- "$PLIST_PATH")" "$LOG_DIR"
  cat >"$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$script_path</string>
    <string>run</string>
    <string>$cron_path</string>
    <string>$config</string>
    <string>$data_dir</string>
    <string>$output</string>
    <string>$bin</string>
  </array>
  <key>StartInterval</key>
  <integer>900</integer>
  <key>RunAtLoad</key>
  <false/>
  <key>StandardOutPath</key>
  <string>$stdout</string>
  <key>StandardErrorPath</key>
  <string>$stderr</string>
</dict>
</plist>
EOF
}

launchctl() {
  command launchctl "$@"
}

case "${1:-}" in
  install)
    write_plist
    if [[ "$DRY_RUN" == "1" ]]; then
      printf '%s\n' "openrouter: dry-run wrote $PLIST_PATH"
      exit 0
    fi
    launchctl bootout "gui/$(id -u)" "$PLIST_PATH" >/dev/null 2>&1 || true
    launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH"
    printf '%s\n' "openrouter: installed LaunchAgent $LABEL"
    ;;
  uninstall)
    if [[ "$DRY_RUN" != "1" ]]; then
      launchctl bootout "gui/$(id -u)" "$PLIST_PATH" >/dev/null 2>&1 || true
    fi
    rm -f -- "$PLIST_PATH"
    printf '%s\n' "openrouter: uninstalled LaunchAgent $LABEL"
    ;;
  status)
    if [[ "$DRY_RUN" == "1" ]]; then
      [[ -f "$PLIST_PATH" ]] && printf '%s\n' "openrouter: plist exists at $PLIST_PATH" || printf '%s\n' "openrouter: plist is not installed"
      exit 0
    fi
    launchctl print "gui/$(id -u)/$LABEL"
    ;;
  run)
    [[ $# -eq 6 ]] || { usage >&2; exit 2; }
    export OPENROUTER_CONFIG=$3 OPENROUTER_DATA_DIR=$4 OPENROUTER_OUTPUT=$5 OPENROUTER_BIN=$6
    exec "$2"
    ;;
  start)
    launchctl kickstart "gui/$(id -u)/$LABEL"
    ;;
  validate)
    write_plist
    plutil -lint "$PLIST_PATH"
    printf '%s\n' "openrouter: valid plist at $PLIST_PATH"
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

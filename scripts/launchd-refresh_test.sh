#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/openrouter-launchd-test.XXXXXX")
trap 'rm -rf "$TEST_ROOT"' EXIT
PLIST_PATH="$TEST_ROOT/Library/LaunchAgents/refresh with spaces.plist"

OPENROUTER_LAUNCHD_DRY_RUN=1 \
OPENROUTER_LAUNCHD_PLIST="$PLIST_PATH" \
OPENROUTER_CONFIG="$TEST_ROOT/config with spaces.yaml" \
OPENROUTER_DATA_DIR="$TEST_ROOT/data with spaces" \
OPENROUTER_OUTPUT="$TEST_ROOT/output with spaces.md" \
OPENROUTER_BIN="$TEST_ROOT/bin with spaces/openrouter" \
"$SCRIPT_DIR/launchd-refresh.sh" install

test -f "$PLIST_PATH"
plutil -lint "$PLIST_PATH"
plutil -extract ProgramArguments xml1 -o - "$PLIST_PATH" | grep -Fq 'data with spaces'
test "$(plutil -extract StartInterval raw -o - "$PLIST_PATH")" = '900'
test "$(plutil -extract RunAtLoad raw -o - "$PLIST_PATH")" = 'false'
! plutil -extract StartCalendarInterval raw -o - "$PLIST_PATH" >/dev/null 2>&1
plutil -extract ProgramArguments xml1 -o - "$PLIST_PATH" | grep -Fq 'cron-refresh.sh'

OPENROUTER_LAUNCHD_DRY_RUN=1 OPENROUTER_LAUNCHD_PLIST="$PLIST_PATH" "$SCRIPT_DIR/launchd-refresh.sh" uninstall
test ! -e "$PLIST_PATH"
printf '%s\n' 'launchd-refresh: dry-run install/uninstall checks passed'

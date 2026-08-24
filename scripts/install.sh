#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf '%s\n' "Usage: $0 install SOURCE DEST VERSION" "       $0 uninstall DEST"
}

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit "${2:-1}"
}

validate_absolute_path() {
  path=$1
  name=$2
  test -n "$path" || fail "$name must not be empty"
  case "$path" in
    /*) ;;
    *) fail "$name must be an absolute path: $path" ;;
  esac
}

check_trusted_symlink() {
  path=$1
  if test -L "$path"; then
    resolved=$(cd -P "$path" && pwd -P) || fail "cannot resolve destination path: $path"
    case "$resolved" in
      /private/tmp|/private/var) ;;
      *) fail "destination path contains untrusted symlink: $path -> $resolved" ;;
    esac
  fi
}

command -v stat >/dev/null 2>&1 || fail 'stat is required'

ensure_trusted_directory() {
  directory=$1
  test -n "$directory" || fail 'destination directory must not be empty'
  case "$directory" in
    /*) current=/; components=${directory#/} ;;
    *) fail "destination directory must be absolute: $directory" ;;
  esac
  old_ifs=$IFS
  IFS=/
  set -f
  for component in $components; do
    test -n "$component" || continue
    test "$component" != . && test "$component" != .. || fail "destination contains unsafe path component: $directory"
    if test "$current" = /; then
      current="/$component"
    else
      current="$current/$component"
    fi
    check_trusted_symlink "$current"
    if test -e "$current"; then
      test -d "$current" || fail "destination path component is not a directory: $current"
    else
      mkdir -p "$current"
      test -d "$current" || fail "destination path component is not a directory: $current"
    fi
    check_trusted_symlink "$current"
  done
  set +f
  IFS=$old_ifs
}

acquire_lock() {
  lock_dir=$1/.openrouter-install.lock
  lock_owner_file=$lock_dir/owner
  lock_timeout_seconds=${OPENROUTER_INSTALL_LOCK_TIMEOUT_SECONDS:-60}
  case "$lock_timeout_seconds" in
    ''|*[!0-9]*) fail 'OPENROUTER_INSTALL_LOCK_TIMEOUT_SECONDS must be a non-negative integer' ;;
  esac
  lock_acquired=no
  lock_token=$(printf '%s:%s:%s' "$$" "$RANDOM" "$(date +%s 2>/dev/null || date +%s)")
  attempts=0
  while ! mkdir "$lock_dir" 2>/dev/null; do
    attempts=$((attempts + 1))
    test "$attempts" -le "$((lock_timeout_seconds * 10))" || fail "timed out waiting ${lock_timeout_seconds} seconds for installation lock: $lock_dir"
    sleep 0.1
  done
  lock_acquired=yes
  trap cleanup EXIT HUP INT TERM
  printf '%s\n' "$lock_token" > "$lock_owner_file" || fail "cannot write installation lock owner: $lock_owner_file"
  chmod 600 "$lock_owner_file" || fail "cannot protect installation lock owner: $lock_owner_file"
  test "$(cat "$lock_owner_file")" = "$lock_token" || fail "installation lock owner verification failed: $lock_owner_file"
}

cleanup() {
  status=$?
  rm -f "${temporary:-}" "${marker_temporary:-}" "${backup_binary:-}" "${backup_marker:-}" "${removal_binary:-}" "${removal_marker:-}"
  if test "${lock_acquired:-no}" = yes && test -f "$lock_owner_file" && test "$(cat "$lock_owner_file" 2>/dev/null)" = "$lock_token"; then
    rm -f "$lock_owner_file"
    rmdir "$lock_dir" 2>/dev/null || true
  fi
  exit "$status"
}

marker_path() {
  printf '%s.openrouter-owner\n' "$1"
}

write_marker() {
  marker_file=$1
  marker_destination=$2
  marker_version=$3
  printf '%s\n' 'openrouter-installer-marker-v1' "identifier=openrouter-model-tracker" "destination=$marker_destination" "version=$marker_version" > "$marker_file"
  chmod 600 "$marker_file"
}

valid_marker() {
  marker_file=$1
  expected_destination=$2
  expected_binary=$3
  test -f "$marker_file" || return 1
  test "$(stat -c '%a' "$marker_file" 2>/dev/null || stat -f '%Lp' "$marker_file")" = 600 || return 1
  test "$(sed -n '1p' "$marker_file")" = 'openrouter-installer-marker-v1' || return 1
  test "$(sed -n '2p' "$marker_file")" = 'identifier=openrouter-model-tracker' || return 1
  test "$(sed -n '3p' "$marker_file")" = "destination=$expected_destination" || return 1
  marker_version=$(sed -n '4p' "$marker_file")
  case "$marker_version" in version=?) ;; version=??*) ;; *) return 1 ;; esac
  test "$(wc -l < "$marker_file" | tr -d ' ')" = 4 || return 1
  actual_version=$("$expected_binary" --version 2>/dev/null) || return 1
  actual_short_version=$("$expected_binary" version 2>/dev/null) || return 1
  test "$actual_version" = "openrouter version ${marker_version#version=}" || return 1
  test "$actual_short_version" = "openrouter ${marker_version#version=}" || return 1
}

case "${1:-}" in
  install)
    test "$#" -eq 4 || { usage >&2; exit 2; }
    source=$2
    destination=$3
    expected_version=$4
    validate_absolute_path "$destination" destination
    destination_dir=${destination%/*}
    validate_absolute_path "$destination_dir" 'destination directory'
    test -f "$source" && test -x "$source" || fail "source executable is missing or not executable: $source"
    test "${destination##*/}" != "$destination" || fail "destination must include a file name"
    test -n "$expected_version" || fail 'expected version must not be empty'
    test ! -d "$destination" || fail "refusing to replace directory: $destination"
    test ! -L "$destination" || fail "refusing to replace symlink: $destination"
    actual_version=$("$source" --version 2>/dev/null) || fail 'source executable rejected --version'
    test "$actual_version" = "openrouter version $expected_version" || fail "source version mismatch: expected openrouter version $expected_version, got $actual_version"
    actual_short_version=$("$source" version 2>/dev/null) || fail 'source executable rejected version'
    test "$actual_short_version" = "openrouter $expected_version" || fail "source version command mismatch: expected openrouter $expected_version, got $actual_short_version"
    "$source" --help >/dev/null 2>&1 || fail 'source executable rejected --help'
    ensure_trusted_directory "$destination_dir"
    acquire_lock "$destination_dir"
    test ! -d "$destination" || fail "refusing to replace directory: $destination"
    test ! -L "$destination" || fail "refusing to replace symlink: $destination"
    mode=$(stat -c '%a' "$source" 2>/dev/null || stat -f '%Lp' "$source")
    temporary=$(mktemp "$destination_dir/.${destination##*/}.tmp.XXXXXX")
    cp "$source" "$temporary"
    chmod "$mode" "$temporary"
    test -x "$temporary" || fail 'temporary artifact is not executable'
    test "$("$temporary" --version)" = "openrouter version $expected_version" || fail 'temporary --version verification failed'
    test "$("$temporary" version)" = "openrouter $expected_version" || fail 'temporary version verification failed'
    "$temporary" --help >/dev/null 2>&1 || fail 'temporary --help verification failed'
    marker=$(marker_path "$destination")
    marker_temporary=$(mktemp "$destination_dir/.${destination##*/}.owner.tmp.XXXXXX")
    write_marker "$marker_temporary" "$destination" "$expected_version"
    valid_marker "$marker_temporary" "$destination" "$temporary" || fail 'temporary ownership marker verification failed'
    backup_binary=$(mktemp "$destination_dir/.${destination##*/}.binary-backup.XXXXXX")
    backup_marker=$(mktemp "$destination_dir/.${destination##*/}.marker-backup.XXXXXX")
    rm -f "$backup_binary" "$backup_marker"
    if test -e "$destination"; then mv "$destination" "$backup_binary"; fi
    if test -e "$marker"; then mv "$marker" "$backup_marker"; fi
    if ! mv "$temporary" "$destination" || ! mv "$marker_temporary" "$marker"; then
      rm -f "$destination" "$marker"
      test ! -e "$backup_binary" || mv "$backup_binary" "$destination"
      test ! -e "$backup_marker" || mv "$backup_marker" "$marker"
      fail 'installation replacement failed; previous binary and marker restored'
    fi
    installed_mode=$(stat -c '%a' "$destination" 2>/dev/null || stat -f '%Lp' "$destination")
    test "$installed_mode" = "$mode" || fail "installed artifact mode changed: expected $mode, got $installed_mode"
    rm -f "$backup_binary" "$backup_marker"
    printf '%s\n' "Installed $destination (version $expected_version)"
    ;;
  uninstall)
    test "$#" -eq 2 || { usage >&2; exit 2; }
    destination=$2
    validate_absolute_path "$destination" destination
    test "${destination##*/}" != "$destination" || fail "destination must include a file name"
    destination_dir=${destination%/*}
    validate_absolute_path "$destination_dir" 'destination directory'
    if test -e "$destination_dir" || test -L "$destination_dir"; then
      ensure_trusted_directory "$destination_dir"
      acquire_lock "$destination_dir"
    fi
    marker=$(marker_path "$destination")
    test ! -d "$destination" || fail "refusing to remove directory: $destination"
    test ! -L "$destination" || fail "refusing to remove symlink: $destination"
    if test ! -e "$marker"; then
      printf 'WARN: no valid ownership marker at %s; preserving unmanaged destination\n' "$destination"
      exit 0
    fi
    valid_marker "$marker" "$destination" "$destination" || fail "refusing to remove unmanaged or invalid installation; preserved $destination and $marker"
    removal_binary=$(mktemp "$destination_dir/.${destination##*/}.remove.XXXXXX")
    removal_marker=$(mktemp "$destination_dir/.${destination##*/}.remove-marker.XXXXXX")
    rm -f "$removal_binary" "$removal_marker"
    mv "$destination" "$removal_binary" || fail "cannot stage managed binary removal; preserved $destination and $marker"
    if ! mv "$marker" "$removal_marker"; then
      mv "$removal_binary" "$destination" || true
      fail "cannot stage ownership marker removal; preserved $destination and $marker"
    fi
    rm -f "$removal_binary" "$removal_marker"
    printf '%s\n' "PASS: removed managed installation $destination and ownership marker"
    ;;
  --help|-h)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

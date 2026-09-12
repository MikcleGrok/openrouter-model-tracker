#!/usr/bin/env bash
# scripts/winget-manifest.sh — generate, verify, and submit the winget-pkgs
# manifest for a released Windows build.
#
# This repository has no CI (see docs/security.md) and the release process is
# entirely local/manual (make release-local, make release-github). Winget
# publication follows the same pattern: no wingetcreate (Windows-only binary,
# no Windows host in this workflow), no GitHub Actions winget-releaser —
# instead this script reads the already-produced local-release evidence
# (dist/local-release/<version>/manifest.json, SHA256SUMS) and drives `gh`
# directly, the same way scripts/sync-homebrew-formula.sh derives the
# Homebrew formula from the checked-out tag rather than printing values by
# hand. See .task/winget-install-support/plan.md for the full rationale.
#
# Modes (mutually exclusive; default --generate):
#   --generate  write the 3 manifest YAML files from local-release evidence
#   --check     verify the generated files match SHA256SUMS, and that the
#               GitHub Release for this tag is actually published (the
#               ordering guard: winget-submit must run after release-github)
#   --submit    fork sync -> branch -> upload 3 files via the contents API
#               -> open a PR against winget-pkgs. WINGET_DRY_RUN=1 prints the
#               exact command sequence without running any mutating `gh`
#               call or touching the network.
#
# Environment (all optional, matching the Makefile's own variables of the
# same name):
#   WINGET_PACKAGE_IDENTIFIER  default: MikcleGrok.openrouter-model-tracker
#   WINGET_MANIFEST_VERSION    default: 1.12.0 (winget manifest schema)
#   WINGET_FORK                default: MikcleGrok/winget-pkgs
#   WINGET_PKGS_REPOSITORY     default: microsoft/winget-pkgs
#   GITHUB_REPOSITORY          default: MikcleGrok/openrouter-model-tracker
#   RELEASE_ARTIFACT_DIR       default: <repo>/dist/local-release/<version>
#   WINGET_DIR                 default: $RELEASE_ARTIFACT_DIR/winget
#   VERSION / TAG_VERSION      resolved the same way the Makefile resolves
#                              them if not set (TAG_VERSION from an exact
#                              `git describe --tags --exact-match`, VERSION
#                              from TAG_VERSION with the leading `v` dropped)
#   WINGET_DRY_RUN             --submit only; 1 previews without mutating

set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

usage() {
  printf '%s\n' "Usage: $0 [--generate|--check|--submit] [--help]"
  printf '%s\n' ''
  printf '%s\n' '  --generate (default)  write the 3 winget manifest YAML files from'
  printf '%s\n' '                        dist/local-release/<version> evidence'
  printf '%s\n' '  --check               verify the generated files against SHA256SUMS and'
  printf '%s\n' '                        the published GitHub Release (must run after'
  printf '%s\n' '                        make release-github)'
  printf '%s\n' '  --submit              fork sync, branch, upload the 3 files via the'
  printf '%s\n' '                        contents API, open a PR against winget-pkgs.'
  printf '%s\n' '                        WINGET_DRY_RUN=1 previews the gh command'
  printf '%s\n' '                        sequence without mutating anything.'
  printf '%s\n' ''
  printf '%s\n' 'Environment: WINGET_PACKAGE_IDENTIFIER, WINGET_MANIFEST_VERSION, WINGET_FORK,'
  printf '%s\n' 'WINGET_PKGS_REPOSITORY, GITHUB_REPOSITORY, RELEASE_ARTIFACT_DIR, WINGET_DIR,'
  printf '%s\n' 'VERSION, TAG_VERSION, WINGET_DRY_RUN.'
}

MODE=generate
while test "$#" -gt 0; do
  case "$1" in
    --generate) MODE=generate ;;
    --check) MODE=check ;;
    --submit) MODE=submit ;;
    --help|-h) usage; exit 0 ;;
    --*) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
    *) printf 'Unexpected argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

PACKAGE_IDENTIFIER="${WINGET_PACKAGE_IDENTIFIER:-MikcleGrok.openrouter-model-tracker}"
MANIFEST_SCHEMA_VERSION="${WINGET_MANIFEST_VERSION:-1.12.0}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-MikcleGrok/openrouter-model-tracker}"
WINGET_FORK="${WINGET_FORK:-MikcleGrok/winget-pkgs}"
WINGET_PKGS_REPOSITORY="${WINGET_PKGS_REPOSITORY:-microsoft/winget-pkgs}"
WINGET_DRY_RUN="${WINGET_DRY_RUN:-0}"

case "$PACKAGE_IDENTIFIER" in
  *.*) ;;
  *) printf '%s\n' "BLOCKED: WINGET_PACKAGE_IDENTIFIER must be Publisher.PackageName: $PACKAGE_IDENTIFIER" >&2; exit 1 ;;
esac
PUBLISHER="${PACKAGE_IDENTIFIER%%.*}"
PACKAGE_NAME="${PACKAGE_IDENTIFIER#*.}"
FIRST_LETTER="$(printf '%s' "$PUBLISHER" | cut -c1 | tr '[:upper:]' '[:lower:]')"

# --- exact-tag resolution (mirrors scripts/sync-homebrew-formula.sh) -------
TAG="${TAG_VERSION:-}"
if test -z "$TAG"; then
  TAG="$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || true)"
fi
test -n "$TAG" || { printf '%s\n' 'BLOCKED: an exact release tag is required (HEAD must be tagged vMAJOR.MINOR.PATCH).' >&2; exit 1; }
case "$TAG" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) printf '%s\n' "BLOCKED: tag must be a vMAJOR.MINOR.PATCH tag: $TAG" >&2; exit 1 ;;
esac
git -C "$ROOT" rev-parse --verify --quiet "$TAG^{commit}" >/dev/null || { printf '%s\n' "BLOCKED: cannot resolve exact tag $TAG to a commit." >&2; exit 1; }

VERSION="${VERSION:-${TAG#v}}"
test "$TAG" = "v$VERSION" || { printf '%s\n' "BLOCKED: TAG ($TAG) and VERSION (v$VERSION) disagree -- refusing to generate a manifest whose InstallerUrl (built from TAG) would not match the artifact filename (built from VERSION)." >&2; exit 1; }

RELEASE_ARTIFACT_DIR="${RELEASE_ARTIFACT_DIR:-$ROOT/dist/local-release/$VERSION}"
WINGET_DIR="${WINGET_DIR:-$RELEASE_ARTIFACT_DIR/winget}"
MANIFEST_JSON="$RELEASE_ARTIFACT_DIR/manifest.json"
SHA256SUMS_FILE="$RELEASE_ARTIFACT_DIR/SHA256SUMS"
WINDOWS_ARTIFACT="artifacts/openrouter-$VERSION-windows-amd64.zip"
ASSET_NAME="openrouter-$VERSION-windows-amd64.zip"

MANIFEST_DIR="$WINGET_DIR/manifests/$FIRST_LETTER/$PUBLISHER/$PACKAGE_NAME/$VERSION"
VERSION_FILE="$MANIFEST_DIR/$PACKAGE_IDENTIFIER.yaml"
INSTALLER_FILE="$MANIFEST_DIR/$PACKAGE_IDENTIFIER.installer.yaml"
LOCALE_FILE="$MANIFEST_DIR/$PACKAGE_IDENTIFIER.locale.en-US.yaml"

require_evidence() {
  test -d "$RELEASE_ARTIFACT_DIR" || { printf '%s\n' "BLOCKED: local release directory not found: $RELEASE_ARTIFACT_DIR (run make release-local first)" >&2; exit 1; }
  test -s "$MANIFEST_JSON" || { printf '%s\n' "BLOCKED: local release manifest is missing or empty: $MANIFEST_JSON" >&2; exit 1; }
  test -s "$SHA256SUMS_FILE" || { printf '%s\n' "BLOCKED: SHA256SUMS is missing or empty: $SHA256SUMS_FILE" >&2; exit 1; }
  command -v jq >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: jq is required to read the local release manifest.' >&2; exit 1; }
}

# Resolves and cross-checks the Windows artifact digest, and sets
# INSTALLER_SHA256 (uppercase) and RELEASE_DATE (from manifest.json's
# built_at) as a side effect.
resolve_windows_evidence() {
  local manifest_digest sums_digest built_at
  manifest_digest="$(jq -r --arg artifact "$WINDOWS_ARTIFACT" '.artifacts[] | select(.artifact == $artifact) | .sha256' "$MANIFEST_JSON")"
  test -n "$manifest_digest" || { printf '%s\n' "BLOCKED: no windows-amd64 artifact entry in $MANIFEST_JSON (expected $WINDOWS_ARTIFACT)" >&2; exit 1; }
  sums_digest="$(awk -v artifact="$WINDOWS_ARTIFACT" '$2 == artifact { print $1 }' "$SHA256SUMS_FILE")"
  test -n "$sums_digest" || { printf '%s\n' "BLOCKED: no SHA256SUMS entry for $WINDOWS_ARTIFACT" >&2; exit 1; }
  test "$manifest_digest" = "$sums_digest" || { printf '%s\n' "BLOCKED: manifest.json and SHA256SUMS digests disagree for $WINDOWS_ARTIFACT" >&2; exit 1; }
  INSTALLER_SHA256="$(printf '%s' "$sums_digest" | tr '[:lower:]' '[:upper:]')"

  built_at="$(jq -r '.built_at // empty' "$MANIFEST_JSON")"
  test -n "$built_at" || { printf '%s\n' "BLOCKED: manifest.json has no built_at" >&2; exit 1; }
  RELEASE_DATE="${built_at%%T*}"
}

cmd_generate() {
  require_evidence
  resolve_windows_evidence

  local installer_url release_notes_url license_url
  installer_url="https://github.com/$GITHUB_REPOSITORY/releases/download/$TAG/$ASSET_NAME"
  release_notes_url="https://github.com/$GITHUB_REPOSITORY/releases/tag/$TAG"
  license_url="https://github.com/$GITHUB_REPOSITORY/blob/master/LICENSE"

  mkdir -p "$MANIFEST_DIR"

  cat > "$VERSION_FILE" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.$MANIFEST_SCHEMA_VERSION.schema.json
PackageIdentifier: $PACKAGE_IDENTIFIER
PackageVersion: $VERSION
DefaultLocale: en-US
ManifestType: version
ManifestVersion: $MANIFEST_SCHEMA_VERSION
EOF

  cat > "$INSTALLER_FILE" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.$MANIFEST_SCHEMA_VERSION.schema.json
PackageIdentifier: $PACKAGE_IDENTIFIER
PackageVersion: $VERSION
InstallerType: zip
NestedInstallerType: portable
NestedInstallerFiles:
- RelativeFilePath: openrouter.exe
  PortableCommandAlias: openrouter
Commands:
- openrouter
ReleaseDate: $RELEASE_DATE
Installers:
- Architecture: x64
  InstallerUrl: $installer_url
  InstallerSha256: $INSTALLER_SHA256
ManifestType: installer
ManifestVersion: $MANIFEST_SCHEMA_VERSION
EOF

  cat > "$LOCALE_FILE" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.$MANIFEST_SCHEMA_VERSION.schema.json
PackageIdentifier: $PACKAGE_IDENTIFIER
PackageVersion: $VERSION
PackageLocale: en-US
Publisher: $PUBLISHER
PackageName: $PACKAGE_NAME
License: MIT
LicenseUrl: $license_url
ShortDescription: Compare AI models on OpenRouter by quality and price.
Description: |-
  openrouter-model-tracker collects a live catalog of OpenRouter models
  (pricing, context) and matches it against independent quality scores --
  SWE-bench Verified (vals.ai and swebench.com), LMArena Elo, and GPQA
  Diamond (vals.ai) -- through a hand-curated map and a structured identity
  gate rather than fuzzy name matching. Paid models are ranked by a
  quality/price metric and grouped into tiers oriented around Claude
  Opus/Sonnet/Haiku. Data is available as an interactive TUI, a plain-text
  CLI table, and a generated Markdown report.
Moniker: omt
Tags:
- ai
- benchmark
- cli
- llm
- openrouter
- tui
ReleaseNotesUrl: $release_notes_url
ManifestType: defaultLocale
ManifestVersion: $MANIFEST_SCHEMA_VERSION
EOF

  printf '%s\n' "Generated winget manifests for $PACKAGE_IDENTIFIER $VERSION in $MANIFEST_DIR"
}

cmd_check() {
  require_evidence
  resolve_windows_evidence

  for f in "$VERSION_FILE" "$INSTALLER_FILE" "$LOCALE_FILE"; do
    test -f "$f" || { printf '%s\n' "BLOCKED: generated manifest is missing: $f (run make winget-manifest first)" >&2; exit 1; }
  done

  local actual_digest
  actual_digest="$(awk -F': ' '/^ *InstallerSha256:/ { print $2; exit }' "$INSTALLER_FILE")"
  test -n "$actual_digest" || { printf '%s\n' "BLOCKED: could not read InstallerSha256 from $INSTALLER_FILE" >&2; exit 1; }
  test "$actual_digest" = "$INSTALLER_SHA256" || { printf '%s\n' "BLOCKED: $INSTALLER_FILE InstallerSha256 ($actual_digest) does not match SHA256SUMS ($INSTALLER_SHA256)" >&2; exit 1; }

  local actual_path
  actual_path="$(awk -F': ' '/RelativeFilePath:/ { print $2; exit }' "$INSTALLER_FILE")"
  test "$actual_path" = "openrouter.exe" || { printf '%s\n' "BLOCKED: $INSTALLER_FILE RelativeFilePath is '$actual_path', expected openrouter.exe" >&2; exit 1; }

  command -v gh >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is required to verify the published GitHub Release.' >&2; exit 1; }
  local release_json view_status gh_stderr_file
  gh_stderr_file="$(mktemp)"
  view_status=0
  # Keep gh's stderr out of the stream we hand to jq -- a stray warning on an
  # otherwise-successful call must not turn into a confusing jq parse error
  # in place of a clean BLOCKED: message.
  release_json="$(gh release view "$TAG" --repo "$GITHUB_REPOSITORY" --json assets 2>"$gh_stderr_file")" || view_status=$?
  if test "$view_status" -ne 0; then
    printf '%s\n' "BLOCKED: GitHub Release $TAG not found on $GITHUB_REPOSITORY -- winget-submit must run after make release-github." >&2
    cat "$gh_stderr_file" >&2
    rm -f "$gh_stderr_file"
    exit 1
  fi
  rm -f "$gh_stderr_file"

  command -v jq >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: jq is required to inspect the GitHub Release assets.' >&2; exit 1; }
  local asset_present asset_digest digest_verified
  asset_present="$(printf '%s' "$release_json" | jq -r --arg name "$ASSET_NAME" '.assets[] | select(.name == $name) | .name')"
  test -n "$asset_present" || { printf '%s\n' "BLOCKED: GitHub Release $TAG has no asset named $ASSET_NAME" >&2; exit 1; }

  # GitHub computes a `digest` field ("sha256:<hex>") for uploaded release
  # assets when available; compare it if gh's json exposes it, but don't
  # fail closed on its absence alone -- it is not guaranteed by the API for
  # every asset. Asset presence above is the load-bearing ordering guard.
  asset_digest="$(printf '%s' "$release_json" | jq -r --arg name "$ASSET_NAME" '.assets[] | select(.name == $name) | (.digest // empty)')"
  digest_verified=0
  if test -n "$asset_digest"; then
    asset_digest="$(printf '%s' "$asset_digest" | sed -E 's/^sha256://' | tr '[:lower:]' '[:upper:]')"
    test "$asset_digest" = "$INSTALLER_SHA256" || { printf '%s\n' "BLOCKED: GitHub Release asset digest ($asset_digest) does not match SHA256SUMS ($INSTALLER_SHA256)" >&2; exit 1; }
    digest_verified=1
  fi

  if test "$digest_verified" -eq 1; then
    printf '%s\n' "winget manifests verified: $TAG asset $ASSET_NAME is published on $GITHUB_REPOSITORY, digest cross-verified against SHA256SUMS ($INSTALLER_SHA256)"
  else
    printf '%s\n' "winget manifests verified: $TAG asset $ASSET_NAME is published on $GITHUB_REPOSITORY (name/presence only -- GitHub reported no digest for this asset, so it was not cross-verified against SHA256SUMS $INSTALLER_SHA256)"
  fi
}

cmd_submit() {
  require_evidence
  resolve_windows_evidence

  for f in "$VERSION_FILE" "$INSTALLER_FILE" "$LOCALE_FILE"; do
    test -f "$f" || { printf '%s\n' "BLOCKED: generated manifest is missing: $f (run make winget-manifest first)" >&2; exit 1; }
  done

  local fork_owner branch pr_title pr_body relative_dir
  fork_owner="${WINGET_FORK%%/*}"
  branch="$PACKAGE_IDENTIFIER-$VERSION"
  relative_dir="manifests/$FIRST_LETTER/$PUBLISHER/$PACKAGE_NAME/$VERSION"
  pr_title="New version: $PACKAGE_IDENTIFIER version $VERSION"
  pr_body="Automated update for $PACKAGE_IDENTIFIER version $VERSION.

- InstallerUrl: https://github.com/$GITHUB_REPOSITORY/releases/download/$TAG/$ASSET_NAME
- InstallerSha256: $INSTALLER_SHA256
- Generated by scripts/winget-manifest.sh, verified against SHA256SUMS and the published release by scripts/winget-manifest.sh --check."

  local -a cmds=()
  cmds+=("gh repo sync $WINGET_FORK --source $WINGET_PKGS_REPOSITORY --branch master")
  cmds+=("base_sha=\$(gh api repos/$WINGET_FORK/git/ref/heads/master --jq .object.sha)")
  cmds+=("gh api repos/$WINGET_FORK/git/refs -f ref=refs/heads/$branch -f sha=\$base_sha")
  for f in "$VERSION_FILE" "$INSTALLER_FILE" "$LOCALE_FILE"; do
    cmds+=("gh api repos/$WINGET_FORK/contents/$relative_dir/$(basename "$f") --method PUT -f message='$pr_title' -f content=<base64 of $f> -f branch=$branch")
  done
  cmds+=("gh pr create --repo $WINGET_PKGS_REPOSITORY --head $fork_owner:$branch --base master --title '$pr_title' --body '<pr body>'")

  if test "$WINGET_DRY_RUN" = 1; then
    printf '%s\n' 'DRY RUN: the following commands would run (no network call made):'
    printf '%s\n' "${cmds[@]}"
    return 0
  fi

  command -v gh >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is required to submit the winget-pkgs PR.' >&2; exit 1; }
  gh auth status >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is not authenticated; run gh auth login' >&2; exit 1; }

  gh repo sync "$WINGET_FORK" --source "$WINGET_PKGS_REPOSITORY" --branch master
  local base_sha
  base_sha="$(gh api "repos/$WINGET_FORK/git/ref/heads/master" --jq .object.sha)"
  test -n "$base_sha" || { printf '%s\n' "BLOCKED: could not resolve $WINGET_FORK master sha after sync" >&2; exit 1; }
  gh api "repos/$WINGET_FORK/git/refs" -f "ref=refs/heads/$branch" -f "sha=$base_sha"

  for f in "$VERSION_FILE" "$INSTALLER_FILE" "$LOCALE_FILE"; do
    local content
    content="$(base64 < "$f" | tr -d '\n')"
    gh api "repos/$WINGET_FORK/contents/$relative_dir/$(basename "$f")" --method PUT \
      -f "message=$pr_title" -f "content=$content" -f "branch=$branch"
  done

  gh pr create --repo "$WINGET_PKGS_REPOSITORY" --head "$fork_owner:$branch" --base master \
    --title "$pr_title" --body "$pr_body"
}

case "$MODE" in
  generate) cmd_generate ;;
  check) cmd_check ;;
  submit) cmd_submit ;;
esac

#!/usr/bin/env bash
# scripts/scoop-manifest.sh — generate, verify, and submit the Scoop bucket
# manifest for a released Windows build.
#
# Unlike winget-pkgs (a shared, externally-moderated repository), the Scoop
# bucket is a repository this project owns outright: no fork, no branch, no
# PR, no moderator — a single JSON manifest written directly to the bucket
# repository's default branch via the GitHub contents API. This mirrors
# scripts/winget-manifest.sh's own pattern (no CI, local/manual release
# process, evidence read from dist/local-release/<version>) with the fork/PR
# machinery removed and a read-then-PUT idempotency check added in its place.
# See .task/scoop-install-support/plan.md for the full rationale.
#
# Modes (mutually exclusive; default --generate):
#   --generate  write the bucket manifest JSON file from local-release
#               evidence (dist/local-release/<version>/manifest.json,
#               SHA256SUMS)
#   --check     verify the generated manifest matches SHA256SUMS, and that
#               the GitHub Release for this tag is actually published (the
#               ordering guard: scoop-submit must run after release-github)
#   --submit    GET the bucket manifest path -> plan Add (404) or Update
#               (200, carrying the existing blob sha) -> PUT via the contents
#               API. SCOOP_DRY_RUN=1 previews the planned action without
#               making any mutating `gh` call (the read-only GET still runs).
#               A local file whose git blob sha already matches the bucket's
#               is a no-op in either mode.
#
# Environment (all optional, matching the Makefile's own variables of the
# same name):
#   SCOOP_BUCKET_REPOSITORY  default: MikcleGrok/scoop-bucket
#   SCOOP_BUCKET_BRANCH      default: main
#   SCOOP_APP_ID             default: openrouter-model-tracker
#   GITHUB_REPOSITORY        default: MikcleGrok/openrouter-model-tracker
#   RELEASE_ARTIFACT_DIR     default: <repo>/dist/local-release/<version>
#   SCOOP_DIR                default: $RELEASE_ARTIFACT_DIR/scoop
#   VERSION / TAG_VERSION    resolved the same way the Makefile resolves
#                            them if not set (TAG_VERSION from an exact
#                            `git describe --tags --exact-match`, VERSION
#                            from TAG_VERSION with the leading `v` dropped)
#   SCOOP_DRY_RUN            --submit only; 1 previews without mutating

set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

usage() {
  printf '%s\n' "Usage: $0 [--generate|--check|--submit] [--help]"
  printf '%s\n' ''
  printf '%s\n' '  --generate (default)  write the Scoop bucket manifest JSON from'
  printf '%s\n' '                        dist/local-release/<version> evidence'
  printf '%s\n' '  --check               verify the generated manifest against SHA256SUMS and'
  printf '%s\n' '                        the published GitHub Release (must run after'
  printf '%s\n' '                        make release-github)'
  printf '%s\n' '  --submit              GET the bucket manifest path, plan Add/Update, PUT'
  printf '%s\n' '                        via the contents API. SCOOP_DRY_RUN=1 previews the'
  printf '%s\n' '                        planned action without mutating anything.'
  printf '%s\n' ''
  printf '%s\n' 'Environment: SCOOP_BUCKET_REPOSITORY, SCOOP_BUCKET_BRANCH, SCOOP_APP_ID,'
  printf '%s\n' 'GITHUB_REPOSITORY, RELEASE_ARTIFACT_DIR, SCOOP_DIR, VERSION, TAG_VERSION,'
  printf '%s\n' 'SCOOP_DRY_RUN.'
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

SCOOP_BUCKET_REPOSITORY="${SCOOP_BUCKET_REPOSITORY:-MikcleGrok/scoop-bucket}"
SCOOP_BUCKET_BRANCH="${SCOOP_BUCKET_BRANCH:-main}"
SCOOP_APP_ID="${SCOOP_APP_ID:-openrouter-model-tracker}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-MikcleGrok/openrouter-model-tracker}"
SCOOP_DRY_RUN="${SCOOP_DRY_RUN:-0}"

# --- exact-tag resolution (mirrors scripts/winget-manifest.sh) -------------
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
test "$TAG" = "v$VERSION" || { printf '%s\n' "BLOCKED: TAG ($TAG) and VERSION (v$VERSION) disagree -- refusing to generate a manifest whose url (built from TAG) would not match the artifact filename (built from VERSION)." >&2; exit 1; }

RELEASE_ARTIFACT_DIR="${RELEASE_ARTIFACT_DIR:-$ROOT/dist/local-release/$VERSION}"
SCOOP_DIR="${SCOOP_DIR:-$RELEASE_ARTIFACT_DIR/scoop}"
MANIFEST_JSON="$RELEASE_ARTIFACT_DIR/manifest.json"
SHA256SUMS_FILE="$RELEASE_ARTIFACT_DIR/SHA256SUMS"
WINDOWS_ARTIFACT="artifacts/openrouter-$VERSION-windows-amd64.zip"
ASSET_NAME="openrouter-$VERSION-windows-amd64.zip"

BUCKET_DIR="$SCOOP_DIR/bucket"
MANIFEST_FILE="$BUCKET_DIR/$SCOOP_APP_ID.json"
CONTENTS_PATH="bucket/$SCOOP_APP_ID.json"

require_evidence() {
  test -d "$RELEASE_ARTIFACT_DIR" || { printf '%s\n' "BLOCKED: local release directory not found: $RELEASE_ARTIFACT_DIR (run make release-local first)" >&2; exit 1; }
  test -s "$MANIFEST_JSON" || { printf '%s\n' "BLOCKED: local release manifest is missing or empty: $MANIFEST_JSON" >&2; exit 1; }
  test -s "$SHA256SUMS_FILE" || { printf '%s\n' "BLOCKED: SHA256SUMS is missing or empty: $SHA256SUMS_FILE" >&2; exit 1; }
  command -v jq >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: jq is required to read the local release manifest.' >&2; exit 1; }
}

# Resolves and cross-checks the Windows artifact digest, and sets
# INSTALLER_HASH (lowercase, straight from SHA256SUMS -- Scoop does not
# require uppercase the way winget's InstallerSha256 does) as a side effect.
resolve_windows_evidence() {
  local manifest_digest sums_digest
  manifest_digest="$(jq -r --arg artifact "$WINDOWS_ARTIFACT" '.artifacts[] | select(.artifact == $artifact) | .sha256' "$MANIFEST_JSON")"
  test -n "$manifest_digest" || { printf '%s\n' "BLOCKED: no windows-amd64 artifact entry in $MANIFEST_JSON (expected $WINDOWS_ARTIFACT)" >&2; exit 1; }
  sums_digest="$(awk -v artifact="$WINDOWS_ARTIFACT" '$2 == artifact { print $1 }' "$SHA256SUMS_FILE")"
  test -n "$sums_digest" || { printf '%s\n' "BLOCKED: no SHA256SUMS entry for $WINDOWS_ARTIFACT" >&2; exit 1; }
  test "$manifest_digest" = "$sums_digest" || { printf '%s\n' "BLOCKED: manifest.json and SHA256SUMS digests disagree for $WINDOWS_ARTIFACT" >&2; exit 1; }
  INSTALLER_HASH="$sums_digest"
}

cmd_generate() {
  require_evidence
  resolve_windows_evidence

  local installer_url autoupdate_url_template
  installer_url="https://github.com/$GITHUB_REPOSITORY/releases/download/$TAG/$ASSET_NAME"
  # The literal 8-character string "$version" (escaped below so the shell
  # does not expand it) is Scoop's own autoupdate template placeholder, not
  # a shell variable -- it stays unexpanded in the generated JSON for
  # Scoop's checkver/auto-pr tooling to substitute later.
  autoupdate_url_template="https://github.com/$GITHUB_REPOSITORY/releases/download/v\$version/openrouter-\$version-windows-amd64.zip"

  mkdir -p "$BUCKET_DIR"

  jq -n \
    --arg version "$VERSION" \
    --arg description 'Compare AI models on OpenRouter by quality and price.' \
    --arg homepage "https://github.com/$GITHUB_REPOSITORY" \
    --arg license 'MIT' \
    --arg url "$installer_url" \
    --arg hash "$INSTALLER_HASH" \
    --arg checkver_github "https://github.com/$GITHUB_REPOSITORY" \
    --arg autoupdate_url "$autoupdate_url_template" \
    '{
      "$schema": "https://raw.githubusercontent.com/ScoopInstaller/Scoop/master/schema.json",
      version: $version,
      description: $description,
      homepage: $homepage,
      license: $license,
      architecture: {
        "64bit": {
          url: $url,
          hash: $hash
        }
      },
      bin: ["openrouter.exe", ["openrouter.exe", "omt"]],
      checkver: { github: $checkver_github },
      autoupdate: {
        architecture: {
          "64bit": {
            url: $autoupdate_url
          }
        }
      }
    }' > "$MANIFEST_FILE"

  printf '%s\n' "Generated scoop manifest for $SCOOP_APP_ID $VERSION at $MANIFEST_FILE"
}

cmd_check() {
  require_evidence
  resolve_windows_evidence

  test -f "$MANIFEST_FILE" || { printf '%s\n' "BLOCKED: generated scoop manifest is missing: $MANIFEST_FILE (run make scoop-manifest first)" >&2; exit 1; }

  jq -e . "$MANIFEST_FILE" >/dev/null 2>&1 || { printf '%s\n' "BLOCKED: $MANIFEST_FILE is not valid JSON" >&2; exit 1; }

  local actual_version actual_url actual_hash actual_autoupdate_url substituted_url
  actual_version="$(jq -r '.version' "$MANIFEST_FILE")"
  test "$actual_version" = "$VERSION" || { printf '%s\n' "BLOCKED: $MANIFEST_FILE version ($actual_version) does not match VERSION ($VERSION)" >&2; exit 1; }

  actual_url="$(jq -r '.architecture."64bit".url' "$MANIFEST_FILE")"
  case "$actual_url" in
    *"$TAG"*"$ASSET_NAME"*) ;;
    *) printf '%s\n' "BLOCKED: $MANIFEST_FILE architecture.64bit.url ($actual_url) does not contain tag $TAG and asset $ASSET_NAME" >&2; exit 1 ;;
  esac

  actual_hash="$(jq -r '.architecture."64bit".hash' "$MANIFEST_FILE")"
  test "$actual_hash" = "$INSTALLER_HASH" || { printf '%s\n' "BLOCKED: $MANIFEST_FILE architecture.64bit.hash ($actual_hash) does not match SHA256SUMS ($INSTALLER_HASH)" >&2; exit 1; }

  actual_autoupdate_url="$(jq -r '.autoupdate.architecture."64bit".url' "$MANIFEST_FILE")"
  substituted_url="${actual_autoupdate_url//\$version/$VERSION}"
  test "$substituted_url" = "$actual_url" || { printf '%s\n' "BLOCKED: $MANIFEST_FILE autoupdate url template, with \$version substituted, does not equal architecture.64bit.url byte-for-byte" >&2; exit 1; }

  command -v gh >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is required to verify the published GitHub Release.' >&2; exit 1; }
  local release_json view_status gh_stderr_file
  gh_stderr_file="$(mktemp)"
  view_status=0
  # Keep gh's stderr out of the stream we hand to jq -- a stray warning on an
  # otherwise-successful call must not turn into a confusing jq parse error
  # in place of a clean BLOCKED: message.
  release_json="$(gh release view "$TAG" --repo "$GITHUB_REPOSITORY" --json assets 2>"$gh_stderr_file")" || view_status=$?
  if test "$view_status" -ne 0; then
    printf '%s\n' "BLOCKED: GitHub Release $TAG not found on $GITHUB_REPOSITORY -- scoop-submit must run after make release-github." >&2
    cat "$gh_stderr_file" >&2
    rm -f "$gh_stderr_file"
    exit 1
  fi
  rm -f "$gh_stderr_file"

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
    asset_digest="$(printf '%s' "$asset_digest" | sed -E 's/^sha256://' | tr '[:upper:]' '[:lower:]')"
    test "$asset_digest" = "$INSTALLER_HASH" || { printf '%s\n' "BLOCKED: GitHub Release asset digest ($asset_digest) does not match SHA256SUMS ($INSTALLER_HASH)" >&2; exit 1; }
    digest_verified=1
  fi

  if test "$digest_verified" -eq 1; then
    printf '%s\n' "scoop manifest verified: $TAG asset $ASSET_NAME is published on $GITHUB_REPOSITORY, digest cross-verified against SHA256SUMS ($INSTALLER_HASH)"
  else
    printf '%s\n' "scoop manifest verified: $TAG asset $ASSET_NAME is published on $GITHUB_REPOSITORY (name/presence only -- GitHub reported no digest for this asset, so it was not cross-verified against SHA256SUMS $INSTALLER_HASH)"
  fi
}

cmd_submit() {
  require_evidence
  resolve_windows_evidence

  test -s "$MANIFEST_FILE" || { printf '%s\n' "BLOCKED: generated scoop manifest is missing or empty: $MANIFEST_FILE (run make scoop-manifest first)" >&2; exit 1; }
  jq -e . "$MANIFEST_FILE" >/dev/null 2>&1 || { printf '%s\n' "BLOCKED: $MANIFEST_FILE is not valid JSON" >&2; exit 1; }

  command -v gh >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is required to submit the scoop bucket manifest.' >&2; exit 1; }

  # Read-only GET of the bucket manifest path -- runs in both real --submit
  # and --submit SCOOP_DRY_RUN=1: dry-run only skips *mutating* calls, and
  # this isn't one (same principle as winget-manifest.sh's package-existence
  # check).
  local api_path gh_stderr_file get_status get_json
  api_path="repos/$SCOOP_BUCKET_REPOSITORY/contents/$CONTENTS_PATH?ref=$SCOOP_BUCKET_BRANCH"
  gh_stderr_file="$(mktemp)"
  get_status=0
  get_json="$(gh api "$api_path" 2>"$gh_stderr_file")" || get_status=$?

  local action existing_sha commit_message
  if test "$get_status" -eq 0; then
    rm -f "$gh_stderr_file"

    command -v jq >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: jq is required to inspect the bucket manifest contents response.' >&2; exit 1; }
    existing_sha="$(printf '%s' "$get_json" | jq -r '.sha // empty')"
    test -n "$existing_sha" || { printf '%s\n' "BLOCKED: gh api $api_path returned no .sha for an existing file" >&2; exit 1; }

    command -v git >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: git is required to compute the local blob sha.' >&2; exit 1; }
    local local_sha
    local_sha="$(git -C "$ROOT" hash-object "$MANIFEST_FILE")"

    # Idempotency, free: the contents API's .sha is a git blob sha, so a
    # matching local blob sha means the bucket already has this exact
    # content. No PUT, in dry-run or real mode -- a repeated
    # `make scoop-submit` for an unchanged release is a safe no-op.
    if test "$local_sha" = "$existing_sha"; then
      printf '%s\n' "bucket manifest already up to date: $SCOOP_BUCKET_REPOSITORY/$CONTENTS_PATH (sha $existing_sha)"
      return 0
    fi

    action=update
    commit_message="Update $SCOOP_APP_ID to $VERSION"
    printf '%s\n' "Bucket manifest exists in $SCOOP_BUCKET_REPOSITORY (sha $existing_sha) -- this will be an Update"
  elif test "$get_status" -eq 1 && grep -Eiq '(^|[^0-9])404([^0-9]|$)|not found' "$gh_stderr_file"; then
    rm -f "$gh_stderr_file"
    action=add
    existing_sha=""
    commit_message="Add $SCOOP_APP_ID $VERSION"
    printf '%s\n' "Bucket manifest does not exist yet in $SCOOP_BUCKET_REPOSITORY -- this will be an Add"
  else
    printf '%s\n' "BLOCKED: could not determine whether $CONTENTS_PATH exists in $SCOOP_BUCKET_REPOSITORY -- gh api $api_path failed unexpectedly (exit $get_status)." >&2
    cat "$gh_stderr_file" >&2
    rm -f "$gh_stderr_file"
    exit 1
  fi

  local cmd_preview
  cmd_preview="gh api repos/$SCOOP_BUCKET_REPOSITORY/contents/$CONTENTS_PATH --method PUT -f message='$commit_message' -f content=<base64 of $MANIFEST_FILE> -f branch=$SCOOP_BUCKET_BRANCH"
  if test "$action" = update; then
    cmd_preview="$cmd_preview -f sha=$existing_sha"
  fi

  if test "$SCOOP_DRY_RUN" = 1; then
    printf '%s\n' 'DRY RUN: the following command would run (no mutating network call made):'
    printf '%s\n' "$cmd_preview"
    return 0
  fi

  gh auth status >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is not authenticated; run gh auth login' >&2; exit 1; }

  local content
  content="$(base64 < "$MANIFEST_FILE" | tr -d '\n')"
  if test "$action" = update; then
    gh api "repos/$SCOOP_BUCKET_REPOSITORY/contents/$CONTENTS_PATH" --method PUT \
      -f "message=$commit_message" -f "content=$content" -f "branch=$SCOOP_BUCKET_BRANCH" -f "sha=$existing_sha"
  else
    gh api "repos/$SCOOP_BUCKET_REPOSITORY/contents/$CONTENTS_PATH" --method PUT \
      -f "message=$commit_message" -f "content=$content" -f "branch=$SCOOP_BUCKET_BRANCH"
  fi
}

case "$MODE" in
  generate) cmd_generate ;;
  check) cmd_check ;;
  submit) cmd_submit ;;
esac

#!/usr/bin/env bash
# scripts/verify-published-assets.sh -- thin parameter adapter for the
# canonical read-only verifier bin/guide-distribution-verify
# (guide-tools/11-distribution-verifier.md). It supplies paths, expected
# version and a disposable smoke command; it never reimplements any
# verification logic, and the fixed profile/root/artifact/manifest/metadata
# arguments cannot be overridden through forwarded arguments (migration
# rules 1, 3 and 4).
#
# Profile: archive.
#
# Not `formula`: this checkout tracks no source-build tap formula
# referencing itself by git tag/revision -- `scripts/verify-distribution.sh`
# already covers that shape for the disposable local dev-smoke tap
# (Formula/openrouter.rb, `make check-homebrew-formula`), a different,
# still-valid purpose from this script.
#
# Not `prebuilt` either, although that is the asset-channel profile on
# paper. Its `url-<asset>` check requires the asset URL to match
# `*/releases/download/<tag>/<asset>` where <tag> is the value of `--tag`,
# and `--tag` is separately required to be a strict SemVer
# `vMAJOR.MINOR.PATCH`. The canonical Homebrew formula for this project
# (`mikclegrok/tools/openrouter-model-tracker.rb`, verified live via `brew
# info`) publishes assets in the shared MikcleGrok/tools release repository
# under a per-project NAMESPACED tag --
# `.../releases/download/openrouter-model-tracker-v1.16.6/openrouter-1.16.6-darwin-arm64.tar.gz`
# -- so no value of `--tag` satisfies both constraints at once: `v1.16.6`
# fails url-<asset>, and `openrouter-model-tracker-v1.16.6` fails the SemVer
# `tag` check. uni-chat's scripts/verify-distribution.sh documents the same
# upstream verifier gap for the identical MikcleGrok/tools shared-repo
# pattern; this is not a defect in either project's formula.
#
# `archive` is the profile this project's self-repo GitHub Release channel
# (README.md onboarding record, `channels`) actually satisfies: it
# checks the artifact's own digest against an external checksum manifest,
# exact tag/version/commit agreement with the release metadata `make
# release-local` produces, a safe artifact path, and a real install smoke
# into a disposable prefix.
#
# Preconditions, all enforced by the verifier itself and all fail closed: a
# clean checkout whose HEAD is the exact release tag commit, plus a built
# dist/local-release/<version>/ with its SHA256SUMS and manifest.json
# alongside. That is a post-tag state, which is why `make release-local`
# calls this and `make check` does not.

set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
guide_tools_root=${GUIDE_TOOLS_ROOT:-$(CDPATH= cd -- "$root/../guide-tools" && pwd)}

version=${VERSION:-$(git -C "$root" describe --tags --exact-match 2>/dev/null || true)}
tag=${TAG:-}

# --tag and --version are absorbed rather than forwarded: both are resolved
# to one canonical pair below and passed exactly once, so a forwarded flag
# can never disagree with the artifact paths derived from it.
safe_args=()
while test "$#" -gt 0; do
  case "$1" in
    --tag) test "$#" -ge 2 || { printf '%s\n' "$1 requires a value" >&2; exit 2; }; tag=$2; shift 2;;
    --version) test "$#" -ge 2 || { printf '%s\n' "$1 requires a value" >&2; exit 2; }; version=$2; shift 2;;
    --format|--output) test "$#" -ge 2 || { printf '%s\n' "$1 requires a value" >&2; exit 2; }; safe_args+=("$1" "$2"); shift 2;;
    --check|--help|-h) safe_args+=("$1"); shift;;
    *) printf 'ERROR: unsupported wrapper argument: %s\n' "$1" >&2; exit 2;;
  esac
done

if test -n "$tag"; then
  test "$tag" = "v${tag#v}" || { printf '%s\n' 'ERROR: TAG must be a canonical release tag (vMAJOR.MINOR.PATCH).' >&2; exit 2; }
  version=${tag#v}
fi
version=${version#v}
test -n "$version" || { printf '%s\n' 'ERROR: VERSION, TAG or an exact release tag on HEAD is required.' >&2; exit 2; }
test "$version" != 0.0.0-dev || { printf '%s\n' 'ERROR: distribution verification needs a real release version, not an untagged dev build.' >&2; exit 2; }

os=${OS:-$(go env GOOS)}
arch=${ARCH:-$(go env GOARCH)}
release_dir=${RELEASE_DIR:-$root/dist/local-release/$version}
asset="openrouter-$version-$os-$arch.tar.gz"
artifact="$release_dir/artifacts/$asset"
metadata="$release_dir/manifest.json"
sha256sums="$release_dir/SHA256SUMS"

# archive's manifest lookup matches the artifact's path relative to --root,
# then falls back to its bare basename. This project's combined SHA256SUMS
# records paths relative to $release_dir ("artifacts/<asset>"), which
# matches neither directly, so pull just this one asset's recorded digest
# into a derived single-entry manifest keyed by its bare basename instead of
# re-deriving the digest from the artifact bytes themselves (which would
# trivially always match and defeat the point of an external manifest).
manifest_work=$(mktemp -d "${TMPDIR:-/tmp}/openrouter-distribution-check.XXXXXX")
trap 'rm -rf "$manifest_work"' EXIT
manifest="$manifest_work/SHA256SUMS"
awk -v asset="$asset" '$0 ~ ("[[:space:]]artifacts/" asset "$") { sub(/artifacts\//, ""); print }' "$sha256sums" > "$manifest" 2>/dev/null || : > "$manifest"

"$guide_tools_root/bin/guide-distribution-verify" "${safe_args[@]}" \
  --profile archive \
  --root "$root" \
  --tag "v$version" \
  --version "$version" \
  --artifact "$artifact" \
  --manifest "$manifest" \
  --metadata "$metadata" \
  --smoke-command make \
  --smoke-arg -C --smoke-arg "$root" \
  --smoke-arg install-smoke

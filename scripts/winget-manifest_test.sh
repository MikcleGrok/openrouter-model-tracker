#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

SCRIPT="$ROOT/scripts/winget-manifest.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# scripts/winget-manifest.sh resolves its exact release tag the same way
# scripts/sync-homebrew-formula.sh does when TAG_VERSION is not supplied
# (git describe --tags --exact-match against HEAD), so this test tags the
# current commit itself instead of hardcoding a tag from whichever commit
# the test was first written against -- same pattern as
# scripts/provenance_profile_test.sh. A fictitious version distinct from
# provenance_profile_test.sh's own v0.0.0 avoids any chance of collision if
# both happen to run around the same time.
test_tag=v0.0.9
test_version=0.0.9

# fixture_dir/fake_bin are populated below; cleanup guards on them so the
# trap is safe to install before either mktemp call runs.
fixture_dir=""
fake_bin=""
cleanup() {
  rm -rf "$fixture_dir" "$fake_bin"
  git tag -d "$test_tag" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Register the trap (above) before creating the tag: if anything between tag
# creation and trap setup were to fail (e.g. mktemp -d), the tag would leak
# into the real repo with nothing left to clean it up.
git tag "$test_tag" HEAD

fixture_dir="$(mktemp -d)"
winget_dir="$fixture_dir/winget-out"
fake_bin="$(mktemp -d)"

# --- fixture: a fake dist/local-release/<version> directory ---------------
digest_lower="deadbeef00112233445566778899aabbccddeeff00112233445566778899aa12"
mkdir -p "$fixture_dir/release/artifacts"
: > "$fixture_dir/release/artifacts/openrouter-$test_version-windows-amd64.zip"
printf '%s  artifacts/openrouter-%s-windows-amd64.zip\n' "$digest_lower" "$test_version" \
  > "$fixture_dir/release/SHA256SUMS"
printf '{"schema":"openrouter-model-tracker/local-release-v1","version":"%s","tag":"%s","commit":"%s","built_at":"2026-09-01T12:00:00Z","artifacts":[{"artifact":"artifacts/openrouter-%s-windows-amd64.zip","sha256":"%s"}]}\n' \
  "$test_version" "$test_tag" "$(git rev-parse HEAD)" "$test_version" "$digest_lower" \
  > "$fixture_dir/release/manifest.json"

run_winget() {
  env \
    PATH="$PATH" \
    WINGET_PACKAGE_IDENTIFIER=MikcleGrok.openrouter-model-tracker \
    WINGET_MANIFEST_VERSION=1.12.0 \
    WINGET_FORK=MikcleGrok/winget-pkgs \
    WINGET_PKGS_REPOSITORY=microsoft/winget-pkgs \
    GITHUB_REPOSITORY=MikcleGrok/openrouter-model-tracker \
    RELEASE_ARTIFACT_DIR="$fixture_dir/release" \
    WINGET_DIR="$winget_dir" \
    VERSION="$test_version" \
    TAG_VERSION="$test_tag" \
    WINGET_DRY_RUN="${WINGET_DRY_RUN:-0}" \
    "$SCRIPT" "$@"
}

# --- --generate -------------------------------------------------------------
run_winget --generate > "$fixture_dir/generate.out" 2>&1 \
  || { cat "$fixture_dir/generate.out" >&2; fail 'winget-manifest.sh --generate failed'; }

manifest_dir="$winget_dir/manifests/m/MikcleGrok/openrouter-model-tracker/$test_version"
version_file="$manifest_dir/MikcleGrok.openrouter-model-tracker.yaml"
installer_file="$manifest_dir/MikcleGrok.openrouter-model-tracker.installer.yaml"
locale_file="$manifest_dir/MikcleGrok.openrouter-model-tracker.locale.en-US.yaml"

test -d "$manifest_dir" || fail "expected manifest directory not found: $manifest_dir"

actual_files="$(cd "$manifest_dir" && ls -1 | sort)"
expected_files="$(printf '%s\n' \
  'MikcleGrok.openrouter-model-tracker.installer.yaml' \
  'MikcleGrok.openrouter-model-tracker.locale.en-US.yaml' \
  'MikcleGrok.openrouter-model-tracker.yaml' | sort)"
test "$actual_files" = "$expected_files" \
  || { printf 'expected:\n%s\ngot:\n%s\n' "$expected_files" "$actual_files" >&2; fail "unexpected file set in $manifest_dir"; }

grep -Fqx -- '- RelativeFilePath: openrouter.exe' "$installer_file" \
  || fail "installer manifest missing '- RelativeFilePath: openrouter.exe'"

expected_digest_upper="$(printf '%s' "$digest_lower" | tr '[:lower:]' '[:upper:]')"
grep -Fq "InstallerSha256: $expected_digest_upper" "$installer_file" \
  || fail 'installer manifest InstallerSha256 is not the expected uppercase digest'
grep -Fq "$digest_lower" "$installer_file" \
  && fail 'installer manifest InstallerSha256 must be uppercase, found the lowercase form'

grep -Fq 'PackageIdentifier: MikcleGrok.openrouter-model-tracker' "$version_file" \
  || fail 'version manifest missing PackageIdentifier'
grep -Fq "InstallerUrl: https://github.com/MikcleGrok/openrouter-model-tracker/releases/download/$test_tag/openrouter-$test_version-windows-amd64.zip" "$installer_file" \
  || fail 'installer manifest InstallerUrl does not match the expected release asset URL'
grep -Fq 'License: MIT' "$locale_file" || fail 'locale manifest missing License: MIT'

grep -Fqx -- '  PortableCommandAlias: openrouter' "$installer_file" \
  || fail "installer manifest missing '  PortableCommandAlias: openrouter'"
grep -Fqx -- 'Commands:' "$installer_file" \
  || fail "installer manifest missing 'Commands:'"
grep -Fqx -- '- openrouter' "$installer_file" \
  || fail "installer manifest missing '- openrouter' under Commands:"
grep -Fqx 'ReleaseDate: 2026-09-01' "$installer_file" \
  || fail "installer manifest ReleaseDate does not match manifest.json's built_at date"
grep -Fqx 'Moniker: omt' "$locale_file" \
  || fail "locale manifest missing 'Moniker: omt'"
grep -Fqx "LicenseUrl: https://github.com/MikcleGrok/openrouter-model-tracker/blob/master/LICENSE" "$locale_file" \
  || fail 'locale manifest LicenseUrl does not match the expected repository URL'
for f in "$version_file" "$installer_file" "$locale_file"; do
  grep -Fqx 'ManifestVersion: 1.12.0' "$f" \
    || fail "$f missing 'ManifestVersion: 1.12.0' (WINGET_MANIFEST_VERSION)"
done

printf '%s\n' "generate: OK ($manifest_dir)"

# --- --check must fail closed before the GitHub Release is published ------
cat > "$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
  "release view") exit 1 ;;
  *) printf 'unexpected gh invocation in --check fixture stub: %s\n' "$*" >&2; exit 90 ;;
esac
EOF
chmod 0755 "$fake_bin/gh"

old_path="$PATH"
PATH="$fake_bin:$PATH"
if run_winget --check > "$fixture_dir/check-unpublished.out" 2>&1; then
  PATH="$old_path"
  fail '--check unexpectedly passed with no published GitHub Release'
fi
PATH="$old_path"
grep -Fq 'BLOCKED:' "$fixture_dir/check-unpublished.out" \
  || fail '--check must fail closed with a BLOCKED: message when the release is not yet published'
grep -Fq 'release-github' "$fixture_dir/check-unpublished.out" \
  || fail '--check failure message must name release-github as the required ordering'

printf '%s\n' 'check (unpublished release, fails closed): OK'

# --- --check succeeds once the GitHub Release is published and the asset's
#     digest matches SHA256SUMS (exercises cmd_check's success path: parsing
#     InstallerSha256/RelativeFilePath out of the generated YAML and cross-
#     verifying against a stubbed `gh release view --json assets`) ----------
cat > "$fake_bin/gh" <<GHSTUB
#!/usr/bin/env bash
case "\$1 \$2" in
  "release view") printf '%s' '{"assets":[{"name":"openrouter-$test_version-windows-amd64.zip","digest":"sha256:$digest_lower"}]}' ;;
  *) printf 'unexpected gh invocation in --check success fixture stub: %s\n' "\$*" >&2; exit 90 ;;
esac
GHSTUB
chmod 0755 "$fake_bin/gh"

check_published_out="$fixture_dir/check-published.out"
old_path="$PATH"
PATH="$fake_bin:$PATH"
if ! run_winget --check > "$check_published_out" 2>&1; then
  PATH="$old_path"
  cat "$check_published_out" >&2
  fail '--check unexpectedly failed with a published GitHub Release and a matching asset digest'
fi
PATH="$old_path"

grep -Fq 'winget manifests verified' "$check_published_out" \
  || fail '--check success path did not print the verified-success message'
grep -Fq 'digest cross-verified' "$check_published_out" \
  || fail '--check success path did not report the digest as cross-verified against SHA256SUMS'

printf '%s\n' 'check (published release, digest cross-verified): OK'

# --- --submit with WINGET_DRY_RUN=1 must not touch the network ------------
cat > "$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
printf 'gh must not be invoked when WINGET_DRY_RUN=1: %s\n' "$*" >&2
exit 91
EOF
chmod 0755 "$fake_bin/gh"

dry_run_out="$fixture_dir/submit-dry-run.out"
old_path="$PATH"
PATH="$fake_bin:$PATH"
WINGET_DRY_RUN=1
if ! run_winget --submit > "$dry_run_out" 2>&1; then
  PATH="$old_path"
  cat "$dry_run_out" >&2
  fail 'winget-manifest.sh --submit WINGET_DRY_RUN=1 exited non-zero'
fi
PATH="$old_path"
WINGET_DRY_RUN=0

grep -Fq 'DRY RUN' "$dry_run_out" || fail '--submit WINGET_DRY_RUN=1 did not print a DRY RUN preview'
grep -Fq 'gh repo sync MikcleGrok/winget-pkgs --source microsoft/winget-pkgs' "$dry_run_out" \
  || fail '--submit dry run is missing the fork-sync command'
grep -Fq 'gh api repos/MikcleGrok/winget-pkgs/git/refs' "$dry_run_out" \
  || fail '--submit dry run is missing the branch-create command'
manifest_path="manifests/m/MikcleGrok/openrouter-model-tracker/$test_version"
grep -Fq "gh api repos/MikcleGrok/winget-pkgs/contents/$manifest_path/MikcleGrok.openrouter-model-tracker.yaml" "$dry_run_out" \
  || fail '--submit dry run is missing the version-file upload command'
grep -Fq "gh api repos/MikcleGrok/winget-pkgs/contents/$manifest_path/MikcleGrok.openrouter-model-tracker.installer.yaml" "$dry_run_out" \
  || fail '--submit dry run is missing the installer-file upload command'
grep -Fq "gh api repos/MikcleGrok/winget-pkgs/contents/$manifest_path/MikcleGrok.openrouter-model-tracker.locale.en-US.yaml" "$dry_run_out" \
  || fail '--submit dry run is missing the locale-file upload command'
grep -Fq 'gh pr create --repo microsoft/winget-pkgs --head MikcleGrok:' "$dry_run_out" \
  || fail '--submit dry run is missing the gh pr create command'

printf '%s\n' 'submit --dry-run (offline, no gh call made): OK'

# --- --help exits 0 and does not require any evidence ----------------------
"$SCRIPT" --help > /dev/null || fail '--help must exit 0'

printf '%s\n' 'winget-manifest.sh offline checks passed'

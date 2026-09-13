#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

SCRIPT="$ROOT/scripts/scoop-manifest.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# scripts/scoop-manifest.sh resolves its exact release tag the same way
# scripts/winget-manifest.sh does when TAG_VERSION is not supplied (git
# describe --tags --exact-match against HEAD), so this test tags the current
# commit itself instead of hardcoding a tag from whichever commit the test
# was first written against -- same pattern as scripts/winget-manifest_test.sh.
# A fictitious version distinct from provenance_profile_test.sh's own v0.0.0
# and winget-manifest_test.sh's own v0.0.9 avoids any chance of collision if
# more than one of these tests happen to run around the same time.
test_tag=v0.0.8
test_version=0.0.8

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
scoop_dir="$fixture_dir/scoop-out"
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

run_scoop() {
  env \
    PATH="$PATH" \
    SCOOP_BUCKET_REPOSITORY=MikcleGrok/scoop-bucket \
    SCOOP_BUCKET_BRANCH=main \
    SCOOP_APP_ID=openrouter-model-tracker \
    GITHUB_REPOSITORY=MikcleGrok/openrouter-model-tracker \
    RELEASE_ARTIFACT_DIR="$fixture_dir/release" \
    SCOOP_DIR="$scoop_dir" \
    VERSION="$test_version" \
    TAG_VERSION="$test_tag" \
    SCOOP_DRY_RUN="${SCOOP_DRY_RUN:-0}" \
    "$SCRIPT" "$@"
}

# --- --generate -------------------------------------------------------------
run_scoop --generate > "$fixture_dir/generate.out" 2>&1 \
  || { cat "$fixture_dir/generate.out" >&2; fail 'scoop-manifest.sh --generate failed'; }

manifest_file="$scoop_dir/bucket/openrouter-model-tracker.json"
test -f "$manifest_file" || fail "expected bucket manifest not found: $manifest_file"

command -v jq >/dev/null 2>&1 || fail 'jq is required to run this test'
jq -e . "$manifest_file" >/dev/null 2>&1 || fail 'generated bucket manifest is not valid JSON'

expected_url="https://github.com/MikcleGrok/openrouter-model-tracker/releases/download/$test_tag/openrouter-$test_version-windows-amd64.zip"

grep -Fq '"$schema": "https://raw.githubusercontent.com/ScoopInstaller/Scoop/master/schema.json"' "$manifest_file" \
  || fail 'bucket manifest missing $schema'
grep -Fq "\"version\": \"$test_version\"" "$manifest_file" \
  || fail 'bucket manifest version does not match VERSION'
grep -Fq '"description": "Compare AI models on OpenRouter by quality and price."' "$manifest_file" \
  || fail 'bucket manifest missing description'
grep -Fq '"homepage": "https://github.com/MikcleGrok/openrouter-model-tracker"' "$manifest_file" \
  || fail 'bucket manifest missing homepage'
grep -Fq '"license": "MIT"' "$manifest_file" \
  || fail 'bucket manifest missing license'
grep -Fq "\"url\": \"$expected_url\"" "$manifest_file" \
  || fail 'bucket manifest architecture.64bit.url does not match the expected release asset URL'
grep -Fq "\"hash\": \"$digest_lower\"" "$manifest_file" \
  || fail 'bucket manifest architecture.64bit.hash is not the expected lowercase digest, straight from SHA256SUMS'
grep -Fq '"bin": [' "$manifest_file" || fail 'bucket manifest missing bin array'
grep -Fq '"openrouter.exe"' "$manifest_file" || fail 'bucket manifest bin array missing openrouter.exe'
grep -Fq '"openrouter-model-tracker"' "$manifest_file" \
  || fail 'bucket manifest bin array missing openrouter-model-tracker shim name'
grep -Fq '"omt"' "$manifest_file" || fail 'bucket manifest bin array missing omt alias'
grep -Fq '"openrouter",' "$manifest_file" \
  && fail 'bucket manifest must not shim a bare "openrouter" command name'
grep -Fq '"github": "https://github.com/MikcleGrok/openrouter-model-tracker"' "$manifest_file" \
  || fail 'bucket manifest missing checkver.github'

# The autoupdate url template must stay literally "$version" (8 chars,
# unexpanded by the shell) -- Scoop's own template engine substitutes it
# later, not this script.
grep -Fq '$version' "$manifest_file" \
  || fail 'bucket manifest autoupdate url template lost its literal $version placeholder'

# Real invariant, not just a literal-string check: substituting the real
# version into that template must equal architecture.64bit.url byte-for-byte.
autoupdate_url="$(jq -r '.autoupdate.architecture."64bit".url' "$manifest_file")"
substituted_url="${autoupdate_url//\$version/$test_version}"
test "$substituted_url" = "$expected_url" \
  || fail "autoupdate url template with \$version substituted ($substituted_url) does not equal architecture.64bit.url ($expected_url)"

printf '%s\n' "generate: OK ($manifest_file)"

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
if run_scoop --check > "$fixture_dir/check-unpublished.out" 2>&1; then
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
#     digest matches SHA256SUMS ----------------------------------------------
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
if ! run_scoop --check > "$check_published_out" 2>&1; then
  PATH="$old_path"
  cat "$check_published_out" >&2
  fail '--check unexpectedly failed with a published GitHub Release and a matching asset digest'
fi
PATH="$old_path"

grep -Fq 'scoop manifest verified' "$check_published_out" \
  || fail '--check success path did not print the verified-success message'
grep -Fq 'digest cross-verified' "$check_published_out" \
  || fail '--check success path did not report the digest as cross-verified against SHA256SUMS'

printf '%s\n' 'check (published release, digest cross-verified): OK'

# --- --submit --dry-run when the bucket manifest does not yet exist (404) --
cat > "$fake_bin/gh" <<EOF
#!/usr/bin/env bash
case "\$1 \$2" in
  "api repos/MikcleGrok/scoop-bucket/contents/bucket/openrouter-model-tracker.json?ref=main")
    printf 'gh: HTTP 404: Not Found (https://api.github.com/repos/MikcleGrok/scoop-bucket/contents/bucket/openrouter-model-tracker.json)\n' >&2
    exit 1
    ;;
  *)
    printf 'gh must not be invoked (mutating call attempted) when SCOOP_DRY_RUN=1: %s\n' "\$*" >&2
    exit 91
    ;;
esac
EOF
chmod 0755 "$fake_bin/gh"

submit_dry_run_404_out="$fixture_dir/submit-dry-run-404.out"
old_path="$PATH"
PATH="$fake_bin:$PATH"
SCOOP_DRY_RUN=1
if ! run_scoop --submit > "$submit_dry_run_404_out" 2>&1; then
  PATH="$old_path"
  SCOOP_DRY_RUN=0
  cat "$submit_dry_run_404_out" >&2
  fail 'scoop-manifest.sh --submit SCOOP_DRY_RUN=1 exited non-zero (404 branch)'
fi
PATH="$old_path"
SCOOP_DRY_RUN=0

grep -Fq 'Bucket manifest does not exist yet in MikcleGrok/scoop-bucket -- this will be an Add' "$submit_dry_run_404_out" \
  || fail '--submit dry run (404 branch) did not print the planned-Add informational line'
grep -Fq "Add openrouter-model-tracker $test_version" "$submit_dry_run_404_out" \
  || fail '--submit dry run (404 branch) commit message is not "Add openrouter-model-tracker <version>"'
grep -Fq 'DRY RUN' "$submit_dry_run_404_out" || fail '--submit dry run (404 branch) did not print a DRY RUN preview'
grep -Fq 'gh api repos/MikcleGrok/scoop-bucket/contents/bucket/openrouter-model-tracker.json --method PUT' "$submit_dry_run_404_out" \
  || fail '--submit dry run (404 branch) is missing the planned PUT command'
grep -Fq -- '-f sha=' "$submit_dry_run_404_out" \
  && fail '--submit dry run (404 branch) must not carry a sha (new file, no existing blob)'

printf '%s\n' 'submit --dry-run, bucket path 404 (plans Add, zero mutating calls): OK'

# --- --submit --dry-run when the bucket manifest exists with a different
#     blob sha (200, sha mismatch) -- plans an Update carrying that sha ----
existing_sha="1111111111111111111111111111111111111111"
cat > "$fake_bin/gh" <<EOF
#!/usr/bin/env bash
case "\$1 \$2" in
  "api repos/MikcleGrok/scoop-bucket/contents/bucket/openrouter-model-tracker.json?ref=main")
    printf '%s' '{"sha":"$existing_sha"}'
    ;;
  *)
    printf 'gh must not be invoked (mutating call attempted) when SCOOP_DRY_RUN=1: %s\n' "\$*" >&2
    exit 91
    ;;
esac
EOF
chmod 0755 "$fake_bin/gh"

submit_dry_run_update_out="$fixture_dir/submit-dry-run-update.out"
old_path="$PATH"
PATH="$fake_bin:$PATH"
SCOOP_DRY_RUN=1
if ! run_scoop --submit > "$submit_dry_run_update_out" 2>&1; then
  PATH="$old_path"
  SCOOP_DRY_RUN=0
  cat "$submit_dry_run_update_out" >&2
  fail 'scoop-manifest.sh --submit SCOOP_DRY_RUN=1 exited non-zero (200/sha-mismatch branch)'
fi
PATH="$old_path"
SCOOP_DRY_RUN=0

grep -Fq "Bucket manifest exists in MikcleGrok/scoop-bucket (sha $existing_sha) -- this will be an Update" "$submit_dry_run_update_out" \
  || fail '--submit dry run (sha-mismatch branch) did not print the planned-Update informational line'
grep -Fq "Update openrouter-model-tracker to $test_version" "$submit_dry_run_update_out" \
  || fail '--submit dry run (sha-mismatch branch) commit message is not "Update openrouter-model-tracker to <version>"'
grep -Fq -- "-f sha=$existing_sha" "$submit_dry_run_update_out" \
  || fail '--submit dry run (sha-mismatch branch) planned PUT does not carry the existing blob sha'
grep -Fq 'DRY RUN' "$submit_dry_run_update_out" \
  || fail '--submit dry run (sha-mismatch branch) did not print a DRY RUN preview'

printf '%s\n' 'submit --dry-run, bucket path 200 with different sha (plans Update with sha, zero mutating calls): OK'

# --- --submit --dry-run when the bucket already has the identical blob sha
#     (200, sha match) -- "already up to date", zero calls beyond the GET --
manifest_blob_sha="$(git -C "$ROOT" hash-object "$manifest_file")"
cat > "$fake_bin/gh" <<EOF
#!/usr/bin/env bash
call_count_file="$fixture_dir/noop-call-count"
count=0
test -f "\$call_count_file" && count="\$(cat "\$call_count_file")"
count=\$((count + 1))
printf '%s' "\$count" > "\$call_count_file"
case "\$1 \$2" in
  "api repos/MikcleGrok/scoop-bucket/contents/bucket/openrouter-model-tracker.json?ref=main")
    if test "\$count" -gt 1; then
      printf 'gh must be invoked exactly once (the GET) when the local blob sha already matches: %s\n' "\$*" >&2
      exit 92
    fi
    printf '%s' '{"sha":"$manifest_blob_sha"}'
    ;;
  *)
    printf 'gh must not be invoked at all beyond the GET when the local blob sha already matches: %s\n' "\$*" >&2
    exit 91
    ;;
esac
EOF
chmod 0755 "$fake_bin/gh"
rm -f "$fixture_dir/noop-call-count"

submit_dry_run_noop_out="$fixture_dir/submit-dry-run-noop.out"
old_path="$PATH"
PATH="$fake_bin:$PATH"
SCOOP_DRY_RUN=1
if ! run_scoop --submit > "$submit_dry_run_noop_out" 2>&1; then
  PATH="$old_path"
  SCOOP_DRY_RUN=0
  cat "$submit_dry_run_noop_out" >&2
  fail 'scoop-manifest.sh --submit SCOOP_DRY_RUN=1 exited non-zero (200/sha-match no-op branch)'
fi
PATH="$old_path"
SCOOP_DRY_RUN=0

grep -Fq "already up to date" "$submit_dry_run_noop_out" \
  || fail '--submit dry run (sha-match branch) did not report "already up to date"'
grep -Fq "$manifest_blob_sha" "$submit_dry_run_noop_out" \
  || fail '--submit dry run (sha-match branch) did not report the matching blob sha'
test "$(cat "$fixture_dir/noop-call-count")" -eq 1 \
  || fail '--submit dry run (sha-match branch) must call gh exactly once (the GET only)'

printf '%s\n' 'submit --dry-run, bucket blob sha already matches (no-op, exactly one gh call): OK'

# --- --submit --dry-run fails closed when the GET itself fails for a reason
#     other than 404 (network error, bad auth, etc.) -------------------------
cat > "$fake_bin/gh" <<EOF
#!/usr/bin/env bash
case "\$1 \$2" in
  "api repos/MikcleGrok/scoop-bucket/contents/bucket/openrouter-model-tracker.json?ref=main")
    printf 'gh: could not resolve host github.com\n' >&2
    exit 1
    ;;
  *)
    printf 'gh must not be invoked (mutating call attempted) when SCOOP_DRY_RUN=1: %s\n' "\$*" >&2
    exit 91
    ;;
esac
EOF
chmod 0755 "$fake_bin/gh"

submit_dry_run_error_out="$fixture_dir/submit-dry-run-error.out"
old_path="$PATH"
PATH="$fake_bin:$PATH"
SCOOP_DRY_RUN=1
if run_scoop --submit > "$submit_dry_run_error_out" 2>&1; then
  PATH="$old_path"
  SCOOP_DRY_RUN=0
  cat "$submit_dry_run_error_out" >&2
  fail '--submit dry run unexpectedly succeeded when the GET failed for a non-404 reason'
fi
PATH="$old_path"
SCOOP_DRY_RUN=0

grep -Fq 'BLOCKED:' "$submit_dry_run_error_out" \
  || fail '--submit dry run must fail closed with a BLOCKED: message when the GET fails unexpectedly'

printf '%s\n' 'submit --dry-run, GET fails unexpectedly (fails closed): OK'

# --- --help exits 0 and does not require any evidence ----------------------
"$SCRIPT" --help > /dev/null || fail '--help must exit 0'

printf '%s\n' 'scoop-manifest.sh --help: OK'

printf '%s\n' 'scoop-manifest.sh offline checks passed'

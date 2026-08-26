#!/usr/bin/env bash
set -euo pipefail

ROOT="$(dirname "${BASH_SOURCE[0]}")/.."
cd "$ROOT"
mkdir -p .release
rm -f .release/signature-verification.json .release/provenance-verification.json .release/verify-blob.txt

fake_bin="$(mktemp -d)"
fake_evidence="$(mktemp -d)"
trap 'rm -rf "$fake_bin" "$fake_evidence"; rm -f .release/signature-verification.json .release/provenance-verification.json .release/verify-blob.txt /tmp/provenance-profile-test.out /tmp/provenance-profile-test-make.out' EXIT
cat > "$fake_bin/cosign" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 'cosign must not be called by local profile' >&2
exit 99
EOF
chmod 0755 "$fake_bin/cosign"
printf '%s\n' '{}' > "$fake_evidence/manifest.json"
printf '%s\n' '{}' > "$fake_evidence/signature.bundle.json"

for profile in local candidate; do
  output="$(PATH="$fake_bin:$PATH" env -u COSIGN_PRIVATE_KEY PROVENANCE_PROFILE="$profile" ./scripts/verify-provenance.sh signature)"
  test "$output" = "NOT APPLICABLE: PROVENANCE_PROFILE=$profile disables cosign signature (see .release/signature-verification.json)"
  test "$(jq -r '.status + ":" + .profile' .release/signature-verification.json)" = "not-applicable:$profile"
done

if make -n sign PROVENANCE_PROFILE=publised > /tmp/provenance-profile-test-make.out 2>&1; then
  printf '%s\n' 'unknown profile unexpectedly passed Makefile validation' >&2
  exit 1
fi
grep -Fq "BLOCKED: unknown PROVENANCE_PROFILE 'publised'" /tmp/provenance-profile-test-make.out

make -n verify-provenance PROVENANCE_PROFILE=external | grep -Fq 'cmd/evidencecheck --published-evidence'

cat > "$fake_bin/cosign" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  verify-blob) exit 0 ;;
  *) printf '%s\n' "unexpected cosign operation: $*" >&2; exit 98 ;;
esac
EOF
chmod 0755 "$fake_bin/cosign"
PATH="$fake_bin:$PATH" env -u COSIGN_PRIVATE_KEY PROVENANCE_PROFILE=external TAG_VERSION=v1.14.37 VERSION=1.14.37 COSIGN_PUBLIC_KEY="$ROOT/cosign.pub" RELEASE_MANIFEST="$fake_evidence/manifest.json" RELEASE_MANIFEST_SIG="$fake_evidence/signature.bundle.json" ./scripts/verify-provenance.sh signature
test "$(jq -r '.status + ":" + .profile' .release/signature-verification.json)" = "verified:external"

rm -f .release/signature-verification.json "$fake_evidence/signature.bundle.json"
if PATH="$fake_bin:$PATH" env -u COSIGN_PRIVATE_KEY PROVENANCE_PROFILE=external TAG_VERSION=v1.14.37 VERSION=1.14.37 COSIGN_PUBLIC_KEY="$ROOT/cosign.pub" RELEASE_MANIFEST="$fake_evidence/manifest.json" RELEASE_MANIFEST_SIG="$fake_evidence/signature.bundle.json" ./scripts/verify-provenance.sh signature > /tmp/provenance-profile-test.out 2>&1; then
  printf '%s\n' 'external profile unexpectedly passed without signature bundle' >&2
  exit 1
fi
grep -Fq 'BLOCKED: required evidence file is missing or empty' /tmp/provenance-profile-test.out
printf '%s\n' 'provenance profile checks passed'

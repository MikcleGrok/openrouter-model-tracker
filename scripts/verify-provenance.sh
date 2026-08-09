#!/usr/bin/env bash
# scripts/verify-provenance.sh — cosign-based release provenance check.
#
# Replaces the old unconditional NO-OP verify-provenance/signature Makefile
# targets. Signing mechanism, evidence layout and check order are per
# ~/projects/tools/guide-tools/.task/go-guide-compliance/signing-design.md
# §5.5/§7 (static cosign key, key-pair mode, no Fulcio/Rekor/transparency
# log). openrouter-model-tracker is the last of the five projects in that
# design's rollout; uni-db (branch signing-provenance) is the pattern this
# script was copied from.
#
# Invoked with a positional SCOPE argument by two Makefile targets that
# share this script:
#
#   signature  — make signature. Checks only that the committed public key
#                is present and that `cosign verify-blob` succeeds against
#                release-manifest.json. Writes
#                $EVIDENCE_DIR/signature-verification.json.
#
#   full       — make verify-provenance (depends on `signature`, and this
#                script re-verifies verify-blob independently rather than
#                trusting the prior target's exit). Adds
#                `cosign verify-blob-attestation` and the manifest content
#                cross-checks (tag/commit/version, artifact digest, SBOM
#                digest), writes $EVIDENCE_DIR/provenance-verification.json
#                as this script's own diagnostic record, and — only once
#                every check above has actually passed — writes
#                $PUBLISHED_EVIDENCE conforming to the existing
#                openrouter-model-tracker/published-evidence/v1 schema
#                already validated by cmd/evidencecheck --published-evidence
#                (see cmd/evidencecheck/main.go's validatePublished; the
#                Makefile invokes it right after this script exits 0, so
#                the shape written here MUST match that schema exactly).
#
# Both scopes honour PROVENANCE_PROFILE:
#
#   candidate  — applicability no-op. Used pre-tag (release-check, PR
#                gates), where no release tag and no signed evidence exist
#                yet. Always exits 0 and writes an explicit "not-applicable"
#                evidence record instead of skipping silently or writing a
#                $PUBLISHED_EVIDENCE that doesn't actually exist yet.
#   published  — real verification (default). Requires an exact release tag
#                at HEAD and signed evidence in the .release evidence
#                directory. Fails closed (BLOCKED + non-zero exit) on any
#                missing tool, missing file, or mismatch.
#
# Required env (set by the Makefile targets of the same name; all have
# defaults below matching the Makefile's own variables):
#   PROVENANCE_PROFILE   candidate|published (default: published)
#   TAG_VERSION          exact vMAJOR.MINOR.PATCH tag under test
#   VERSION              TAG_VERSION without the leading v
#   COSIGN_PUBLIC_KEY    path to the committed public key (default:
#                        cosign.pub)
#   RELEASE_MANIFEST     path to release-manifest.json
#   RELEASE_MANIFEST_SIG path to its cosign signature bundle (both scopes)
#   RELEASE_MANIFEST_ATT path to its cosign attestation bundle (full scope)
#   BIN                  path to the built binary (full scope; default:
#                        bin/openrouter)
#   SBOM_FILE            path to the generated SBOM (full scope)
#   GITHUB_REPOSITORY    owner/repo, mirrored into published evidence
#                        "source" (full scope)
#   PUBLISHED_EVIDENCE   output path for published-evidence.json (full
#                        scope, published profile only)
#
# Always run via `cd $(ROOT) && ... scripts/verify-provenance.sh`, so every
# path above is resolved relative to the repository root regardless of the
# caller's own working directory.

set -euo pipefail

SCOPE="${1:-}"
case "$SCOPE" in
  signature|full) ;;
  *) echo "BLOCKED: usage: $0 signature|full" >&2; exit 1 ;;
esac

PROVENANCE_PROFILE="${PROVENANCE_PROFILE:-published}"
EVIDENCE_DIR=".release"
TAG_VERSION="${TAG_VERSION:-}"
VERSION="${VERSION:-}"
COSIGN_PUBLIC_KEY="${COSIGN_PUBLIC_KEY:-cosign.pub}"
RELEASE_MANIFEST="${RELEASE_MANIFEST:-$EVIDENCE_DIR/release-manifest.json}"
RELEASE_MANIFEST_SIG="${RELEASE_MANIFEST_SIG:-$EVIDENCE_DIR/release-manifest.json.sig.bundle.json}"
RELEASE_MANIFEST_ATT="${RELEASE_MANIFEST_ATT:-$EVIDENCE_DIR/release-manifest.json.att.bundle.json}"
BIN="${BIN:-bin/openrouter}"
SBOM_FILE="${SBOM_FILE:-$EVIDENCE_DIR/sbom.spdx.json}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-MikcleGrok/openrouter-model-tracker}"
PUBLISHED_EVIDENCE="${PUBLISHED_EVIDENCE:-$EVIDENCE_DIR/published-evidence.json}"

mkdir -p "$EVIDENCE_DIR"
commit="$(git rev-parse HEAD)"

sig_out="$EVIDENCE_DIR/signature-verification.json"
prov_out="$EVIDENCE_DIR/provenance-verification.json"

fail_sig() {
  reason="$1"
  printf '{"schema":"openrouter-model-tracker/signature-verification/v1","profile":"published","status":"blocked","reason":%s,"commit":"%s"}\n' \
    "$(printf '%s' "$reason" | jq -Rs .)" "$commit" > "$sig_out"
  echo "BLOCKED: $reason" >&2
  exit 1
}

fail_prov() {
  reason="$1"
  printf '{"schema":"openrouter-model-tracker/provenance-verification/v1","profile":"published","status":"blocked","reason":%s,"commit":"%s"}\n' \
    "$(printf '%s' "$reason" | jq -Rs .)" "$commit" > "$prov_out"
  echo "BLOCKED: $reason" >&2
  exit 1
}

# --- candidate profile: explicit applicability no-op ---------------------

if [ "$PROVENANCE_PROFILE" = "candidate" ]; then
  if [ "$SCOPE" = "signature" ]; then
    out="$sig_out"; schema="openrouter-model-tracker/signature-verification/v1"
  else
    out="$prov_out"; schema="openrouter-model-tracker/provenance-verification/v1"
  fi
  cat > "$out" <<EOF
{"schema":"$schema","profile":"candidate","status":"not-applicable","reason":"no release tag exists yet at this commit; signed evidence is produced by release.yml only after a tag is pushed","commit":"$commit"}
EOF
  echo "PROVENANCE_PROFILE=candidate: $SCOPE check not applicable before a release tag exists (see $out)"
  exit 0
fi

if [ "$PROVENANCE_PROFILE" != "published" ]; then
  echo "BLOCKED: unknown PROVENANCE_PROFILE '$PROVENANCE_PROFILE' (expected candidate|published)" >&2
  exit 1
fi

# --- published profile: real verification --------------------------------

command -v cosign >/dev/null 2>&1 || fail_sig "cosign is required to verify release provenance"
command -v jq >/dev/null 2>&1 || fail_sig "jq is required to verify release provenance"

test -n "$TAG_VERSION" || fail_sig "TAG_VERSION must identify the exact tag under verification"
printf '%s' "$TAG_VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || fail_sig "TAG_VERSION must be an exact vMAJOR.MINOR.PATCH tag: $TAG_VERSION"
test "$(git rev-parse --verify --quiet "$TAG_VERSION^{commit}" 2>/dev/null || true)" = "$commit" || fail_sig "$TAG_VERSION does not identify HEAD ($commit)"

for f in "$RELEASE_MANIFEST" "$RELEASE_MANIFEST_SIG"; do
  test -s "$f" || fail_sig "required evidence file is missing or empty: $f"
done

# 1. public key present and non-empty; try the active key, then the
#    previous one, per the rollover story in signing-design.md §6.
key_used=""
for candidate_key in "$COSIGN_PUBLIC_KEY" "${COSIGN_PUBLIC_KEY}.previous"; do
  if [ -s "$candidate_key" ]; then
    key_used="$candidate_key"
    break
  fi
done
test -n "$key_used" || fail_sig "no usable public key found ($COSIGN_PUBLIC_KEY or ${COSIGN_PUBLIC_KEY}.previous)"

# 2. cosign verify-blob
verify_blob_with_key() {
  cosign verify-blob --key "$1" --bundle "$RELEASE_MANIFEST_SIG" --insecure-ignore-tlog=true "$RELEASE_MANIFEST" > "$EVIDENCE_DIR/verify-blob.txt" 2>&1
}

verified=0
if verify_blob_with_key "$key_used"; then
  verified=1
elif [ "$key_used" = "$COSIGN_PUBLIC_KEY" ] && [ -s "${COSIGN_PUBLIC_KEY}.previous" ]; then
  if verify_blob_with_key "${COSIGN_PUBLIC_KEY}.previous"; then
    key_used="${COSIGN_PUBLIC_KEY}.previous"
    verified=1
  fi
fi
test "$verified" -eq 1 || fail_sig "cosign verify-blob failed against $key_used (see $EVIDENCE_DIR/verify-blob.txt)"

key_sha256="$(openssl dgst -sha256 -r "$key_used" | cut -d' ' -f1)"

cat > "$sig_out" <<EOF
{"schema":"openrouter-model-tracker/signature-verification/v1","profile":"published","status":"verified","commit":"$commit","tag":"$TAG_VERSION","manifest":"$RELEASE_MANIFEST","signing_key":"$key_used","signing_key_sha256":"$key_sha256","checks":{"public_key_present":"pass","verify_blob":"pass"},"native_outputs":["$EVIDENCE_DIR/verify-blob.txt"]}
EOF
echo "verified: release manifest signature for $TAG_VERSION at $commit (key: $key_used)"

if [ "$SCOPE" = "signature" ]; then
  exit 0
fi

# --- full scope: attestation + content cross-checks + published evidence -

test -s "$RELEASE_MANIFEST_ATT" || fail_prov "required evidence file is missing or empty: $RELEASE_MANIFEST_ATT"

verify_attestation_with_key() {
  cosign verify-blob-attestation --key "$1" --type slsaprovenance1 --bundle "$RELEASE_MANIFEST_ATT" --insecure-ignore-tlog=true --check-claims=true "$RELEASE_MANIFEST" > "$EVIDENCE_DIR/verify-blob-attestation.txt" 2>&1
}

att_verified=0
if verify_attestation_with_key "$key_used"; then
  att_verified=1
elif [ "$key_used" != "${COSIGN_PUBLIC_KEY}.previous" ] && [ -s "${COSIGN_PUBLIC_KEY}.previous" ]; then
  if verify_attestation_with_key "${COSIGN_PUBLIC_KEY}.previous"; then
    key_used="${COSIGN_PUBLIC_KEY}.previous"
    att_verified=1
  fi
fi
test "$att_verified" -eq 1 || fail_prov "cosign verify-blob-attestation failed against $key_used (see $EVIDENCE_DIR/verify-blob-attestation.txt)"

manifest_tag="$(jq -r '.tag' "$RELEASE_MANIFEST")"
manifest_commit="$(jq -r '.commit' "$RELEASE_MANIFEST")"
manifest_version="$(jq -r '.version' "$RELEASE_MANIFEST")"

test "$manifest_tag" = "$TAG_VERSION" || fail_prov "manifest tag '$manifest_tag' does not match TAG_VERSION '$TAG_VERSION'"
test "$manifest_commit" = "$commit" || fail_prov "manifest commit '$manifest_commit' does not match HEAD '$commit'"
if [ -n "$VERSION" ]; then
  test "$manifest_version" = "$VERSION" || fail_prov "manifest version '$manifest_version' does not match VERSION '$VERSION'"
fi

artifact_count="$(jq '.artifacts | length' "$RELEASE_MANIFEST")"
test "$artifact_count" -eq 1 || fail_prov "manifest must list exactly one artifact (bin/openrouter), found $artifact_count"

artifact_path="$(jq -r '.artifacts[0].path' "$RELEASE_MANIFEST")"
artifact_expected="$(jq -r '.artifacts[0].sha256' "$RELEASE_MANIFEST")"
test "$artifact_path" = "$BIN" || fail_prov "manifest artifact path '$artifact_path' does not match expected '$BIN'"
test -f "$artifact_path" || fail_prov "manifest references artifact not present in this checkout: $artifact_path"
artifact_actual="$(openssl dgst -sha256 -r "$artifact_path" | cut -d' ' -f1)"
test "$artifact_actual" = "$artifact_expected" || fail_prov "artifact digest mismatch for $artifact_path: expected $artifact_expected, actual $artifact_actual"

sbom_path="$(jq -r '.sbom.path' "$RELEASE_MANIFEST")"
sbom_expected="$(jq -r '.sbom.sha256' "$RELEASE_MANIFEST")"
test -f "$sbom_path" || fail_prov "manifest references SBOM not present in this checkout: $sbom_path"
sbom_actual="$(openssl dgst -sha256 -r "$sbom_path" | cut -d' ' -f1)"
test "$sbom_actual" = "$sbom_expected" || fail_prov "SBOM digest mismatch for $sbom_path: expected $sbom_expected, actual $sbom_actual"

cat > "$prov_out" <<EOF
{"schema":"openrouter-model-tracker/provenance-verification/v1","profile":"published","status":"verified","commit":"$commit","tag":"$TAG_VERSION","manifest":"$RELEASE_MANIFEST","signing_key":"$key_used","signing_key_sha256":"$key_sha256","checks":{"public_key_present":"pass","verify_blob":"pass","verify_blob_attestation":"pass","manifest_tag":"pass","manifest_commit":"pass","manifest_version":"pass","artifact_digest":"pass","sbom_digest":"pass"},"native_outputs":["$EVIDENCE_DIR/verify-blob.txt","$EVIDENCE_DIR/verify-blob-attestation.txt"]}
EOF

# Existing consumer schema: cmd/evidencecheck --published-evidence already
# validates openrouter-model-tracker/published-evidence/v1 (see
# cmd/evidencecheck/main.go) — write to that exact shape instead of
# inventing a parallel one. The Makefile invokes evidencecheck against this
# file right after this script exits.
checksum_file="$EVIDENCE_DIR/openrouter.sha256"
test -s "$checksum_file" || fail_prov "checksum evidence is missing or empty: $checksum_file (run make checksums first)"

cat > "$PUBLISHED_EVIDENCE" <<EOF
{"schema":"openrouter-model-tracker/published-evidence/v1","version":"$manifest_version","tag":"$manifest_tag","commit":"$manifest_commit","artifact":"$artifact_path","digest":"$artifact_actual","source":"https://github.com/$GITHUB_REPOSITORY/releases/tag/$manifest_tag","checksum":"$checksum_file","signature":"$RELEASE_MANIFEST_SIG","provenance":"$RELEASE_MANIFEST_ATT"}
EOF

echo "verified: release provenance for $TAG_VERSION at $commit (key: $key_used); published evidence written to $PUBLISHED_EVIDENCE"

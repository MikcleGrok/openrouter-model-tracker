package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRequiresAllArguments(t *testing.T) {
	if err := validate("", "checksum", "artifact", "v1.2.3", "commit", "1.2.3"); err == nil {
		t.Fatal("validate() accepted missing manifest path")
	}
}

func TestValidateChecksExactIdentityAndDigest(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "bin", "openrouter")
	checksumPath := filepath.Join(dir, ".release", "openrouter.sha256")
	manifestPath := filepath.Join(dir, ".release", "manifest.json")
	artifact := []byte("local artifact")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, artifact, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(artifact)
	digestText := hex.EncodeToString(digest[:])
	if err := os.WriteFile(checksumPath, []byte(digestText+"  bin/openrouter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"version":"1.2.3","tag":"v1.2.3","commit":"0123456789012345678901234567890123456789","artifact":"bin/openrouter","digest":"%s"}
`, digestText)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validate(manifestPath, checksumPath, artifactPath, "v1.2.3", "0123456789012345678901234567890123456789", "1.2.3"); err != nil {
		t.Fatalf("validate() rejected valid evidence: %v", err)
	}
	if err := validate(manifestPath, checksumPath, artifactPath, "v1.2.4", "0123456789012345678901234567890123456789", "1.2.4"); err == nil {
		t.Fatal("validate() accepted evidence for a different release")
	}
	if err := validate(manifestPath, checksumPath, filepath.Join(dir, "other"), "v1.2.3", "0123456789012345678901234567890123456789", "1.2.3"); err == nil {
		t.Fatal("validate() accepted an artifact outside the manifest path")
	}
}

func writePublishedEvidenceFixture(t *testing.T, dir string, digest string) string {
	t.Helper()
	// path layout mirrors what release-check/verify-provenance actually
	// produce: <root>/.release/published-evidence.json referencing sibling
	// evidence files under the same .release/ directory.
	evidenceDir := filepath.Join(dir, ".release")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	checksumPath := filepath.Join(evidenceDir, "openrouter.sha256")
	if err := os.WriteFile(checksumPath, []byte(digest+"  bin/openrouter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"release-manifest.json.sig.bundle.json", "release-manifest.json.att.bundle.json"} {
		if err := os.WriteFile(filepath.Join(evidenceDir, name), []byte("bundle"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(evidenceDir, "published-evidence.json")
	data := fmt.Sprintf(`{"schema":"openrouter-model-tracker/published-evidence/v1","version":"1.2.3","tag":"v1.2.3","commit":"0123456789012345678901234567890123456789","artifact":"bin/openrouter","digest":"%s","source":"https://github.com/MikcleGrok/openrouter-model-tracker/releases/tag/v1.2.3","checksum":".release/openrouter.sha256","signature":".release/release-manifest.json.sig.bundle.json","provenance":".release/release-manifest.json.att.bundle.json"}`, digest)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidatePublishedAcceptsSelfConsistentEvidence(t *testing.T) {
	dir := t.TempDir()
	digest := "0123456789012345678901234567890123456789012345678901234567890123"[:64]
	path := writePublishedEvidenceFixture(t, dir, digest)
	if err := validatePublished(path, "v1.2.3", "0123456789012345678901234567890123456789", "1.2.3"); err != nil {
		t.Fatalf("validatePublished() rejected valid evidence: %v", err)
	}
}

func TestValidatePublishedRejectsIdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	digest := "0123456789012345678901234567890123456789012345678901234567890123"[:64]
	path := writePublishedEvidenceFixture(t, dir, digest)
	if err := validatePublished(path, "v1.2.4", "0123456789012345678901234567890123456789", "1.2.4"); err == nil {
		t.Fatal("validatePublished() accepted evidence for a different release")
	}
}

func TestValidatePublishedRejectsDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	digest := "0123456789012345678901234567890123456789012345678901234567890123"[:64]
	path := writePublishedEvidenceFixture(t, dir, digest)
	corrupted := strings.Replace(string(mustReadFile(t, path)), digest, strings.Repeat("f", 64), 1)
	if err := os.WriteFile(path, []byte(corrupted), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePublished(path, "v1.2.3", "0123456789012345678901234567890123456789", "1.2.3"); err == nil {
		t.Fatal("validatePublished() accepted evidence with a corrupted digest field")
	}
}

func TestValidatePublishedRejectsMissingReferencedFile(t *testing.T) {
	dir := t.TempDir()
	digest := "0123456789012345678901234567890123456789012345678901234567890123"[:64]
	path := writePublishedEvidenceFixture(t, dir, digest)
	if err := os.Remove(filepath.Join(dir, ".release", "release-manifest.json.sig.bundle.json")); err != nil {
		t.Fatal(err)
	}
	if err := validatePublished(path, "v1.2.3", "0123456789012345678901234567890123456789", "1.2.3"); err == nil {
		t.Fatal("validatePublished() accepted evidence referencing a missing signature file")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRequiresAllArguments(t *testing.T) {
	if err := validate("", "checksum", "artifact", "v1.2.3", "commit", "1.2.3"); err == nil {
		t.Fatal("validate() accepted missing manifest path")
	}
}

func TestValidateChecksExactIdentityAndDigest(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "openrouter")
	checksumPath := filepath.Join(dir, "openrouter.sha256")
	manifestPath := filepath.Join(dir, "manifest.json")
	artifact := []byte("local artifact")
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
}

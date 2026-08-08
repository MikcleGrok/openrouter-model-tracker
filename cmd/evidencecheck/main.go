package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type manifest struct {
	Version  string `json:"version"`
	Tag      string `json:"tag"`
	Commit   string `json:"commit"`
	Artifact string `json:"artifact"`
	Digest   string `json:"digest"`
}

type publishedEvidence struct {
	Schema     string `json:"schema"`
	Version    string `json:"version"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
	Artifact   string `json:"artifact"`
	Digest     string `json:"digest"`
	Source     string `json:"source"`
	Checksum   string `json:"checksum"`
	Signature  string `json:"signature"`
	Provenance string `json:"provenance"`
}

func main() {
	manifestPath := flag.String("manifest", "", "manifest path")
	checksumPath := flag.String("checksum", "", "checksum path")
	artifactPath := flag.String("artifact", "", "artifact path")
	tag := flag.String("tag", "", "expected exact tag")
	commit := flag.String("commit", "", "expected commit")
	version := flag.String("version", "", "expected normalized version")
	publishedEvidencePath := flag.String("published-evidence", "", "published evidence path")
	flag.Parse()
	var err error
	if *publishedEvidencePath != "" {
		err = validatePublished(*publishedEvidencePath, *tag, *commit, *version)
	} else {
		err = validate(*manifestPath, *checksumPath, *artifactPath, *tag, *commit, *version)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validate(manifestPath, checksumPath, artifactPath, tag, commit, version string) error {
	if manifestPath == "" || checksumPath == "" || artifactPath == "" || tag == "" || commit == "" || version == "" {
		return errors.New("all evidencecheck arguments are required")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("manifest is not valid JSON: %w", err)
	}
	expectedKeys := map[string]bool{"version": true, "tag": true, "commit": true, "artifact": true, "digest": true}
	if len(raw) != len(expectedKeys) {
		return errors.New("manifest schema has missing or extra fields")
	}
	for key := range raw {
		if !expectedKeys[key] {
			return fmt.Errorf("manifest has unknown field %q", key)
		}
	}
	var got manifest
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if got.Version != version || got.Tag != tag || got.Commit != commit || got.Artifact != "bin/openrouter" {
		return errors.New("manifest identity does not match the exact tag, commit, version, or artifact")
	}
	if !commitPattern.MatchString(got.Commit) || !shaPattern.MatchString(got.Digest) {
		return errors.New("manifest commit or digest has invalid schema")
	}
	root := filepath.Dir(filepath.Dir(manifestPath))
	expectedArtifact, err := filepath.Abs(filepath.Join(root, got.Artifact))
	if err != nil {
		return fmt.Errorf("resolve manifest artifact: %w", err)
	}
	actualArtifact, err := filepath.Abs(artifactPath)
	if err != nil || filepath.Clean(actualArtifact) != filepath.Clean(expectedArtifact) {
		return errors.New("artifact path does not match manifest artifact identity")
	}
	expectedChecksum, err := filepath.Abs(filepath.Join(root, ".release", "openrouter.sha256"))
	if err != nil {
		return fmt.Errorf("resolve manifest checksum: %w", err)
	}
	actualChecksum, err := filepath.Abs(checksumPath)
	if err != nil || filepath.Clean(actualChecksum) != filepath.Clean(expectedChecksum) {
		return errors.New("checksum path does not match the local release evidence")
	}
	checksum, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	parts := strings.Split(strings.TrimSuffix(string(checksum), "\n"), "  ")
	if len(parts) != 2 || parts[1] != "bin/openrouter" || !shaPattern.MatchString(parts[0]) || parts[0] != got.Digest {
		return errors.New("checksum evidence has invalid schema or does not match manifest")
	}
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	digest := sha256.Sum256(artifact)
	if hex.EncodeToString(digest[:]) != got.Digest {
		return errors.New("artifact digest does not match manifest")
	}
	return nil
}

// validatePublished checks the published-evidence.json written by
// scripts/verify-provenance.sh after it has already run the real
// cryptographic checks (cosign verify-blob / verify-blob-attestation
// against the committed cosign.pub) and the manifest content cross-checks
// (tag/commit/version, artifact digest, SBOM digest) — see
// ~/projects/tools/guide-tools/.task/go-guide-compliance/signing-design.md
// §5.5/§7. This function does not itself perform cryptographic
// verification (evidencecheck has no cosign dependency); it is the
// content-shape gate that confirms the evidence the shell script wrote is
// self-consistent and matches the exact release under test, the same role
// validate() plays for the plain local manifest above.
func validatePublished(path, tag, commit, version string) error {
	if path == "" || tag == "" || commit == "" || version == "" {
		return errors.New("all evidencecheck arguments are required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read published evidence: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("published evidence is not valid JSON: %w", err)
	}
	expectedKeys := map[string]bool{
		"schema": true, "version": true, "tag": true, "commit": true, "artifact": true,
		"digest": true, "source": true, "checksum": true, "signature": true, "provenance": true,
	}
	if len(raw) != len(expectedKeys) {
		return errors.New("published evidence schema has missing or extra fields")
	}
	for key := range raw {
		if !expectedKeys[key] {
			return fmt.Errorf("published evidence has unknown field %q", key)
		}
	}
	var got publishedEvidence
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("decode published evidence: %w", err)
	}
	if got.Schema != "openrouter-model-tracker/published-evidence/v1" {
		return fmt.Errorf("published evidence has unexpected schema %q", got.Schema)
	}
	if got.Version != version || got.Tag != tag || got.Commit != commit || got.Artifact != "bin/openrouter" {
		return errors.New("published evidence identity does not match the exact tag, commit, version, or artifact")
	}
	if !commitPattern.MatchString(got.Commit) || !shaPattern.MatchString(got.Digest) {
		return errors.New("published evidence commit or digest has invalid schema")
	}
	if strings.TrimSpace(got.Source) == "" {
		return errors.New("published evidence source must not be empty")
	}
	root := filepath.Dir(filepath.Dir(path))
	for _, ref := range []struct{ name, value string }{
		{"checksum", got.Checksum}, {"signature", got.Signature}, {"provenance", got.Provenance},
	} {
		if strings.TrimSpace(ref.value) == "" {
			return fmt.Errorf("published evidence %s must not be empty", ref.name)
		}
		info, err := os.Stat(filepath.Join(root, ref.value))
		if err != nil {
			return fmt.Errorf("published evidence %s references a missing file: %w", ref.name, err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("published evidence %s references an empty file: %s", ref.name, ref.value)
		}
	}
	checksumData, err := os.ReadFile(filepath.Join(root, got.Checksum))
	if err != nil {
		return fmt.Errorf("read published evidence checksum: %w", err)
	}
	parts := strings.Split(strings.TrimSuffix(string(checksumData), "\n"), "  ")
	if len(parts) != 2 || parts[1] != "bin/openrouter" || !shaPattern.MatchString(parts[0]) || parts[0] != got.Digest {
		return errors.New("published evidence checksum does not match the declared digest")
	}
	return nil
}

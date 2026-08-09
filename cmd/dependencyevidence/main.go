package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type evidence struct {
	Schema         string                      `json:"schema"`
	GeneratedAt    string                      `json:"generated_at"`
	Commit         string                      `json:"commit"`
	InputDigest    string                      `json:"input_digest"`
	ScanStatus     string                      `json:"scan_status"`
	Findings       []finding                   `json:"findings"`
	PolicyDecision string                      `json:"policy_decision"`
	Tools          map[string]tool             `json:"tools"`
	Database       map[string]databaseEvidence `json:"database"`
	NativeOutputs  map[string]outputEvidence   `json:"native_outputs"`
}

type finding struct {
	Scanner string `json:"scanner"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

type tool struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type databaseEvidence struct {
	Source string `json:"source"`
}

type outputEvidence struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func main() {
	commit := flag.String("commit", "", "repository commit")
	inputDigest := flag.String("input-digest", "", "dependency input digest")
	govulnStatus := flag.String("govuln-status", "", "govulncheck status")
	modStatus := flag.String("mod-status", "", "go mod verify status")
	govulnVersion := flag.String("govuln-version", "", "govulncheck version")
	osvStatus := flag.String("osv-status", "", "osv-scanner status")
	osvVersion := flag.String("osv-version", "", "osv-scanner version")
	db := flag.String("database", "", "scanner database metadata")
	govulnOutput := flag.String("govuln-output", "", "govulncheck native output")
	osvOutput := flag.String("osv-output", "", "osv-scanner native output")
	output := flag.String("output", "", "evidence output")
	flag.Parse()
	if *output == "" || *commit == "" || *inputDigest == "" {
		fatal("output, commit, and input-digest are required")
	}
	statuses := map[string]string{"go-mod-verify": *modStatus, "govulncheck": *govulnStatus, "osv-scanner": *osvStatus}
	overall, policy := classify(statuses)
	scanners := make([]string, 0, len(statuses))
	for scanner := range statuses {
		scanners = append(scanners, scanner)
	}
	sort.Strings(scanners)
	findings := make([]finding, 0, len(statuses))
	for _, scanner := range scanners {
		status := statuses[scanner]
		findings = append(findings, finding{Scanner: scanner, Status: status, Detail: statusDetail(status)})
	}
	nativeOutputs := map[string]outputEvidence{}
	for scanner, path := range map[string]string{"govulncheck": *govulnOutput, "osv-scanner": *osvOutput} {
		metadata, err := inspectOutput(path)
		if err != nil {
			fatal(fmt.Sprintf("inspect %s output: %v", scanner, err))
		}
		nativeOutputs[scanner] = metadata
	}
	got := evidence{Schema: "openrouter-model-tracker/dependency-evidence/v2", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Commit: *commit, InputDigest: *inputDigest, ScanStatus: overall, Findings: findings, PolicyDecision: policy, Tools: map[string]tool{"govulncheck": {Status: *govulnStatus, Version: *govulnVersion}, "osv-scanner": {Status: *osvStatus, Version: *osvVersion}}, Database: map[string]databaseEvidence{"govulncheck": {Source: *db}, "osv-scanner": {Source: *db}}, NativeOutputs: nativeOutputs}
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0o600); err != nil {
		fatal(err.Error())
	}
	if overall != "passed" {
		fmt.Fprintf(os.Stderr, "dependency-check policy decision: %s (scan_status=%s); non-passed scanners:\n", policy, overall)
		for _, f := range findings {
			if f.Status != "passed" {
				fmt.Fprintf(os.Stderr, "  - %s: %s (%s)\n", f.Scanner, f.Status, f.Detail)
			}
		}
		fmt.Fprintf(os.Stderr, "see %s and the native scanner outputs for full detail.\n", *output)
		os.Exit(1)
	}
}

func inspectOutput(path string) (outputEvidence, error) {
	if path == "" {
		return outputEvidence{}, fmt.Errorf("path is required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return outputEvidence{}, err
	}
	digest := sha256.Sum256(b)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return outputEvidence{}, err
	}
	return outputEvidence{Path: filepath.Clean(absolute), Bytes: int64(len(b)), SHA256: hex.EncodeToString(digest[:])}, nil
}

func classify(statuses map[string]string) (string, string) {
	overall, policy := "passed", "allow"
	passed, failed := 0, 0
	for _, status := range statuses {
		if status == "passed" {
			passed++
			continue
		}
		failed++
		if status == "blocked" {
			overall, policy = "blocked", "blocked"
		} else {
			if overall != "blocked" {
				overall = "error"
			}
			policy = "deny"
		}
	}
	if passed > 0 && failed > 0 && overall != "blocked" {
		overall = "partial"
	}
	return overall, policy
}

func statusDetail(status string) string {
	switch status {
	case "passed":
		return "scanner completed successfully"
	case "blocked":
		return "scanner executable or required database is unavailable"
	case "partial":
		return "scanner produced incomplete evidence"
	default:
		return "scanner returned an error"
	}
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(2) }

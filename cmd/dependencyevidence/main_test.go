package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyDependencyStatuses(t *testing.T) {
	tests := []struct {
		name               string
		statuses           map[string]string
		scanStatus, policy string
	}{
		{name: "clean", statuses: map[string]string{"a": "clean", "b": "clean"}, scanStatus: "clean", policy: "allow"},
		{name: "findings", statuses: map[string]string{"a": "findings", "b": "clean"}, scanStatus: "findings", policy: "deny"},
		{name: "error", statuses: map[string]string{"a": "error", "b": "error"}, scanStatus: "error", policy: "deny"},
		{name: "partial", statuses: map[string]string{"a": "error", "b": "clean"}, scanStatus: "partial", policy: "deny"},
		{name: "error-outranks-findings", statuses: map[string]string{"a": "error", "b": "findings"}, scanStatus: "partial", policy: "deny"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanStatus, policy := classify(tt.statuses)
			if scanStatus != tt.scanStatus || policy != tt.policy {
				t.Fatalf("classify() = %q, %q; want %q, %q", scanStatus, policy, tt.scanStatus, tt.policy)
			}
		})
	}
}

func TestInspectOutputRecordsVerifiableFileEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scanner.txt")
	if err := os.WriteFile(path, []byte("scanner output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inspectOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != path || got.Bytes != 15 || len(got.SHA256) != 64 {
		t.Fatalf("inspectOutput() = %+v", got)
	}
}

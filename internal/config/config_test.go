package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "data_dir: /Users/sergey/projects/openrouter-model-tracker\n" +
		"default_output: /Users/sergey/projects/bobash/docs/openrouter-model-comparison.md\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DataDir != "/Users/sergey/projects/openrouter-model-tracker" {
		t.Errorf("DataDir = %q", got.DataDir)
	}
	if got.DefaultOutput != "/Users/sergey/projects/bobash/docs/openrouter-model-comparison.md" {
		t.Errorf("DefaultOutput = %q", got.DefaultOutput)
	}
}

func TestLoadMissingFileIsZeroConfig(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load on a missing file returned %v, want nil error", err)
	}
	if got.DataDir != "" || got.DefaultOutput != "" {
		t.Errorf("got %+v, want the zero Config", got)
	}
}

func TestLoadBadYAMLIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("data_dir: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for malformed YAML, want an error")
	}
}

func TestDefaultPath(t *testing.T) {
	got := DefaultPath()
	if !strings.HasSuffix(got, filepath.Join(".config", "openrouter", "config.yaml")) {
		t.Errorf("DefaultPath = %q, want it to end with .config/openrouter/config.yaml", got)
	}
}

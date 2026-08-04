package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestResolveOptionsFromConfig(t *testing.T) {
	cfg := writeConfig(t, "data_dir: /data\ndefault_output: /out/doc.md\n")

	opts, err := resolveOptions(cfg, "", "")
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	if opts.DataDir != "/data" || opts.OutputPath != "/out/doc.md" {
		t.Errorf("opts = %+v, want the config values", opts)
	}
}

func TestResolveOptionsFlagsOverrideConfig(t *testing.T) {
	cfg := writeConfig(t, "data_dir: /data\ndefault_output: /out/doc.md\n")

	opts, err := resolveOptions(cfg, "/flag-data", "/flag/doc.md")
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	if opts.DataDir != "/flag-data" || opts.OutputPath != "/flag/doc.md" {
		t.Errorf("opts = %+v, want the flag values to win over the config", opts)
	}
}

func TestResolveOptionsNeedsBothValues(t *testing.T) {
	empty := writeConfig(t, "")

	if _, err := resolveOptions(empty, "", "/out/doc.md"); err == nil || !strings.Contains(err.Error(), "data_dir") {
		t.Errorf("err = %v, want it to name data_dir", err)
	}
	if _, err := resolveOptions(empty, "/data", ""); err == nil || !strings.Contains(err.Error(), "default_output") {
		t.Errorf("err = %v, want it to name default_output", err)
	}
}

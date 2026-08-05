package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v\noutput: %s", args, err, output.String())
	}
	return output.String()
}

func TestVersionFlag(t *testing.T) {
	if got := executeCLI(t, "--version"); got != "openrouter version dev\n" {
		t.Errorf("--version = %q, want %q", got, "openrouter version dev\\n")
	}
}

func TestRootHelpIncludesVersion(t *testing.T) {
	output := executeCLI(t, "--help")
	if !strings.Contains(output, "Version: dev") || !strings.Contains(output, "--version") || !strings.Contains(output, "version for openrouter") {
		t.Errorf("root help does not describe --version:\n%s", output)
	}
}

func TestReleaseLikeVersionInjection(t *testing.T) {
	original := version
	defer func() { version = original }()
	version = "v0.1.0"

	if got := executeCLI(t, "--version"); got != "openrouter version v0.1.0\n" {
		t.Errorf("release-like --version = %q, want %q", got, "openrouter version v0.1.0\\n")
	}
	if got := executeCLI(t, "version"); got != "openrouter v0.1.0\n" {
		t.Errorf("release-like version = %q, want %q", got, "openrouter v0.1.0\\n")
	}
	if output := executeCLI(t, "--help"); !strings.Contains(output, "Version: v0.1.0") {
		t.Errorf("release-like root help does not show the version:\n%s", output)
	}
}

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

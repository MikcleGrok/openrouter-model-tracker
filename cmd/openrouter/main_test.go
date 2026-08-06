package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
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

func executeCLIError(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestVersionFlag(t *testing.T) {
	if got := executeCLI(t, "--version"); got != "openrouter version dev\n" {
		t.Errorf("--version = %q, want %q", got, "openrouter version dev\\n")
	}
}

func TestRootHelpIncludesVersion(t *testing.T) {
	output := executeCLI(t, "--help")
	for _, want := range []string{
		"Fetch fresh data and overwrite the document",
		"Show price history",
		"path to config.yaml",
		"project directory with model-map.tsv",
		"version for openrouter",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("root help does not contain %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"Пересобирает", "Собрать свежие данные", "Показать историю цен", "путь к config.yaml"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("root help contains old Russian text %q:\n%s", unwanted, output)
		}
	}
	if !strings.Contains(output, "Version: dev") || !strings.Contains(output, "--version") {
		t.Errorf("root help does not describe --version:\n%s", output)
	}
}

func TestSubcommandHelpIsEnglish(t *testing.T) {
	tests := []struct {
		command string
		wanted  []string
		old     []string
	}{
		{command: "refresh", wanted: []string{"Fetch fresh data and overwrite the document", "path to generated markdown", "write nothing"}, old: []string{"Собрать свежие данные", "путь генерируемого markdown", "ничего не писать"}},
		{command: "check", wanted: []string{"Report only: new candidates, removed slugs, and notes.yaml gaps; write nothing", "path to generated markdown"}, old: []string{"Только отчёт", "путь генерируемого markdown"}},
		{command: "history", wanted: []string{"Show price history", "filter by slug", "show observations after RFC3339", "format: markdown or tsv"}, old: []string{"Показать историю цен", "фильтр по slug", "показывать наблюдения после RFC3339", "формат: markdown или tsv"}},
		{command: "version", wanted: []string{"Show the binary version"}, old: []string{"Показать версию бинарника"}},
		{command: "init", wanted: []string{"Create a user config and local cache directory"}, old: []string{"Создать"}},
		{command: "tui", wanted: []string{"Browse local model data in an interactive terminal table", "--refresh-interval", "automatic live refresh interval"}},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			output := executeCLI(t, tt.command, "--help")
			for _, want := range tt.wanted {
				if !strings.Contains(output, want) {
					t.Errorf("%s help does not contain %q:\n%s", tt.command, want, output)
				}
			}
			for _, unwanted := range tt.old {
				if strings.Contains(output, unwanted) {
					t.Errorf("%s help contains old Russian text %q:\n%s", tt.command, unwanted, output)
				}
			}
		})
	}
}

func TestRefreshAliasesPreserveRefreshCommand(t *testing.T) {
	root := newRootCmd()
	refresh, _, err := root.Find([]string{"refresh"})
	if err != nil {
		t.Fatalf("find refresh: %v", err)
	}
	if refresh.Use != "refresh" || !reflect.DeepEqual(refresh.Aliases, []string{"update", "up"}) {
		t.Fatalf("refresh command = use %q, aliases %v", refresh.Use, refresh.Aliases)
	}
}

func TestInitCreatesConfigAndCache(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "nested", "config.yaml")
	dataDir := filepath.Join(root, "project")

	output := executeCLI(t, "init", "--config", configPath, "--data-dir", dataDir)
	if !strings.Contains(output, "Created: "+configPath) || !strings.Contains(output, "Created: "+filepath.Join(dataDir, "cache")) {
		t.Fatalf("init output = %q", output)
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(body) != "# User configuration for openrouter. Relative paths are resolved from the current directory.\ndata_dir: "+dataDir+"\ndefault_output: docs/openrouter-model-comparison.md\n" {
		t.Errorf("config = %q", body)
	}
	if info, err := os.Stat(filepath.Join(dataDir, "cache")); err != nil || !info.IsDir() {
		t.Fatalf("cache directory stat = %v, info = %+v", err, info)
	}
}

func TestInitIsIdempotentAndDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("custom: value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := executeCLI(t, "init", "--config", configPath, "--data-dir", dataDir)
	if !strings.Contains(output, "Already exists: "+configPath) || !strings.Contains(output, "Already exists: "+filepath.Join(dataDir, "cache")) {
		t.Fatalf("init output = %q", output)
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "custom: value\n" {
		t.Errorf("config was overwritten: %q", body)
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

func TestResolveTUIOptionsAllowsMinimalConfig(t *testing.T) {
	cfg := writeConfig(t, "data_dir: /data\n")
	opts, err := resolveTUIOptions(cfg, "", "")
	if err != nil {
		t.Fatalf("resolveTUIOptions: %v", err)
	}
	if opts.DataDir != "/data" || opts.OutputPath != "" {
		t.Fatalf("opts = %+v, want data_dir only", opts)
	}
}

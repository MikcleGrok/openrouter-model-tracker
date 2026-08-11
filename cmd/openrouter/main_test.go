package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
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

func TestResolveTUIFilterCLIOverridesSavedValue(t *testing.T) {
	if got := resolveTUIFilter("free", true, "paid", true, "quality>=75"); got != "free" {
		t.Fatalf("explicit TUI filter = %q, want free", got)
	}
	if got := resolveTUIFilter("", false, "paid", true, "quality>=75"); got != "paid" {
		t.Fatalf("saved TUI filter = %q, want paid", got)
	}
	if got := resolveTUIFilter("", false, "", false, "quality>=75"); got != "quality>=75" {
		t.Fatalf("default TUI filter = %q, want quality>=75", got)
	}
	if got := resolveTUIFilter("", false, "", true, "quality>=75"); got != "" {
		t.Fatalf("explicitly cleared TUI filter = %q, want empty", got)
	}
}

func TestResolveTUIFilterFromPersistedConfigFeedsStructuredEditor(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("tui_filter: tier:opus,quality>=75,context>=0,input<=0.00,output<=0.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	filter := resolveTUIFilter("", false, cfg.TUIFilter, cfg.TUIFilterSet, cfg.DefaultFilter)
	m := tuiModel{configPath: configPath, filter: filter, filterFormExplicit: cfg.TUIFilterSet}
	m.openFilterEditor()
	if m.filterDraft.tier != "opus" || m.filterDraft.quality != "75" || m.filterDraft.context != "" || m.filterDraft.input != "" || m.filterDraft.output != "" {
		t.Fatalf("resolved persisted filter draft = %+v", m.filterDraft)
	}
	m, _ = m.applyFilterDraft()
	if m.filter != "tier:opus,quality>=75" {
		t.Fatalf("resolved persisted filter after Enter = %q, want tier:opus,quality>=75", m.filter)
	}
	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.TUIFilter != "tier:opus,quality>=75" {
		t.Fatalf("resolved persisted filter saved after Enter = %q, want tier:opus,quality>=75", saved.TUIFilter)
	}
}

func TestResolveOptionsConfigAndCLIPrecedence(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	body := "data_dir: configured\ndefault_output: configured.md\ncache:\n  dir: shared-cache\n  ttl: 2h\n  request_timeout: 45s\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveOptions(configPath, "override", "override.md")
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	if got.DataDir != "override" || got.OutputPath != "override.md" || got.CacheDir != filepath.Join("override", "shared-cache") {
		t.Fatalf("paths = %+v", got)
	}
	if got.CacheTTL != 2*time.Hour || got.RequestTimeout != 45*time.Second {
		t.Fatalf("durations = %v, %v", got.CacheTTL, got.RequestTimeout)
	}
}

func TestResolveOptionsPreservesExplicitZeroCacheDurations(t *testing.T) {
	configPath := writeConfig(t, "data_dir: /data\ndefault_output: /out.md\ncache:\n  ttl: 0s\n  request_timeout: 0s\n")
	opts, err := resolveOptions(configPath, "", "")
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	if opts.CacheTTL != 0 || !opts.CacheTTLSet || opts.RequestTimeout != 0 || !opts.RequestTimeoutSet {
		t.Fatalf("explicit zero options = %+v", opts)
	}
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

func TestBashCompletion(t *testing.T) {
	output := executeCLI(t, "completion", "bash")
	if strings.TrimSpace(output) == "" {
		t.Fatal("bash completion output is empty")
	}
}

func completionSuggestions(t *testing.T, args ...string) map[string]bool {
	t.Helper()
	output := executeCLI(t, append([]string{"__complete"}, args...)...)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("completion output has no suggestions and directive: %q", output)
	}
	directiveIndex := -1
	for i, line := range lines {
		if strings.HasPrefix(line, ":") {
			directiveIndex = i
			break
		}
	}
	if directiveIndex == -1 {
		t.Fatalf("completion output has no directive line: %q", output)
	}
	suggestions := make(map[string]bool)
	for _, line := range lines[:directiveIndex] {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			t.Fatalf("completion line has unexpected format %q in %q", line, output)
		}
		suggestions[parts[0]] = true
	}
	return suggestions
}

func TestCompletionSuggestions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{""}, want: []string{"table", "tui", "completion"}},
		{name: "tui flags", args: []string{"tui", "--"}, want: []string{"--refresh-interval", "--ranking", "--sort", "--filter", "--limit", "--reverse", "--slug"}},
		{name: "table flags", args: []string{"table", "--"}, want: []string{"--ranking", "--task-fit", "--sort", "--filter", "--limit", "--notes", "--no-pager", "--reverse", "--slug"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completionSuggestions(t, tt.args...)
			for _, want := range tt.want {
				if !got[want] {
					t.Errorf("completion suggestions do not contain %q: %v", want, got)
				}
			}
		})
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
		{command: "tui", wanted: []string{"Browse local model data in an interactive terminal table", "--refresh-interval", "--ranking", "tier-priority", "mixed-utility", "default mixed-utility", "automatic live refresh interval"}},
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
	if !strings.Contains(string(body), "switch_source: [space]") {
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

func TestInitUsesExistingConfigCacheDir(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: data\ncache:\n  dir: custom-cache\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := executeCLI(t, "init", "--config", configPath)
	want := filepath.Join(root, "data", "custom-cache")
	if !strings.Contains(output, "Created: "+want) {
		t.Fatalf("init output = %q, want custom cache path %q", output, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("custom cache directory stat = %v, info = %+v", err, info)
	}
}

func TestReleaseLikeVersionInjection(t *testing.T) {
	original := version
	defer func() { version = original }()
	version = "0.1.0"

	if got := executeCLI(t, "--version"); got != "openrouter version 0.1.0\n" {
		t.Errorf("release-like --version = %q, want %q", got, "openrouter version 0.1.0\\n")
	}
	if got := executeCLI(t, "version"); got != "openrouter 0.1.0\n" {
		t.Errorf("release-like version = %q, want %q", got, "openrouter 0.1.0\\n")
	}
	if output := executeCLI(t, "--help"); !strings.Contains(output, "Version: 0.1.0") {
		t.Errorf("release-like root help does not show the version:\n%s", output)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if !strings.Contains(body, "default_filter:") {
		body += "\ndefault_filter: \"\"\n"
	}
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

func TestResolveOptionsAnchorsRelativeConfigPathsToConfigDirectory(t *testing.T) {
	cfg := writeConfig(t, "data_dir: data\ndefault_output: reports/doc.md\n")

	opts, err := resolveOptions(cfg, "", "")
	if err != nil {
		t.Fatalf("resolveOptions: %v", err)
	}
	wantRoot := filepath.Dir(cfg)
	if opts.DataDir != filepath.Join(wantRoot, "data") || opts.OutputPath != filepath.Join(wantRoot, "reports/doc.md") {
		t.Errorf("opts = %+v, want paths relative to %s", opts, wantRoot)
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

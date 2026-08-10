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
		"default_output: /Users/sergey/projects/bobash/docs/openrouter-model-comparison.md\n" +
		"ranking:\n  mixed_utility:\n    price_weight: 3.5\n"
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
	if got.DefaultFilter != DefaultFilter {
		t.Errorf("DefaultFilter = %q, want %q", got.DefaultFilter, DefaultFilter)
	}
	if got.MixedUtilityPriceWeight() != 3.5 {
		t.Errorf("MixedUtilityPriceWeight = %v", got.MixedUtilityPriceWeight())
	}
}

func TestSaveTUIFilterPreservesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "# keep this comment\ndata_dir: project\nranking:\n  mixed_utility:\n    price_weight: 3.5\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveTUIFilter(path, "paid,quality>=90"); err != nil {
		t.Fatalf("SaveTUIFilter: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TUIFilter != "paid,quality>=90" || got.DataDir != "project" || got.MixedUtilityPriceWeight() != 3.5 {
		t.Fatalf("config after save = %+v", got)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "keep this comment") {
		t.Fatalf("SaveTUIFilter dropped comment: %q", updated)
	}
}

func TestLoadRejectsFormulaAndPriceWeightTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "ranking:\n  mixed_utility:\n    price_weight: 0\n    formula:\n      op: neg\n      args:\n        - var: score\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "formula cannot be combined with price_weight") {
		t.Fatalf("Load error = %v, want clear formula/price_weight conflict", err)
	}
}

func TestMixedUtilityPriceWeightDefaultsWhenMissing(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.MixedUtilityPriceWeight() != DefaultMixedUtilityPriceWeight {
		t.Fatalf("MixedUtilityPriceWeight = %v, want %v", got.MixedUtilityPriceWeight(), DefaultMixedUtilityPriceWeight)
	}
}

func TestLoadPreservesExplicitEmptyTUIFilterAndDefaultFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("default_filter: paid\ntui_filter: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultFilter != "paid" || !got.TUIFilterSet || got.TUIFilter != "" {
		t.Fatalf("config = %+v, want explicit default and empty TUI filter", got)
	}
}

func TestLoadRejectsUnknownTierInFilters(t *testing.T) {
	for _, field := range []string{"default_filter", "tui_filter"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(field+": tier:unknown\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), "unknown tier") || !strings.Contains(err.Error(), "opus, sonnet, haiku, free") {
				t.Fatalf("Load error = %v, want unknown tier and allowed values", err)
			}
		})
	}
}

func TestLoadRejectsInvalidMixedUtilityPriceWeight(t *testing.T) {
	for _, value := range []string{"-1", ".NaN", ".Inf", "-.Inf"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			body := "ranking:\n  mixed_utility:\n    price_weight: " + value + "\n"
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load accepted invalid price weight")
			}
		})
	}
}

func TestLoadMissingFileIsZeroConfig(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load on a missing file returned %v, want nil error", err)
	}
	if got.DataDir != "" || got.DefaultOutput != "" || got.DefaultFilter != DefaultFilter {
		t.Errorf("got %+v, want zero paths and default filter", got)
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

func TestInitRejectsConfigDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(path, t.TempDir()); err == nil || !strings.Contains(err.Error(), "config path is a directory") {
		t.Fatalf("Init error = %v, want config path type error", err)
	}
}

func TestInitPreservesRelativeDataDir(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	configPath := filepath.Join(root, "config.yaml")
	dataDir := filepath.Join("relative", "project")
	if _, err := Init(configPath, dataDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DataDir != dataDir {
		t.Errorf("DataDir = %q, want %q", got.DataDir, dataDir)
	}
	if info, err := os.Stat(filepath.Join(dataDir, "cache")); err != nil || !info.IsDir() {
		t.Fatalf("relative cache directory stat = %v, info = %+v", err, info)
	}
}

func TestInitResolvesRelativeDataDirFromConfigDirectory(t *testing.T) {
	configDir := t.TempDir()
	callerDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.yaml")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(callerDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if _, err := Init(configPath, "data"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if info, err := os.Stat(filepath.Join(configDir, "data", "cache")); err != nil || !info.IsDir() {
		t.Fatalf("config-relative cache stat = %v, info = %+v", err, info)
	}
	if _, err := os.Stat(filepath.Join(callerDir, "data")); !os.IsNotExist(err) {
		t.Fatalf("caller-relative data unexpectedly exists: %v", err)
	}
}

func TestInitRejectsCacheFile(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "cache")
	if err := os.WriteFile(cachePath, []byte("user data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if _, err := Init(configPath, root); err == nil || !strings.Contains(err.Error(), "cache path is not a directory") {
		t.Fatalf("Init error = %v, want cache path type error", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config stat error = %v, want rollback removal", err)
	}
	body, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "user data\n" {
		t.Fatalf("cache file changed: %q", body)
	}
}

func TestInitRollsBackConfigWhenCacheInspectionFails(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.WriteFile(dataDir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if _, err := Init(configPath, dataDir); err == nil {
		t.Fatal("Init returned nil error, want cache inspection error")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config stat error = %v, want rollback removal", err)
	}
	body, err := os.ReadFile(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "not a directory\n" {
		t.Fatalf("data path changed: %q", body)
	}
}

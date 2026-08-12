package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if got.TUISteps != DefaultTUISteps() {
		t.Errorf("TUISteps = %+v, want %+v", got.TUISteps, DefaultTUISteps())
	}
	if got.TUIKeymap["main"]["open_settings"][0] != "o" {
		t.Fatalf("default TUI keymap = %+v", got.TUIKeymap)
	}
	if got.TUIKeymap["main"]["switch_source"][0] != "space" {
		t.Fatalf("default main source binding = %+v", got.TUIKeymap["main"]["switch_source"])
	}
	if got.TUISteps.InputCents != 5 || got.TUISteps.OutputCents != 5 {
		t.Errorf("default price steps = %d/%d cents, want 5/5", got.TUISteps.InputCents, got.TUISteps.OutputCents)
	}
	if got.Cache.EffectiveDir() != DefaultCacheDir || got.Table.Limit != 0 || got.TUI.Limit != 0 {
		t.Errorf("operational defaults = cache %q, table limit %d, tui limit %d", got.Cache.EffectiveDir(), got.Table.Limit, got.TUI.Limit)
	}
	if got.Table.EffectiveNameWidth() != DefaultNameWidth {
		t.Fatalf("default name width = %d, want %d", got.Table.EffectiveNameWidth(), DefaultNameWidth)
	}
	if got.Table.EffectiveIconGap() != 1 {
		t.Fatalf("default icon gap = %d, want 1", got.Table.EffectiveIconGap())
	}
	ttl, _ := got.Cache.EffectiveTTL()
	timeout, _ := got.Cache.EffectiveRequestTimeout()
	interval, _ := got.TUI.EffectiveRefreshInterval()
	if ttl != DefaultCacheTTL || timeout != DefaultRequestTimeout || interval != DefaultTUIRefreshInterval {
		t.Errorf("duration defaults = %v, %v, %v", ttl, timeout, interval)
	}
}

func TestLoadMissingFileUsesDefaultIconGap(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Table.IconGap != 1 || got.Table.EffectiveIconGap() != 1 {
		t.Fatalf("missing-file icon gap = %d/%d, want 1", got.Table.IconGap, got.Table.EffectiveIconGap())
	}
}

func TestLoadEmptyFileUsesDefaultIconGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Table.IconGap != 1 || got.Table.EffectiveIconGap() != 1 {
		t.Fatalf("empty-file icon gap = %d/%d, want 1", got.Table.IconGap, got.Table.EffectiveIconGap())
	}
}

func TestIconGapUsesConfiguredRangeAndFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	for _, test := range []struct {
		value string
		want  int
	}{
		{"0", 0}, {"1", 1}, {"3", 3}, {"8", 8}, {"-1", 1}, {"9", 1}, {"invalid", 1},
	} {
		body := "table:\n  icon_gap: " + test.value + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.Table.EffectiveIconGap() != test.want {
			t.Fatalf("icon gap %q = %d, err %v, want %d", test.value, got.Table.EffectiveIconGap(), err, test.want)
		}
	}
}

func TestLoadIconsUsesDefaultsAndCustomOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "icons:\n  manufacturers:\n    OpenAI: ' 🧩 '\n    Custom Vendor: '🛠️'\n  unknown: '  '\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icons.Icon(" OpenAI Labs ") != "🧩" || got.Icons.Icon("Anthropic") != "🔶" || got.Icons.Icon("Unknown") != "❔" {
		t.Fatalf("icons = %+v", got.Icons)
	}
}

func TestLoadIconsAllowsCustomMetaIcon(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("icons:\n  manufacturers:\n    meta: '🧪'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Icons.Icon("Meta AI") != "🧪" {
		t.Fatalf("custom Meta icon = %q, err %v", got.Icons.Icon("Meta AI"), err)
	}
}

func TestNameWidthUsesConfiguredValueAndFallsBackSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("table:\n  name_width: 72\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Table.EffectiveNameWidth() != 72 {
		t.Fatalf("configured name width = %d, err %v", got.Table.EffectiveNameWidth(), err)
	}
	for _, value := range []string{"0", "-4", "121", "not-a-number"} {
		if err := os.WriteFile(path, []byte("table:\n  name_width: "+value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.Table.EffectiveNameWidth() != DefaultNameWidth {
			t.Fatalf("fallback for %q = %d, err %v", value, got.Table.EffectiveNameWidth(), err)
		}
	}
}

func TestIconPrefersMostSpecificOverlappingManufacturerKey(t *testing.T) {
	icons := IconConfig{Manufacturers: map[string]string{"ai": "AI", "openai": "OpenAI", "beta": "Beta", "alpha": "Alpha"}, Unknown: "Unknown"}
	for range 20 {
		if got := icons.Icon("OpenAI Labs"); got != "OpenAI" {
			t.Fatalf("overlapping manufacturer icon = %q, want OpenAI", got)
		}
		if got := icons.Icon("Alpha Beta vendor"); got != "Alpha" {
			t.Fatalf("equal-length manufacturer tie-break icon = %q, want Alpha", got)
		}
	}
}

func TestIconOverlappingKeysPreserveDefaultsAndFallbacks(t *testing.T) {
	icons := IconConfig{Manufacturers: map[string]string{"ai": "AI", "openai": "OpenAI2", "custom": "Custom"}, Unknown: "Question"}
	if got := icons.Icon("OpenAI"); got != "OpenAI2" {
		t.Fatalf("custom specific manufacturer icon = %q, want OpenAI2", got)
	}
	if got := icons.Icon("Anthropic"); got != "🔶" {
		t.Fatalf("default manufacturer icon = %q, want 🔶", got)
	}
	if got := icons.Icon("Custom Vendor"); got != "Custom" {
		t.Fatalf("custom manufacturer icon = %q, want Custom", got)
	}
	if got := icons.Icon("Unknown Vendor"); got != "Question" {
		t.Fatalf("custom unknown fallback = %q, want Question", got)
	}
}

func TestLoadIconsRejectsMalformedValuesWithoutBreakingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("icons:\n  manufacturers:\n    OpenAI: 'bad\nicon'\n  unknown: ' '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icons.Icon("OpenAI") != "🌀" || got.Icons.Icon("new vendor") != "❔" {
		t.Fatalf("malformed icon fallback = %+v", got.Icons)
	}
}

func TestInitTemplateDocumentsIcons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Init(path, "."); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "manufacturers:") || !strings.Contains(string(body), "meta: 'Ⓜ️'") || !strings.Contains(string(body), "name_width: 40") || !strings.Contains(string(body), "icon_gap: 1") || !strings.Contains(string(body), "unknown: '❔'") {
		t.Fatalf("template does not document icons: %s", body)
	}
}

func TestLoadTUIKeymapSupportsCustomScalarAndListBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "tui_keymap:\n  main:\n    open_settings: ' z '\n    open_details: [enter, d]\n    switch_source: x\n  settings:\n    switch_source: enter\n  columns:\n    toggle: ' space '\n    apply: ['  enter  ']\n  detail:\n    close: ['  esc  ']\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.TUIKeymap["main"]["open_settings"], ",") != "z" || len(got.TUIKeymap["main"]["open_details"]) != 2 || got.TUIKeymap["main"]["switch_source"][0] != "x" || got.TUIKeymap["settings"]["switch_source"][0] != "enter" || got.TUIKeymap["columns"]["toggle"][0] != "space" || got.TUIKeymap["columns"]["apply"][0] != "enter" || got.TUIKeymap["detail"]["close"][0] != "esc" {
		t.Fatalf("custom TUI keymap = %+v", got.TUIKeymap)
	}
}

func TestLoadRejectsInvalidTUIKeymap(t *testing.T) {
	for name, body := range map[string]string{
		"unknown action":        "tui_keymap:\n  main:\n    nope: x\n",
		"unknown context":       "tui_keymap:\n  popup:\n    close: esc\n",
		"empty binding":         "tui_keymap:\n  main:\n    open_settings: [\"\"]\n",
		"conflict":              "tui_keymap:\n  main:\n    open_settings: [x]\n    help: [x]\n",
		"conflict with default": "tui_keymap:\n  main:\n    open_settings: l\n",
		"space alias conflict":  "tui_keymap:\n  columns:\n    toggle: [' ']\n    apply: [space]\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "tui_keymap") {
				t.Fatalf("Load error = %v, want tui_keymap validation error", err)
			}
		})
	}
}

func TestLoadTUIStepsAndRejectsNegativeValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tui_steps: {quality_points: 7, context_tokens: 1000, input_cents: 2, output_cents: 3}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.TUISteps != (TUISteps{QualityPoints: 7, ContextTokens: 1000, InputCents: 2, OutputCents: 3}) {
		t.Fatalf("Load TUISteps = %+v, err %v", got.TUISteps, err)
	}
	if err := os.WriteFile(path, []byte("tui_steps:\n  input: -1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "tui_steps.input must be non-negative") {
		t.Fatalf("Load accepted negative TUI step: %v", err)
	}
}

func TestLoadLegacyTUIStepsPreservesPercentageSemantics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tui_steps: {quality: 7, context: 10, input: 2, output: 3}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.TUISteps.Legacy || got.TUISteps.Quality != 7 || got.TUISteps.Context != 10 || got.TUISteps.Input != 2 || got.TUISteps.Output != 3 {
		t.Fatalf("legacy TUISteps = %+v", got.TUISteps)
	}
}

func TestLoadRejectsMixedLegacyAndNewTUISteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tui_steps: {quality: 7, input_cents: 2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "mixes legacy keys") {
		t.Fatalf("Load error = %v, want clear mixed-schema error", err)
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

func TestSaveTUIFilterEmptyRemovesPersistedOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("data_dir: project\ntui_filter: paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveTUIFilter(path, ""); err != nil {
		t.Fatalf("SaveTUIFilter: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TUIFilterSet || got.TUIFilter != "" {
		t.Fatalf("cleared config = %+v, want no persisted TUI filter", got)
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
	if got.DefaultFilter != "paid" || !got.DefaultFilterSet || !got.TUIFilterSet || got.TUIFilter != "" {
		t.Fatalf("config = %+v, want explicit default and empty TUI filter", got)
	}
}

func TestTUILayoutDefaultsAndSaveReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("default_filter: quality>=75,has-q/p,availability:paid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultFilter != DefaultFilter || got.TUI.Layout != DefaultTUILayout || got.TUI.TopN != DefaultTUITopN {
		t.Fatalf("defaults = filter %q, layout %q, top_n %d", got.DefaultFilter, got.TUI.Layout, got.TUI.TopN)
	}
	if err := SaveTUILayout(path, "top-paid-free", 5); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got.TUI.Layout != "top-paid-free" || got.TUI.TopN != 5 {
		t.Fatalf("saved layout = %+v, err %v", got.TUI, err)
	}
}

func TestLoadAcceptsNewAvailabilityAndQualityPriceFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "default_filter: quality>=75,has-q/p,availability:paid\ntui_filter: has-q/p,availability:any\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.DefaultFilter != "quality>=75,has-q/p,availability:paid" || got.TUIFilter != "has-q/p,availability:any" {
		t.Fatalf("filters = %+v, err %v", got, err)
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

func TestLoadOperationalDefaultsAndCustomValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "cache:\n  dir: ../shared-cache\n  ttl: 2h\n  request_timeout: 45s\ntable:\n  limit: 12\ntui:\n  refresh_interval: 0s\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Cache.EffectiveDir() != "../shared-cache" || got.Table.Limit != 12 || got.TUI.Limit != 0 {
		t.Fatalf("config = %+v", got)
	}
	ttl, _ := got.Cache.EffectiveTTL()
	timeout, _ := got.Cache.EffectiveRequestTimeout()
	interval, _ := got.TUI.EffectiveRefreshInterval()
	if ttl != 2*time.Hour || timeout != 45*time.Second || interval != 0 {
		t.Fatalf("durations = %v, %v, %v", ttl, timeout, interval)
	}
}

func TestLoadPreservesExplicitZeroCacheDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("cache:\n  ttl: 0s\n  request_timeout: 0s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ttl, _ := got.Cache.EffectiveTTL()
	timeout, _ := got.Cache.EffectiveRequestTimeout()
	if !got.Cache.TTLSet || !got.Cache.RequestTimeoutSet || ttl != 0 || timeout != 0 {
		t.Fatalf("explicit zero cache config = %+v, ttl=%v timeout=%v", got.Cache, ttl, timeout)
	}
}

func TestLoadRejectsInvalidOperationalValues(t *testing.T) {
	for _, body := range []string{"cache:\n  ttl: never\n", "cache:\n  request_timeout: -1s\n", "table:\n  limit: -1\n", "tui:\n  limit: -1\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("Load accepted invalid config %q", body)
		}
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

func TestInitUsesExistingRelativeCustomCacheDir(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: data\ncache:\n  dir: snapshots\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(configPath, ""); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "data", "snapshots")); err != nil || !info.IsDir() {
		t.Fatalf("relative custom cache directory stat = %v, info = %+v", err, info)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "cache")); !os.IsNotExist(err) {
		t.Fatalf("default cache directory unexpectedly exists: %v", err)
	}
}

func TestInitUsesExistingAbsoluteCustomCacheDir(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(t.TempDir(), "absolute-cache")
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("data_dir: data\ncache:\n  dir: "+custom+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(configPath, ""); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if info, err := os.Stat(custom); err != nil || !info.IsDir() {
		t.Fatalf("absolute custom cache directory stat = %v, info = %+v", err, info)
	}
}

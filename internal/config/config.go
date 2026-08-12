// Package config reads ~/.config/openrouter/config.yaml, so that a bare
// `openrouter refresh` already knows where the project data lives and where the
// generated document goes. Relative runtime paths are anchored to the config
// file by the command layer.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sboborikin/openrouter-model-tracker/internal/filter"
	"github.com/sboborikin/openrouter-model-tracker/internal/keymap"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"gopkg.in/yaml.v3"
)

// Config is the whole configuration file.
type Config struct {
	DataDir          string        `yaml:"data_dir"`
	DefaultOutput    string        `yaml:"default_output"`
	DefaultFilter    string        `yaml:"default_filter"`
	DefaultFilterSet bool          `yaml:"-"`
	Cache            CacheConfig   `yaml:"cache"`
	Table            TableConfig   `yaml:"table"`
	TUI              TUIConfig     `yaml:"tui"`
	TUIFilter        string        `yaml:"tui_filter"`
	TUIFilterSet     bool          `yaml:"-"`
	TUISteps         TUISteps      `yaml:"tui_steps"`
	TUIKeymap        TUIKeymap     `yaml:"tui_keymap"`
	Ranking          RankingConfig `yaml:"ranking"`
	Icons            IconConfig    `yaml:"icons"`
}

// IconConfig controls manufacturer badges shown by the CLI and TUI.
type IconConfig struct {
	Manufacturers map[string]string `yaml:"manufacturers"`
	Unknown       string            `yaml:"unknown"`
}

var defaultManufacturerIcons = map[string]string{
	"openai": "🌀", "anthropic": "🔶", "google": "🌐", "meta": "Ⓜ️",
	"deepseek": "🐋", "qwen": "🌸", "mistral": "🌪️", "xai": "🚀", "xiaomi": "ⓧ", "nvidia": "Ⓝ",
}

const defaultUnknownIcon = "❔"

func DefaultIconConfig() IconConfig {
	return IconConfig{Manufacturers: cloneStringMap(defaultManufacturerIcons), Unknown: defaultUnknownIcon}
}

func (c IconConfig) WithDefaults() IconConfig {
	defaults := DefaultIconConfig()
	for name, icon := range c.Manufacturers {
		name = normalizeIconName(name)
		if name == "" {
			continue
		}
		if validIcon(icon) {
			defaults.Manufacturers[name] = strings.TrimSpace(icon)
		}
	}
	if validIcon(c.Unknown) {
		defaults.Unknown = strings.TrimSpace(c.Unknown)
	}
	return defaults
}

func (c IconConfig) Icon(name string) string {
	c = c.WithDefaults()
	normalized := normalizeIconName(name)
	match := ""
	icon := c.Unknown
	for candidate, candidateIcon := range c.Manufacturers {
		if !strings.Contains(normalized, candidate) {
			continue
		}
		if len(candidate) > len(match) || (len(candidate) == len(match) && candidate < match) {
			match, icon = candidate, candidateIcon
		}
	}
	return icon
}

func normalizeIconName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func validIcon(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || r == '\x1b' || r < 0x20 })
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// TUIKeymap groups configurable bindings by screen context and action.
type TUIBindings []string

func (b *TUIBindings) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*b = TUIBindings{value.Value}
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("bindings must be a string or list of strings")
	}
	var bindings []string
	if err := value.Decode(&bindings); err != nil {
		return err
	}
	*b = bindings
	return nil
}

type TUIKeymap map[string]map[string]TUIBindings

var defaultTUIKeymap = TUIKeymap{
	"main":     {"open_settings": {"o"}, "open_details": {"enter", "right", "l"}, "close": {"esc", "left", "h"}, "help": {"?"}, "full_help": {"f1"}, "navigate_up": {"up", "k"}, "navigate_down": {"down", "j"}, "switch_source": {"space"}, "cycle_availability": {"p"}, "toggle_layout": {"v"}},
	"settings": {"close": {"esc", "o"}, "navigate_up": {"up", "k"}, "navigate_down": {"down", "j"}, "switch_source": {"space", "enter"}},
	"detail":   {"close": {"esc", "left", "h"}, "navigate_up": {"up", "k"}, "navigate_down": {"down", "j"}},
	"help":     {"close": {"esc", "?"}, "full_help": {"f1"}, "navigate_up": {"up", "k"}, "navigate_down": {"down", "j"}},
	"columns":  {"close": {"esc"}, "navigate_up": {"up", "k"}, "navigate_down": {"down", "j"}, "toggle": {"space"}, "apply": {"enter"}},
	"filter":   {"close": {"esc"}, "navigate_up": {"up", "k"}, "navigate_down": {"down", "j"}, "toggle": {"space"}, "apply": {"enter"}},
}

var tuiKeymapActions = map[string]map[string]bool{
	"main":     {"open_settings": true, "open_details": true, "close": true, "help": true, "full_help": true, "navigate_up": true, "navigate_down": true, "switch_source": true, "cycle_availability": true, "toggle_layout": true},
	"settings": {"close": true, "navigate_up": true, "navigate_down": true, "switch_source": true},
	"detail":   {"close": true, "navigate_up": true, "navigate_down": true},
	"help":     {"close": true, "full_help": true, "navigate_up": true, "navigate_down": true},
	"columns":  {"close": true, "navigate_up": true, "navigate_down": true, "toggle": true, "apply": true},
	"filter":   {"close": true, "navigate_up": true, "navigate_down": true, "toggle": true, "apply": true},
}

func DefaultTUIKeymap() TUIKeymap {
	return cloneTUIKeymap(defaultTUIKeymap)
}

func (k TUIKeymap) WithDefaults() TUIKeymap {
	result := DefaultTUIKeymap()
	for context, actions := range k {
		if _, ok := tuiKeymapActions[context]; !ok {
			continue
		}
		for action, bindings := range actions {
			result[context][action] = canonicalTUIBindings(bindings)
		}
	}
	return result
}

func canonicalTUIBindings(bindings TUIBindings) TUIBindings {
	result := make(TUIBindings, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, keymap.CanonicalBinding(binding))
	}
	return result
}

func cloneTUIKeymap(source TUIKeymap) TUIKeymap {
	result := make(TUIKeymap, len(source))
	for context, actions := range source {
		result[context] = make(map[string]TUIBindings, len(actions))
		for action, bindings := range actions {
			result[context][action] = append([]string(nil), bindings...)
		}
	}
	return result
}

type CacheConfig struct {
	Dir               string `yaml:"dir"`
	TTL               string `yaml:"ttl"`
	RequestTimeout    string `yaml:"request_timeout"`
	TTLSet            bool   `yaml:"-"`
	RequestTimeoutSet bool   `yaml:"-"`
}

type TableConfig struct {
	Sort        string    `yaml:"sort"`
	Ranking     string    `yaml:"ranking"`
	ScoreSource string    `yaml:"score_source"`
	Limit       int       `yaml:"limit"`
	TaskFit     string    `yaml:"task_fit"`
	NameWidth   NameWidth `yaml:"name_width"`
	IconGap     IconGap   `yaml:"icon_gap"`
	IconGaps    IconGaps  `yaml:"icon_gaps"`
}

// IconGap is the number of spaces after the fixed manufacturer icon slot.
type IconGap int

const (
	DefaultIconGap IconGap = 1
	MaxIconGap     IconGap = 8
)

// IconGaps overrides the gap after the fixed icon slot for matching manufacturers.
type IconGaps map[string]IconGap

var defaultIconGaps = map[string]IconGap{}

func DefaultIconGaps() IconGaps {
	result := make(IconGaps, len(defaultIconGaps))
	for name, gap := range defaultIconGaps {
		result[name] = gap
	}
	return result
}

func (g *IconGaps) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		*g = IconGaps{}
		return nil
	}
	result := make(IconGaps, len(value.Content)/2)
	for i := 0; i < len(value.Content); i += 2 {
		name := normalizeIconName(value.Content[i].Value)
		if name == "" {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value.Content[i+1].Value))
		if err != nil {
			result[name] = -1
			continue
		}
		result[name] = IconGap(parsed)
	}
	*g = result
	return nil
}

func (g IconGaps) WithDefaults() IconGaps {
	result := DefaultIconGaps()
	for rawName, gap := range g {
		name := normalizeIconName(rawName)
		if name == "" {
			continue
		}
		if gap < 0 || gap > MaxIconGap {
			delete(result, name)
			continue
		}
		result[name] = gap
	}
	return result
}

func (g IconGaps) EffectiveGap(name string, global int) int {
	normalized := normalizeIconName(name)
	match := ""
	gap := global
	for candidate, candidateGap := range g {
		if !strings.Contains(normalized, candidate) || candidateGap < 0 || candidateGap > MaxIconGap || len(candidate) < len(match) || (len(candidate) == len(match) && candidate >= match) {
			continue
		}
		match, gap = candidate, int(candidateGap)
	}
	return gap
}

func (g *IconGap) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := strconv.Atoi(strings.TrimSpace(value.Value))
	if err != nil || parsed < 0 || parsed > int(MaxIconGap) {
		*g = DefaultIconGap
		return nil
	}
	*g = IconGap(parsed)
	return nil
}

// NameWidth is the preferred display width of the first table/TUI column.
type NameWidth int

const (
	DefaultNameWidth = 40
	MaxNameWidth     = 120
)

func (w *NameWidth) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := strconv.Atoi(strings.TrimSpace(value.Value))
	if err != nil || parsed <= 0 || parsed > MaxNameWidth {
		*w = DefaultNameWidth
		return nil
	}
	*w = NameWidth(parsed)
	return nil
}

func (c TableConfig) EffectiveNameWidth() int {
	if c.NameWidth <= 0 || c.NameWidth > MaxNameWidth {
		return DefaultNameWidth
	}
	return int(c.NameWidth)
}

func (c TableConfig) EffectiveIconGap() int {
	if c.IconGap < 0 || c.IconGap > MaxIconGap {
		return int(DefaultIconGap)
	}
	return int(c.IconGap)
}

func (c TableConfig) EffectiveIconGapFor(name string) int {
	return c.IconGaps.EffectiveGap(name, c.EffectiveIconGap())
}

type TUIConfig struct {
	RefreshInterval string `yaml:"refresh_interval"`
	Sort            string `yaml:"sort"`
	Ranking         string `yaml:"ranking"`
	ScoreSource     string `yaml:"score_source"`
	Limit           int    `yaml:"limit"`
	Layout          string `yaml:"layout"`
	TopN            int    `yaml:"top_n"`
}

// TUISteps contains unit-specific steps used by the structured TUI filter editor.
type TUISteps struct {
	QualityPoints int `yaml:"quality_points"`
	ContextTokens int `yaml:"context_tokens"`
	InputCents    int `yaml:"input_cents"`
	OutputCents   int `yaml:"output_cents"`
	// These fields are accepted for configs written before v1.13.10. They keep
	// their old percentage semantics and are not emitted by init.
	Quality int  `yaml:"quality"`
	Context int  `yaml:"context"`
	Input   int  `yaml:"input"`
	Output  int  `yaml:"output"`
	Legacy  bool `yaml:"-"`
}

type RankingConfig struct {
	MixedUtility ranking.Config `yaml:"mixed_utility"`
}

const DefaultMixedUtilityPriceWeight = ranking.DefaultPriceWeight
const DefaultFilter = "quality>=75,has-q/p,availability:paid"
const DefaultTUILayout = "all"
const DefaultTUITopN = 3
const DefaultCacheDir = "cache"
const DefaultCacheTTL = 12 * time.Hour
const DefaultRequestTimeout = 30 * time.Second
const DefaultTUIRefreshInterval = 5 * time.Minute

const (
	DefaultTUIQualityPoints = 5
	DefaultTUIContextTokens = 8192
	DefaultTUIInputCents    = 5
	DefaultTUIOutputCents   = 5
)

func DefaultTUISteps() TUISteps {
	return TUISteps{QualityPoints: DefaultTUIQualityPoints, ContextTokens: DefaultTUIContextTokens, InputCents: DefaultTUIInputCents, OutputCents: DefaultTUIOutputCents}
}

func (s TUISteps) WithDefaults() TUISteps {
	if s.Legacy || (s.QualityPoints == 0 && s.ContextTokens == 0 && s.InputCents == 0 && s.OutputCents == 0 && (s.Quality != 0 || s.Context != 0 || s.Input != 0 || s.Output != 0)) {
		s.Legacy = true
		if s.Quality == 0 {
			s.Quality = DefaultTUIQualityPoints
		}
		if s.Context == 0 {
			s.Context = 5
		}
		if s.Input == 0 {
			s.Input = 5
		}
		if s.Output == 0 {
			s.Output = 5
		}
		return s
	}
	defaults := DefaultTUISteps()
	if s.QualityPoints == 0 {
		s.QualityPoints = defaults.QualityPoints
	}
	if s.ContextTokens == 0 {
		s.ContextTokens = defaults.ContextTokens
	}
	if s.InputCents == 0 {
		s.InputCents = defaults.InputCents
	}
	if s.OutputCents == 0 {
		s.OutputCents = defaults.OutputCents
	}
	return s
}

func (c CacheConfig) EffectiveDir() string {
	if c.Dir == "" {
		return DefaultCacheDir
	}
	return c.Dir
}

func (c CacheConfig) EffectiveTTL() (time.Duration, error) {
	return configuredDuration(c.TTL, DefaultCacheTTL, "cache.ttl", c.TTLSet)
}

func (c CacheConfig) EffectiveRequestTimeout() (time.Duration, error) {
	return configuredDuration(c.RequestTimeout, DefaultRequestTimeout, "cache.request_timeout", c.RequestTimeoutSet)
}

func (c TUIConfig) EffectiveRefreshInterval() (time.Duration, error) {
	return configuredDuration(c.RefreshInterval, DefaultTUIRefreshInterval, "tui.refresh_interval", false)
}

func configuredDuration(value string, fallback time.Duration, name string, set bool) (time.Duration, error) {
	if !set && value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf("%s must be a non-negative duration such as 5m", name)
	}
	return duration, nil
}

const template = "# User configuration for openrouter. Relative paths are resolved from this config file.\n" +
	"data_dir: .\n" +
	"default_output: docs/openrouter-model-comparison.md\n" +
	"default_filter: quality>=75,has-q/p,availability:paid\n" +
	"icons:\n" +
	"  manufacturers: {openai: '🌀', anthropic: '🔶', google: '🌐', meta: 'Ⓜ️', deepseek: '🐋', qwen: '🌸', mistral: '🌪️', xai: '🚀', xiaomi: 'ⓧ', nvidia: 'Ⓝ'}\n" +
	"  unknown: '❔'\n" +
	"tui_steps: {quality_points: 5, context_tokens: 8192, input_cents: 5, output_cents: 5}\n" +
	"tui_keymap:\n" +
	"  main: {open_settings: [o], open_details: [enter, right, l], help: ['?'], full_help: [f1], navigate_up: [up, k], navigate_down: [down, j], switch_source: [space], cycle_availability: [p], toggle_layout: [v]}\n" +
	"  settings: {close: [esc, o], navigate_up: [up, k], navigate_down: [down, j], switch_source: [space, enter]}\n" +
	"  detail: {close: [esc, left, h], navigate_up: [up, k], navigate_down: [down, j]}\n" +
	"  help: {close: [esc, '?'], full_help: [f1], navigate_up: [up, k], navigate_down: [down, j]}\n" +
	"  columns: {close: [esc], navigate_up: [up, k], navigate_down: [down, j], toggle: [space], apply: [enter]}\n" +
	"  filter: {close: [esc], navigate_up: [up, k], navigate_down: [down, j], toggle: [space], apply: [enter]}\n" +
	"cache:\n" +
	"  dir: cache\n" +
	"  ttl: 12h\n" +
	"  request_timeout: 30s\n" +
	"table:\n" +
	"  sort: q/p\n" +
	"  ranking: mixed\n" +
	"  score_source: swebench\n" +
	"  limit: 0\n" +
	"  task_fit: short\n" +
	"  name_width: 40\n" +
	"  icon_gap: 1\n" +
	"tui:\n" +
	"  refresh_interval: 5m\n" +
	"  sort: q/p\n" +
	"  ranking: mixed\n" +
	"  score_source: swebench\n" +
	"  limit: 0\n" +
	"  layout: all\n" +
	"  top_n: 3\n" +
	"\n" +
	"ranking:\n" +
	"  mixed_utility:\n" +
	"    price_weight: 10\n" +
	"    price:\n" +
	"      input_weight: 3\n" +
	"      output_weight: 1\n" +
	"    tier_factors: {opus: 1, sonnet: 1, haiku: 0.5, free: 0, default: 0}\n" +
	"    # formula and price_weight cannot be used together; see README for the whitelist.\n"

// Init creates the user config and the cache directory without replacing existing paths.
func Init(path, dataDir string) ([]string, error) {
	configExists := false
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, fmt.Errorf("config: config path is a directory: %s", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("config: inspect %s: %w", path, err)
	} else if err == nil {
		configExists = true
	}
	if dataDir == "" {
		dataDir = "."
		if configExists {
			configured, err := existingInitPaths(path)
			if err != nil {
				return nil, err
			}
			if configured.dataDir != "" {
				dataDir = configured.dataDir
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("config: create parent directory: %w", err)
	}
	created := make([]string, 0, 2)
	configCreated := false
	rollbackConfig := func() {
		if configCreated {
			_ = os.Remove(path)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		created = append(created, "Already exists: "+path)
	} else if err != nil {
		return nil, fmt.Errorf("config: create %s: %w", path, err)
	} else {
		configCreated = true
		body, err := configTemplate(dataDir)
		if err != nil {
			file.Close()
			rollbackConfig()
			return nil, err
		}
		if _, err := file.WriteString(body); err != nil {
			file.Close()
			rollbackConfig()
			return nil, fmt.Errorf("config: write %s: %w", path, err)
		}
		if err := file.Close(); err != nil {
			rollbackConfig()
			return nil, fmt.Errorf("config: close %s: %w", path, err)
		}
		created = append(created, "Created: "+path)
	}

	cacheRoot := dataDir
	if !filepath.IsAbs(cacheRoot) {
		cacheRoot = filepath.Join(filepath.Dir(path), cacheRoot)
	}
	cacheDir := DefaultCacheDir
	if configExists {
		configured, err := existingInitPaths(path)
		if err != nil {
			return nil, err
		}
		if configured.cacheDir != "" {
			cacheDir = configured.cacheDir
		}
	}
	cachePath := cacheDir
	if !filepath.IsAbs(cachePath) {
		cachePath = filepath.Join(cacheRoot, cachePath)
	}
	cacheInfo, cacheErr := os.Stat(cachePath)
	if cacheErr != nil && !errors.Is(cacheErr, fs.ErrNotExist) {
		rollbackConfig()
		return nil, fmt.Errorf("config: inspect cache directory: %w", cacheErr)
	}
	if cacheErr == nil {
		if !cacheInfo.IsDir() {
			rollbackConfig()
			return nil, fmt.Errorf("config: cache path is not a directory: %s", cachePath)
		}
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		rollbackConfig()
		return nil, fmt.Errorf("config: create cache directory: %w", err)
	}
	if cacheErr == nil {
		created = append(created, "Already exists: "+cachePath)
	} else {
		created = append(created, "Created: "+cachePath)
	}
	return created, nil
}

type initPaths struct {
	dataDir  string
	cacheDir string
}

func existingInitPaths(path string) (initPaths, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return initPaths{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(b, &document); err != nil {
		return initPaths{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return initPaths{dataDir: yamlMappingValue(document, "data_dir"), cacheDir: yamlNestedMappingValue(document, "cache", "dir")}, nil
}

func configTemplate(dataDir string) (string, error) {
	if dataDir == "." {
		return template, nil
	}
	return strings.Replace(template, "data_dir: .", "data_dir: "+dataDir, 1), nil
}

// Load reads the config. A missing file is not an error: every value it holds
// also has a command-line flag.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{DefaultFilter: DefaultFilter, Table: TableConfig{IconGap: DefaultIconGap, IconGaps: DefaultIconGaps()}, TUISteps: DefaultTUISteps(), TUIKeymap: DefaultTUIKeymap(), Icons: DefaultIconConfig()}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	var c Config
	decoder := yaml.NewDecoder(strings.NewReader(string(b)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil && err != io.EOF {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(b, &document); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	c.TUIFilterSet = yamlMappingHasKey(document, "tui_filter")
	c.DefaultFilterSet = yamlMappingHasKey(document, "default_filter")
	c.Cache.TTLSet = yamlNestedMappingHasKey(document, "cache", "ttl")
	c.Cache.RequestTimeoutSet = yamlNestedMappingHasKey(document, "cache", "request_timeout")
	if !yamlMappingHasKey(document, "default_filter") {
		c.DefaultFilter = DefaultFilter
	}
	for name, value := range map[string]int{"quality_points": c.TUISteps.QualityPoints, "context_tokens": c.TUISteps.ContextTokens, "input_cents": c.TUISteps.InputCents, "output_cents": c.TUISteps.OutputCents, "quality": c.TUISteps.Quality, "context": c.TUISteps.Context, "input": c.TUISteps.Input, "output": c.TUISteps.Output} {
		if value < 0 {
			return Config{}, fmt.Errorf("config: %s: tui_steps.%s must be non-negative", path, name)
		}
	}
	if !yamlNestedMappingHasKey(document, "tui_steps", "quality_points") && !yamlNestedMappingHasKey(document, "tui_steps", "context_tokens") && !yamlNestedMappingHasKey(document, "tui_steps", "input_cents") && !yamlNestedMappingHasKey(document, "tui_steps", "output_cents") && (yamlNestedMappingHasKey(document, "tui_steps", "quality") || yamlNestedMappingHasKey(document, "tui_steps", "context") || yamlNestedMappingHasKey(document, "tui_steps", "input") || yamlNestedMappingHasKey(document, "tui_steps", "output")) {
		c.TUISteps.Legacy = true
	}
	legacySteps := yamlNestedMappingHasKey(document, "tui_steps", "quality") || yamlNestedMappingHasKey(document, "tui_steps", "context") || yamlNestedMappingHasKey(document, "tui_steps", "input") || yamlNestedMappingHasKey(document, "tui_steps", "output")
	newSteps := yamlNestedMappingHasKey(document, "tui_steps", "quality_points") || yamlNestedMappingHasKey(document, "tui_steps", "context_tokens") || yamlNestedMappingHasKey(document, "tui_steps", "input_cents") || yamlNestedMappingHasKey(document, "tui_steps", "output_cents")
	if legacySteps && newSteps {
		return Config{}, fmt.Errorf("config: tui_steps mixes legacy keys (quality/context/input/output) with new keys (quality_points/context_tokens/input_cents/output_cents); choose one schema")
	}
	c.TUISteps = c.TUISteps.WithDefaults()
	c.Icons = c.Icons.WithDefaults()
	c.Table.IconGaps = c.Table.IconGaps.WithDefaults()
	if !yamlNestedMappingHasKey(document, "table", "icon_gap") {
		c.Table.IconGap = DefaultIconGap
	}
	if err := validateTUIKeymap(path, c.TUIKeymap); err != nil {
		return Config{}, err
	}
	c.TUIKeymap = c.TUIKeymap.WithDefaults()
	if err := validateTUIKeymap(path, c.TUIKeymap); err != nil {
		return Config{}, err
	}
	if c.Table.Limit < 0 || c.TUI.Limit < 0 {
		return Config{}, fmt.Errorf("config: table.limit and tui.limit must be non-negative")
	}
	if c.TUI.Layout == "" {
		c.TUI.Layout = DefaultTUILayout
	}
	if c.TUI.TopN == 0 {
		c.TUI.TopN = DefaultTUITopN
	}
	if c.TUI.Layout != "" && c.TUI.Layout != "all" && c.TUI.Layout != "top-paid-free" {
		return Config{}, fmt.Errorf("config: %s: tui.layout must be all or top-paid-free", path)
	}
	if c.TUI.TopN < 0 {
		return Config{}, fmt.Errorf("config: %s: tui.top_n must be non-negative", path)
	}
	if _, err := c.Cache.EffectiveTTL(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if _, err := c.Cache.EffectiveRequestTimeout(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if _, err := c.TUI.EffectiveRefreshInterval(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	for name, value := range map[string]string{"default_filter": c.DefaultFilter, "tui_filter": c.TUIFilter} {
		if err := filter.ValidateTiers(value); err != nil {
			return Config{}, fmt.Errorf("config: %s: invalid %s: %w", path, name, err)
		}
		if err := filter.ValidateAvailability(value); err != nil {
			return Config{}, fmt.Errorf("config: %s: invalid %s: %w", path, name, err)
		}
	}
	if _, err := ranking.Compile(c.Ranking.MixedUtility); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return c, nil
}

func validateTUIKeymap(path string, mappings TUIKeymap) error {
	for context, actions := range mappings {
		allowed, ok := tuiKeymapActions[context]
		if !ok {
			return fmt.Errorf("config: %s: tui_keymap.%s is an unknown context", path, context)
		}
		seen := map[string]string{}
		for action, bindings := range actions {
			if !allowed[action] {
				return fmt.Errorf("config: %s: tui_keymap.%s.%s is an unknown action", path, context, action)
			}
			for _, binding := range bindings {
				binding = keymap.CanonicalBinding(binding)
				if binding == "" {
					return fmt.Errorf("config: %s: tui_keymap.%s.%s contains an empty binding", path, context, action)
				}
				if previous, exists := seen[binding]; exists {
					return fmt.Errorf("config: %s: tui_keymap.%s has conflicting binding %q for actions %s and %s", path, context, binding, previous, action)
				}
				seen[binding] = action
			}
		}
	}
	return nil
}

func yamlMappingHasKey(document yaml.Node, key string) bool {
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return false
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return true
		}
	}
	return false
}

func yamlNestedMappingHasKey(document yaml.Node, parent, key string) bool {
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return false
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != parent || root.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(root.Content[i+1].Content); j += 2 {
			if root.Content[i+1].Content[j].Value == key {
				return true
			}
		}
	}
	return false
}

func yamlMappingValue(document yaml.Node, key string) string {
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return ""
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1].Value
		}
	}
	return ""
}

func yamlNestedMappingValue(document yaml.Node, parent, key string) string {
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return ""
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != parent || root.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(root.Content[i+1].Content); j += 2 {
			if root.Content[i+1].Content[j].Value == key {
				return root.Content[i+1].Content[j+1].Value
			}
		}
	}
	return ""
}

func (c Config) MixedUtilityPriceWeight() float64 {
	if c.Ranking.MixedUtility.PriceWeight == nil {
		return DefaultMixedUtilityPriceWeight
	}
	return *c.Ranking.MixedUtility.PriceWeight
}

func (c Config) CompiledMixedUtility() (ranking.Compiled, error) {
	return ranking.Compile(c.Ranking.MixedUtility)
}

// SaveTUIFilter updates only the persisted TUI filter in the user config.
func SaveTUIFilter(path, filter string) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		body = []byte("{}\n")
	} else if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	if len(document.Content) == 0 {
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("config: %s: root must be a mapping", path)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "tui_filter" {
			if filter == "" {
				root.Content = append(root.Content[:i], root.Content[i+2:]...)
				return writeYAML(path, &document)
			}
			root.Content[i+1].Kind = yaml.ScalarNode
			root.Content[i+1].Tag = "!!str"
			root.Content[i+1].Value = filter
			return writeYAML(path, &document)
		}
	}
	if filter == "" {
		return writeYAML(path, &document)
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tui_filter"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: filter})
	return writeYAML(path, &document)
}

// SaveTUILayout persists the interactive layout settings without rewriting
// unrelated user configuration.
func SaveTUILayout(path, layout string, topN int) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		body = []byte("{}\n")
	} else if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	if len(document.Content) == 0 {
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("config: %s: root must be a mapping", path)
	}
	var tui *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "tui" {
			tui = root.Content[i+1]
			break
		}
	}
	if tui == nil {
		tui = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tui"}, tui)
	}
	for key, value := range map[string]string{"layout": layout, "top_n": strconv.Itoa(topN)} {
		found := false
		for i := 0; i+1 < len(tui.Content); i += 2 {
			if tui.Content[i].Value == key {
				tui.Content[i+1].Kind, tui.Content[i+1].Tag, tui.Content[i+1].Value = yaml.ScalarNode, "!!str", value
				if key == "top_n" {
					tui.Content[i+1].Tag = "!!int"
				}
				found = true
				break
			}
		}
		if !found {
			valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			if key == "top_n" {
				valueNode.Tag = "!!int"
			}
			tui.Content = append(tui.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, valueNode)
		}
	}
	return writeYAML(path, &document)
}

func writeYAML(path string, document *yaml.Node) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create parent directory: %w", err)
	}
	var out strings.Builder
	if err := yaml.NewEncoder(&out).Encode(document); err != nil {
		return fmt.Errorf("config: encode %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.yaml.*")
	if err != nil {
		return fmt.Errorf("config: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("config: chmod temporary file: %w", err)
	}
	if _, err := tmp.WriteString(out.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("config: write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("config: replace %s: %w", path, err)
	}
	return nil
}

// DefaultPath is where the config lives unless --config says otherwise.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "openrouter", "config.yaml")
	}
	return filepath.Join(home, ".config", "openrouter", "config.yaml")
}

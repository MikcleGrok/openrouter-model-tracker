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
	"strings"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/filter"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"gopkg.in/yaml.v3"
)

// Config is the whole configuration file.
type Config struct {
	DataDir       string        `yaml:"data_dir"`
	DefaultOutput string        `yaml:"default_output"`
	DefaultFilter string        `yaml:"default_filter"`
	Cache         CacheConfig   `yaml:"cache"`
	Table         TableConfig   `yaml:"table"`
	TUI           TUIConfig     `yaml:"tui"`
	TUIFilter     string        `yaml:"tui_filter"`
	TUIFilterSet  bool          `yaml:"-"`
	TUISteps      TUISteps      `yaml:"tui_steps"`
	Ranking       RankingConfig `yaml:"ranking"`
}

type CacheConfig struct {
	Dir               string `yaml:"dir"`
	TTL               string `yaml:"ttl"`
	RequestTimeout    string `yaml:"request_timeout"`
	TTLSet            bool   `yaml:"-"`
	RequestTimeoutSet bool   `yaml:"-"`
}

type TableConfig struct {
	Sort        string `yaml:"sort"`
	Ranking     string `yaml:"ranking"`
	ScoreSource string `yaml:"score_source"`
	Limit       int    `yaml:"limit"`
	TaskFit     string `yaml:"task_fit"`
}

type TUIConfig struct {
	RefreshInterval string `yaml:"refresh_interval"`
	Sort            string `yaml:"sort"`
	Ranking         string `yaml:"ranking"`
	ScoreSource     string `yaml:"score_source"`
	Limit           int    `yaml:"limit"`
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
const DefaultFilter = "quality>=75"
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
	"default_filter: quality>=75\n" +
	"tui_steps: {quality_points: 5, context_tokens: 8192, input_cents: 5, output_cents: 5}\n" +
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
	"tui:\n" +
	"  refresh_interval: 5m\n" +
	"  sort: q/p\n" +
	"  ranking: mixed\n" +
	"  score_source: swebench\n" +
	"  limit: 0\n" +
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
		return Config{DefaultFilter: DefaultFilter, TUISteps: DefaultTUISteps()}, nil
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
	if c.Table.Limit < 0 || c.TUI.Limit < 0 {
		return Config{}, fmt.Errorf("config: table.limit and tui.limit must be non-negative")
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
	}
	if _, err := ranking.Compile(c.Ranking.MixedUtility); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return c, nil
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
			root.Content[i+1].Kind = yaml.ScalarNode
			root.Content[i+1].Tag = "!!str"
			root.Content[i+1].Value = filter
			return writeYAML(path, &document)
		}
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tui_filter"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: filter})
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

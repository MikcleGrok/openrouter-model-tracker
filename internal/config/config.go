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

	"github.com/sboborikin/openrouter-model-tracker/internal/filter"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
	"gopkg.in/yaml.v3"
)

// Config is the whole configuration file.
type Config struct {
	DataDir       string        `yaml:"data_dir"`
	DefaultOutput string        `yaml:"default_output"`
	DefaultFilter string        `yaml:"default_filter"`
	TUIFilter     string        `yaml:"tui_filter"`
	TUIFilterSet  bool          `yaml:"-"`
	Ranking       RankingConfig `yaml:"ranking"`
}

type RankingConfig struct {
	MixedUtility ranking.Config `yaml:"mixed_utility"`
}

const DefaultMixedUtilityPriceWeight = ranking.DefaultPriceWeight
const DefaultFilter = "quality>=75"

const template = "# User configuration for openrouter. Relative paths are resolved from this config file.\n" +
	"data_dir: .\n" +
	"default_output: docs/openrouter-model-comparison.md\n" +
	"default_filter: quality>=75\n" +
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
	if dataDir == "" {
		dataDir = "."
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, fmt.Errorf("config: config path is a directory: %s", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("config: inspect %s: %w", path, err)
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
	cachePath := filepath.Join(cacheRoot, "cache")
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

func configTemplate(dataDir string) (string, error) {
	if dataDir == "." {
		return template, nil
	}
	return fmt.Sprintf("# User configuration for openrouter. Relative paths are resolved from this config file.\ndata_dir: %s\ndefault_output: docs/openrouter-model-comparison.md\ndefault_filter: quality>=75\n\nranking:\n  mixed_utility:\n    price_weight: 10\n    price:\n      input_weight: 3\n      output_weight: 1\n    tier_factors:\n      opus: 1\n      sonnet: 1\n      haiku: 0.5\n      free: 0\n      default: 0\n    # formula and price_weight cannot be used together; see README for the whitelist.\n", dataDir), nil
}

// Load reads the config. A missing file is not an error: every value it holds
// also has a command-line flag.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{DefaultFilter: DefaultFilter}, nil
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
	if !yamlMappingHasKey(document, "default_filter") {
		c.DefaultFilter = DefaultFilter
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

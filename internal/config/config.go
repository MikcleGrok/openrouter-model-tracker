// Package config reads ~/.config/openrouter/config.yaml, so that a bare
// `openrouter refresh` already knows where the project data lives and where the
// generated document goes.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the whole configuration file.
type Config struct {
	DataDir       string `yaml:"data_dir"`
	DefaultOutput string `yaml:"default_output"`
}

// Load reads the config. A missing file is not an error: every value it holds
// also has a command-line flag.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return c, nil
}

// DefaultPath is where the config lives unless --config says otherwise.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "openrouter", "config.yaml")
	}
	return filepath.Join(home, ".config", "openrouter", "config.yaml")
}

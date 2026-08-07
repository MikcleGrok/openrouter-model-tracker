// Package config reads ~/.config/openrouter/config.yaml, so that a bare
// `openrouter refresh` already knows where the project data lives and where the
// generated document goes. Relative runtime paths are anchored to the config
// file by the command layer.
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

const template = "# User configuration for openrouter. Relative paths are resolved from this config file.\n" +
	"data_dir: .\n" +
	"default_output: docs/openrouter-model-comparison.md\n"

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
	body, err := yaml.Marshal(Config{DataDir: dataDir, DefaultOutput: "docs/openrouter-model-comparison.md"})
	if err != nil {
		return "", fmt.Errorf("config: encode template: %w", err)
	}
	return "# User configuration for openrouter. Relative paths are resolved from this config file.\n" + string(body), nil
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

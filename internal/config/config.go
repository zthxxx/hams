// Package config handles loading and merging hams configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the merged hams configuration from all levels.
type Config struct {
	// ProfileTag identifies which profile directory to use (e.g., "macOS", "openwrt").
	ProfileTag string `yaml:"profile_tag"`
	// MachineID is a user-defined name for this machine (e.g., "MacbookProM5X").
	MachineID string `yaml:"machine_id"`
	// StoreRepo is the path or GitHub shorthand for the hams store repository.
	StoreRepo string `yaml:"store_repo"`
	// StorePath is the resolved absolute path to the store directory.
	StorePath string `yaml:"store_path"`
	// LLMCLI is the path to the LLM CLI tool for tag/intro enrichment.
	LLMCLI string `yaml:"llm_cli"`
	// ProviderPriority defines the execution order for providers at the same DAG level.
	ProviderPriority []string `yaml:"provider_priority"`
}

// DefaultProviderPriority is the built-in provider execution order.
var DefaultProviderPriority = []string{
	"homebrew", "apt", "pnpm", "npm", "uv", "go", "cargo",
	"vscode-ext", "mas", "git", "defaults", "duti", "bash",
}

// Paths holds the resolved directory paths for hams.
type Paths struct {
	ConfigHome string // HAMS_CONFIG_HOME (~/.config/hams/)
	DataHome   string // HAMS_DATA_HOME (~/.local/share/hams/)
}

// ResolvePaths determines the hams directory paths from environment variables.
func ResolvePaths() Paths {
	configHome := os.Getenv("HAMS_CONFIG_HOME")
	if configHome == "" {
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig == "" {
			home, _ := os.UserHomeDir()
			xdgConfig = filepath.Join(home, ".config")
		}
		configHome = filepath.Join(xdgConfig, "hams")
	}

	dataHome := os.Getenv("HAMS_DATA_HOME")
	if dataHome == "" {
		xdgData := os.Getenv("XDG_DATA_HOME")
		if xdgData == "" {
			home, _ := os.UserHomeDir()
			xdgData = filepath.Join(home, ".local", "share")
		}
		dataHome = filepath.Join(xdgData, "hams")
	}

	return Paths{
		ConfigHome: configHome,
		DataHome:   dataHome,
	}
}

// GlobalConfigPath returns the path to the global hams config file.
func (p Paths) GlobalConfigPath() string {
	return filepath.Join(p.ConfigHome, "hams.config.yaml")
}

// Load reads and merges configuration from all levels:
// 1. Built-in defaults
// 2. Global config (~/.config/hams/hams.config.yaml)
// 3. Project-level config (<store>/hams.config.yaml)
// 4. Local overrides (<store>/hams.config.local.yaml).
func Load(paths Paths, storePath string) (*Config, error) {
	cfg := &Config{
		ProviderPriority: DefaultProviderPriority,
	}

	// Level 1: built-in defaults (already set above).

	// Level 2: global config.
	globalPath := paths.GlobalConfigPath()
	if err := mergeFromFile(cfg, globalPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading global config %s: %w", globalPath, err)
	}

	// Level 3: project-level config.
	if storePath != "" {
		projectPath := filepath.Join(storePath, "hams.config.yaml")
		if err := mergeFromFile(cfg, projectPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading project config %s: %w", projectPath, err)
		}

		// Level 4: local overrides.
		localPath := filepath.Join(storePath, "hams.config.local.yaml")
		if err := mergeFromFile(cfg, localPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading local config %s: %w", localPath, err)
		}
	}

	// Resolve store path.
	if cfg.StorePath == "" && storePath != "" {
		cfg.StorePath = storePath
	}

	return cfg, nil
}

// ProfileDir returns the absolute path to the active profile directory.
func (c *Config) ProfileDir() string {
	tag := c.ProfileTag
	if tag == "" {
		tag = "default"
	}
	return filepath.Join(c.StorePath, tag)
}

// StateDir returns the absolute path to the state directory for this machine.
func (c *Config) StateDir() string {
	id := c.MachineID
	if id == "" {
		id = "unknown"
	}
	return filepath.Join(c.StorePath, ".state", id)
}

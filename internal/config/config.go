// Package config handles loading and merging hams configuration files.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
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
// Names must match provider Manifest().Name (lowercased by registry).
var DefaultProviderPriority = []string{
	"brew", "apt", "pnpm", "npm", "uv", "goinstall", "cargo",
	"code-ext", "mas", "git-config", "git-clone", "defaults", "duti", "bash", "ansible",
}

// Paths holds the resolved directory paths for hams.
type Paths struct {
	ConfigHome     string // HAMS_CONFIG_HOME (~/.config/hams/)
	DataHome       string // HAMS_DATA_HOME (~/.local/share/hams/)
	ConfigFilePath string // Explicit config file path from --config flag (overrides GlobalConfigPath).
}

// ResolvePaths determines the hams directory paths from environment variables.
func ResolvePaths() Paths {
	configHome := os.Getenv("HAMS_CONFIG_HOME")
	if configHome == "" {
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig == "" {
			home, _ := os.UserHomeDir() //nolint:errcheck // best-effort home directory fallback
			xdgConfig = filepath.Join(home, ".config")
		}
		configHome = filepath.Join(xdgConfig, "hams")
	}

	dataHome := os.Getenv("HAMS_DATA_HOME")
	if dataHome == "" {
		xdgData := os.Getenv("XDG_DATA_HOME")
		if xdgData == "" {
			home, _ := os.UserHomeDir() //nolint:errcheck // best-effort home directory fallback
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
// If ConfigFilePath is set (via --config flag), it takes precedence.
func (p Paths) GlobalConfigPath() string {
	if p.ConfigFilePath != "" {
		return p.ConfigFilePath
	}
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

	// Level 2a: global config.
	globalPath := paths.GlobalConfigPath()
	if err := mergeFromFile(cfg, globalPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading global config %s: %w", globalPath, err)
	}

	// Level 2b: global local overrides (for sensitive keys written outside a store context).
	globalLocalPath := filepath.Join(paths.ConfigHome, "hams.config.local.yaml")
	if err := mergeFromFile(cfg, globalLocalPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading global local config %s: %w", globalLocalPath, err)
	}

	// If no explicit storePath but the global config defines one, use it.
	if storePath == "" && cfg.StorePath != "" {
		storePath = cfg.StorePath
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

	if err := cfg.Validate(); err != nil {
		return nil, err
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

// Validate checks that required configuration fields are set.
// Returns nil if the configuration is valid for operations that need a store.
func (c *Config) Validate() error {
	// StorePath is allowed to be empty (not all commands need it).
	// ProfileTag and MachineID have defaults, so just warn if empty.
	if c.StorePath != "" {
		if c.ProfileTag == "" {
			slog.Warn("profile_tag is empty, using 'default'")
		}
		if c.MachineID == "" {
			slog.Warn("machine_id is empty, using 'unknown'")
		}
	}
	return nil
}

// sensitiveKeys are config keys that should be written to .local.yaml files.
var sensitiveKeys = map[string]bool{
	"llm_cli": true,
}

// sensitivePatterns are substrings that mark a key as sensitive.
var sensitivePatterns = []string{
	"token", "secret", "password", "credential",
}

// IsSensitiveKey returns true if the key should be stored in a .local.yaml file.
// Matches exact keys in sensitiveKeys or keys containing sensitive substrings.
func IsSensitiveKey(key string) bool {
	if sensitiveKeys[key] {
		return true
	}
	lower := strings.ToLower(key)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// ValidConfigKeys lists the keys that can be set via `hams config set`.
var ValidConfigKeys = []string{"profile_tag", "machine_id", "store_path", "store_repo", "llm_cli"}

// IsValidConfigKey returns true if the key is a recognized settable config key.
func IsValidConfigKey(key string) bool {
	return slices.Contains(ValidConfigKeys, key)
}

// WriteConfigKey reads the appropriate config file, updates a single key, and writes it back atomically.
// Sensitive keys are written to the store's local config; other keys go to the global config file.
func WriteConfigKey(paths Paths, storePath, key, value string) error {
	var targetPath string
	if IsSensitiveKey(key) {
		slog.Info("sensitive key detected, routing to .local.yaml", "key", key)
		if storePath == "" {
			// Fall back to a global local config next to the global config.
			targetPath = filepath.Join(paths.ConfigHome, "hams.config.local.yaml")
		} else {
			targetPath = filepath.Join(storePath, "hams.config.local.yaml")
		}
	} else {
		targetPath = paths.GlobalConfigPath()
	}

	// Read existing file into a generic map to preserve unknown fields.
	existing := make(map[string]any)
	data, err := os.ReadFile(targetPath) //nolint:gosec // config paths are user-specified
	if err == nil {
		if unmarshalErr := yaml.Unmarshal(data, &existing); unmarshalErr != nil {
			return fmt.Errorf("parsing config %s: %w", targetPath, unmarshalErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading config %s: %w", targetPath, err)
	}

	existing[key] = value

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return hamsfile.AtomicWrite(targetPath, out)
}

// Package config handles loading and merging hams configuration files.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
//
// Precedence for the resolved store path:
//  1. A non-empty `storePath` argument (CLI `--store` flag) ALWAYS wins.
//  2. Otherwise, the global config's `store_path:` is used.
//  3. Otherwise, cfg.StorePath stays empty and the caller surfaces
//     "no store directory configured".
func Load(paths Paths, storePath string) (*Config, error) {
	cfg := &Config{
		ProviderPriority: DefaultProviderPriority,
	}

	// Preserve the caller's explicit --store override so it wins over
	// anything the config files say. Without this capture, a global
	// config with `store_path: /configured` silently beat
	// `hams --store=/alt refresh` — the CLI flag was ignored for
	// every command that read cfg.StorePath instead of flags.Store.
	explicitStoreOverride := storePath

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

	// Level 3: project-level config (rejects machine-scoped fields).
	if storePath != "" {
		projectPath := filepath.Join(storePath, "hams.config.yaml")
		if err := mergeFromStoreFile(cfg, projectPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading project config %s: %w", projectPath, err)
		}

		// Level 4: local overrides (same rejection rule as level 3).
		localPath := filepath.Join(storePath, "hams.config.local.yaml")
		if err := mergeFromStoreFile(cfg, localPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading local overrides %s: %w", localPath, err)
		}
	}

	// Resolve final store path. An explicit --store override always
	// wins; otherwise fall back to whatever the merged config has,
	// or to the derived storePath if the config left it blank.
	switch {
	case explicitStoreOverride != "":
		cfg.StorePath = explicitStoreOverride
	case cfg.StorePath == "" && storePath != "":
		cfg.StorePath = storePath
	}

	// Expand `~` in store_path so config entries like
	// `store_path: ~/Project/hams-store` work the same way users
	// type them on the CLI. Without this, the literal `~` becomes
	// a path component that never matches the real home directory,
	// silently producing "no providers match" on every run.
	if expanded, expErr := expandHome(cfg.StorePath); expErr == nil {
		cfg.StorePath = expanded
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ExpandHome expands a leading `~/` to the current user's home
// directory. Returns the input unchanged when there's no tilde prefix
// or when home resolution fails. Shared with the CLI flag path so
// `--config=~/foo` and `--store=~/bar` expand the same way the
// config-file `store_path: ~/…` does (cycle 85), and `hams
// --config=~/my.yaml` behaves symmetrically (cycle 89).
func ExpandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path, err
	}
	return filepath.Join(home, path[2:]), nil
}

// expandHome is the lowercase alias kept for the existing call in
// Load; ExpandHome is the exported entry point.
func expandHome(path string) (string, error) { return ExpandHome(path) }

// ProfileDir returns the absolute path to the active profile directory.
// Cycle 195: sanitizes ProfileTag so a malicious or mistyped value
// like `../etc` cannot escape StorePath via filepath.Join's clean-up.
func (c *Config) ProfileDir() string {
	tag := sanitizePathSegment(c.ProfileTag, "default")
	return filepath.Join(c.StorePath, tag)
}

// StateDir returns the absolute path to the state directory for this machine.
// Cycle 195: sanitizes MachineID for the same reason — `machine_id: ../..`
// previously wrote state files under StorePath's parent.
func (c *Config) StateDir() string {
	id := sanitizePathSegment(c.MachineID, "unknown")
	return filepath.Join(c.StorePath, ".state", id)
}

// sanitizePathSegment collapses any path traversal / separator chars to
// the fallback. The only valid forms are a bare identifier: letters,
// digits, `.`, `-`, `_`. Empty → fallback. Anything else → fallback.
// Rejection is silent (returns fallback) rather than erroring because
// these are derived fields read at many call sites; validating once at
// config.Load would be cleaner but violates the cycle 92 contract that
// explicit --profile is accepted as-is. The sanitize here is a
// last-defense that prevents the filesystem-escape regardless of how
// the invalid value entered the config.
func sanitizePathSegment(s, fallback string) string {
	if s == "" {
		return fallback
	}
	// Reject path separators (both Unix and Windows conventions).
	if strings.ContainsAny(s, `/\`) {
		return fallback
	}
	// Reject "." and ".." which could collapse via filepath.Clean.
	if s == "." || s == ".." {
		return fallback
	}
	return s
}

// warnOnceProfileTag and warnOnceMachineID dedup the "using default"
// warnings so they fire at most once per process, even when config.Load
// runs multiple times per command (e.g., once during provider registration
// and again when the command action executes).
var (
	warnOnceProfileTag sync.Once
	warnOnceMachineID  sync.Once
)

// Validate checks that required configuration fields are set.
// Returns nil if the configuration is valid for operations that need a store.
func (c *Config) Validate() error {
	// StorePath is allowed to be empty (not all commands need it).
	// ProfileTag and MachineID have defaults, so just warn if empty.
	if c.StorePath != "" {
		if c.ProfileTag == "" {
			warnOnceProfileTag.Do(func() {
				slog.Warn("profile_tag is empty, using 'default'")
			})
		}
		if c.MachineID == "" {
			warnOnceMachineID.Do(func() {
				slog.Warn("machine_id is empty, using 'unknown'")
			})
		}
	}
	return nil
}

// ResetValidationWarnOnce resets the once-guards so tests can exercise the
// warn path repeatedly. Do not call outside tests.
func ResetValidationWarnOnce() {
	warnOnceProfileTag = sync.Once{}
	warnOnceMachineID = sync.Once{}
}

// sensitiveKeys are config keys that should be written to .local.yaml files.
var sensitiveKeys = map[string]bool{
	"llm_cli": true,
}

// sensitivePatterns are substrings that mark a key as sensitive.
// Per schema-design spec §"Sensitive Config Key Detection" — keys
// containing any of these substrings auto-route to hams.config.local.yaml.
// Note on "key": broad by design; any unusual identifier like "monkey_X"
// would also match, but the spec explicitly requires this pattern
// because common API-key key names (api_key, openai_key, etc.) should
// be caught without requiring each integration to pre-register.
var sensitivePatterns = []string{
	"token", "key", "secret", "password", "credential",
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

// ReadRawConfigKey reads a single key from the appropriate config file
// using the same routing as WriteConfigKey: sensitive keys from
// hams.config.local.yaml (store-local or global fallback), non-sensitive
// from the global hams.config.yaml. Returns the value + found=true when
// the key is present, "" + found=false when not. Used by `hams config
// get` to support arbitrary sensitive keys (e.g., notification.bark_token)
// that are not fields on the typed Config struct.
func ReadRawConfigKey(paths Paths, storePath, key string) (value string, found bool, err error) {
	targetPath := paths.GlobalConfigPath()
	if IsSensitiveKey(key) {
		if storePath == "" {
			targetPath = filepath.Join(paths.ConfigHome, "hams.config.local.yaml")
		} else {
			targetPath = filepath.Join(storePath, "hams.config.local.yaml")
		}
	}

	data, err := os.ReadFile(targetPath) //nolint:gosec // config paths are user-specified
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("reading config %s: %w", targetPath, err)
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", false, fmt.Errorf("parsing config %s: %w", targetPath, err)
	}

	raw, ok := m[key]
	if !ok {
		return "", false, nil
	}
	return fmt.Sprint(raw), true, nil
}

// UnsetConfigKey deletes a key from the appropriate config file,
// using the same routing as WriteConfigKey: sensitive keys from
// hams.config.local.yaml (store-local or global fallback), non-
// sensitive from the global hams.config.yaml. Returns nil if the
// key is not present (idempotent) OR the target file does not yet
// exist — the user's intent is "this key shouldn't be set", and
// both "not present" and "file missing" satisfy that intent.
// Documented as `hams config unset <key>` in docs/cli/config.mdx
// but previously not implemented — users had to hand-edit YAML.
func UnsetConfigKey(paths Paths, storePath, key string) error {
	targetPath := paths.GlobalConfigPath()
	if IsSensitiveKey(key) {
		if storePath == "" {
			targetPath = filepath.Join(paths.ConfigHome, "hams.config.local.yaml")
		} else {
			targetPath = filepath.Join(storePath, "hams.config.local.yaml")
		}
	}

	data, err := os.ReadFile(targetPath) //nolint:gosec // config paths are user-specified
	if err != nil {
		if os.IsNotExist(err) {
			return nil // file missing == key not set, already at desired state
		}
		return fmt.Errorf("reading config %s: %w", targetPath, err)
	}

	existing := make(map[string]any)
	if unmarshalErr := yaml.Unmarshal(data, &existing); unmarshalErr != nil {
		return fmt.Errorf("parsing config %s: %w", targetPath, unmarshalErr)
	}

	if _, present := existing[key]; !present {
		return nil // already not present, nothing to do
	}
	delete(existing, key)

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return hamsfile.AtomicWrite(targetPath, out)
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

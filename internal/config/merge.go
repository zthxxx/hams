package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// mergeFromFile reads a YAML config file and merges non-zero values into cfg.
// Used for global-level config files where all fields are valid.
func mergeFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // config paths are user-specified, not user-input tainted
	if err != nil {
		return err
	}

	var overlay Config
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return fmt.Errorf("loading config %s: %w", path, err)
	}

	mergeConfig(cfg, &overlay)
	return nil
}

// mergeFromStoreFile reads a store-level YAML config, rejects machine-scoped
// fields (profile_tag, machine_id), and merges the remaining values.
//
// Store-level files are both the project-tracked `<store>/hams.config.yaml`
// and its gitignored `<store>/hams.config.local.yaml` companion. Both are
// subject to the same scope rejection rule — machine_id and profile_tag are
// machine-identity and belong in ~/.config/hams/, not in any store repo.
func mergeFromStoreFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // config paths are user-specified, not user-input tainted
	if err != nil {
		return err
	}

	var overlay Config
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return fmt.Errorf("loading store config %s: %w", path, err)
	}

	if err := validateStoreScope(&overlay, path); err != nil {
		return err
	}

	mergeConfig(cfg, &overlay)
	return nil
}

// validateStoreScope rejects fields that must only appear in the global
// config. Returns a user-actionable error pointing to the correct location.
func validateStoreScope(cfg *Config, path string) error {
	var field string
	switch {
	case cfg.ProfileTag != "":
		field = "profile_tag"
	case cfg.MachineID != "":
		field = "machine_id"
	default:
		return nil
	}
	return fmt.Errorf(
		"config: %s: field %q is machine-scoped and must not be set at store level. "+
			"Move it to ~/.config/hams/hams.config.yaml "+
			"(or hams.config.local.yaml for untracked per-machine overrides)",
		path, field,
	)
}

// mergeConfig applies non-zero fields from overlay onto base.
func mergeConfig(base, overlay *Config) {
	if overlay.ProfileTag != "" {
		base.ProfileTag = overlay.ProfileTag
	}
	if overlay.MachineID != "" {
		base.MachineID = overlay.MachineID
	}
	if overlay.StoreRepo != "" {
		base.StoreRepo = overlay.StoreRepo
	}
	if overlay.StorePath != "" {
		base.StorePath = overlay.StorePath
	}
	if overlay.LLMCLI != "" {
		base.LLMCLI = overlay.LLMCLI
	}
	if len(overlay.ProviderPriority) > 0 {
		base.ProviderPriority = overlay.ProviderPriority
	}
}

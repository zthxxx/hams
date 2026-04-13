package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// mergeFromFile reads a YAML config file and merges non-zero values into cfg.
func mergeFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // config paths are user-specified, not user-input tainted
	if err != nil {
		return err
	}

	var overlay Config
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return err
	}

	mergeConfig(cfg, &overlay)
	return nil
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

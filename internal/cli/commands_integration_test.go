//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
)

// TestIntegration_ConfigSet verifies that writing a config key via WriteConfigKey
// persists the value to disk and that it can be loaded back correctly.
func TestIntegration_ConfigSet(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")

	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	paths := config.Paths{
		ConfigHome: configHome,
		DataHome:   dataHome,
	}

	// Set profile_tag via WriteConfigKey (non-sensitive -> global config).
	if err := config.WriteConfigKey(paths, "", "profile_tag", "macOS"); err != nil {
		t.Fatalf("WriteConfigKey profile_tag: %v", err)
	}

	// Set machine_id via WriteConfigKey.
	if err := config.WriteConfigKey(paths, "", "machine_id", "test-host"); err != nil {
		t.Fatalf("WriteConfigKey machine_id: %v", err)
	}

	// Verify the global config file was written.
	globalPath := paths.GlobalConfigPath()
	data, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("ReadFile global config: %v", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal global config: %v", err)
	}

	if got := raw["profile_tag"]; got != "macOS" {
		t.Errorf("profile_tag = %v, want %q", got, "macOS")
	}
	if got := raw["machine_id"]; got != "test-host" {
		t.Errorf("machine_id = %v, want %q", got, "test-host")
	}

	// Load config via the standard Load function and verify round-trip.
	cfg, err := config.Load(paths, "")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.ProfileTag != "macOS" {
		t.Errorf("cfg.ProfileTag = %q, want %q", cfg.ProfileTag, "macOS")
	}
	if cfg.MachineID != "test-host" {
		t.Errorf("cfg.MachineID = %q, want %q", cfg.MachineID, "test-host")
	}
}

// TestIntegration_ConfigSetSensitive verifies that sensitive keys are written
// to the local config file rather than the global config.
func TestIntegration_ConfigSetSensitive(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	storeDir := filepath.Join(root, "store")

	if err := os.MkdirAll(storeDir, 0o750); err != nil {
		t.Fatalf("MkdirAll store: %v", err)
	}

	paths := config.Paths{
		ConfigHome: configHome,
		DataHome:   filepath.Join(root, "data"),
	}

	// llm_cli is a sensitive key -> should go to local config.
	if err := config.WriteConfigKey(paths, storeDir, "llm_cli", "/usr/bin/llm"); err != nil {
		t.Fatalf("WriteConfigKey llm_cli: %v", err)
	}

	localPath := filepath.Join(storeDir, "hams.config.local.yaml")
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("ReadFile local config: %v", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal local config: %v", err)
	}

	if got := raw["llm_cli"]; got != "/usr/bin/llm" {
		t.Errorf("llm_cli = %v, want %q", got, "/usr/bin/llm")
	}

	// Verify the value is NOT in the global config.
	globalPath := paths.GlobalConfigPath()
	if _, err := os.Stat(globalPath); err == nil {
		globalData, _ := os.ReadFile(globalPath)
		var globalRaw map[string]interface{}
		if uErr := yaml.Unmarshal(globalData, &globalRaw); uErr == nil {
			if _, exists := globalRaw["llm_cli"]; exists {
				t.Error("llm_cli should not appear in global config")
			}
		}
	}
}

// TestIntegration_ConfigEdit is skipped because it requires an interactive terminal.
func TestIntegration_ConfigEdit(t *testing.T) {
	t.Skip("config edit requires an interactive terminal")
}

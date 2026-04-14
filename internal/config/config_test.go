package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths_Defaults(t *testing.T) {
	// Unset overrides to test defaults.
	t.Setenv("HAMS_CONFIG_HOME", "")
	t.Setenv("HAMS_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	paths := ResolvePaths()

	home, _ := os.UserHomeDir() //nolint:errcheck // test fallback
	wantConfig := filepath.Join(home, ".config", "hams")
	wantData := filepath.Join(home, ".local", "share", "hams")

	if paths.ConfigHome != wantConfig {
		t.Errorf("ConfigHome = %q, want %q", paths.ConfigHome, wantConfig)
	}
	if paths.DataHome != wantData {
		t.Errorf("DataHome = %q, want %q", paths.DataHome, wantData)
	}
}

func TestResolvePaths_XDGOverride(t *testing.T) {
	t.Setenv("HAMS_CONFIG_HOME", "")
	t.Setenv("HAMS_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")

	paths := ResolvePaths()

	if paths.ConfigHome != "/tmp/xdg-config/hams" {
		t.Errorf("ConfigHome = %q, want /tmp/xdg-config/hams", paths.ConfigHome)
	}
	if paths.DataHome != "/tmp/xdg-data/hams" {
		t.Errorf("DataHome = %q, want /tmp/xdg-data/hams", paths.DataHome)
	}
}

func TestResolvePaths_DirectOverride(t *testing.T) {
	t.Setenv("HAMS_CONFIG_HOME", "/custom/config")
	t.Setenv("HAMS_DATA_HOME", "/custom/data")

	paths := ResolvePaths()

	if paths.ConfigHome != "/custom/config" {
		t.Errorf("ConfigHome = %q, want /custom/config", paths.ConfigHome)
	}
	if paths.DataHome != "/custom/data" {
		t.Errorf("DataHome = %q, want /custom/data", paths.DataHome)
	}
}

func TestLoad_EmptyStore(t *testing.T) {
	paths := Paths{ConfigHome: t.TempDir(), DataHome: t.TempDir()}
	cfg, err := Load(paths, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.ProviderPriority) == 0 {
		t.Error("expected default ProviderPriority to be populated")
	}
	if cfg.ProviderPriority[0] != "brew" {
		t.Errorf("first default priority = %q, want 'brew'", cfg.ProviderPriority[0])
	}
}

func TestLoad_4LevelMerge(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()

	// Level 2: global config.
	globalCfg := filepath.Join(configHome, "hams.config.yaml")
	writeYAML(t, globalCfg, "profile_tag: macOS\nmachine_id: global-machine\n")

	// Level 3: project config.
	projectCfg := filepath.Join(storeDir, "hams.config.yaml")
	writeYAML(t, projectCfg, "machine_id: project-machine\nllm_cli: claude\n")

	// Level 4: local override.
	localCfg := filepath.Join(storeDir, "hams.config.local.yaml")
	writeYAML(t, localCfg, "machine_id: local-machine\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	cfg, err := Load(paths, storeDir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// ProfileTag from global (not overridden).
	if cfg.ProfileTag != "macOS" {
		t.Errorf("ProfileTag = %q, want 'macOS'", cfg.ProfileTag)
	}
	// MachineID overridden by local (level 4 > level 3 > level 2).
	if cfg.MachineID != "local-machine" {
		t.Errorf("MachineID = %q, want 'local-machine'", cfg.MachineID)
	}
	// LLMCLI from project (level 3).
	if cfg.LLMCLI != "claude" {
		t.Errorf("LLMCLI = %q, want 'claude'", cfg.LLMCLI)
	}
}

func TestLoad_MissingFilesOK(t *testing.T) {
	paths := Paths{ConfigHome: t.TempDir(), DataHome: t.TempDir()}
	cfg, err := Load(paths, t.TempDir())
	if err != nil {
		t.Fatalf("Load should not error on missing files: %v", err)
	}
	if cfg.ProfileTag != "" {
		t.Errorf("ProfileTag should be empty, got %q", cfg.ProfileTag)
	}
}

func TestProfileDir(t *testing.T) {
	cfg := &Config{StorePath: "/store", ProfileTag: "macOS"}
	if got := cfg.ProfileDir(); got != "/store/macOS" {
		t.Errorf("ProfileDir() = %q, want /store/macOS", got)
	}
}

func TestProfileDir_DefaultTag(t *testing.T) {
	cfg := &Config{StorePath: "/store"}
	if got := cfg.ProfileDir(); got != "/store/default" {
		t.Errorf("ProfileDir() = %q, want /store/default", got)
	}
}

func TestStateDir(t *testing.T) {
	cfg := &Config{StorePath: "/store", MachineID: "MyMac"}
	if got := cfg.StateDir(); got != "/store/.state/MyMac" {
		t.Errorf("StateDir() = %q, want /store/.state/MyMac", got)
	}
}

func TestIsSensitiveKey_ExactMatch(t *testing.T) {
	t.Parallel()
	if !IsSensitiveKey("llm_cli") {
		t.Error("llm_cli should be sensitive (exact match)")
	}
}

func TestIsSensitiveKey_SubstringMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key       string
		sensitive bool
	}{
		{"notification.bark_token", true},
		{"api_secret", true},
		{"db_password", true},
		{"oauth_credential", true},
		{"profile_tag", false},
		{"machine_id", false},
		{"store_path", false},
	}

	for _, tc := range cases {
		if IsSensitiveKey(tc.key) != tc.sensitive {
			t.Errorf("IsSensitiveKey(%q) = %v, want %v", tc.key, !tc.sensitive, tc.sensitive)
		}
	}
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeYAML: %v", err)
	}
}

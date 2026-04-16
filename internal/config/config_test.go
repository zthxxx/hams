package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	// Level 2a: global config holds machine-scoped fields.
	globalCfg := filepath.Join(configHome, "hams.config.yaml")
	writeYAML(t, globalCfg, "profile_tag: macOS\nmachine_id: global-machine\nllm_cli: base\n")

	// Level 2b: global local override bumps machine_id.
	globalLocal := filepath.Join(configHome, "hams.config.local.yaml")
	writeYAML(t, globalLocal, "machine_id: local-machine\n")

	// Level 3: project (store) config tweaks non-machine-scoped fields.
	projectCfg := filepath.Join(storeDir, "hams.config.yaml")
	writeYAML(t, projectCfg, "llm_cli: claude\n")

	// Level 4: store local override wins for non-machine-scoped fields.
	localCfg := filepath.Join(storeDir, "hams.config.local.yaml")
	writeYAML(t, localCfg, "llm_cli: codex\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	cfg, err := Load(paths, storeDir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.ProfileTag != "macOS" {
		t.Errorf("ProfileTag = %q, want 'macOS'", cfg.ProfileTag)
	}
	if cfg.MachineID != "local-machine" {
		t.Errorf("MachineID = %q, want 'local-machine' (from global local)", cfg.MachineID)
	}
	if cfg.LLMCLI != "codex" {
		t.Errorf("LLMCLI = %q, want 'codex' (from store local override)", cfg.LLMCLI)
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
		{"api_key", true},        // "key" pattern — required by schema-design spec
		{"openai_api_key", true}, // compound name with "key"
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

// C1: store hams.config.yaml with profile_tag rejected with actionable error.
func TestLoad_C1_StoreProfileTagRejected(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()

	projectCfg := filepath.Join(storeDir, "hams.config.yaml")
	writeYAML(t, projectCfg, "profile_tag: dev\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	_, err := Load(paths, storeDir)
	if err == nil {
		t.Fatal("Load should reject store-level profile_tag")
	}
	msg := err.Error()
	if !strings.Contains(msg, projectCfg) {
		t.Errorf("error should contain offending file path %q, got %q", projectCfg, msg)
	}
	if !strings.Contains(msg, "profile_tag") {
		t.Errorf("error should name profile_tag, got %q", msg)
	}
	if !strings.Contains(msg, "hams.config.yaml") {
		t.Errorf("error should point to global hams.config.yaml, got %q", msg)
	}
}

// C2: store hams.config.yaml with machine_id rejected.
func TestLoad_C2_StoreMachineIDRejected(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()

	projectCfg := filepath.Join(storeDir, "hams.config.yaml")
	writeYAML(t, projectCfg, "machine_id: sandbox\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	_, err := Load(paths, storeDir)
	if err == nil {
		t.Fatal("Load should reject store-level machine_id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "machine_id") {
		t.Errorf("error should name machine_id, got %q", msg)
	}
	if !strings.Contains(msg, projectCfg) {
		t.Errorf("error should contain offending path, got %q", msg)
	}
}

// C3: store hams.config.local.yaml with profile_tag also rejected (symmetric strictness).
func TestLoad_C3_StoreLocalMachineScopedRejected(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()

	localCfg := filepath.Join(storeDir, "hams.config.local.yaml")
	writeYAML(t, localCfg, "profile_tag: dev\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	_, err := Load(paths, storeDir)
	if err == nil {
		t.Fatal("Load should reject store-local profile_tag (symmetric with tracked file)")
	}
	if !strings.Contains(err.Error(), localCfg) {
		t.Errorf("error should contain store-local path %q, got %q", localCfg, err.Error())
	}
}

// C4: global hams.config.yaml with profile_tag + machine_id is accepted.
func TestLoad_C4_GlobalMachineScopedAccepted(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()

	globalCfg := filepath.Join(configHome, "hams.config.yaml")
	writeYAML(t, globalCfg, "profile_tag: macOS\nmachine_id: MacM5X\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	cfg, err := Load(paths, storeDir)
	if err != nil {
		t.Fatalf("Load should accept global machine-scoped fields: %v", err)
	}
	if cfg.ProfileTag != "macOS" {
		t.Errorf("ProfileTag = %q, want 'macOS'", cfg.ProfileTag)
	}
	if cfg.MachineID != "MacM5X" {
		t.Errorf("MachineID = %q, want 'MacM5X'", cfg.MachineID)
	}
}

// C5: store config without machine-scoped fields loads successfully and merges normally.
func TestLoad_C5_StoreWithoutMachineScopedOk(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()

	globalCfg := filepath.Join(configHome, "hams.config.yaml")
	writeYAML(t, globalCfg, "profile_tag: dev\nmachine_id: sandbox\n")

	projectCfg := filepath.Join(storeDir, "hams.config.yaml")
	writeYAML(t, projectCfg, "llm_cli: claude\nprovider_priority:\n  - bash\n  - apt\n")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	cfg, err := Load(paths, storeDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ProfileTag != "dev" {
		t.Errorf("ProfileTag = %q, want 'dev' (from global)", cfg.ProfileTag)
	}
	if cfg.MachineID != "sandbox" {
		t.Errorf("MachineID = %q, want 'sandbox' (from global)", cfg.MachineID)
	}
	if cfg.LLMCLI != "claude" {
		t.Errorf("LLMCLI = %q, want 'claude' (from store)", cfg.LLMCLI)
	}
	if len(cfg.ProviderPriority) != 2 || cfg.ProviderPriority[0] != "bash" {
		t.Errorf("ProviderPriority = %v, want [bash, apt] (from store)", cfg.ProviderPriority)
	}
}

// TestReadRawConfigKey_SensitiveFromStoreLocal asserts that sensitive
// keys (e.g., notification.bark_token) are read from the store-level
// `hams.config.local.yaml` — matching where `WriteConfigKey` puts them.
func TestReadRawConfigKey_SensitiveFromStoreLocal(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()
	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}

	localPath := filepath.Join(storeDir, "hams.config.local.yaml")
	writeYAML(t, localPath, "notification.bark_token: mytoken\napi_key: sk-xxx\n")

	value, ok, err := ReadRawConfigKey(paths, storeDir, "notification.bark_token")
	if err != nil {
		t.Fatalf("ReadRawConfigKey: %v", err)
	}
	if !ok {
		t.Fatal("expected key to be found")
	}
	if value != "mytoken" {
		t.Errorf("value = %q, want %q", value, "mytoken")
	}
}

// TestReadRawConfigKey_UnsetReturnsFalse asserts missing keys return
// ("", false, nil) — no error, just absence. Enables scripting-friendly
// `hams config get <unset>` behavior (empty output, exit 0).
func TestReadRawConfigKey_UnsetReturnsFalse(t *testing.T) {
	paths := Paths{ConfigHome: t.TempDir(), DataHome: t.TempDir()}
	value, ok, err := ReadRawConfigKey(paths, t.TempDir(), "notification.bark_token")
	if err != nil {
		t.Fatalf("ReadRawConfigKey on missing file: %v", err)
	}
	if ok {
		t.Error("expected ok=false for unset key")
	}
	if value != "" {
		t.Errorf("value = %q, want empty string", value)
	}
}

// TestIsValidConfigKey covers the whitelist check used by `hams
// config set` before accepting a key. Previously 0% covered; any
// future refactor of ValidConfigKeys would go unnoticed.
func TestIsValidConfigKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key  string
		want bool
	}{
		{"profile_tag", true},
		{"machine_id", true},
		{"store_path", true},
		{"store_repo", true},
		{"llm_cli", true},
		{"profile_tg", false},              // typo
		{"notification.bark_token", false}, // sensitive-pattern, not whitelist
		{"", false},
	}
	for _, tc := range cases {
		if got := IsValidConfigKey(tc.key); got != tc.want {
			t.Errorf("IsValidConfigKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestWriteConfigKey_GlobalVsLocal covers the routing behavior of
// WriteConfigKey: non-sensitive keys land in the global config,
// sensitive keys route to hams.config.local.yaml. Both paths were
// previously 0% covered even though they're the core of `hams config set`.
func TestWriteConfigKey_GlobalVsLocal(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()
	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}

	// Non-sensitive key → global config.
	if err := WriteConfigKey(paths, storeDir, "profile_tag", "macOS"); err != nil {
		t.Fatalf("WriteConfigKey profile_tag: %v", err)
	}
	globalPath := filepath.Join(configHome, "hams.config.yaml")
	globalBytes, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	if !strings.Contains(string(globalBytes), "profile_tag: macOS") {
		t.Errorf("global config missing profile_tag; got %q", globalBytes)
	}
	// Local override should NOT contain the non-sensitive value.
	localPath := filepath.Join(storeDir, "hams.config.local.yaml")
	if _, statErr := os.Stat(localPath); statErr == nil {
		b, _ := os.ReadFile(localPath) //nolint:errcheck // read-back assertion; any read error will fail the Contains check below
		if strings.Contains(string(b), "profile_tag") {
			t.Errorf("non-sensitive key leaked into local config; got %q", b)
		}
	}

	// Sensitive key → local config.
	if writeErr := WriteConfigKey(paths, storeDir, "notification.bark_token", "abc123"); writeErr != nil {
		t.Fatalf("WriteConfigKey bark_token: %v", writeErr)
	}
	localBytes, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read local config: %v", err)
	}
	if !strings.Contains(string(localBytes), "notification.bark_token: abc123") {
		t.Errorf("local config missing bark_token; got %q", localBytes)
	}
	// Global config must NOT leak the secret.
	globalBytes2, _ := os.ReadFile(globalPath) //nolint:errcheck // read-back assertion; any read error will fail the Contains check below
	if strings.Contains(string(globalBytes2), "bark_token") {
		t.Errorf("sensitive key leaked into global config: %q", globalBytes2)
	}
}

// TestLoad_MalformedGlobalYAMLSurfaces asserts that a malformed global
// config file returns an error containing the file path, so users can
// fix the broken file. Previously the error was silently ignored by
// apply.go's storePath-resolution path, demoting it to a confusing
// "no store directory configured" message.
func TestLoad_MalformedGlobalYAMLSurfaces(t *testing.T) {
	configHome := t.TempDir()
	globalCfg := filepath.Join(configHome, "hams.config.yaml")
	writeYAML(t, globalCfg, "not : valid : yaml: at all")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	_, err := Load(paths, "")
	if err == nil {
		t.Fatal("expected error loading malformed YAML")
	}
	if !strings.Contains(err.Error(), globalCfg) {
		t.Errorf("error should reference the broken file path %q; got %q", globalCfg, err.Error())
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error should identify the root cause (yaml parse error); got %q", err.Error())
	}
}

// TestLoad_MalformedStoreYAMLSurfaces is the equivalent check for
// <store>/hams.config.yaml — the error SHALL name the specific file.
func TestLoad_MalformedStoreYAMLSurfaces(t *testing.T) {
	configHome := t.TempDir()
	storeDir := t.TempDir()
	projectCfg := filepath.Join(storeDir, "hams.config.yaml")
	writeYAML(t, projectCfg, "not : valid : yaml: at all")

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	_, err := Load(paths, storeDir)
	if err == nil {
		t.Fatal("expected error loading malformed store YAML")
	}
	if !strings.Contains(err.Error(), projectCfg) {
		t.Errorf("error should reference the broken store file path %q; got %q", projectCfg, err.Error())
	}
}

// TestValidate_WarnsOncePerProcess asserts that repeated Validate() calls
// with empty profile_tag/machine_id fire at most one slog.Warn per field.
// Before the once-guard, `hams list` duplicated the warnings 2x per
// command (once during provider registration, once during command action).
func TestValidate_WarnsOncePerProcess(t *testing.T) {
	// Reset to a known clean state; also arrange a buffer to capture logs.
	ResetValidationWarnOnce()
	t.Cleanup(ResetValidationWarnOnce)

	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	c := &Config{StorePath: "/some/store"}
	for range 5 {
		if err := c.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	}

	out := buf.String()
	profileCount := strings.Count(out, "profile_tag is empty")
	machineCount := strings.Count(out, "machine_id is empty")
	if profileCount != 1 {
		t.Errorf("profile_tag warning fired %d times, want 1", profileCount)
	}
	if machineCount != 1 {
		t.Errorf("machine_id warning fired %d times, want 1", machineCount)
	}
}

// TestLoad_StorePathTildeExpansion locks in cycle 85: when a user
// writes `store_path: ~/Project/hams-store` in the global config, the
// loaded cfg.StorePath MUST be the expanded absolute path. Previously,
// the literal `~` became a path component that never matched the real
// home directory — silently producing "No providers match" on every
// run because os.Stat(profileDir) always failed.
func TestLoad_StorePathTildeExpansion(t *testing.T) {
	configHome := t.TempDir()
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Write a config with ~-prefixed store_path.
	globalPath := filepath.Join(configHome, "hams.config.yaml")
	if err := os.WriteFile(globalPath, []byte("store_path: ~/my-store\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	paths := Paths{ConfigHome: configHome, DataHome: t.TempDir()}
	cfg, err := Load(paths, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(fakeHome, "my-store")
	if cfg.StorePath != want {
		t.Errorf("cfg.StorePath = %q, want %q (tilde expansion)", cfg.StorePath, want)
	}
}

// TestExpandHome_NoTildePrefix asserts paths without a leading `~/`
// are returned unchanged (avoid surprising mid-path expansions).
func TestExpandHome_NoTildePrefix(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"/abs/path", "relative/path", "", "~somelogin/path"} {
		got, err := expandHome(input)
		if err != nil {
			t.Errorf("expandHome(%q): %v", input, err)
		}
		if got != input {
			t.Errorf("expandHome(%q) = %q, want unchanged", input, got)
		}
	}
}

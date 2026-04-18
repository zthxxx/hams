package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/storeinit"
)

// TestEnsureGlobalConfig_CreatesWhenMissing asserts the auto-init helper
// writes a default global config to the empty config home and the file
// contains the canonical `tag:` key plus a hostname-derived `machine_id`.
func TestEnsureGlobalConfig_CreatesWhenMissing(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	home := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: t.TempDir()}

	if err := EnsureGlobalConfig(paths, nil); err != nil {
		t.Fatalf("EnsureGlobalConfig: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("read auto-created config: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "tag: "+DefaultTag) {
		t.Errorf("expected tag: %s in config; got %q", DefaultTag, body)
	}
	if !strings.Contains(body, "machine_id: ") {
		t.Errorf("expected machine_id: in config; got %q", body)
	}
}

// TestEnsureGlobalConfig_IsIdempotent asserts a second invocation does
// NOT overwrite hand-edited content. Critical because users WILL edit
// the auto-created config — clobbering on every CLI invocation is
// unacceptable.
func TestEnsureGlobalConfig_IsIdempotent(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	home := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: t.TempDir()}

	if err := EnsureGlobalConfig(paths, nil); err != nil {
		t.Fatalf("first EnsureGlobalConfig: %v", err)
	}

	custom := []byte("# user-edited\ntag: customX\nstore_path: /elsewhere\n")
	configPath := filepath.Join(home, "hams.config.yaml")
	if err := os.WriteFile(configPath, custom, 0o600); err != nil {
		t.Fatalf("write custom: %v", err)
	}

	if err := EnsureGlobalConfig(paths, nil); err != nil {
		t.Fatalf("second EnsureGlobalConfig: %v", err)
	}

	got, _ := os.ReadFile(configPath) //nolint:errcheck // read-back assertion; failure visible below
	if string(got) != string(custom) {
		t.Errorf("EnsureGlobalConfig clobbered hand-edited file\nwant: %q\ngot:  %q", custom, got)
	}
}

// TestEnsureStoreReady_AutoInitsAtDefaultLocation asserts that a clean
// data home triggers a Bootstrap at ${HAMS_DATA_HOME}/store and the
// path is persisted into the global config.
func TestEnsureStoreReady_AutoInitsAtDefaultLocation(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	home := t.TempDir()
	data := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: data}

	storePath, autoInited, err := EnsureStoreReady(paths, &config.Config{}, "", nil)
	if err != nil {
		t.Fatalf("EnsureStoreReady: %v", err)
	}
	if !autoInited {
		t.Errorf("autoInited = false, want true on fresh data home")
	}

	wantPath := filepath.Join(data, DefaultAutoStoreSubdir)
	if storePath != wantPath {
		t.Errorf("storePath = %q, want %q", storePath, wantPath)
	}
	if !storeinit.Bootstrapped(storePath) {
		t.Errorf("Bootstrapped(%s) = false after EnsureStoreReady", storePath)
	}

	// Persist check: global config should now contain store_path.
	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("config.Load after auto-init: %v", err)
	}
	if cfg.StorePath != wantPath {
		t.Errorf("persisted store_path = %q, want %q", cfg.StorePath, wantPath)
	}
}

// TestEnsureStoreReady_RespectsCLIOverride asserts --store wins over the
// auto-init default. Critical so power users with a configured store
// don't accidentally trigger auto-init.
func TestEnsureStoreReady_RespectsCLIOverride(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	paths := config.Paths{ConfigHome: t.TempDir(), DataHome: t.TempDir()}
	override := "/custom/path"

	storePath, autoInited, err := EnsureStoreReady(paths, &config.Config{}, override, nil)
	if err != nil {
		t.Fatalf("EnsureStoreReady with override: %v", err)
	}
	if autoInited {
		t.Errorf("autoInited = true, want false when --store provided")
	}
	if storePath != override {
		t.Errorf("storePath = %q, want %q", storePath, override)
	}
	// Auto-init must NOT have created the default location.
	defaultPath := filepath.Join(paths.DataHome, DefaultAutoStoreSubdir)
	if _, err := os.Stat(defaultPath); err == nil {
		t.Errorf("default store dir was unexpectedly created at %s", defaultPath)
	}
}

// TestIsAutoInitDisabled_HonorsEnv asserts the opt-out env var is
// recognized in its three documented forms.
func TestIsAutoInitDisabled_HonorsEnv(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	if IsAutoInitDisabled() {
		t.Error("default IsAutoInitDisabled should be false")
	}
	for _, v := range []string{"1", "true", "TRUE", "yes", "YES"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("HAMS_NO_AUTO_INIT", v)
			if !IsAutoInitDisabled() {
				t.Errorf("HAMS_NO_AUTO_INIT=%q should disable auto-init", v)
			}
		})
	}
}

// TestRouteToProvider_AutoInitFiresWhenStoreMissing locks in the
// dispatch-side auto-init contract: a brand-new user running
// `hams brew install jq` should NOT see "no store directory configured".
// Instead, autoInitForProvider creates a default store at
// ${HAMS_DATA_HOME}/store and populates flags.Store so the downstream
// provider sees a valid store path.
func TestRouteToProvider_AutoInitFiresWhenStoreMissing(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	t.Setenv("HAMS_CONFIG_HOME", t.TempDir())
	t.Setenv("HAMS_DATA_HOME", t.TempDir())

	mock := &mockProvider{name: "brew", displayName: "Homebrew"}
	flags := &provider.GlobalFlags{}
	if err := routeToProvider(testContext(t), mock, []string{"install", "jq"}, flags); err != nil {
		t.Fatalf("routeToProvider: %v", err)
	}

	if mock.lastFlags == nil {
		t.Fatal("provider not invoked — autoInit short-circuited dispatch")
	}
	if mock.lastFlags.Store == "" {
		t.Errorf("flags.Store stayed empty after auto-init; expected populated default store path")
	}
	if !strings.HasSuffix(mock.lastFlags.Store, DefaultAutoStoreSubdir) {
		t.Errorf("flags.Store = %q, want suffix %q", mock.lastFlags.Store, DefaultAutoStoreSubdir)
	}
	if !storeinit.Bootstrapped(mock.lastFlags.Store) {
		t.Errorf("Bootstrapped(%s) = false", mock.lastFlags.Store)
	}
}

// TestRouteToProvider_AutoInitSkippedWhenStoreOverridden asserts --store
// suppresses auto-init even on a fresh data home.
func TestRouteToProvider_AutoInitSkippedWhenStoreOverridden(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	t.Setenv("HAMS_CONFIG_HOME", t.TempDir())
	dataHome := t.TempDir()
	t.Setenv("HAMS_DATA_HOME", dataHome)

	override := t.TempDir()
	mock := &mockProvider{name: "brew", displayName: "Homebrew"}
	flags := &provider.GlobalFlags{Store: override}
	if err := routeToProvider(testContext(t), mock, []string{"install", "jq"}, flags); err != nil {
		t.Fatalf("routeToProvider: %v", err)
	}
	if mock.lastFlags.Store != override {
		t.Errorf("flags.Store = %q, want %q (override should win)", mock.lastFlags.Store, override)
	}
	defaultPath := filepath.Join(dataHome, DefaultAutoStoreSubdir)
	if _, err := os.Stat(defaultPath); err == nil {
		t.Errorf("auto-init created default store at %s despite --store override", defaultPath)
	}
}

// testContext is a thin wrapper around context.Background that mirrors
// the helper pattern in provider_cmd_test.go without re-declaring it.
func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

// TestEnsureStoreReady_DryRunHasNoSideEffects locks in CLAUDE.md's
// first-principle: `--dry-run` MUST NOT touch the filesystem, even on
// the fresh-machine auto-init path that would otherwise mkdir /
// git-init / write templates. Asserts that:
//
//  1. The store directory is NOT created under HAMS_DATA_HOME.
//  2. The global config is NOT persisted (no store_path key write).
//  3. The stderr sink captures a `[dry-run] Would ...` preview line.
//
// Regression gate for the "auto-init-ux-hardening" task's dry-run
// short-circuit requirement.
func TestEnsureStoreReady_DryRunHasNoSideEffects(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	home := t.TempDir()
	data := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: data}

	var stderr bytes.Buffer
	flags := &provider.GlobalFlags{DryRun: true, Err: &stderr}

	storePath, autoInited, err := EnsureStoreReady(paths, &config.Config{}, "", flags)
	if err != nil {
		t.Fatalf("EnsureStoreReady (dry-run): %v", err)
	}
	if autoInited {
		t.Errorf("autoInited = true, want false under dry-run (no Bootstrap ran)")
	}
	wantPath := filepath.Join(data, DefaultAutoStoreSubdir)
	if storePath != wantPath {
		t.Errorf("storePath = %q, want preview target %q", storePath, wantPath)
	}

	// Filesystem invariant: the target dir must NOT exist on disk.
	if _, err := os.Stat(wantPath); err == nil {
		t.Errorf("dry-run created target directory at %s — expected no side effect", wantPath)
	}
	// Global config invariant: the helper must NOT have written
	// store_path back. A missing config file is the strongest form
	// of "no side effect" — we assert the file doesn't exist.
	if _, err := os.Stat(paths.GlobalConfigPath()); err == nil {
		t.Errorf("dry-run wrote global config at %s — expected no side effect",
			paths.GlobalConfigPath())
	}
	// Preview line invariant: the stderr sink MUST have captured the
	// `[dry-run] Would ...` line. Substring check is tolerant of
	// locale drift (the same key is translated).
	got := stderr.String()
	if !strings.Contains(got, "[dry-run]") || !strings.Contains(got, "store") {
		t.Errorf("stderr preview missing; got: %q", got)
	}
}

// TestEnsureGlobalConfig_DryRunSkipsWrite mirrors the above for the
// global-config helper: `--dry-run` MUST NOT write
// ~/.config/hams/hams.config.yaml on a pristine host.
func TestEnsureGlobalConfig_DryRunSkipsWrite(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	home := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: t.TempDir()}

	var stderr bytes.Buffer
	flags := &provider.GlobalFlags{DryRun: true, Err: &stderr}

	if err := EnsureGlobalConfig(paths, flags); err != nil {
		t.Fatalf("EnsureGlobalConfig (dry-run): %v", err)
	}

	if _, err := os.Stat(paths.GlobalConfigPath()); err == nil {
		t.Errorf("dry-run wrote global config at %s — expected no side effect",
			paths.GlobalConfigPath())
	}
	got := stderr.String()
	if !strings.Contains(got, "[dry-run]") || !strings.Contains(got, "global config") {
		t.Errorf("stderr preview missing; got: %q", got)
	}
}

// TestEnsureStoreReady_SeedsIdentity asserts that after a real (non-
// dry-run) Bootstrap, the global config now contains profile_tag +
// machine_id — so subsequent `hams <provider> …` invocations don't
// see the "profile_tag empty / machine_id empty" nudge. Pins
// config.HostnameLookup to a deterministic value so the test doesn't
// depend on the real CI hostname.
func TestEnsureStoreReady_SeedsIdentity(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")
	t.Setenv("HAMS_MACHINE_ID", "")

	origHostname := config.HostnameLookup
	t.Cleanup(func() { config.HostnameLookup = origHostname })
	config.HostnameLookup = func() (string, error) { return "testbox", nil }

	home := t.TempDir()
	data := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: data}

	if _, _, err := EnsureStoreReady(paths, &config.Config{}, "", nil); err != nil {
		t.Fatalf("EnsureStoreReady: %v", err)
	}

	// Re-load the config from disk: the seeded identity keys must
	// now round-trip through the YAML + UnmarshalYAML path.
	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("config.Load after seed: %v", err)
	}
	if cfg.ProfileTag != config.DefaultProfileTag {
		t.Errorf("profile_tag = %q, want %q after seed",
			cfg.ProfileTag, config.DefaultProfileTag)
	}
	if cfg.MachineID != "testbox" {
		t.Errorf("machine_id = %q, want %q after seed (via HostnameLookup seam)",
			cfg.MachineID, "testbox")
	}
}

// TestEnsureStoreReady_RespectsPreSetIdentity asserts the seed helper
// MUST NOT overwrite a user-supplied value. If the user ran `hams
// config set profile_tag macOS` before their first provider install,
// the scaffolder keeps "macOS" — it does not silently downgrade to
// "default".
func TestEnsureStoreReady_RespectsPreSetIdentity(t *testing.T) {
	t.Setenv("HAMS_NO_AUTO_INIT", "")

	home := t.TempDir()
	data := t.TempDir()
	paths := config.Paths{ConfigHome: home, DataHome: data}

	// Pre-seed the global config with a user-chosen identity.
	userConfig := "profile_tag: macOS\nmachine_id: laptop-m5x\n"
	if err := os.WriteFile(paths.GlobalConfigPath(), []byte(userConfig), 0o600); err != nil {
		t.Fatalf("pre-seed global config: %v", err)
	}

	if _, _, err := EnsureStoreReady(paths, &config.Config{}, "", nil); err != nil {
		t.Fatalf("EnsureStoreReady: %v", err)
	}

	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("config.Load after seed: %v", err)
	}
	if cfg.ProfileTag != "macOS" {
		t.Errorf("seedIdentityIfMissing overwrote user profile_tag: got %q, want %q",
			cfg.ProfileTag, "macOS")
	}
	if cfg.MachineID != "laptop-m5x" {
		t.Errorf("seedIdentityIfMissing overwrote user machine_id: got %q, want %q",
			cfg.MachineID, "laptop-m5x")
	}
}

package cli

import (
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

	if err := EnsureGlobalConfig(paths); err != nil {
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

	if err := EnsureGlobalConfig(paths); err != nil {
		t.Fatalf("first EnsureGlobalConfig: %v", err)
	}

	custom := []byte("# user-edited\ntag: customX\nstore_path: /elsewhere\n")
	configPath := filepath.Join(home, "hams.config.yaml")
	if err := os.WriteFile(configPath, custom, 0o600); err != nil {
		t.Fatalf("write custom: %v", err)
	}

	if err := EnsureGlobalConfig(paths); err != nil {
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

	storePath, autoInited, err := EnsureStoreReady(paths, &config.Config{}, "")
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

	storePath, autoInited, err := EnsureStoreReady(paths, &config.Config{}, override)
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

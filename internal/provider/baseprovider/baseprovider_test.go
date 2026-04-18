package baseprovider_test

import (
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
)

func TestEffectiveConfig_NilCfg(t *testing.T) {
	t.Parallel()
	got := baseprovider.EffectiveConfig(nil, nil)
	if got == nil {
		t.Fatal("expected non-nil even with nil inputs")
	}
}

func TestEffectiveConfig_FlagsOverrideCfg(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{StorePath: "/from-cfg", ProfileTag: "macOS"}
	flags := &provider.GlobalFlags{Store: "/from-flag", Profile: "linux"}

	got := baseprovider.EffectiveConfig(cfg, flags)
	if got.StorePath != "/from-flag" {
		t.Errorf("StorePath = %q, want /from-flag", got.StorePath)
	}
	if got.ProfileTag != "linux" {
		t.Errorf("ProfileTag = %q, want linux", got.ProfileTag)
	}
	// Ensure original cfg untouched (no aliasing).
	if cfg.StorePath != "/from-cfg" {
		t.Errorf("original cfg.StorePath mutated: %q", cfg.StorePath)
	}
}

func TestHamsfilePath_RoutesToProfileDir(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{StorePath: "/store", ProfileTag: "macOS"}
	flags := &provider.GlobalFlags{}
	got, err := baseprovider.HamsfilePath(cfg, "apt", nil, flags)
	if err != nil {
		t.Fatalf("HamsfilePath: %v", err)
	}
	want := filepath.Join("/store", "macOS", "apt.hams.yaml")
	if got != want {
		t.Errorf("HamsfilePath = %q, want %q", got, want)
	}
}

func TestHamsfilePath_LocalFlagSwitchesSuffix(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{StorePath: "/store", ProfileTag: "macOS"}
	flags := &provider.GlobalFlags{}
	got, err := baseprovider.HamsfilePath(cfg, "apt", map[string]string{"local": ""}, flags)
	if err != nil {
		t.Fatalf("HamsfilePath: %v", err)
	}
	want := filepath.Join("/store", "macOS", "apt.hams.local.yaml")
	if got != want {
		t.Errorf("HamsfilePath = %q, want %q", got, want)
	}
}

func TestHamsfilePath_EmptyStoreErrors(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	flags := &provider.GlobalFlags{}
	_, err := baseprovider.HamsfilePath(cfg, "apt", nil, flags)
	if err == nil {
		t.Fatal("expected error for empty store path")
	}
}

func TestLoadOrCreateHamsfile_FreshFile(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	cfg := &config.Config{StorePath: store, ProfileTag: "default"}
	flags := &provider.GlobalFlags{}

	hf, err := baseprovider.LoadOrCreateHamsfile(cfg, "apt", nil, flags)
	if err != nil {
		t.Fatalf("LoadOrCreateHamsfile: %v", err)
	}
	if hf == nil {
		t.Fatal("expected non-nil File")
	}
}

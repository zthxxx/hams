package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

func TestNewApp_CreatesApp(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.Name != "hams" {
		t.Errorf("app.Name = %q, want 'hams'", app.Name)
	}
}

func TestNewApp_VersionFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "--version"})
	if err != nil {
		t.Fatalf("--version error: %v", err)
	}
}

func TestNewApp_HelpFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "--help"})
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}
}

// TestNewApp_VersionSubcommandAvailable asserts `hams version`
// routes to a dedicated subcommand (distinct from --version).
// Surfaces the detailed build info that --version omits.
func TestNewApp_VersionSubcommandAvailable(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	if err := app.Run(context.Background(), []string{"hams", "version"}); err != nil {
		t.Fatalf("`hams version` error: %v", err)
	}
}

// TestProviderUsageDescription_NonPackageProvidersHaveSpecificNouns asserts
// each non-package provider maps to its correct verb/noun, so `hams --help`
// no longer advertises git-config, defaults, etc. as managing "packages".
func TestProviderUsageDescription_NonPackageProvidersHaveSpecificNouns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, displayName, wantSub string
	}{
		{"git-config", "git-config", "git config entries"},
		{"git-clone", "git-clone", "cloned git repositories"},
		{"defaults", "defaults", "macOS defaults"},
		{"duti", "duti", "default-app associations"},
		{"bash", "bash", "bash provisioning"},
		{"ansible", "ansible", "Ansible playbooks"},
		{"code-ext", "code-ext", "VS Code extensions"},
	}
	for _, tc := range cases {
		got := providerUsageDescription(tc.name, tc.displayName)
		if !strings.Contains(got, tc.wantSub) {
			t.Errorf("%s: got %q, want substring %q", tc.name, got, tc.wantSub)
		}
		if strings.Contains(got, "packages") {
			t.Errorf("%s: non-package provider should not say 'packages', got %q", tc.name, got)
		}
	}
}

// TestProviderUsageDescription_PackageProvidersUsePackageTemplate asserts
// the fallback for actual package-class providers still says "Manage X packages".
func TestProviderUsageDescription_PackageProvidersUsePackageTemplate(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, displayName string }{
		{"brew", "Homebrew"},
		{"apt", "apt"},
		{"pnpm", "pnpm"},
		{"npm", "npm"},
		{"uv", "uv"},
		{"goinstall", "goinstall"},
		{"cargo", "cargo"},
		{"mas", "mas"},
	}
	for _, tc := range cases {
		got := providerUsageDescription(tc.name, tc.displayName)
		wantSub := "Manage " + tc.displayName + " packages"
		if got != wantSub {
			t.Errorf("%s: got %q, want %q", tc.name, got, wantSub)
		}
	}
}

// TestProviderUsageDescription_UnknownProviderFallsBack asserts future
// external plugins get the package-class default rather than an empty string.
func TestProviderUsageDescription_UnknownProviderFallsBack(t *testing.T) {
	t.Parallel()
	got := providerUsageDescription("future-external", "future-external")
	if got == "" {
		t.Error("unknown provider must not return empty usage")
	}
	if !strings.Contains(got, "future-external") {
		t.Errorf("fallback should contain display name, got %q", got)
	}
}

// TestNewApp_ProviderCommandsAreSorted asserts that provider subcommands
// appear in alphabetical order regardless of Go map iteration randomness —
// so `hams --help` produces reproducible output across runs.
func TestNewApp_ProviderCommandsAreSorted(t *testing.T) {
	// Save and restore registry to avoid cross-test contamination.
	orig := providerRegistry
	t.Cleanup(func() { providerRegistry = orig })

	providerRegistry = map[string]ProviderHandler{
		"zeta":  &mockProvider{name: "zeta", displayName: "Zeta"},
		"alpha": &mockProvider{name: "alpha", displayName: "Alpha"},
		"mango": &mockProvider{name: "mango", displayName: "Mango"},
		"beta":  &mockProvider{name: "beta", displayName: "Beta"},
	}

	// Run NewApp many times; provider ordering MUST stay identical.
	var firstOrder []string
	for i := range 20 {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		var order []string
		for _, c := range app.Commands {
			switch c.Name {
			case "zeta", "alpha", "mango", "beta":
				order = append(order, c.Name)
			}
		}
		if i == 0 {
			firstOrder = order
			want := []string{"alpha", "beta", "mango", "zeta"}
			for j, w := range want {
				if j >= len(order) || order[j] != w {
					t.Fatalf("expected sorted providers %v, got %v", want, order)
				}
			}
		} else {
			for j, name := range order {
				if j >= len(firstOrder) || firstOrder[j] != name {
					t.Fatalf("iteration %d: order changed; was %v, now %v", i, firstOrder, order)
				}
			}
		}
	}
}

// TestResolvePaths_TildeExpansionForConfig locks in cycle 89: when
// the user types `hams --config=~/my.yaml`, shell leaves `~/` as a
// literal (bash only tilde-expands `~/...` at the start of a
// separate argument). hams MUST expand it itself — otherwise
// `paths.ConfigFilePath` stores `~/my.yaml`, which never matches
// the real file on disk and every config read silently falls back
// to defaults.
func TestResolvePaths_TildeExpansionForConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	flags := &provider.GlobalFlags{Config: "~/my-config.yaml"}
	paths := resolvePaths(flags)

	wantConfigFile := filepath.Join(fakeHome, "my-config.yaml")
	if paths.ConfigFilePath != wantConfigFile {
		t.Errorf("ConfigFilePath = %q, want %q", paths.ConfigFilePath, wantConfigFile)
	}
	if flags.Config != wantConfigFile {
		t.Errorf("flags.Config after resolvePaths = %q, want %q (callers reading flags.Config elsewhere need the expanded value)", flags.Config, wantConfigFile)
	}
	if paths.ConfigHome != fakeHome {
		t.Errorf("ConfigHome = %q, want %q", paths.ConfigHome, fakeHome)
	}
}

// TestResolvePaths_TildeExpansionForStore locks the same invariant
// for --store. Without it, `hams --store=~/my-store apply` would
// miss the actual store on disk and silently fall through to "no
// store directory configured".
func TestResolvePaths_TildeExpansionForStore(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	flags := &provider.GlobalFlags{Store: "~/my-store"}
	_ = resolvePaths(flags)

	wantStore := filepath.Join(fakeHome, "my-store")
	if flags.Store != wantStore {
		t.Errorf("flags.Store after resolvePaths = %q, want %q", flags.Store, wantStore)
	}
}

// TestResolvePaths_AbsolutePathsUnchanged asserts the expansion is
// a no-op for paths that don't start with `~/` (absolute or relative
// without tilde prefix).
func TestResolvePaths_AbsolutePathsUnchanged(t *testing.T) {
	flags := &provider.GlobalFlags{
		Config: "/abs/path/config.yaml",
		Store:  "/abs/path/store",
	}
	paths := resolvePaths(flags)

	if flags.Config != "/abs/path/config.yaml" {
		t.Errorf("Config changed unexpectedly: %q", flags.Config)
	}
	if flags.Store != "/abs/path/store" {
		t.Errorf("Store changed unexpectedly: %q", flags.Store)
	}
	if paths.ConfigFilePath != "/abs/path/config.yaml" {
		t.Errorf("ConfigFilePath = %q", paths.ConfigFilePath)
	}
}

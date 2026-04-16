package cli

import (
	"context"
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

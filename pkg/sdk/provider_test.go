package sdk

import (
	"testing"
)

func TestPluginDiscoveryPaths(t *testing.T) {
	t.Parallel()
	paths := PluginDiscoveryPaths()
	if len(paths) == 0 {
		t.Fatal("PluginDiscoveryPaths should return at least one path")
	}
	for _, p := range paths {
		if p == "" {
			t.Error("path should not be empty")
		}
	}
}

func TestPluginManifest_NameAndDisplayName(t *testing.T) {
	t.Parallel()
	m := PluginManifest{
		Name:        "test",
		DisplayName: "Test Provider",
	}
	if m.Name != "test" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "Test Provider" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
}

func TestPluginManifest_DependsOn(t *testing.T) {
	t.Parallel()
	dep := PluginDepend{Provider: "npm", Package: "pnpm"}
	m := PluginManifest{
		DependsOn: []PluginDepend{dep},
	}
	if len(m.DependsOn) != 1 {
		t.Fatalf("DependsOn len = %d, want 1", len(m.DependsOn))
	}
	if m.DependsOn[0].Provider != "npm" {
		t.Errorf("DependsOn[0].Provider = %q", m.DependsOn[0].Provider)
	}
	if m.DependsOn[0].Package != "pnpm" {
		t.Errorf("DependsOn[0].Package = %q", m.DependsOn[0].Package)
	}
}

func TestPluginProbeResult_Fields(t *testing.T) {
	t.Parallel()
	r := PluginProbeResult{
		ID:      "htop",
		State:   "ok",
		Version: "3.2.1",
	}
	if r.ID != "htop" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.State != "ok" {
		t.Errorf("State = %q", r.State)
	}
	if r.Version != "3.2.1" {
		t.Errorf("Version = %q", r.Version)
	}
}

func TestPluginDepend_PlatformConditional(t *testing.T) {
	t.Parallel()
	d := PluginDepend{
		Provider: "homebrew",
		Package:  "visual-studio-code",
		Platform: "darwin",
	}
	if d.Platform != "darwin" {
		t.Errorf("Platform = %q, want darwin", d.Platform)
	}
	if d.Provider != "homebrew" {
		t.Errorf("Provider = %q", d.Provider)
	}
	if d.Package != "visual-studio-code" {
		t.Errorf("Package = %q", d.Package)
	}
}

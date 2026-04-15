package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// stubProvider is a minimal Provider used for filter tests. Only Manifest
// is consulted by HasArtifacts / FilterByArtifacts.
type stubProvider struct {
	name       string
	filePrefix string
}

func (s stubProvider) Manifest() provider.Manifest {
	return provider.Manifest{Name: s.name, FilePrefix: s.filePrefix, DisplayName: s.name}
}
func (s stubProvider) Bootstrap(context.Context) error { return nil }
func (s stubProvider) Probe(context.Context, *state.File) ([]provider.ProbeResult, error) {
	return nil, nil
}
func (s stubProvider) Plan(context.Context, *hamsfile.File, *state.File) ([]provider.Action, error) {
	return nil, nil
}
func (s stubProvider) Apply(context.Context, provider.Action) error { return nil }
func (s stubProvider) Remove(context.Context, string) error         { return nil }
func (s stubProvider) List(context.Context, *hamsfile.File, *state.File) (string, error) {
	return "", nil
}

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

func TestHasArtifacts_NoFiles_ReturnsFalse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := stubProvider{name: "apt", filePrefix: "apt"}
	if provider.HasArtifacts(p, filepath.Join(root, "profile"), filepath.Join(root, "state")) {
		t.Errorf("HasArtifacts = true, want false (no files)")
	}
}

func TestHasArtifacts_HamsfileOnly_ReturnsTrue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	stateDir := filepath.Join(root, "state")
	touch(t, profileDir, "apt.hams.yaml")
	p := stubProvider{name: "apt", filePrefix: "apt"}
	if !provider.HasArtifacts(p, profileDir, stateDir) {
		t.Errorf("HasArtifacts = false, want true (hamsfile present)")
	}
}

func TestHasArtifacts_LocalHamsfileOnly_ReturnsTrue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	stateDir := filepath.Join(root, "state")
	touch(t, profileDir, "apt.hams.local.yaml")
	p := stubProvider{name: "apt", filePrefix: "apt"}
	if !provider.HasArtifacts(p, profileDir, stateDir) {
		t.Errorf("HasArtifacts = false, want true (local hamsfile present)")
	}
}

func TestHasArtifacts_StateOnly_ReturnsTrue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	stateDir := filepath.Join(root, "state")
	touch(t, stateDir, "apt.state.yaml")
	p := stubProvider{name: "apt", filePrefix: "apt"}
	if !provider.HasArtifacts(p, profileDir, stateDir) {
		t.Errorf("HasArtifacts = false, want true (state file present)")
	}
}

func TestHasArtifacts_BothPresent_ReturnsTrue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	stateDir := filepath.Join(root, "state")
	touch(t, profileDir, "apt.hams.yaml")
	touch(t, stateDir, "apt.state.yaml")
	p := stubProvider{name: "apt", filePrefix: "apt"}
	if !provider.HasArtifacts(p, profileDir, stateDir) {
		t.Errorf("HasArtifacts = false, want true (both present)")
	}
}

func TestHasArtifacts_FilePrefixFallsBackToName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	stateDir := filepath.Join(root, "state")
	// Provider declares Name but no FilePrefix — fallback rule uses Name.
	touch(t, profileDir, "ansible.hams.yaml")
	p := stubProvider{name: "ansible", filePrefix: ""}
	if !provider.HasArtifacts(p, profileDir, stateDir) {
		t.Errorf("HasArtifacts = false, want true (Name fallback)")
	}
}

func TestHasArtifacts_EmptyPaths_ReturnsFalse(t *testing.T) {
	t.Parallel()
	p := stubProvider{name: "apt", filePrefix: "apt"}
	if provider.HasArtifacts(p, "", "") {
		t.Errorf("HasArtifacts = true, want false (empty paths)")
	}
}

func TestFilterByArtifacts_PreservesOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profileDir := filepath.Join(root, "profile")
	stateDir := filepath.Join(root, "state")
	touch(t, profileDir, "apt.hams.yaml")
	touch(t, stateDir, "pnpm.state.yaml")
	// brew has neither → filtered out.

	providers := []provider.Provider{
		stubProvider{name: "apt", filePrefix: "apt"},
		stubProvider{name: "homebrew", filePrefix: "Homebrew"},
		stubProvider{name: "pnpm", filePrefix: "pnpm"},
	}
	got := provider.FilterByArtifacts(providers, profileDir, stateDir)
	if len(got) != 2 {
		t.Fatalf("FilterByArtifacts returned %d providers, want 2 (apt + pnpm)", len(got))
	}
	if got[0].Manifest().Name != "apt" {
		t.Errorf("got[0] = %q, want apt (order preserved)", got[0].Manifest().Name)
	}
	if got[1].Manifest().Name != "pnpm" {
		t.Errorf("got[1] = %q, want pnpm (order preserved)", got[1].Manifest().Name)
	}
}

func TestFilterByArtifacts_EmptyWhenNoneQualify(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	providers := []provider.Provider{
		stubProvider{name: "apt", filePrefix: "apt"},
		stubProvider{name: "homebrew", filePrefix: "Homebrew"},
	}
	got := provider.FilterByArtifacts(providers, filepath.Join(root, "profile"), filepath.Join(root, "state"))
	if got == nil {
		t.Error("FilterByArtifacts returned nil, want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("FilterByArtifacts returned %d providers, want 0", len(got))
	}
}

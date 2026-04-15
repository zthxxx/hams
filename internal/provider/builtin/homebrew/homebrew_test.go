package homebrew

import (
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	p := New(nil)
	m := p.Manifest()
	if m.Name != "brew" {
		t.Errorf("Name = %q, want 'brew'", m.Name)
	}
	if m.DisplayName != "Homebrew" {
		t.Errorf("DisplayName = %q, want 'Homebrew'", m.DisplayName)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %d, want ClassPackage", m.ResourceClass)
	}
	if len(m.DependsOn) != 1 {
		t.Fatalf("DependsOn = %d, want 1", len(m.DependsOn))
	}
	if m.DependsOn[0].Provider != "bash" {
		t.Errorf("DependsOn[0].Provider = %q, want 'bash'", m.DependsOn[0].Provider)
	}
}

func TestName(t *testing.T) {
	p := New(nil)
	if p.Name() != "brew" {
		t.Errorf("Name() = %q, want 'brew'", p.Name())
	}
	if p.DisplayName() != "Homebrew" {
		t.Errorf("DisplayName() = %q, want 'Homebrew'", p.DisplayName())
	}
}

// TestLoadOrCreateHamsfile_MissingFileReturnsEmpty locks in the regression
// fix for `os.IsNotExist` not traversing `%w`-wrapped errors. Before the
// fix, a fresh store with no Homebrew.hams.yaml caused
// `loadOrCreateHamsfile` to return a wrapped read error instead of an
// empty in-memory hamsfile, breaking every CLI install/remove on first
// use against a brand-new store.
func TestLoadOrCreateHamsfile_MissingFileReturnsEmpty(t *testing.T) {
	storeDir := t.TempDir()
	p := New(&config.Config{StorePath: storeDir, ProfileTag: "test"})

	hf, err := p.loadOrCreateHamsfile(nil, &provider.GlobalFlags{})
	if err != nil {
		t.Fatalf("loadOrCreateHamsfile on missing file = %v, want nil", err)
	}
	if hf == nil {
		t.Fatal("loadOrCreateHamsfile returned nil hamsfile")
	}
	wantPath := filepath.Join(storeDir, "test", "Homebrew.hams.yaml")
	if hf.Path != wantPath {
		t.Errorf("hf.Path = %q, want %q", hf.Path, wantPath)
	}
	if hf.Root == nil {
		t.Fatal("hf.Root is nil; expected an empty mapping document node")
	}
}

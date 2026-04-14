package homebrew

import (
	"testing"

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

package ansible

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New()
	m := p.Manifest()
	if m.Name != "ansible" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "Ansible" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if m.Platform != provider.PlatformAll {
		t.Errorf("Platform = %q", m.Platform)
	}
	if m.ResourceClass != provider.ClassCheckBased {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Name() != "ansible" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "Ansible" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

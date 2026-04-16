package ansible

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "ansible" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "Ansible" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	if m.ResourceClass != provider.ClassCheckBased {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if p.Name() != "ansible" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "Ansible" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

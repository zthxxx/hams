package git

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := NewConfigProvider(nil)
	m := p.Manifest()
	if m.Name != "git-config" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "git config" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	if m.ResourceClass != provider.ClassKVConfig {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := NewConfigProvider(nil)
	if p.Name() != "git-config" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "git config" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

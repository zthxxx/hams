package git

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := NewConfigProvider()
	m := p.Manifest()
	if m.Name != "git-config" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "git config" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if m.Platform != provider.PlatformAll {
		t.Errorf("Platform = %q", m.Platform)
	}
	if m.ResourceClass != provider.ClassKVConfig {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := NewConfigProvider()
	if p.Name() != "git-config" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "git config" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

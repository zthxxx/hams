package goinstall

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New()
	m := p.Manifest()
	if m.Name != "goinstall" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "go install" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if m.Platform != provider.PlatformAll {
		t.Errorf("Platform = %q", m.Platform)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestInjectLatest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"golang.org/x/tools/gopls", "golang.org/x/tools/gopls@latest"},
		{"golang.org/x/tools/gopls@v0.14.0", "golang.org/x/tools/gopls@v0.14.0"},
		{"github.com/golangci/golangci-lint/cmd/golangci-lint@latest", "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"},
		{"simple-tool", "simple-tool@latest"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := injectLatest(tt.input)
			if got != tt.want {
				t.Errorf("injectLatest(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Name() != "goinstall" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "go install" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

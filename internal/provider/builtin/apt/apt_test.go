package apt

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(&sudo.RecordingBuilder{})
	m := p.Manifest()
	if m.Name != "apt" {
		t.Errorf("Name = %q, want apt", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformLinux {
		t.Errorf("Platforms = %v, want [linux]", m.Platforms)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q, want package", m.ResourceClass)
	}
	if m.FilePrefix != "apt" {
		t.Errorf("FilePrefix = %q", m.FilePrefix)
	}
}

func TestParseDpkgVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "standard dpkg output",
			output: `Package: curl
Status: install ok installed
Priority: optional
Section: web
Version: 7.88.1-10+deb12u5
Architecture: amd64`,
			want: "7.88.1-10+deb12u5",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "no version line",
			output: "Package: curl\nStatus: installed\n",
			want:   "",
		},
		{
			name:   "version only",
			output: "Version: 1.0.0\n",
			want:   "1.0.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseDpkgVersion(tt.output)
			if got != tt.want {
				t.Errorf("parseDpkgVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(&sudo.RecordingBuilder{})
	if p.Name() != "apt" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "apt" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

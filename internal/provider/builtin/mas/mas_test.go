package mas

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "mas" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "Mac App Store" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformDarwin {
		t.Errorf("Platforms = %v, want [darwin]", m.Platforms)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestParseMasList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name: "standard mas list",
			output: `497799835  Xcode (15.2)
409183694  Keynote (13.2)
1295203466  Microsoft Remote Desktop (10.9.5)`,
			want: map[string]string{
				"497799835":  "15.2",
				"409183694":  "13.2",
				"1295203466": "10.9.5",
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name:   "single app no version",
			output: "12345 SomeApp\n",
			want:   map[string]string{"12345": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseMasList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseMasList() returned %d entries, want %d: %v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != v {
					t.Errorf("parseMasList()[%q] = %q, want %q", k, gotV, v)
				}
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if p.Name() != "mas" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "Mac App Store" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

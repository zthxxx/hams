package uv

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "uv" {
		t.Errorf("Name = %q", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestParseUvToolList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name: "standard uv tool list",
			output: `black v24.2.0
ruff v0.3.0
mypy v1.8.0`,
			want: map[string]string{
				"black": "24.2.0",
				"ruff":  "0.3.0",
				"mypy":  "1.8.0",
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name:   "tool without version",
			output: "some-tool\n",
			want:   map[string]string{"some-tool": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseUvToolList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseUvToolList() returned %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != v {
					t.Errorf("parseUvToolList()[%q] = %q, want %q", k, gotV, v)
				}
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	if p.Name() != "uv" || p.DisplayName() != "uv" {
		t.Errorf("Name()=%q DisplayName()=%q", p.Name(), p.DisplayName())
	}
}

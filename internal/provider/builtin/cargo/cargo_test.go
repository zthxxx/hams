package cargo

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New()
	m := p.Manifest()
	if m.Name != "cargo" {
		t.Errorf("Name = %q", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestParseCargoList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name: "standard cargo install list",
			output: `bat v0.24.0:
    bat
ripgrep v14.1.0:
    rg
fd-find v10.1.0:
    fd`,
			want: map[string]string{
				"bat":     "0.24.0",
				"ripgrep": "14.1.0",
				"fd-find": "10.1.0",
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name: "single crate no binaries listed",
			output: `tokei v12.1.2:
`,
			want: map[string]string{"tokei": "12.1.2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseCargoList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseCargoList() returned %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != v {
					t.Errorf("parseCargoList()[%q] = %q, want %q", k, gotV, v)
				}
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Name() != "cargo" || p.DisplayName() != "cargo" {
		t.Errorf("Name()=%q DisplayName()=%q", p.Name(), p.DisplayName())
	}
}

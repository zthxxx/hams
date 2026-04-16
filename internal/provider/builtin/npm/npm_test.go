package npm

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "npm" {
		t.Errorf("Name = %q", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestParseNpmList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name: "standard npm list output",
			output: `{
  "dependencies": {
    "serve": {
      "version": "14.2.0"
    },
    "typescript": {
      "version": "5.3.3"
    }
  }
}`,
			want: map[string]string{"serve": "", "typescript": ""},
		},
		{
			name:   "empty output",
			output: "{}",
			want:   map[string]string{},
		},
		{
			name:   "no dependencies",
			output: `{"name":"root"}`,
			want:   map[string]string{},
		},
		{
			name: "nested metadata must not leak as packages",
			output: `{
  "dependencies": {
    "lodash": {
      "version": "4.17.21",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"
    }
  }
}`,
			want: map[string]string{"lodash": ""},
		},
		{
			name:   "invalid JSON",
			output: `not json at all`,
			want:   map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseNpmList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseNpmList() returned %d entries, want %d; got=%v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != v {
					t.Errorf("parseNpmList()[%q] = %q, want %q", k, gotV, v)
				}
			}
			for k := range got {
				if _, ok := tt.want[k]; !ok {
					t.Errorf("parseNpmList() returned unexpected key %q", k)
				}
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if p.Name() != "npm" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "npm" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

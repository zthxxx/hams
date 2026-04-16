package pnpm

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "pnpm" {
		t.Errorf("Name = %q", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	// Two DependsOn entries: DAG ordering (npm → pnpm) + script host (bash).
	// See pnpm.go Manifest() for the full rationale.
	if len(m.DependsOn) != 2 {
		t.Fatalf("DependsOn len = %d, want 2; got %v", len(m.DependsOn), m.DependsOn)
	}
	if m.DependsOn[0].Provider != "npm" || m.DependsOn[0].Script != "" {
		t.Errorf("DependsOn[0] expected DAG-only {Provider: npm}, got %+v", m.DependsOn[0])
	}
	if m.DependsOn[1].Provider != "bash" || m.DependsOn[1].Script == "" {
		t.Errorf("DependsOn[1] expected scripted {Provider: bash, Script: ...}, got %+v", m.DependsOn[1])
	}
}

func TestParsePnpmList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name: "standard pnpm list output",
			output: `{
  "dependencies": {
    "serve": {
      "version": "14.2.0"
    }
  }
}`,
			want: map[string]string{"serve": ""},
		},
		{
			name:   "empty",
			output: "{}",
			want:   map[string]string{},
		},
		{
			name: "nested metadata must not leak as packages",
			output: `{
  "dependencies": {
    "typescript": {
      "version": "5.3.3",
      "from": "typescript@5.3.3",
      "resolved": "https://registry.npmjs.org/typescript/-/typescript-5.3.3.tgz"
    }
  }
}`,
			want: map[string]string{"typescript": ""},
		},
		{
			name:   "invalid JSON",
			output: `not json`,
			want:   map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parsePnpmList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parsePnpmList() returned %d entries, want %d; got=%v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != v {
					t.Errorf("parsePnpmList()[%q] = %q, want %q", k, gotV, v)
				}
			}
			for k := range got {
				if _, ok := tt.want[k]; !ok {
					t.Errorf("parsePnpmList() returned unexpected key %q", k)
				}
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if p.Name() != "pnpm" || p.DisplayName() != "pnpm" {
		t.Errorf("Name()=%q DisplayName()=%q", p.Name(), p.DisplayName())
	}
}

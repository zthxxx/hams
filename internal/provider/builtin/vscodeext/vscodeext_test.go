package vscodeext

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "code-ext" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.DisplayName != "VS Code Extensions" {
		t.Errorf("DisplayName = %q", m.DisplayName)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformAll {
		t.Errorf("Platforms = %v", m.Platforms)
	}
	if len(m.DependsOn) != 1 || m.DependsOn[0].Provider != "brew" {
		t.Errorf("DependsOn = %v", m.DependsOn)
	}
}

func TestParseExtensionList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name: "standard extension list",
			output: `ms-python.python@2024.2.0
dbaeumer.vscode-eslint@3.0.5
esbenp.prettier-vscode@10.1.0`,
			want: map[string]string{
				"ms-python.python":       "2024.2.0",
				"dbaeumer.vscode-eslint": "3.0.5",
				"esbenp.prettier-vscode": "10.1.0",
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name:   "case insensitive",
			output: "MS-Python.Python@1.0.0\n",
			want:   map[string]string{"ms-python.python": "1.0.0"},
		},
		{
			name:   "extension without version",
			output: "some.extension\n",
			want:   map[string]string{"some.extension": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseExtensionList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseExtensionList() returned %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != v {
					t.Errorf("parseExtensionList()[%q] = %q, want %q", k, gotV, v)
				}
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if p.Name() != "code-ext" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "VS Code Extensions" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

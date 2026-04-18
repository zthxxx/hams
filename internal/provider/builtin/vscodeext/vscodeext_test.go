package vscodeext

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// TestProbe_MatchesPinnedVersionIDs locks in cycle 188: a state
// entry like "foo.bar@1.2.3" (recorded by `hams code install
// publisher.ext@1.2.3`) previously NEVER matched the installed
// map (which keys on bare publisher.ext — parseExtensionList
// drops the version from the key). So Probe always reported
// StateFailed for pinned IDs and drift detection was broken for
// any user who pinned a version.
func TestProbe_MatchesPinnedVersionIDs(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	fake.Seed("foo.bar", "1.2.3")

	sf := state.New("code", "test-machine")
	sf.SetResource("foo.bar@1.2.3", state.StateOK)
	sf.SetResource("other.ext", state.StateOK)

	p := New(nil, fake)
	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	byID := make(map[string]provider.ProbeResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}

	// Pinned ID must match via the @-stripped lookup.
	pinned := byID["foo.bar@1.2.3"]
	if pinned.State != state.StateOK {
		t.Errorf("pinned @version ID state = %v, want StateOK", pinned.State)
	}
	if pinned.Version != "1.2.3" {
		t.Errorf("pinned Version = %q, want 1.2.3", pinned.Version)
	}

	// Absent extension stays StateFailed.
	absent := byID["other.ext"]
	if absent.State != state.StateFailed {
		t.Errorf("absent ID state = %v, want StateFailed", absent.State)
	}
}

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "code" {
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
	p := New(nil, NewFakeCmdRunner())
	if p.Name() != "code" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "VS Code Extensions" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

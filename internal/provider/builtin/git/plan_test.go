package git

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// TestClonePlan_StructuredFields covers the primary clone-parse path:
// entries with `urn`/`remote`/`path` fields are parsed into
// cloneResource values and attached to each Action. Also asserts the
// count of produced Actions matches the number of valid URNs.
func TestClonePlan_StructuredFields(t *testing.T) {
	t.Parallel()
	yamlDoc := `
repos:
  - urn: urn:hams:git-clone:dotfiles
    remote: https://github.com/zthxxx/dotfiles.git
    path: ~/dotfiles
  - urn: urn:hams:git-clone:hams
    remote: https://github.com/zthxxx/hams.git
    path: ~/code/hams
    branch: main
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := NewCloneProvider(&config.Config{})
	observed := state.New("git-clone", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(actions))
	}

	byID := make(map[string]cloneResource)
	for _, a := range actions {
		res, ok := a.Resource.(cloneResource)
		if !ok {
			t.Errorf("action %q has Resource type %T, want cloneResource", a.ID, a.Resource)
			continue
		}
		byID[a.ID] = res
	}
	df := byID["urn:hams:git-clone:dotfiles"]
	if df.Remote != "https://github.com/zthxxx/dotfiles.git" || df.Path != "~/dotfiles" {
		t.Errorf("dotfiles resource = %+v", df)
	}
	hams := byID["urn:hams:git-clone:hams"]
	if hams.Branch != "main" {
		t.Errorf("hams Branch = %q, want 'main'", hams.Branch)
	}
}

// TestClonePlan_LegacyScalarEntry covers the scalar-format fallback:
// a sequence item that's a bare string gets split on " -> " into
// remote/path. Keeps backward compat with early schema versions.
func TestClonePlan_LegacyScalarEntry(t *testing.T) {
	t.Parallel()
	yamlDoc := `
repos:
  - "https://github.com/zthxxx/dotfiles.git -> ~/dotfiles"
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := NewCloneProvider(&config.Config{})
	observed := state.New("git-clone", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("got %d actions, want 1", len(actions))
	}
	res, ok := actions[0].Resource.(cloneResource)
	if !ok {
		t.Fatalf("Resource = %T, want cloneResource", actions[0].Resource)
	}
	if res.Remote != "https://github.com/zthxxx/dotfiles.git" || res.Path != "~/dotfiles" {
		t.Errorf("resource = %+v", res)
	}
}

// TestClonePlan_MappingRootRequired asserts that a non-mapping root
// returns an error rather than silently planning zero actions — a
// malformed hamsfile should be loud.
func TestClonePlan_MappingRootRequired(t *testing.T) {
	t.Parallel()
	// Sequence at root, not a mapping.
	yamlDoc := `["should-fail"]`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := NewCloneProvider(&config.Config{})
	observed := state.New("git-clone", "test")
	if _, err := p.Plan(context.Background(), hf, observed); err == nil {
		t.Error("expected error for non-mapping root")
	}
}

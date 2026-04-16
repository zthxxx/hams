package homebrew

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// TestPlan_MarksCaskResourcesForApply drives Plan + caskApps together:
// packages under the `cask:` tag in the hamsfile get a BrewResource
// with IsCask=true attached to their Action, so Apply can inject
// `--cask` via the runner. Packages under other tags (e.g., `cli:`)
// keep their default (nil) Resource.
func TestPlan_MarksCaskResourcesForApply(t *testing.T) {
	t.Parallel()
	yamlDoc := `
cli:
  - urn: ripgrep
  - urn: jq
cask:
  - urn: visual-studio-code
  - urn: docker
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(nil, NewFakeCmdRunner())
	observed := state.New("brew", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("got %d actions, want 4", len(actions))
	}
	caskByID := map[string]bool{}
	for _, a := range actions {
		if res, ok := a.Resource.(BrewResource); ok {
			caskByID[a.ID] = res.IsCask
		}
	}
	for _, cask := range []string{"visual-studio-code", "docker"} {
		if !caskByID[cask] {
			t.Errorf("cask %q should have BrewResource{IsCask:true}; got %v", cask, caskByID)
		}
	}
	// CLI-tagged packages should NOT appear in the caskByID map (no
	// Resource set → the range-branch above is skipped for them).
	for _, cli := range []string{"ripgrep", "jq"} {
		if caskByID[cli] {
			t.Errorf("cli-tagged %q must not be marked IsCask", cli)
		}
	}
}

// TestPlan_EmptyHamsfile returns zero actions.
func TestPlan_EmptyHamsfile(t *testing.T) {
	t.Parallel()
	hf := &hamsfile.File{Path: "empty.yaml", Root: &yaml.Node{Kind: yaml.DocumentNode}}
	p := New(nil, NewFakeCmdRunner())
	observed := state.New("brew", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan(empty): %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("empty hamsfile produced %d actions, want 0", len(actions))
	}
}

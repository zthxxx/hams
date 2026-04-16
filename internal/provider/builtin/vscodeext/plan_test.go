package vscodeext

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// TestPlan_WrapsComputePlanWithHooks covers the previously 0% Plan
// function. Matches the pattern used across builtin providers
// (cycles 21/22/29/30/31).
func TestPlan_WrapsComputePlanWithHooks(t *testing.T) {
	t.Parallel()
	yamlDoc := `
extensions:
  - urn: urn:hams:code-ext:golang.go
  - urn: urn:hams:code-ext:rust-lang.rust-analyzer
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(NewFakeCmdRunner())
	observed := state.New("code-ext", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(actions))
	}
	for _, a := range actions {
		if a.Type != provider.ActionInstall {
			t.Errorf("action %q has Type=%v, want Install", a.ID, a.Type)
		}
	}
}

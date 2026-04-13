package provider

import (
	"testing"

	"github.com/zthxxx/hams/internal/state"

	"pgregory.net/rapid"
)

// Property: DAG of providers with no cycles always produces a valid topological order.
func TestProperty_DAGAcyclic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 10).Draw(t, "n")
		names := make([]string, n)
		for i := range n {
			names[i] = rapid.StringMatching(`[a-z]{3,8}`).Draw(t, "name")
		}

		// Deduplicate.
		seen := make(map[string]bool)
		var unique []string
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				unique = append(unique, name)
			}
		}

		// Build providers with forward-only deps (no cycles).
		providers := make([]Provider, 0, len(unique))
		for i, name := range unique {
			var deps []DependOn
			if i > 0 {
				// Each provider can only depend on providers that come before it.
				depIdx := rapid.IntRange(0, i-1).Draw(t, "dep")
				deps = append(deps, DependOn{Provider: unique[depIdx]})
			}
			providers = append(providers, newStubWithDeps(name, deps...))
		}

		sorted, err := ResolveDAG(providers)
		if err != nil {
			t.Fatalf("ResolveDAG error on acyclic graph: %v", err)
		}

		if len(sorted) != len(unique) {
			t.Fatalf("sorted has %d, want %d", len(sorted), len(unique))
		}

		// Verify topological order: each provider appears after all its deps.
		position := make(map[string]int)
		for i, p := range sorted {
			position[p.Manifest().Name] = i
		}
		for _, p := range sorted {
			for _, dep := range p.Manifest().DependsOn {
				depPos, ok := position[dep.Provider]
				if !ok {
					continue
				}
				myPos := position[p.Manifest().Name]
				if depPos >= myPos {
					t.Errorf("%s (pos %d) should come after dep %s (pos %d)", p.Manifest().Name, myPos, dep.Provider, depPos)
				}
			}
		}
	})
}

// Property: plan always produces exactly one action per desired resource.
func TestProperty_PlanOneActionPerDesired(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "n")
		desired := make([]string, n)
		for i := range n {
			desired[i] = rapid.StringMatching(`[a-z]{2,12}`).Draw(t, "app")
		}

		// Deduplicate desired.
		seen := make(map[string]bool)
		var unique []string
		for _, d := range desired {
			if !seen[d] {
				seen[d] = true
				unique = append(unique, d)
			}
		}

		// Create state with random resources.
		sf := state.New("test", "machine")
		for _, d := range unique {
			if rapid.Bool().Draw(t, "installed") {
				sf.SetResource(d, state.StateOK)
			}
		}

		actions := ComputePlan(unique, sf, "")

		// Each desired resource should appear exactly once.
		actionIDs := make(map[string]int)
		for _, a := range actions {
			actionIDs[a.ID]++
		}

		for _, d := range unique {
			count := actionIDs[d]
			if count != 1 {
				t.Errorf("resource %q has %d actions, want 1", d, count)
			}
		}
	})
}

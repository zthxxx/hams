package provider

import (
	"strings"
	"testing"
)

func newStubWithDeps(name string, deps ...DependOn) *stubProvider {
	return &stubProvider{
		manifest: Manifest{
			Name:        name,
			DisplayName: name,
			Platforms:   []Platform{PlatformAll},
			DependsOn:   deps,
		},
	}
}

func TestResolveDAG_NoDeps(t *testing.T) {
	providers := []Provider{
		newStubWithDeps("brew"),
		newStubWithDeps("pnpm"),
		newStubWithDeps("apt"),
	}

	sorted, err := ResolveDAG(providers)
	if err != nil {
		t.Fatalf("ResolveDAG error: %v", err)
	}
	if len(sorted) != 3 {
		t.Fatalf("sorted has %d items, want 3", len(sorted))
	}
}

func TestResolveDAG_LinearChain(t *testing.T) {
	// vscode-ext -> brew -> bash
	providers := []Provider{
		newStubWithDeps("bash"),
		newStubWithDeps("brew", DependOn{Provider: "bash"}),
		newStubWithDeps("vscode-ext", DependOn{Provider: "brew"}),
	}

	sorted, err := ResolveDAG(providers)
	if err != nil {
		t.Fatalf("ResolveDAG error: %v", err)
	}

	names := providerNames(sorted)
	bashIdx := indexOf(names, "bash")
	brewIdx := indexOf(names, "brew")
	vscodeIdx := indexOf(names, "vscode-ext")

	if bashIdx >= brewIdx {
		t.Errorf("bash (%d) should come before brew (%d)", bashIdx, brewIdx)
	}
	if brewIdx >= vscodeIdx {
		t.Errorf("brew (%d) should come before vscode-ext (%d)", brewIdx, vscodeIdx)
	}
}

func TestResolveDAG_Diamond(t *testing.T) {
	// D depends on B and C, both depend on A.
	providers := []Provider{
		newStubWithDeps("a"),
		newStubWithDeps("b", DependOn{Provider: "a"}),
		newStubWithDeps("c", DependOn{Provider: "a"}),
		newStubWithDeps("d", DependOn{Provider: "b"}, DependOn{Provider: "c"}),
	}

	sorted, err := ResolveDAG(providers)
	if err != nil {
		t.Fatalf("ResolveDAG error: %v", err)
	}

	names := providerNames(sorted)
	aIdx := indexOf(names, "a")
	bIdx := indexOf(names, "b")
	cIdx := indexOf(names, "c")
	dIdx := indexOf(names, "d")

	if aIdx >= bIdx || aIdx >= cIdx {
		t.Errorf("a (%d) should come before b (%d) and c (%d)", aIdx, bIdx, cIdx)
	}
	if bIdx >= dIdx || cIdx >= dIdx {
		t.Errorf("b (%d) and c (%d) should come before d (%d)", bIdx, cIdx, dIdx)
	}
}

func TestResolveDAG_Cycle(t *testing.T) {
	providers := []Provider{
		newStubWithDeps("a", DependOn{Provider: "b"}),
		newStubWithDeps("b", DependOn{Provider: "a"}),
	}

	_, err := ResolveDAG(providers)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want to contain 'cycle'", err.Error())
	}
}

func TestResolveDAG_SingleNode(t *testing.T) {
	providers := []Provider{
		newStubWithDeps("solo"),
	}

	sorted, err := ResolveDAG(providers)
	if err != nil {
		t.Fatalf("ResolveDAG error: %v", err)
	}
	if len(sorted) != 1 || sorted[0].Manifest().Name != "solo" {
		t.Errorf("sorted = %v, want [solo]", providerNames(sorted))
	}
}

// TestResolveDAG_ZeroIndegreePriority documents the current behavior that
// zero-indegree (no-dependency) providers are ordered ALPHABETICALLY by
// ResolveDAG, NOT by the input slice order.
//
// Why it matters: `internal/config/config.go` `DefaultProviderPriority` AND
// user-supplied `provider_priority` YAML overrides become INERT for root-
// level providers (those with no DependsOn entries). Only providers that
// sit inside a dependency chain get ordering sensitivity from the
// priority list — and even then only via DAG topology, not priority.
//
// Ref: `openspec/specs/builtin-providers/spec.md` "Multiple providers at
// same DAG level" scenario says priority MUST determine order; the
// implementation contradicts that.
//
// This test captures the as-shipped behavior so that a future change can
// either:
//
//	(a) flip the assertion once Kahn's is taught to honor input order
//	    (fixing the spec violation), or
//	(b) update the spec to document alphabetical fallback (and explain
//	    why priority lists become inert for root-level providers).
//
// Do not change this assertion without either (a) or (b).
func TestResolveDAG_ZeroIndegreePriority(t *testing.T) {
	// Pass providers in "priority order": bash first, then apt, then cargo.
	// If priority were honored, output would match input order.
	// If alphabetical, output would be apt, bash, cargo.
	providers := []Provider{
		newStubWithDeps("bash"),
		newStubWithDeps("apt"),
		newStubWithDeps("cargo"),
	}

	sorted, err := ResolveDAG(providers)
	if err != nil {
		t.Fatalf("ResolveDAG error: %v", err)
	}

	names := providerNames(sorted)
	want := []string{"apt", "bash", "cargo"} // ALPHABETICAL, not input order
	for i, n := range names {
		if n != want[i] {
			t.Errorf("position %d: got %q, want %q (zero-indegree nodes ordered alphabetically, NOT by input order)", i, n, want[i])
		}
	}
}

func TestResolveDAG_PlatformFiltering(t *testing.T) {
	// Dep is platform-conditional: only on "windows" (which won't match in tests).
	providers := []Provider{
		newStubWithDeps("bash"),
		newStubWithDeps("brew", DependOn{Provider: "bash", Platform: "windows"}),
	}

	sorted, err := ResolveDAG(providers)
	if err != nil {
		t.Fatalf("ResolveDAG error: %v", err)
	}

	// Since the dep is filtered out, brew has no deps — should be in output.
	if len(sorted) != 2 {
		t.Fatalf("sorted has %d items, want 2", len(sorted))
	}
}

func providerNames(providers []Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = strings.ToLower(p.Manifest().Name)
	}
	return names
}

func indexOf(names []string, target string) int {
	for i, n := range names {
		if n == target {
			return i
		}
	}
	return -1
}

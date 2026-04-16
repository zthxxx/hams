package cli

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

// TestBuiltinManifestScriptHostsAreBash enforces the architectural
// invariant surfaced after cycle 6: for every builtin provider, any
// DependsOn entry with a non-empty `.Script` MUST target the `bash`
// provider as its host. Rationale: `provider.RunBootstrap` looks up
// `dep.Provider` in the registry and type-asserts the result to
// `provider.BashScriptRunner`. Only the `bash` provider implements
// that interface — targeting any other (e.g. `npm`, `brew`) makes
// RunBootstrap fail with "host does not implement BashScriptRunner"
// at --bootstrap time on a fresh machine.
//
// DAG-only entries (empty `.Script`, present purely for topological
// ordering) can target any provider.
//
// This invariant is load-bearing for every --bootstrap code path. It
// should be impossible to add a new scripted DependsOn entry without
// this test catching a mistargeted host.
func TestBuiltinManifestScriptHostsAreBash(t *testing.T) {
	registry := provider.NewRegistry()
	registerBuiltins(registry, sudo.DirectBuilder{})

	for _, p := range registry.All() {
		manifest := p.Manifest()
		for i, dep := range manifest.DependsOn {
			if dep.Script == "" {
				continue // DAG-only entry, any Provider is fine
			}
			if dep.Provider != "bash" {
				t.Errorf("provider %q DependsOn[%d] has Script but targets %q; Script entries must target 'bash' (the only BashScriptRunner implementer)",
					manifest.Name, i, dep.Provider)
			}
		}
	}
}

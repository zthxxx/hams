package cli

import (
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/builtin/ansible"
	"github.com/zthxxx/hams/internal/provider/builtin/apt"
	"github.com/zthxxx/hams/internal/provider/builtin/bash"
	"github.com/zthxxx/hams/internal/provider/builtin/cargo"
	"github.com/zthxxx/hams/internal/provider/builtin/defaults"
	"github.com/zthxxx/hams/internal/provider/builtin/duti"
	"github.com/zthxxx/hams/internal/provider/builtin/git"
	"github.com/zthxxx/hams/internal/provider/builtin/goinstall"
	"github.com/zthxxx/hams/internal/provider/builtin/homebrew"
	"github.com/zthxxx/hams/internal/provider/builtin/mas"
	"github.com/zthxxx/hams/internal/provider/builtin/npm"
	"github.com/zthxxx/hams/internal/provider/builtin/pnpm"
	"github.com/zthxxx/hams/internal/provider/builtin/uv"
	"github.com/zthxxx/hams/internal/provider/builtin/vscodeext"
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
// This invariant must hold on EVERY platform: a mistargeted script
// entry on a macOS-only provider (duti, mas, defaults) would escape
// Linux CI if we iterated only registered providers, because
// `registry.Register` silently skips providers whose Platforms
// don't match `runtime.GOOS`. Instead we instantiate every builtin
// directly so the invariant is enforced unconditionally.
func TestBuiltinManifestScriptHostsAreBash(t *testing.T) {
	cfg := &config.Config{}
	all := []provider.Provider{
		homebrew.New(cfg),
		apt.New(cfg, apt.NewRealCmdRunner(sudo.DirectBuilder{})),
		npm.New(npm.NewFakeCmdRunner()),
		pnpm.New(pnpm.NewFakeCmdRunner()),
		uv.New(uv.NewFakeCmdRunner()),
		goinstall.New(goinstall.NewFakeCmdRunner()),
		cargo.New(cargo.NewFakeCmdRunner()),
		git.NewConfigProvider(),
		git.NewCloneProvider(cfg),
		defaults.New(cfg),
		duti.New(),
		mas.New(mas.NewFakeCmdRunner()),
		vscodeext.New(),
		ansible.New(),
		bash.New(),
	}

	for _, p := range all {
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

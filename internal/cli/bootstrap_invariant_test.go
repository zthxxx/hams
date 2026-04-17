package cli

import (
	"runtime"
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
		homebrew.New(cfg, homebrew.NewFakeCmdRunner()),
		apt.New(cfg, apt.NewRealCmdRunner(sudo.DirectBuilder{})),
		npm.New(cfg, npm.NewFakeCmdRunner()),
		pnpm.New(cfg, pnpm.NewFakeCmdRunner()),
		uv.New(cfg, uv.NewFakeCmdRunner()),
		goinstall.New(cfg, goinstall.NewFakeCmdRunner()),
		cargo.New(cfg, cargo.NewFakeCmdRunner()),
		git.NewConfigProvider(cfg),
		git.NewCloneProvider(cfg),
		defaults.New(cfg, defaults.NewFakeCmdRunner()),
		duti.New(cfg, duti.NewFakeCmdRunner()),
		mas.New(cfg, mas.NewFakeCmdRunner()),
		vscodeext.New(cfg, vscodeext.NewFakeCmdRunner()),
		ansible.New(cfg, ansible.NewFakeCmdRunner()),
		bash.New(cfg),
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

// TestRegisterBuiltins_FiltersCLIByPlatform asserts that `registerBuiltins`
// keeps the CLI dispatch registry (`providerRegistry`) consistent with the
// internal provider registry — i.e., macOS-only providers (defaults, duti,
// mas) do NOT appear as `hams <name>` subcommands on Linux. Before this
// fix, they showed up in `hams --help` on Linux and then exec-failed at
// runtime with a confusing "executable not found" error.
func TestRegisterBuiltins_FiltersCLIByPlatform(t *testing.T) {
	// Save and restore registries.
	origProv := providerRegistry
	providerRegistry = make(map[string]ProviderHandler)
	t.Cleanup(func() { providerRegistry = origProv })

	registry := provider.NewRegistry()
	registerBuiltins(registry, sudo.DirectBuilder{})

	darwinOnly := []string{"defaults", "duti", "mas"}
	for _, name := range darwinOnly {
		_, cliHas := providerRegistry[name]
		switch runtime.GOOS {
		case "darwin":
			if !cliHas {
				t.Errorf("on darwin, %q should be in CLI registry", name)
			}
		default:
			if cliHas {
				t.Errorf("on %s, macOS-only provider %q should NOT be in CLI registry (would exec-fail)", runtime.GOOS, name)
			}
		}
	}

	// Sanity: PlatformAll providers are always in the CLI registry.
	for _, name := range []string{"brew", "pnpm", "npm", "uv", "cargo", "bash"} {
		if name == "bash" {
			continue // bash is providerOnly, not in CLI registry by design
		}
		if _, ok := providerRegistry[name]; !ok {
			t.Errorf("%q (PlatformAll) should always be in CLI registry", name)
		}
	}
}

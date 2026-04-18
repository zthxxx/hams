package cli

import (
	"log/slog"
	"os"

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

// cliProvider is a provider that also implements ProviderHandler for CLI routing.
type cliProvider interface {
	provider.Provider
	ProviderHandler
}

// registerBuiltins registers all builtin providers in the registry.
// Each provider is instantiated once and used for both the provider registry
// and CLI handler routing (avoiding double instantiation).
func registerBuiltins(registry *provider.Registry, sudoCmd sudo.CmdBuilder) {
	builtinCfg := loadBuiltinProviderConfig()

	// Build the unified `git` CLI provider once so the same pair of
	// sub-providers (config + clone) is registered with the apply
	// registry AND the CLI dispatcher. Ensures there is ONE instance
	// of each sub-provider across both layers — no double-init, no
	// drift between "what apply sees" and "what the CLI dispatches".
	unifiedGit := git.NewUnifiedProvider(builtinCfg)

	// Providers that implement both Provider and ProviderHandler.
	cliProviders := []cliProvider{
		homebrew.New(builtinCfg, homebrew.NewRealCmdRunner()),
		apt.New(builtinCfg, apt.NewRealCmdRunner(sudoCmd)),
		npm.New(builtinCfg, npm.NewRealCmdRunner()),
		pnpm.New(builtinCfg, pnpm.NewRealCmdRunner()),
		uv.New(builtinCfg, uv.NewRealCmdRunner()),
		goinstall.New(builtinCfg, goinstall.NewRealCmdRunner()),
		cargo.New(builtinCfg, cargo.NewRealCmdRunner()),
		defaults.New(builtinCfg, defaults.NewRealCmdRunner()),
		duti.New(builtinCfg, duti.NewRealCmdRunner()),
		mas.New(builtinCfg, mas.NewRealCmdRunner()),
		vscodeext.New(builtinCfg, vscodeext.NewRealCmdRunner()),
		ansible.New(builtinCfg, ansible.NewRealCmdRunner()),
		bash.New(builtinCfg),
	}

	// providerOnly registers with the apply registry but NOT the CLI
	// dispatcher — these are the two git sub-providers, reached at
	// the CLI layer only through unifiedGit below.
	providerOnly := []provider.Provider{
		unifiedGit.Config(),
		unifiedGit.Clone(),
	}

	// Register all into the provider registry. Platform mismatch
	// (e.g. macOS-only `duti` on Linux) is silently skipped by
	// registry.Register. Apply the SAME platform check before
	// exposing the provider as a CLI subcommand, so `hams --help`
	// and the dispatch path agree with the internal registry —
	// otherwise Linux users see `defaults`/`duti`/`mas` in help,
	// try them, and get a confusing exec-not-found error.
	for _, p := range cliProviders {
		if err := registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", p.Manifest().Name, "error", err)
		}
		if provider.IsPlatformsMatch(p.Manifest().Platforms) {
			RegisterProvider(p)
		}
	}

	for _, p := range providerOnly {
		if err := registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", p.Manifest().Name, "error", err)
		}
	}

	// Register the unified `hams git` CLI entry point separately. It
	// has no apply-side Plan / Apply — the two sub-providers (git-
	// config and git-clone, already registered via providerOnly
	// above) handle that — so it doesn't belong in cliProviders
	// which asserts both interfaces.
	RegisterProvider(unifiedGit)
}

func loadBuiltinProviderConfig() *config.Config {
	flags := &provider.GlobalFlags{}
	stripGlobalFlags(os.Args[1:], flags)

	paths := resolvePaths(flags)

	cfg, err := config.Load(paths, flags.Store, flags.Profile)
	if err != nil {
		slog.Warn("failed to load config for builtin providers", "error", err)
		cfg = &config.Config{}
	}

	// config.Load already overlays --store (cycle 91) and --profile
	// (cycle 219); no further per-builtin manipulation needed. The
	// cfg returned here is shared by every provider's effectiveConfig
	// helper, which still re-applies the same overlays per call so
	// late-arriving flags from sub-CLI dispatch do not get lost.
	return cfg
}

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

	gitConfigProvider := git.NewConfigProvider(builtinCfg)
	gitCloneProvider := git.NewCloneProvider(builtinCfg)

	// Providers that implement both Provider and ProviderHandler.
	// gitConfigProvider / gitCloneProvider are NOT in this slice because
	// their CLI surface lives behind the unified `hams git` entry point
	// (registered separately in cliOnlyHandlers below); they ARE in the
	// applyOnlyProviders slice so apply / refresh still see them.
	//
	// vscodeext.Provider registers directly here as `hams code` — its
	// Manifest.Name + Provider.Name() both return "code" after the
	// code-ext → code rename, so no handler wrapper is needed.
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
		ansible.New(builtinCfg, ansible.NewRealCmdRunner()),
		bash.New(builtinCfg),
		vscodeext.New(builtinCfg, vscodeext.NewRealCmdRunner()),
	}

	// Apply-only providers: full Provider implementations, but their
	// CLI surface is hidden in favor of an aggregated entry point.
	// Apply / refresh still see them as separate Providers with their
	// own state + hamsfiles.
	applyOnlyProviders := []provider.Provider{
		gitConfigProvider,
		gitCloneProvider,
	}

	// CLI-handler-only registrations: aggregator handlers that do NOT
	// have their own apply/refresh state but expose a unified entry
	// point on top of one or more sub-providers. Per CLAUDE.md task
	// list:
	//   - `hams git` routes `git config <args>` and `git clone <args>`
	//     to the underlying ConfigProvider / CloneProvider.
	//
	// (Cursor support, when added, ships as a separate `cursor` provider
	// — not a cli_command override of `code`.)
	cliOnlyHandlers := []ProviderHandler{
		git.NewUnifiedHandler(gitConfigProvider, gitCloneProvider),
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

	for _, p := range applyOnlyProviders {
		if err := registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", p.Manifest().Name, "error", err)
		}
	}

	// CLI-only handlers — no provider-registry hook, so apply / refresh
	// continue to see the underlying sub-providers as separate Providers
	// with their own state files.
	for _, h := range cliOnlyHandlers {
		RegisterProvider(h)
	}
}

func loadBuiltinProviderConfig() *config.Config {
	flags := &provider.GlobalFlags{}
	stripGlobalFlags(os.Args[1:], flags)

	paths := resolvePaths(flags)

	cfg, err := config.Load(paths, flags.Store, flags.EffectiveTag())
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

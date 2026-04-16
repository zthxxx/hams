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

	// Providers that implement both Provider and ProviderHandler.
	cliProviders := []cliProvider{
		homebrew.New(builtinCfg, homebrew.NewRealCmdRunner()),
		apt.New(builtinCfg, apt.NewRealCmdRunner(sudoCmd)),
		npm.New(npm.NewRealCmdRunner()),
		pnpm.New(pnpm.NewRealCmdRunner()),
		uv.New(uv.NewRealCmdRunner()),
		goinstall.New(goinstall.NewRealCmdRunner()),
		cargo.New(cargo.NewRealCmdRunner()),
		git.NewConfigProvider(),
		git.NewCloneProvider(builtinCfg),
		defaults.New(builtinCfg, defaults.NewRealCmdRunner()),
		duti.New(duti.NewRealCmdRunner()),
		mas.New(mas.NewRealCmdRunner()),
		vscodeext.New(vscodeext.NewRealCmdRunner()),
		ansible.New(ansible.NewRealCmdRunner()),
	}

	// Providers that only implement Provider (no CLI handler).
	providerOnly := []provider.Provider{
		bash.New(),
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
}

func loadBuiltinProviderConfig() *config.Config {
	flags := &provider.GlobalFlags{}
	stripGlobalFlags(os.Args[1:], flags)

	paths := resolvePaths(flags)

	cfg, err := config.Load(paths, flags.Store)
	if err != nil {
		slog.Warn("failed to load config for builtin providers", "error", err)
		cfg = &config.Config{}
	}

	if flags.Store != "" {
		cfg.StorePath = flags.Store
	}
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
	}

	return cfg
}

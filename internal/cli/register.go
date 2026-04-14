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
)

// cliProvider is a provider that also implements ProviderHandler for CLI routing.
type cliProvider interface {
	provider.Provider
	ProviderHandler
}

// registerBuiltins registers all builtin providers in the registry.
// Each provider is instantiated once and used for both the provider registry
// and CLI handler routing (avoiding double instantiation).
func registerBuiltins(registry *provider.Registry) {
	builtinCfg := loadBuiltinProviderConfig()

	// Providers that implement both Provider and ProviderHandler.
	cliProviders := []cliProvider{
		homebrew.New(builtinCfg),
		apt.New(),
		npm.New(),
		pnpm.New(),
		uv.New(),
		goinstall.New(),
		cargo.New(),
		git.NewConfigProvider(),
		git.NewCloneProvider(builtinCfg),
		defaults.New(),
		duti.New(),
		mas.New(),
		vscodeext.New(),
		ansible.New(),
	}

	// Providers that only implement Provider (no CLI handler).
	providerOnly := []provider.Provider{
		bash.New(),
	}

	// Register all into the provider registry.
	for _, p := range cliProviders {
		if err := registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", p.Manifest().Name, "error", err)
		}
		// Same instance for CLI routing.
		RegisterProvider(p)
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

package cli

import (
	"log/slog"

	"github.com/zthxxx/hams/internal/provider"
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

// registerBuiltins registers all builtin providers in the registry.
func registerBuiltins(registry *provider.Registry) {
	builtins := []provider.Provider{
		bash.New(),
		homebrew.New(),
		apt.New(),
		npm.New(),
		pnpm.New(),
		uv.New(),
		goinstall.New(),
		cargo.New(),
		git.NewConfigProvider(),
		git.NewCloneProvider(),
		defaults.New(),
		duti.New(),
		mas.New(),
		vscodeext.New(),
	}

	for _, p := range builtins {
		if err := registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", p.Manifest().Name, "error", err)
		}
	}

	// Register CLI handlers for providers that implement ProviderHandler.
	cliHandlers := []ProviderHandler{
		homebrew.New(),
		apt.New(),
		npm.New(),
		pnpm.New(),
		uv.New(),
		goinstall.New(),
		cargo.New(),
		git.NewConfigProvider(),
		git.NewCloneProvider(),
		defaults.New(),
		duti.New(),
		mas.New(),
		vscodeext.New(),
	}
	for _, h := range cliHandlers {
		RegisterProvider(h)
	}
}

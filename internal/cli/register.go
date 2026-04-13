package cli

import (
	"log/slog"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/builtin/bash"
	"github.com/zthxxx/hams/internal/provider/builtin/homebrew"
)

// registerBuiltins registers all builtin providers in the registry.
func registerBuiltins(registry *provider.Registry) {
	builtins := []provider.Provider{
		bash.New(),
		homebrew.New(),
		// Additional builtins will be added here as they are implemented:
		// apt.New(), pnpm.New(), npm.New(), etc.
	}

	for _, p := range builtins {
		if err := registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", p.Manifest().Name, "error", err)
		}
	}

	// Register CLI handlers for providers that implement the ProviderHandler interface.
	RegisterProvider(homebrew.New())
}

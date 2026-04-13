package cli

import (
	"fmt"
	"strings"

	"github.com/zthxxx/hams/internal/cliutil"
)

// ProviderHandler is the interface that provider packages implement
// to handle CLI subcommands routed to them by hams.
type ProviderHandler interface {
	// Name returns the provider's CLI name (e.g., "brew", "pnpm").
	Name() string
	// DisplayName returns the provider's display name (e.g., "Homebrew", "pnpm").
	DisplayName() string
	// HandleCommand receives the full args after the provider name.
	HandleCommand(args []string, flags *cliutil.GlobalFlags) error
}

// providerRegistry holds registered provider handlers.
var providerRegistry = make(map[string]ProviderHandler)

// RegisterProvider registers a provider handler for CLI routing.
func RegisterProvider(handler ProviderHandler) {
	name := strings.ToLower(handler.Name())
	providerRegistry[name] = handler
}

// routeToProvider dispatches args to the provider handler.
// It strips global flags from args and handles --help interception.
func routeToProvider(handler ProviderHandler, args []string, flags *cliutil.GlobalFlags) error {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return showProviderHelp(handler)
		}
	}

	cleaned := stripGlobalFlags(args, flags)
	return handler.HandleCommand(cleaned, flags)
}

func showProviderHelp(handler ProviderHandler) error {
	fmt.Printf("hams %s — Manage %s packages\n\n", handler.Name(), handler.DisplayName())
	fmt.Printf("Usage:\n")
	fmt.Printf("  hams %s <subcommand> [args] [--hams:flags] [-- passthrough]\n\n", handler.Name())
	fmt.Printf("Provider subcommands are defined by the %s provider.\n", handler.DisplayName())
	fmt.Printf("Flags with --hams: prefix are consumed by hams, all others are forwarded.\n")
	fmt.Printf("Use -- to force-forward all subsequent args to the underlying command.\n")
	return nil
}

func stripGlobalFlags(args []string, flags *cliutil.GlobalFlags) []string {
	var cleaned []string
	skip := false
	for i, arg := range args {
		if skip {
			skip = false
			continue
		}
		switch {
		case arg == "--debug":
			flags.Debug = true
		case arg == "--dry-run":
			flags.DryRun = true
		case arg == "--json":
			flags.JSON = true
		case arg == "--no-color":
			flags.NoColor = true
		case strings.HasPrefix(arg, "--config="):
			flags.Config = strings.TrimPrefix(arg, "--config=")
		case arg == "--config" && i+1 < len(args):
			flags.Config = args[i+1]
			skip = true
		case strings.HasPrefix(arg, "--store="):
			flags.Store = strings.TrimPrefix(arg, "--store=")
		case arg == "--store" && i+1 < len(args):
			flags.Store = args[i+1]
			skip = true
		case strings.HasPrefix(arg, "--profile="):
			flags.Profile = strings.TrimPrefix(arg, "--profile=")
		case arg == "--profile" && i+1 < len(args):
			flags.Profile = args[i+1]
			skip = true
		default:
			cleaned = append(cleaned, arg)
		}
	}
	return cleaned
}

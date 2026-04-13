package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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
	// It is responsible for parsing subcommands, extracting --hams: flags,
	// and forwarding remaining args to the wrapped CLI.
	HandleCommand(args []string, flags *cliutil.GlobalFlags) error
}

// providerRegistry holds registered provider handlers.
var providerRegistry = make(map[string]ProviderHandler)

// RegisterProvider registers a provider handler for CLI routing.
func RegisterProvider(handler ProviderHandler) {
	name := strings.ToLower(handler.Name())
	providerRegistry[name] = handler
}

// AddProviderCommands creates Cobra commands for all registered providers
// and attaches them to the root command.
func AddProviderCommands(root *cobra.Command, flags *cliutil.GlobalFlags) {
	for _, handler := range providerRegistry {
		h := handler // capture for closure
		cmd := &cobra.Command{
			Use:                h.Name() + " [subcommand] [args...]",
			Short:              fmt.Sprintf("Manage %s packages", h.DisplayName()),
			DisableFlagParsing: true, // Provider handles its own flag parsing.
			RunE: func(_ *cobra.Command, args []string) error {
				return routeToProvider(h, args, flags)
			},
		}
		root.AddCommand(cmd)
	}
}

// routeToProvider dispatches args to the provider handler.
// It strips global flags from args (since DisableFlagParsing prevents Cobra from parsing them),
// and handles --help interception before forwarding to the provider.
func routeToProvider(handler ProviderHandler, args []string, flags *cliutil.GlobalFlags) error {
	// --help has highest priority: intercept before provider sees it.
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return showProviderHelp(handler)
		}
	}

	// Strip global flags that Cobra couldn't parse due to DisableFlagParsing.
	cleaned := stripGlobalFlags(args, flags)

	return handler.HandleCommand(cleaned, flags)
}

// stripGlobalFlags removes hams global flags from args and applies them to flags struct.
// This is needed because DisableFlagParsing on provider commands prevents Cobra from
// parsing persistent flags on the parent.
func stripGlobalFlags(args []string, flags *cliutil.GlobalFlags) []string {
	var cleaned []string
	for _, arg := range args {
		switch arg {
		case "--debug":
			flags.Debug = true
		case "--dry-run":
			flags.DryRun = true
		case "--json":
			flags.JSON = true
		case "--no-color":
			flags.NoColor = true
		default:
			cleaned = append(cleaned, arg)
		}
	}
	return cleaned
}

// showProviderHelp displays help for a provider.
func showProviderHelp(handler ProviderHandler) error {
	fmt.Printf("hams %s — Manage %s packages\n\n", handler.Name(), handler.DisplayName())
	fmt.Printf("Usage:\n")
	fmt.Printf("  hams %s <subcommand> [args] [--hams:flags] [-- passthrough]\n\n", handler.Name())
	fmt.Printf("Provider subcommands are defined by the %s provider.\n", handler.DisplayName())
	fmt.Printf("Flags with --hams: prefix are consumed by hams, all others are forwarded.\n")
	fmt.Printf("Use -- to force-forward all subsequent args to the underlying command.\n")
	return nil
}

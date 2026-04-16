package cli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zthxxx/hams/internal/provider"
)

// ProviderHandler is the interface that provider packages implement
// to handle CLI subcommands routed to them by hams.
type ProviderHandler interface {
	// Name returns the provider's CLI name (e.g., "brew", "pnpm").
	Name() string
	// DisplayName returns the provider's display name (e.g., "Homebrew", "pnpm").
	DisplayName() string
	// HandleCommand receives passthrough args and pre-split --hams- flags.
	HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error
}

// jsonFlag is the CLI flag that switches global output to JSON.
const jsonFlag = "--json"

// providerRegistry holds registered provider handlers.
var providerRegistry = make(map[string]ProviderHandler)

// RegisterProvider registers a provider handler for CLI routing.
func RegisterProvider(handler ProviderHandler) {
	name := strings.ToLower(handler.Name())
	providerRegistry[name] = handler
}

// routeToProvider dispatches args to the provider handler.
// Single pass: checks --help, strips global flags, and splits --hams- flags.
func routeToProvider(handler ProviderHandler, args []string, flags *provider.GlobalFlags) error {
	hamsFlags, passthrough := parseProviderArgs(args, flags)
	if hamsFlags == nil {
		// --help was found.
		return showProviderHelp(handler)
	}
	// Surface v1.1-deferred --hams-lucky usage so users who pass the
	// flag are not surprised by a silent no-op. See
	// openspec/specs/cli-architecture/spec.md (lucky-defer delta) for
	// the spec stance — Enricher interface has zero implementations
	// in v1; the flag parses but no provider reads hamsFlags["lucky"].
	if _, ok := hamsFlags["lucky"]; ok {
		slog.Warn("--hams-lucky is parsed but silently ignored in v1 (LLM enrichment deferred to v1.1)",
			"provider", handler.Name())
	}
	return handler.HandleCommand(context.TODO(), passthrough, hamsFlags, flags)
}

// parseProviderArgs processes provider args in a single pass:
// extracts global flags into flags, splits --hams- flags, and detects --help.
// Returns nil hamsFlags if --help/-h was found (signals help mode).
func parseProviderArgs(args []string, flags *provider.GlobalFlags) (hamsFlags map[string]string, passthrough []string) {
	hamsFlags = make(map[string]string)
	skip := false
	forceForward := false

	for i, arg := range args {
		if skip {
			skip = false
			continue
		}

		if forceForward {
			passthrough = append(passthrough, arg)
			continue
		}

		if arg == "--help" || arg == "-h" {
			return nil, nil
		}

		if arg == "--" {
			forceForward = true
			passthrough = append(passthrough, arg)
			continue
		}

		// --hams- prefixed flags.
		if strings.HasPrefix(arg, hamsFlagPrefix) {
			key, value := parseHamsFlag(arg[len(hamsFlagPrefix):])
			hamsFlags[key] = value
			continue
		}

		// Global flags.
		switch {
		case arg == "--debug":
			flags.Debug = true
		case arg == "--dry-run":
			flags.DryRun = true
		case arg == jsonFlag:
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
			passthrough = append(passthrough, arg)
		}
	}
	return hamsFlags, passthrough
}

func showProviderHelp(handler ProviderHandler) error {
	fmt.Printf("hams %s — Manage %s packages\n\n", handler.Name(), handler.DisplayName())
	fmt.Printf("Usage:\n")
	fmt.Printf("  hams %s <subcommand> [args] [--hams-flags] [-- passthrough]\n\n", handler.Name())
	fmt.Printf("Provider subcommands are defined by the %s provider.\n", handler.DisplayName())
	fmt.Printf("Flags with --hams- prefix are consumed by hams, all others are forwarded.\n")
	fmt.Printf("Use -- to force-forward all subsequent args to the underlying command.\n")
	return nil
}

// stripGlobalFlags extracts global flags from raw args (used during early bootstrap
// before urfave/cli has parsed, e.g., in register.go config loading).
func stripGlobalFlags(args []string, flags *provider.GlobalFlags) []string {
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
		case arg == jsonFlag:
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

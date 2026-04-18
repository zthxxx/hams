package cli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/storeinit"
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
func routeToProvider(ctx context.Context, handler ProviderHandler, args []string, flags *provider.GlobalFlags) error {
	hamsFlags, passthrough := parseProviderArgs(args, flags)
	if hamsFlags == nil {
		// --help was found.
		return showProviderHelp(handler)
	}
	// Cycle 242: honor --debug for per-provider CLI invocations.
	// Pre-cycle-242 only apply / refresh applied flags.Debug to slog
	// (via logging.Setup), so `hams cargo install foo --debug` parsed
	// the flag into flags.Debug but never raised the slog level →
	// the user got no extra output despite asking for it. Use the
	// stderr-only SetupDebugOnly so short commands don't open a per-
	// invocation log file (apply/refresh still call full Setup with
	// file rotation).
	//
	// Only fires when --debug is set so tests that install their own
	// slog.Default capture handler before invoking app.Run aren't
	// silently clobbered. (Cycle 243 makes the same condition the
	// invariant for the root Before hook too.)
	if flags.Debug {
		logging.SetupDebugOnly(true)
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

	// Collapse --tag / --profile into flags.Profile so every provider's
	// existing effectiveConfig overlay (which only reads flags.Profile)
	// honors `--tag=<t>` too. Fails fast on the conflict case so the
	// user sees the error here rather than in a downstream provider
	// call stack. Aliasing happens AFTER parseProviderArgs so
	// `hams --tag=macOS brew …` (root-level flag) and
	// `hams brew --tag=macOS …` (passthrough-slot flag) both land.
	if flags.Tag != "" || flags.Profile != "" {
		resolved, resolveErr := config.ResolveCLITagOverride(flags.Tag, flags.Profile)
		if resolveErr != nil {
			return resolveErr
		}
		flags.Profile = resolved
		flags.Tag = resolved
	}

	// Auto-scaffold a store on first-ever provider invocation. Turns
	// `hams brew install htop` on a pristine machine from an error
	// into "create the store + init git + record the install" in one
	// command. The storeinit package owns the materialization (git
	// init + go-git fallback + template writes + identity seeding);
	// the CLI layer just delegates. Skipped when --hams-no-scaffold
	// is set so tests and power users can opt out.
	if _, skip := hamsFlags["no-scaffold"]; !skip {
		scaffoldedPath, scaffErr := storeinit.Bootstrap(ctx, resolvePaths(flags), flags)
		if scaffErr != nil {
			return fmt.Errorf("auto-scaffold store: %w", scaffErr)
		}
		if flags.Store == "" {
			flags.Store = scaffoldedPath
		}
	}
	return handler.HandleCommand(ctx, passthrough, hamsFlags, flags)
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
			if hamsFlagFalsey(value) {
				// Cycle 162: explicit false-y values disable the flag.
				continue
			}
			hamsFlags[key] = value
			continue
		}

		// Global flags. Each bool flag accepts both the bare form
		// (`--debug`) and the explicit `=value` forms (`--debug=true`,
		// `--debug=false`) so the parser agrees with urfave/cli's
		// handling at the top-level entrypoint. Previously only the
		// bare form matched, so `hams apt --json=true install foo`
		// leaked `--json=true` through as a passthrough token which
		// apt-get then rejected with "option --json=true is not
		// understood".
		switch {
		case boolFlagMatch(arg, "--debug", &flags.Debug):
		case boolFlagMatch(arg, "--dry-run", &flags.DryRun):
		case boolFlagMatch(arg, jsonFlag, &flags.JSON):
		case boolFlagMatch(arg, "--no-color", &flags.NoColor):
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
		case strings.HasPrefix(arg, "--tag="):
			flags.Tag = strings.TrimPrefix(arg, "--tag=")
		case arg == "--tag" && i+1 < len(args):
			flags.Tag = args[i+1]
			skip = true
		default:
			passthrough = append(passthrough, arg)
		}
	}
	return hamsFlags, passthrough
}

// boolFlagMatch writes to *target if arg matches `flag`, `flag=true`,
// `flag=1`, `flag=false`, or `flag=0`, and returns true so the switch
// treats the arg as consumed. Unknown `flag=value` pairs fall through
// (treated as non-match) and end up in passthrough — that's the
// correct behavior for any flag we DON'T want to steal from the
// provider.
func boolFlagMatch(arg, flag string, target *bool) bool {
	switch arg {
	case flag, flag + "=true", flag + "=1":
		*target = true
		return true
	case flag + "=false", flag + "=0":
		*target = false
		return true
	}
	return false
}

func showProviderHelp(handler ProviderHandler) error {
	fmt.Printf("hams %s — %s\n\n", handler.Name(), providerUsageDescription(handler.Name(), handler.DisplayName()))
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
		// Same boolean-flag handling as parseProviderArgs so this
		// early-bootstrap path doesn't drift. `hams --json=true ...`
		// invoked before urfave/cli has parsed would otherwise leave
		// jsonFlag as an unconsumed arg in register.go.
		switch {
		case boolFlagMatch(arg, "--debug", &flags.Debug):
		case boolFlagMatch(arg, "--dry-run", &flags.DryRun):
		case boolFlagMatch(arg, jsonFlag, &flags.JSON):
		case boolFlagMatch(arg, "--no-color", &flags.NoColor):
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
		case strings.HasPrefix(arg, "--tag="):
			flags.Tag = strings.TrimPrefix(arg, "--tag=")
		case arg == "--tag" && i+1 < len(args):
			flags.Tag = args[i+1]
			skip = true
		default:
			cleaned = append(cleaned, arg)
		}
	}
	return cleaned
}

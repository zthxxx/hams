// Package cli implements command definitions and routes CLI invocations to providers.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
	"github.com/zthxxx/hams/internal/version"
)

// globalFlags extracts GlobalFlags from the urfave/cli context.
//
// `--tag` is the canonical flag; `--profile` is a legacy alias kept as a
// separate flag so config.ResolveCLITagOverride can detect conflicts.
// Callers that need the resolved single value SHOULD call
// config.ResolveCLITagOverride(flags.Tag, flags.Profile) rather than
// picking one field themselves — that keeps the "loud error when they
// disagree" contract in a single place.
func globalFlags(cmd *cli.Command) *provider.GlobalFlags {
	return &provider.GlobalFlags{
		Debug:   cmd.Bool("debug"),
		DryRun:  cmd.Bool("dry-run"),
		JSON:    cmd.Bool("json"),
		NoColor: cmd.Bool("no-color"),
		Config:  cmd.String("config"),
		Store:   cmd.String("store"),
		Tag:     cmd.String("tag"),
		Profile: cmd.String("profile"),
	}
}

// hasJSONFlag detects --json in raw argv since urfave/cli's parsed
// value is not reachable from the top-level Execute error path.
// Accepts the three forms bash users commonly type: `--json`,
// `--json=true`, `--json=false`. Without this, `hams --json=true
// apply` emitted the text error instead of a JSON object — breaking
// scripts that use the explicit boolean form.
//
// Later `--json=X` arguments override earlier ones (so
// `--json --json=false` is treated as false, matching urfave/cli's
// right-wins behavior).
func hasJSONFlag(args []string) bool {
	out := false
	for _, arg := range args {
		switch arg {
		case jsonFlag, jsonFlag + "=true", jsonFlag + "=1":
			out = true
		case jsonFlag + "=false", jsonFlag + "=0":
			out = false
		}
	}
	return out
}

// resolvePaths returns config.Paths with --config flag applied.
// `--config=~/foo.yaml` is expanded to the real home path so
// `hams --config=~/my.yaml` does what users expect — shells do NOT
// expand `~` inside `--flag=~/path` (only as the leading word of a
// separate argument), so hams does it itself. Same expansion also
// applies to --store via the apply/refresh entry points (cycle 89).
func resolvePaths(flags *provider.GlobalFlags) config.Paths {
	paths := config.ResolvePaths()
	if flags.Config != "" {
		expanded, _ := config.ExpandHome(flags.Config) //nolint:errcheck // best-effort; returns input unchanged on error
		flags.Config = expanded
		paths.ConfigHome = filepath.Dir(expanded)
		paths.ConfigFilePath = expanded
	}
	if flags.Store != "" {
		if expanded, expErr := config.ExpandHome(flags.Store); expErr == nil {
			flags.Store = expanded
		}
	}
	return paths
}

// NewApp creates the top-level hams urfave/cli application.
func NewApp(registry *provider.Registry, sudoAcq sudo.Acquirer) *cli.Command {
	flags := globalFlagDefs()

	app := &cli.Command{
		Name:    "hams",
		Usage:   "Declarative IaC environment management for workstations",
		Version: version.Brief(),
		Description: `hams (hamster) wraps existing package managers to auto-record installations
into declarative YAML config files, enabling one-command environment
restoration on new machines.

Use 'hams <provider> install <package>' to install and record.
Use 'hams apply' to replay all installations from config.`,
		Flags: flags,
		// Cycle 243: --debug fires for every Action via the Before hook,
		// not only per-provider CLI dispatch (cycle 242). `hams config
		// get key --debug`, `hams list --debug`, `hams store status
		// --debug`, etc. now produce the same level-promoted slog output
		// as `hams cargo install foo --debug`. apply / refresh still
		// override slog with the full file-rotating Setup later in their
		// Action — that's intentional and harmless (Setup is a superset
		// of SetupDebugOnly).
		//
		// Only fires when --debug is set. The default branch leaves the
		// caller-installed slog.Default alone — important for tests that
		// install their own capture handler before app.Run; if Before
		// unconditionally called SetupDebugOnly, those handlers would
		// be silently overwritten and the tests would lose visibility
		// into the warnings/errors they're asserting on.
		Before: func(_ context.Context, cmd *cli.Command) (context.Context, error) {
			if cmd.Bool("debug") {
				logging.SetupDebugOnly(true)
			}
			return nil, nil
		},
		Commands: []*cli.Command{
			applyCmd(registry, sudoAcq),
			refreshCmd(registry),
			configCmd(),
			storeCmd(),
			listCmd(registry),
			selfUpgradeCmd(),
			versionCmd(),
		},
		// Enable urfave/cli's built-in fuzzy suggestion engine so
		// `hams aply` prints "Did you mean 'apply'?" instead of
		// silently dropping into the default help text.
		Suggest: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			// `hams bogus-command` used to fall through to the root
			// Action and print the help text with exit 0 — scripts
			// couldn't detect typo'd subcommands and users had to
			// re-read the command list to figure out what went wrong.
			// Now: any trailing positional arg that didn't match a
			// subcommand name is treated as a usage error with a
			// pointer back at `--help`, plus a Levenshtein-closest
			// suggestion from the known command list (same engine
			// that powers `hams help <typo>`).
			if cmd.Args().Len() > 0 {
				unknown := cmd.Args().First()
				suggestions := []string{"Run 'hams --help' to see the full list of subcommands"}
				if suggested := cli.SuggestCommand(cmd.Commands, unknown); suggested != "" {
					suggestions = append([]string{fmt.Sprintf("Did you mean 'hams %s'?", suggested)}, suggestions...)
				}
				return hamserr.NewUserError(hamserr.ExitUsageError,
					fmt.Sprintf("unknown command: %q", unknown),
					suggestions...,
				)
			}
			return cli.ShowAppHelp(cmd)
		},
	}

	// Add provider commands dynamically, sorted alphabetically so
	// `hams --help` lists providers in a stable order across runs
	// (Go map iteration is randomized, which produced a different
	// provider order every invocation — confusing for users and
	// breaks reproducible help snapshots in tests/docs).
	names := make([]string, 0, len(providerRegistry))
	for n := range providerRegistry {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h := providerRegistry[n] // capture
		app.Commands = append(app.Commands, &cli.Command{
			Name:            h.Name(),
			Usage:           providerUsageDescription(h.Name(), h.DisplayName()),
			SkipFlagParsing: true,
			Action: func(ctx context.Context, cmd *cli.Command) error {
				return routeToProvider(ctx, h, cmd.Args().Slice(), globalFlags(cmd))
			},
		})
	}

	return app
}

// providerUsageDescription returns the help-line text for a provider's
// `hams <name>` subcommand. Maps each shipped provider to a sensible
// noun ("packages" / "config entries" / "playbooks" / etc.) instead
// of the previous one-size-fits-all "Manage X packages" — that
// default was wrong for the 6 non-package builtins (git-config does
// NOT manage packages, etc.).
//
// Unknown providers (e.g., future external plugins) fall through to
// the package-class default so the help text is never empty.
func providerUsageDescription(name, displayName string) string {
	switch name {
	case "git":
		return "Manage git config + clones (subcommands: config, clone)"
	case "git-config":
		return "Manage git config entries"
	case "git-clone":
		return "Manage cloned git repositories"
	case "defaults":
		return "Manage macOS defaults preferences"
	case "duti":
		return "Manage macOS default-app associations"
	case "bash": //nolint:goconst // single-use provider name; extracting a const for just this case would clutter the switch.
		return "Run bash provisioning scripts"
	case "ansible":
		return "Run Ansible playbooks"
	case "code":
		return "Manage VS Code extensions (Cursor lives behind a separate `cursor` provider)"
	}
	// Package-class default (brew, apt, pnpm, npm, uv, goinstall,
	// cargo, mas) — accurate for installed packages.
	return fmt.Sprintf("Manage %s packages", displayName)
}

// Execute runs the root command with all subcommands wired up.
func Execute() {
	i18n.Init()

	// Create provider registry and register builtins.
	registry := provider.NewRegistry()
	registerBuiltins(registry, &sudo.Builder{})

	app := NewApp(registry, sudo.NewManager())

	// Root context cancels on SIGINT/SIGTERM so long-running providers
	// (brew install, apt-get upgrade, etc.) can observe Ctrl+C and
	// unwind cleanly. stop() must run before os.Exit — defer would not
	// fire after os.Exit, so we call it explicitly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := app.Run(ctx, os.Args)
	stop()

	if err != nil {
		PrintError(err, hasJSONFlag(os.Args))

		exitCode := hamserr.ExitGeneralError
		var ue *hamserr.UserFacingError
		if errors.As(err, &ue) {
			exitCode = ue.Code
		}
		os.Exit(exitCode)
	}
}

func globalFlagDefs() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "debug", Usage: "Enable verbose debug logging"},
		&cli.BoolFlag{Name: "dry-run", Usage: "Show what would be done without making changes"},
		&cli.BoolFlag{Name: "json", Usage: "Output in JSON format (machine-readable)"},
		&cli.BoolFlag{Name: "no-color", Usage: "Disable colored output"},
		&cli.StringFlag{Name: "config", Usage: "Override config file path"},
		&cli.StringFlag{Name: "store", Usage: "Override store directory path"},
		// --tag and --profile are registered separately (NOT as aliases) so
		// config.ResolveCLITagOverride can detect `--tag=macOS --profile=linux`
		// conflict. An alias collapses them too early (urfave/cli's last-
		// value-wins makes the conflict impossible to observe).
		&cli.StringFlag{
			Name:  "tag",
			Usage: "Active profile tag (canonical). Precedence: --tag > --profile > config tag > 'default'.",
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "Legacy alias of --tag. If both are supplied with different values, hams errors out.",
		},
	}
}

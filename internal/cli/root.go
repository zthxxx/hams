// Package cli implements command definitions and routes CLI invocations to providers.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/version"
)

// globalFlags extracts GlobalFlags from the urfave/cli context.
func globalFlags(cmd *cli.Command) *provider.GlobalFlags {
	return &provider.GlobalFlags{
		Debug:   cmd.Bool("debug"),
		DryRun:  cmd.Bool("dry-run"),
		JSON:    cmd.Bool("json"),
		NoColor: cmd.Bool("no-color"),
		Config:  cmd.String("config"),
		Store:   cmd.String("store"),
		Profile: cmd.String("profile"),
	}
}

// resolvePaths returns config.Paths with --config flag applied.
func resolvePaths(flags *provider.GlobalFlags) config.Paths {
	paths := config.ResolvePaths()
	if flags.Config != "" {
		paths.ConfigHome = filepath.Dir(flags.Config)
		paths.ConfigFilePath = flags.Config
	}
	return paths
}

// NewApp creates the top-level hams urfave/cli application.
func NewApp(registry *provider.Registry) *cli.Command {
	flags := globalFlagDefs()

	app := &cli.Command{
		Name:    "hams",
		Usage:   "Declarative IaC environment management for workstations",
		Version: version.Version(),
		Description: `hams (hamster) wraps existing package managers to auto-record installations
into declarative YAML config files, enabling one-command environment
restoration on new machines.

Use 'hams <provider> install <package>' to install and record.
Use 'hams apply' to replay all installations from config.`,
		Flags: flags,
		Commands: []*cli.Command{
			applyCmd(registry),
			refreshCmd(registry),
			configCmd(),
			storeCmd(),
			listCmd(registry),
			selfUpgradeCmd(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowAppHelp(cmd)
		},
	}

	// Add provider commands dynamically.
	for _, handler := range providerRegistry {
		h := handler // capture
		app.Commands = append(app.Commands, &cli.Command{
			Name:            h.Name(),
			Usage:           fmt.Sprintf("Manage %s packages", h.DisplayName()),
			SkipFlagParsing: true,
			Action: func(_ context.Context, cmd *cli.Command) error {
				return routeToProvider(h, cmd.Args().Slice(), globalFlags(cmd))
			},
		})
	}

	return app
}

// Execute runs the root command with all subcommands wired up.
func Execute() {
	i18n.Init()

	// Create provider registry and register builtins.
	registry := provider.NewRegistry()
	registerBuiltins(registry)

	app := NewApp(registry)

	if err := app.Run(context.Background(), os.Args); err != nil {
		flags := &provider.GlobalFlags{}
		// Check if --json was passed.
		for _, arg := range os.Args {
			if arg == "--json" {
				flags.JSON = true
			}
		}
		PrintError(err, flags.JSON)

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
		&cli.StringFlag{Name: "profile", Usage: "Override active profile tag"},
	}
}

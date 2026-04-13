// Package cli implements Cobra command definitions and routes CLI invocations to providers.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zthxxx/hams/internal/cliutil"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/version"
)

// NewRootCmd creates the top-level hams Cobra command.
func NewRootCmd() (*cobra.Command, *cliutil.GlobalFlags) {
	flags := &cliutil.GlobalFlags{}

	root := &cobra.Command{
		Use:   "hams [global-flags] <command> [args]",
		Short: "Declarative IaC environment management for workstations",
		Long: `hams (hamster) wraps existing package managers to auto-record installations
into declarative YAML config files, enabling one-command environment
restoration on new machines.

Use 'hams <provider> install <package>' to install and record.
Use 'hams apply' to replay all installations from config.`,
		Version:       version.Version(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Global flags — must appear between `hams` and the provider/command name.
	pf := root.PersistentFlags()
	pf.BoolVar(&flags.Debug, "debug", false, "Enable verbose debug logging")
	pf.BoolVar(&flags.DryRun, "dry-run", false, "Show what would be done without making changes")
	pf.BoolVar(&flags.JSON, "json", false, "Output in JSON format (machine-readable)")
	pf.BoolVar(&flags.NoColor, "no-color", false, "Disable colored output")
	pf.StringVar(&flags.Config, "config", "", "Override config file path")
	pf.StringVar(&flags.Store, "store", "", "Override store directory path")
	pf.StringVar(&flags.Profile, "profile", "", "Override active profile tag")

	// Set version template.
	root.SetVersionTemplate(fmt.Sprintf("%s\n", version.Info()))

	return root, flags
}

// Execute runs the root command with all subcommands wired up.
func Execute() {
	root, flags := NewRootCmd()

	// Create provider registry and register builtins.
	registry := provider.NewRegistry()
	registerBuiltins(registry)

	// Add subcommands.
	root.AddCommand(NewApplyCmd(flags, registry))
	root.AddCommand(NewRefreshCmd(flags, registry))
	root.AddCommand(NewConfigCmd(flags))
	root.AddCommand(NewStoreCmd(flags))
	root.AddCommand(NewListCmd(flags, registry))
	root.AddCommand(NewSelfUpgradeCmd())
	AddProviderCommands(root, flags)

	if err := root.Execute(); err != nil {
		PrintError(err, flags.JSON)
		os.Exit(cliutil.ExitGeneralError)
	}
}

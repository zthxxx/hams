package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zthxxx/hams/internal/cliutil"
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// NewRefreshCmd creates the `hams refresh` command.
func NewRefreshCmd(flags *cliutil.GlobalFlags, registry *provider.Registry) *cobra.Command {
	var only, except string

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Probe environment to update state with observed reality",
		Long: `Refresh probes all known resources in state to detect drift.
Only resources already tracked in state are probed — no new resources are discovered.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRefresh(cmd.Context(), flags, registry, only, except)
		},
	}

	cmd.Flags().StringVar(&only, "only", "", "Only refresh these providers (comma-separated)")
	cmd.Flags().StringVar(&except, "except", "", "Skip these providers (comma-separated)")

	return cmd
}

func runRefresh(ctx context.Context, flags *cliutil.GlobalFlags, registry *provider.Registry, only, except string) error {
	paths := config.ResolvePaths()
	cfg, err := config.Load(paths, flags.Store)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	stateDir := cfg.StateDir()
	providers := filterProviders(registry.Ordered(cfg.ProviderPriority), only, except)

	slog.Info("refreshing state", "providers", len(providers))
	provider.ProbeAll(ctx, providers, stateDir, cfg.MachineID)

	fmt.Printf("Refresh complete: %d providers probed\n", len(providers))
	return nil
}

// NewConfigCmd creates the `hams config` command.
func NewConfigCmd(flags *cliutil.GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and manage hams configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show all configuration values",
		RunE: func(_ *cobra.Command, _ []string) error {
			paths := config.ResolvePaths()
			cfg, loadErr := config.Load(paths, flags.Store)
			if loadErr != nil {
				return fmt.Errorf("loading config: %w", loadErr)
			}
			fmt.Printf("Config home:       %s\n", logging.TildePath(paths.ConfigHome))
			fmt.Printf("Data home:         %s\n", logging.TildePath(paths.DataHome))
			fmt.Printf("Global config:     %s\n", logging.TildePath(paths.GlobalConfigPath()))
			fmt.Printf("Profile tag:       %s\n", cfg.ProfileTag)
			fmt.Printf("Machine ID:        %s\n", cfg.MachineID)
			fmt.Printf("Store path:        %s\n", logging.TildePath(cfg.StorePath))
			fmt.Printf("LLM CLI:           %s\n", cfg.LLMCLI)
			fmt.Printf("Provider priority: %v\n", cfg.ProviderPriority)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			paths := config.ResolvePaths()
			cfg, loadErr := config.Load(paths, flags.Store)
			if loadErr != nil {
				return fmt.Errorf("loading config: %w", loadErr)
			}
			return printConfigKey(cfg, paths, args[0])
		},
	})

	return cmd
}

func printConfigKey(cfg *config.Config, paths config.Paths, key string) error {
	switch key {
	case "profile_tag":
		fmt.Println(cfg.ProfileTag)
	case "machine_id":
		fmt.Println(cfg.MachineID)
	case "store_path":
		fmt.Println(logging.TildePath(cfg.StorePath))
	case "store_repo":
		fmt.Println(cfg.StoreRepo)
	case "llm_cli":
		fmt.Println(cfg.LLMCLI)
	case "config_home":
		fmt.Println(logging.TildePath(paths.ConfigHome))
	case "data_home":
		fmt.Println(logging.TildePath(paths.DataHome))
	default:
		return cliutil.NewUserError(cliutil.ExitUsageError,
			fmt.Sprintf("unknown config key %q", key),
			"Valid keys: profile_tag, machine_id, store_path, store_repo, llm_cli, config_home, data_home",
		)
	}
	return nil
}

// NewStoreCmd creates the `hams store` command.
func NewStoreCmd(flags *cliutil.GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "store",
		Short: "Show store directory path and status",
		RunE: func(_ *cobra.Command, _ []string) error {
			paths := config.ResolvePaths()
			cfg, err := config.Load(paths, flags.Store)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			storePath := cfg.StorePath
			if storePath == "" {
				return cliutil.NewUserError(cliutil.ExitUsageError,
					"no store directory configured",
					"Set store_path in ~/.config/hams/hams.config.yaml",
					"Or use 'hams apply --from-repo=<user/repo>' to set up a store",
				)
			}

			fmt.Printf("Store path:    %s\n", logging.TildePath(storePath))
			fmt.Printf("Profile dir:   %s\n", logging.TildePath(cfg.ProfileDir()))
			fmt.Printf("State dir:     %s\n", logging.TildePath(cfg.StateDir()))

			// Count hamsfiles.
			profileDir := cfg.ProfileDir()
			entries, readErr := os.ReadDir(profileDir)
			if readErr != nil {
				fmt.Printf("Profile dir:   (not found)\n")
				return nil //nolint:nilerr // intentional: missing profile dir is not an error, just means nothing to show
			}

			hamsfiles := 0
			for _, e := range entries {
				if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
					hamsfiles++
				}
			}
			fmt.Printf("Hamsfiles:     %d\n", hamsfiles)

			return nil
		},
	}
}

// NewListCmd creates the `hams list` command.
func NewListCmd(flags *cliutil.GlobalFlags, registry *provider.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all managed resources across providers",
		RunE: func(_ *cobra.Command, _ []string) error {
			paths := config.ResolvePaths()
			cfg, err := config.Load(paths, flags.Store)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			stateDir := cfg.StateDir()
			for _, p := range registry.Ordered(cfg.ProviderPriority) {
				name := p.Manifest().Name
				statePath := filepath.Join(stateDir, name+".state.yaml")

				sf, loadErr := state.Load(statePath)
				if loadErr != nil {
					continue
				}

				if len(sf.Resources) == 0 {
					continue
				}

				fmt.Printf("\n%s (%d resources):\n", p.Manifest().DisplayName, len(sf.Resources))
				for id, r := range sf.Resources {
					status := string(r.State)
					version := ""
					if r.Version != "" {
						version = " " + r.Version
					}
					fmt.Printf("  %-30s %s%s\n", id, status, version)
				}
			}

			return nil
		},
	}
}

// NewSelfUpgradeCmd creates the `hams self-upgrade` command.
func NewSelfUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "self-upgrade",
		Short: "Upgrade hams to the latest version",
		Long: `Detects how hams was installed and upgrades accordingly:
- Binary download: fetches latest from GitHub Releases
- Homebrew: runs 'brew upgrade hams'`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: implement in task 3.15 detail.
			fmt.Println("self-upgrade: not yet implemented")
			fmt.Println("For now, use: brew upgrade zthxxx/tap/hams")
			fmt.Println("Or download from: https://github.com/zthxxx/hams/releases")
			return nil
		},
	}
}

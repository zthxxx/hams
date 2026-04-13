package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/zthxxx/hams/internal/cliutil"
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

func refreshCmd(registry *provider.Registry) *cli.Command {
	return &cli.Command{
		Name:  "refresh",
		Usage: "Probe environment to update state with observed reality",
		Description: `Refresh probes all known resources in state to detect drift.
Only resources already tracked in state are probed — no new resources are discovered.`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "only", Usage: "Only refresh these providers (comma-separated)"},
			&cli.StringFlag{Name: "except", Usage: "Skip these providers (comma-separated)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
			return runRefresh(ctx, flags, registry, cmd.String("only"), cmd.String("except"))
		},
	}
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

func configCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "View and manage hams configuration",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "Show all configuration values",
				Action: func(_ context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
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
			},
			{
				Name:      "get",
				Usage:     "Get a configuration value",
				ArgsUsage: "<key>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 1 {
						return cliutil.NewUserError(cliutil.ExitUsageError,
							"config get requires a key",
							"Usage: hams config get <key>",
						)
					}
					flags := globalFlags(cmd)
					paths := config.ResolvePaths()
					cfg, loadErr := config.Load(paths, flags.Store)
					if loadErr != nil {
						return fmt.Errorf("loading config: %w", loadErr)
					}
					return printConfigKey(cfg, paths, cmd.Args().First())
				},
			},
		},
	}
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

func storeCmd() *cli.Command {
	return &cli.Command{
		Name:  "store",
		Usage: "Show store directory path and status",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
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

			profileDir := cfg.ProfileDir()
			entries, readErr := os.ReadDir(profileDir)
			if readErr != nil {
				fmt.Printf("Profile dir:   (not found)\n")
				return nil //nolint:nilerr // intentional: missing profile dir is not an error
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

func listCmd(registry *provider.Registry) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all managed resources across providers",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
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
					ver := ""
					if r.Version != "" {
						ver = " " + r.Version
					}
					fmt.Printf("  %-30s %s%s\n", id, status, ver)
				}
			}

			return nil
		},
	}
}

func selfUpgradeCmd() *cli.Command {
	return &cli.Command{
		Name:  "self-upgrade",
		Usage: "Upgrade hams to the latest version",
		Description: `Detects how hams was installed and upgrades accordingly:
- Binary download: fetches latest from GitHub Releases
- Homebrew: runs 'brew upgrade hams'`,
		Action: func(_ context.Context, _ *cli.Command) error {
			fmt.Println("self-upgrade: not yet implemented")
			fmt.Println("For now, use: brew upgrade zthxxx/tap/hams")
			fmt.Println("Or download from: https://github.com/zthxxx/hams/releases")
			return nil
		},
	}
}

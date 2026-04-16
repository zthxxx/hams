package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/selfupdate"
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

func runRefresh(ctx context.Context, flags *provider.GlobalFlags, registry *provider.Registry, only, except string) (retErr error) {
	paths := resolvePaths(flags)
	cfg, err := config.Load(paths, flags.Store)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
	}

	// OTel session is opt-in via HAMS_OTEL=1 (internal/cli/otel.go).
	// Refresh doesn't invoke provider.Execute, so there are no
	// per-provider child spans from executor.go — but the root span
	// + session span machinery still capture the refresh duration
	// and any RecordMetric calls we add later. Keeps the surface
	// parallel with runApply.
	otelSess := maybeStartOTelSession(paths.DataHome, "hams.refresh")
	defer func() {
		status := otelStatusOK
		if retErr != nil {
			status = otelStatusError
		}
		otelSess.End(context.Background(), status)
	}()

	stateDir := cfg.StateDir()
	profileDir := cfg.ProfileDir()

	// Two-stage provider filter (same shape as runApply):
	//   Stage 1 — artifact presence (hamsfile OR state file).
	//   Stage 2 — user-supplied --only / --except.
	allProviders := registry.Ordered(cfg.ProviderPriority)
	stageOneProviders := provider.FilterByArtifacts(allProviders, profileDir, stateDir)
	for _, p := range allProviders {
		if !provider.HasArtifacts(p, profileDir, stateDir) {
			slog.Debug("provider skipped (no hamsfile or state file)", "provider", p.Manifest().Name)
		}
	}
	providers, filterErr := filterProviders(stageOneProviders, only, except, registry.Names())
	if filterErr != nil {
		return filterErr
	}
	if len(providers) == 0 {
		// Distinguish stage-1 empty (no artifacts anywhere) from stage-2
		// empty (artifacts exist but --only/--except excluded them all).
		switch {
		case len(stageOneProviders) == 0:
			fmt.Println("No providers match: no hamsfile or state file present for any registered provider.")
		default:
			fmt.Println("No providers match: --only/--except excluded every provider that has artifacts.")
		}
		return nil
	}

	// Attach root-span attributes now that profile + provider count
	// are resolved. Per observability spec, the root span carries
	// hams.profile + hams.providers.count.
	otelSess.AttachRootAttrs(cfg.ProfileTag, len(providers))

	slog.Info("refreshing state", "providers", len(providers))
	probeStart := time.Now()
	probeResults := provider.ProbeAll(ctx, providers, stateDir, cfg.MachineID)
	probeElapsed := time.Since(probeStart).Milliseconds()
	for name, sf := range probeResults {
		statePath := filepath.Join(stateDir, name+".state.yaml")
		if saveErr := sf.Save(statePath); saveErr != nil {
			slog.Error("failed to save probed state", "provider", name, "error", saveErr)
		}
	}

	// hams.probe.duration metric per observability spec — elapsed
	// time for the refresh ProbeAll phase. Emitted only when OTel
	// is active; no-op when otelSess.Session() is nil.
	if sess := otelSess.Session(); sess != nil {
		sess.RecordMetric("hams.probe.duration", float64(probeElapsed), "ms", map[string]string{
			"hams.command":         "refresh",
			"hams.providers.count": strconv.Itoa(len(providers)),
		})
	}

	// ProbeAll swallows per-provider probe errors (best-effort) and
	// drops the failing provider from the results map. Report the
	// honest probed/planned ratio so a user who ran `refresh --only=brew`
	// on a fresh-Mac without brew sees "0/1 providers probed", not a
	// misleading "1 providers probed".
	probed := len(probeResults)
	planned := len(providers)
	if probed == planned {
		fmt.Printf("Refresh complete: %d providers probed\n", planned)
	} else {
		fmt.Printf("Refresh complete: %d/%d providers probed (%d probe error(s); see log for details)\n",
			probed, planned, planned-probed)
	}
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
					paths := resolvePaths(flags)
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
						return hamserr.NewUserError(hamserr.ExitUsageError,
							"config get requires a key",
							"Usage: hams config get <key>",
						)
					}
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, loadErr := config.Load(paths, flags.Store)
					if loadErr != nil {
						return fmt.Errorf("loading config: %w", loadErr)
					}
					return printConfigKey(cfg, paths, flags.Store, cmd.Args().First())
				},
			},
			{
				Name:      "set",
				Usage:     "Set a configuration value",
				ArgsUsage: "<key> <value>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 2 { //nolint:mnd // exactly 2 args required: key and value
						return hamserr.NewUserError(hamserr.ExitUsageError,
							"config set requires a key and value",
							"Usage: hams config set <key> <value>",
							"Valid keys: "+strings.Join(config.ValidConfigKeys, ", "),
						)
					}
					key := cmd.Args().Get(0)
					value := cmd.Args().Get(1)
					// Accept either whitelisted keys OR keys matching a
					// sensitive pattern (token/secret/password/credential).
					// The sensitive branch supports deferred integrations
					// like `notification.bark_token` per schema-design spec
					// (fix-v1-planning-gaps delta). Without this allowance,
					// `hams config set notification.bark_token abc` was
					// rejected despite the spec saying it auto-routes to
					// hams.config.local.yaml.
					if !config.IsValidConfigKey(key) && !config.IsSensitiveKey(key) {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							fmt.Sprintf("unknown config key %q", key),
							"Valid keys: "+strings.Join(config.ValidConfigKeys, ", "),
							"Or use a key containing token/key/secret/password/credential (auto-routes to .local.yaml)",
						)
					}
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					if err := config.WriteConfigKey(paths, flags.Store, key, value); err != nil {
						return fmt.Errorf("writing config: %w", err)
					}
					target := "global config"
					if config.IsSensitiveKey(key) {
						target = "local config"
					}
					fmt.Printf("Set %s = %s (in %s)\n", key, value, target)
					return nil
				},
			},
			{
				Name:  "edit",
				Usage: "Open the global config file in your editor",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					configPath := paths.GlobalConfigPath()

					// Ensure the config file exists.
					if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
						if mkdirErr := os.MkdirAll(filepath.Dir(configPath), 0o750); mkdirErr != nil {
							return fmt.Errorf("creating config directory: %w", mkdirErr)
						}
						if writeErr := os.WriteFile(configPath, []byte("# hams global configuration\n"), 0o600); writeErr != nil {
							return fmt.Errorf("creating config file: %w", writeErr)
						}
					}

					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = os.Getenv("VISUAL")
					}
					if editor == "" {
						editor = "vi"
					}

					editorCmd := exec.CommandContext(ctx, editor, configPath) //nolint:gosec // editor is user-specified via $EDITOR
					editorCmd.Stdin = os.Stdin
					editorCmd.Stdout = os.Stdout
					editorCmd.Stderr = os.Stderr
					return editorCmd.Run()
				},
			},
		},
	}
}

func printConfigKey(cfg *config.Config, paths config.Paths, storePath, key string) error {
	switch key {
	case "profile_tag":
		fmt.Println(cfg.ProfileTag)
		return nil
	case "machine_id":
		fmt.Println(cfg.MachineID)
		return nil
	case "store_path":
		fmt.Println(logging.TildePath(cfg.StorePath))
		return nil
	case "store_repo":
		fmt.Println(cfg.StoreRepo)
		return nil
	case "llm_cli":
		fmt.Println(cfg.LLMCLI)
		return nil
	case "config_home":
		fmt.Println(logging.TildePath(paths.ConfigHome))
		return nil
	case "data_home":
		fmt.Println(logging.TildePath(paths.DataHome))
		return nil
	}

	// Arbitrary sensitive keys (e.g., notification.bark_token) aren't
	// struct fields — read them directly from the routed file so `get`
	// symmetrically retrieves whatever `set` wrote.
	if config.IsSensitiveKey(key) {
		value, ok, err := config.ReadRawConfigKey(paths, storePath, key)
		if err != nil {
			return err
		}
		if !ok {
			return nil // key unset → empty output, exit 0 (scripting-friendly)
		}
		fmt.Println(value)
		return nil
	}

	return hamserr.NewUserError(hamserr.ExitUsageError,
		fmt.Sprintf("unknown config key %q", key),
		"Valid keys: profile_tag, machine_id, store_path, store_repo, llm_cli, config_home, data_home",
		"Or use a key containing token/key/secret/password/credential (reads from .local.yaml)",
	)
}

func storeCmd() *cli.Command {
	storeStatusAction := func(_ context.Context, cmd *cli.Command) error {
		flags := globalFlags(cmd)
		paths := resolvePaths(flags)
		cfg, err := config.Load(paths, flags.Store)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		storePath := cfg.StorePath
		if storePath == "" {
			return hamserr.NewUserError(hamserr.ExitUsageError,
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
	}

	return &cli.Command{
		Name:   "store",
		Usage:  "Manage the hams store directory",
		Action: storeStatusAction,
		Commands: []*cli.Command{
			{
				Name:   "status",
				Usage:  "Show the current store path, profile, and hamsfile count",
				Action: storeStatusAction,
			},
			{
				Name:  "init",
				Usage: "Initialize a new store directory structure",
				Action: func(_ context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, err := config.Load(paths, flags.Store)
					if err != nil {
						return fmt.Errorf("loading config: %w", err)
					}

					storePath := cfg.StorePath
					if storePath == "" {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							"no store directory configured",
							"Set store_path first: hams config set store_path <path>",
						)
					}

					// Create profile directory.
					profileDir := cfg.ProfileDir()
					if mkErr := os.MkdirAll(profileDir, 0o750); mkErr != nil {
						return fmt.Errorf("creating profile directory: %w", mkErr)
					}

					// Create .state directory.
					stateDir := cfg.StateDir()
					if mkErr := os.MkdirAll(stateDir, 0o750); mkErr != nil {
						return fmt.Errorf("creating state directory: %w", mkErr)
					}

					// Create initial hams.config.yaml if it does not exist.
					storeConfigPath := filepath.Join(storePath, "hams.config.yaml")
					if _, statErr := os.Stat(storeConfigPath); os.IsNotExist(statErr) {
						initial := config.Config{
							ProfileTag: cfg.ProfileTag,
							MachineID:  cfg.MachineID,
						}
						data, marshalErr := yaml.Marshal(&initial)
						if marshalErr != nil {
							return fmt.Errorf("marshaling initial config: %w", marshalErr)
						}
						if writeErr := os.WriteFile(storeConfigPath, data, 0o600); writeErr != nil {
							return fmt.Errorf("writing initial config: %w", writeErr)
						}
					}

					fmt.Printf("Store initialized at %s\n", logging.TildePath(storePath))
					fmt.Printf("  Profile dir: %s\n", logging.TildePath(profileDir))
					fmt.Printf("  State dir:   %s\n", logging.TildePath(stateDir))
					return nil
				},
			},
			{
				Name:  "push",
				Usage: "Commit and push store changes to the remote repository",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, err := config.Load(paths, flags.Store)
					if err != nil {
						return fmt.Errorf("loading config: %w", err)
					}

					storePath := cfg.StorePath
					if storePath == "" {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							"no store directory configured",
							"Set store_path in ~/.config/hams/hams.config.yaml",
						)
					}

					gitAdd := exec.CommandContext(ctx, "git", "-C", storePath, "add", "-A") //nolint:gosec // storePath is user-configured
					gitAdd.Stdin = os.Stdin
					gitAdd.Stdout = os.Stdout
					gitAdd.Stderr = os.Stderr
					if runErr := gitAdd.Run(); runErr != nil {
						return fmt.Errorf("git add: %w", runErr)
					}

					gitCommit := exec.CommandContext(ctx, "git", "-C", storePath, "commit", "-m", "hams: update store") //nolint:gosec // storePath is user-configured
					gitCommit.Stdin = os.Stdin
					gitCommit.Stdout = os.Stdout
					gitCommit.Stderr = os.Stderr
					if runErr := gitCommit.Run(); runErr != nil {
						return fmt.Errorf("git commit: %w", runErr)
					}

					gitPush := exec.CommandContext(ctx, "git", "-C", storePath, "push") //nolint:gosec // storePath is user-configured
					gitPush.Stdin = os.Stdin
					gitPush.Stdout = os.Stdout
					gitPush.Stderr = os.Stderr
					if runErr := gitPush.Run(); runErr != nil {
						return fmt.Errorf("git push: %w", runErr)
					}

					return nil
				},
			},
			{
				Name:  "pull",
				Usage: "Pull latest store changes from the remote repository",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, err := config.Load(paths, flags.Store)
					if err != nil {
						return fmt.Errorf("loading config: %w", err)
					}

					storePath := cfg.StorePath
					if storePath == "" {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							"no store directory configured",
							"Set store_path in ~/.config/hams/hams.config.yaml",
						)
					}

					gitPull := exec.CommandContext(ctx, "git", "-C", storePath, "pull", "--rebase") //nolint:gosec // storePath is user-configured
					gitPull.Stdin = os.Stdin
					gitPull.Stdout = os.Stdout
					gitPull.Stderr = os.Stderr
					return gitPull.Run()
				},
			},
		},
	}
}

// listResource is used for JSON serialization of list output.
type listResource struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	Version     string `json:"version,omitempty"`
}

func listCmd(registry *provider.Registry) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all managed resources across providers",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "only", Usage: "Only list these providers (comma-separated)"},
			&cli.StringFlag{Name: "except", Usage: "Skip these providers (comma-separated)"},
			&cli.StringFlag{Name: "status", Usage: "Filter by resource status (ok, failed, pending, removed)"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
			paths := resolvePaths(flags)
			cfg, err := config.Load(paths, flags.Store)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			providers, filterErr := filterProviders(
				registry.Ordered(cfg.ProviderPriority),
				cmd.String("only"),
				cmd.String("except"),
				registry.Names(),
			)
			if filterErr != nil {
				return filterErr
			}

			statusFilter := cmd.String("status")
			var statusSet map[string]bool
			if statusFilter != "" {
				statusSet = make(map[string]bool)
				for s := range strings.SplitSeq(statusFilter, ",") {
					statusSet[strings.TrimSpace(s)] = true
				}
			}

			stateDir := cfg.StateDir()
			var jsonResults []listResource
			printedAny := false

			for _, p := range providers {
				manifest := p.Manifest()
				displayName := manifest.DisplayName
				filePrefix := manifest.FilePrefix
				if filePrefix == "" {
					filePrefix = manifest.Name
				}
				statePath := filepath.Join(stateDir, filePrefix+".state.yaml")

				sf, loadErr := state.Load(statePath)
				if loadErr != nil {
					continue
				}

				if len(sf.Resources) == 0 {
					continue
				}

				// Collect filtered resources for this provider.
				var filteredIDs []string
				for id, r := range sf.Resources {
					if statusSet != nil && !statusSet[string(r.State)] {
						continue
					}
					filteredIDs = append(filteredIDs, id)
				}

				if len(filteredIDs) == 0 {
					continue
				}

				if flags.JSON {
					for _, id := range filteredIDs {
						r := sf.Resources[id]
						jsonResults = append(jsonResults, listResource{
							Provider:    manifest.Name,
							DisplayName: displayName,
							ID:          id,
							Status:      string(r.State),
							Version:     r.Version,
						})
					}
				} else {
					fmt.Printf("\n%s (%d resources):\n", displayName, len(filteredIDs))
					for _, id := range filteredIDs {
						r := sf.Resources[id]
						status := string(r.State)
						ver := ""
						if r.Version != "" {
							ver = " " + r.Version
						}
						fmt.Printf("  %-30s %s%s\n", id, status, ver)
					}
					printedAny = true
				}
			}

			if flags.JSON {
				if jsonResults == nil {
					jsonResults = []listResource{}
				}
				data, marshalErr := json.MarshalIndent(jsonResults, "", "  ")
				if marshalErr != nil {
					return fmt.Errorf("marshaling JSON output: %w", marshalErr)
				}
				fmt.Println(string(data))
			} else if !printedAny {
				// Empty-state message so users know the command worked but
				// no state files contain resources matching the current
				// filter. Silent exit 0 was indistinguishable from
				// "command hung" or "wrong flag".
				fmt.Println("No managed resources found.")
				fmt.Println("Run 'hams <provider> install <package>' to start managing resources,")
				fmt.Println("or 'hams apply' to replay configurations from the store.")
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
		Action: func(ctx context.Context, _ *cli.Command) error {
			return runSelfUpgrade(ctx)
		},
	}
}

func runSelfUpgrade(ctx context.Context) error {
	paths := config.ResolvePaths()
	channel := selfupdate.DetectChannel(paths)

	switch channel {
	case selfupdate.ChannelHomebrew:
		return runHomebrewUpgrade(ctx)
	case selfupdate.ChannelBinary:
		return runBinaryUpgrade(ctx)
	default:
		return fmt.Errorf("unknown install channel %q", channel)
	}
}

func runHomebrewUpgrade(ctx context.Context) error {
	fmt.Println("Detected Homebrew install, running brew upgrade...")
	cmd := exec.CommandContext(ctx, "brew", "upgrade", "zthxxx/tap/hams")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew upgrade failed: %w", err)
	}
	return nil
}

func runBinaryUpgrade(ctx context.Context) error {
	updater := selfupdate.NewUpdater()
	current := selfupdate.CurrentVersion()

	release, err := updater.LatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("checking latest version: %w", err)
	}

	if selfupdate.IsUpToDate(current, release.Version) {
		fmt.Printf("Already up-to-date (version %s)\n", current)
		return nil
	}

	wantName := selfupdate.AssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == wantName {
			downloadURL = asset.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		return hamserr.NewUserError(hamserr.ExitGeneralError,
			fmt.Sprintf("no release asset found for %s", wantName),
			"Download manually from https://github.com/zthxxx/hams/releases",
		)
	}

	fmt.Printf("Downloading %s (v%s -> v%s)...\n", wantName, current, release.Version)
	body, err := updater.DownloadAsset(ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}
	defer body.Close() //nolint:errcheck // response body close errors are non-actionable

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	if err := selfupdate.ReplaceBinary(exePath, body, ""); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	fmt.Printf("Successfully upgraded hams from v%s to v%s\n", current, release.Version)
	return nil
}

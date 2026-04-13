package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/zthxxx/hams/internal/cliutil"
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/sudo"
)

func applyCmd(registry *provider.Registry) *cli.Command {
	return &cli.Command{
		Name:  "apply",
		Usage: "Apply all configurations from the store",
		Description: `Apply reads all *.hams.yaml files from the active profile directory
and installs/removes/updates resources to match the desired state.

By default, apply runs a refresh first (probing environment for drift).
Use --no-refresh to skip probing and apply based on state alone.`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "from-repo", Usage: "Clone and apply from a GitHub repo (e.g., user/repo)"},
			&cli.BoolFlag{Name: "no-refresh", Usage: "Skip environment probing before applying"},
			&cli.StringFlag{Name: "only", Usage: "Only apply these providers (comma-separated)"},
			&cli.StringFlag{Name: "except", Usage: "Skip these providers (comma-separated)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
			return runApply(ctx, flags, registry,
				cmd.String("from-repo"),
				cmd.Bool("no-refresh"),
				cmd.String("only"),
				cmd.String("except"),
			)
		},
	}
}

func runApply(ctx context.Context, flags *cliutil.GlobalFlags, registry *provider.Registry, fromRepo string, noRefresh bool, only, except string) error {
	if flags.DryRun {
		fmt.Println("[dry-run] Would apply configurations. No changes will be made.")
	}

	paths := config.ResolvePaths()
	if flags.Config != "" {
		paths.ConfigHome = filepath.Dir(flags.Config)
	}

	storePath := flags.Store
	if storePath == "" {
		cfg, err := config.Load(paths, "")
		if err == nil && cfg.StorePath != "" {
			storePath = cfg.StorePath
		}
	}

	if fromRepo != "" {
		slog.Info("from-repo specified", "repo", fromRepo)
		var cloneErr error
		storePath, cloneErr = bootstrapFromRepo(fromRepo, paths)
		if cloneErr != nil {
			return fmt.Errorf("bootstrap from repo: %w", cloneErr)
		}
	}

	if storePath == "" {
		return cliutil.NewUserError(cliutil.ExitUsageError,
			"no store directory configured",
			"Run 'hams apply --from-repo=<user/repo>' to clone a store",
			"Or set store_path in ~/.config/hams/hams.config.yaml",
		)
	}

	cfg, err := config.Load(paths, storePath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.ProfileTag == "" || cfg.MachineID == "" {
		fmt.Println("Not Found Profile in config, init it at first")
		tag, mid, promptErr := promptProfileInit()
		if promptErr != nil {
			return fmt.Errorf("profile init: %w", promptErr)
		}
		cfg.ProfileTag = tag
		cfg.MachineID = mid
	}

	stateDir := cfg.StateDir()
	lock := state.NewLock(stateDir)
	if !flags.DryRun {
		if lockErr := lock.Acquire("hams apply"); lockErr != nil {
			return cliutil.NewUserError(cliutil.ExitLockError, lockErr.Error(),
				fmt.Sprintf("Remove %s/.lock if the previous run crashed", stateDir),
			)
		}
		defer func() {
			if releaseErr := lock.Release(); releaseErr != nil {
				slog.Error("failed to release lock", "error", releaseErr)
			}
		}()
	}

	sudoMgr := sudo.NewManager()
	defer sudoMgr.Stop()

	allProviders := registry.Ordered(cfg.ProviderPriority)
	providers := filterProviders(allProviders, only, except)

	sorted, dagErr := provider.ResolveDAG(providers)
	if dagErr != nil {
		return fmt.Errorf("resolving provider dependencies: %w", dagErr)
	}

	for _, p := range sorted {
		if bootstrapErr := p.Bootstrap(ctx); bootstrapErr != nil {
			slog.Warn("provider bootstrap failed", "provider", p.Manifest().Name, "error", bootstrapErr)
		}
	}

	if !noRefresh {
		slog.Info("refreshing state")
		probeResults := provider.ProbeAll(ctx, sorted, stateDir, cfg.MachineID)
		for filePrefix, sf := range probeResults {
			statePath := filepath.Join(stateDir, filePrefix+".state.yaml")
			if saveErr := sf.Save(statePath); saveErr != nil {
				slog.Error("failed to save probed state", "provider", sf.Provider, "error", saveErr)
			}
		}
	}

	if flags.DryRun {
		return printDryRunPlan(sorted, cfg)
	}

	var allResults []provider.ExecuteResult
	for _, p := range sorted {
		manifest := p.Manifest()
		name := manifest.Name
		filePrefix := manifestFilePrefix(manifest)
		profileDir := cfg.ProfileDir()
		hamsfilePath := filepath.Join(profileDir, filePrefix+".hams.yaml")

		if _, statErr := os.Stat(hamsfilePath); os.IsNotExist(statErr) {
			slog.Debug("no hamsfile for provider, skipping", "provider", name)
			continue
		}

		hf, readErr := hamsfile.Read(hamsfilePath)
		if readErr != nil {
			slog.Error("failed to read hamsfile", "provider", name, "error", readErr)
			continue
		}

		statePath := filepath.Join(stateDir, filePrefix+".state.yaml")
		sf, loadErr := state.Load(statePath)
		if loadErr != nil {
			sf = state.New(name, cfg.MachineID)
		}

		apps := hf.ListApps()
		actions := provider.ComputePlan(apps, sf, sf.ConfigHash)

		result := provider.Execute(ctx, p, actions, sf)
		allResults = append(allResults, result)

		if saveErr := sf.Save(statePath); saveErr != nil {
			slog.Error("failed to save state", "provider", name, "error", saveErr)
		}

		slog.Info("provider complete", "provider", name,
			"installed", result.Installed, "failed", result.Failed, "skipped", result.Skipped)
	}

	merged := provider.MergeResults(allResults)
	fmt.Printf("\nhams apply complete: %d installed, %d updated, %d removed, %d skipped, %d failed\n",
		merged.Installed, merged.Updated, merged.Removed, merged.Skipped, merged.Failed)

	if merged.Failed > 0 {
		return cliutil.NewUserError(cliutil.ExitPartialFailure,
			fmt.Sprintf("%d resources failed", merged.Failed),
			"Run 'hams apply' again to retry failed resources",
			"Use '--debug' for detailed error output",
		)
	}

	return nil
}

func manifestFilePrefix(m provider.Manifest) string {
	if m.FilePrefix != "" {
		return m.FilePrefix
	}
	return m.Name
}

func filterProviders(providers []provider.Provider, only, except string) []provider.Provider {
	if only == "" && except == "" {
		return providers
	}

	if only != "" {
		onlySet := parseCSV(only)
		var filtered []provider.Provider
		for _, p := range providers {
			if onlySet[strings.ToLower(p.Manifest().Name)] {
				filtered = append(filtered, p)
			}
		}
		return filtered
	}

	exceptSet := parseCSV(except)
	var filtered []provider.Provider
	for _, p := range providers {
		if !exceptSet[strings.ToLower(p.Manifest().Name)] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func parseCSV(s string) map[string]bool {
	m := make(map[string]bool)
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part != "" {
			m[part] = true
		}
	}
	return m
}

func printDryRunPlan(providers []provider.Provider, _ *config.Config) error { //nolint:unparam // will return errors when plan details are computed
	fmt.Println("[dry-run] Provider execution order:")
	for i, p := range providers {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.Manifest().DisplayName, p.Manifest().Name)
	}
	fmt.Println("\n[dry-run] No changes made.")
	return nil
}

// SetupLogging initializes logging from global flags and returns the cleanup function.
func SetupLogging(flags *cliutil.GlobalFlags) func() {
	paths := config.ResolvePaths()
	logFile, err := logging.Setup(paths.DataHome, flags.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set up logging: %v\n", err)
		return func() {}
	}

	logPath := logging.TildePath(logging.LogPath(paths.DataHome))
	slog.Info("hams session started", "log", logPath)

	return func() {
		if closeErr := logFile.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close log file: %v\n", closeErr)
		}
	}
}

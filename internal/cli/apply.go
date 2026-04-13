package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/sudo"
)

// NewApplyCmd creates the `hams apply` command.
func NewApplyCmd(flags *GlobalFlags, registry *provider.Registry) *cobra.Command {
	var (
		fromRepo  string
		noRefresh bool
		only      string
		except    string
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply all configurations from the store",
		Long: `Apply reads all *.hams.yaml files from the active profile directory
and installs/removes/updates resources to match the desired state.

By default, apply runs a refresh first (probing environment for drift).
Use --no-refresh to skip probing and apply based on state alone.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runApply(cmd.Context(), flags, registry, fromRepo, noRefresh, only, except)
		},
	}

	cmd.Flags().StringVar(&fromRepo, "from-repo", "", "Clone and apply from a GitHub repo (e.g., user/repo)")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip environment probing before applying")
	cmd.Flags().StringVar(&only, "only", "", "Only apply these providers (comma-separated)")
	cmd.Flags().StringVar(&except, "except", "", "Skip these providers (comma-separated)")

	return cmd
}

func runApply(ctx context.Context, flags *GlobalFlags, registry *provider.Registry, fromRepo string, noRefresh bool, only, except string) error {
	if flags.DryRun {
		fmt.Println("[dry-run] Would apply configurations. No changes will be made.")
	}

	// Load configuration.
	paths := config.ResolvePaths()
	if flags.Config != "" {
		paths.ConfigHome = filepath.Dir(flags.Config)
	}

	storePath := flags.Store
	if storePath == "" {
		// Try to resolve from config.
		cfg, err := config.Load(paths, "")
		if err == nil && cfg.StorePath != "" {
			storePath = cfg.StorePath
		}
	}

	if fromRepo != "" {
		slog.Info("from-repo specified", "repo", fromRepo)
		// TODO: implement go-git clone in task 3.9.
		fmt.Printf("Would clone %s to %s/repo/\n", fromRepo, paths.DataHome)
		return nil
	}

	if storePath == "" {
		return NewUserError(ExitUsageError,
			"no store directory configured",
			"Run 'hams apply --from-repo=<user/repo>' to clone a store",
			"Or set store_path in ~/.config/hams/hams.config.yaml",
		)
	}

	cfg, err := config.Load(paths, storePath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Acquire lock.
	stateDir := cfg.StateDir()
	lock := state.NewLock(stateDir)
	if !flags.DryRun {
		if lockErr := lock.Acquire("hams apply"); lockErr != nil {
			return NewUserError(ExitLockError, lockErr.Error(),
				fmt.Sprintf("Remove %s/.lock if the previous run crashed", stateDir),
			)
		}
		defer func() {
			if releaseErr := lock.Release(); releaseErr != nil {
				slog.Error("failed to release lock", "error", releaseErr)
			}
		}()
	}

	// Acquire sudo if any provider needs it.
	sudoMgr := sudo.NewManager()
	defer sudoMgr.Stop()
	// TODO: check which providers need sudo before prompting.

	// Get providers in execution order.
	allProviders := registry.Ordered(cfg.ProviderPriority)
	providers := filterProviders(allProviders, only, except)

	// Resolve DAG.
	sorted, dagErr := provider.ResolveDAG(providers)
	if dagErr != nil {
		return fmt.Errorf("resolving provider dependencies: %w", dagErr)
	}

	// Refresh (probe environment).
	if !noRefresh {
		slog.Info("refreshing state")
		provider.ProbeAll(ctx, sorted, stateDir, cfg.MachineID)
	}

	if flags.DryRun {
		return printDryRunPlan(sorted, cfg)
	}

	// Execute providers sequentially.
	var allResults []provider.ExecuteResult
	for _, p := range sorted {
		name := p.Manifest().Name
		profileDir := cfg.ProfileDir()
		hamsfilePath := filepath.Join(profileDir, name+".hams.yaml")

		if _, statErr := os.Stat(hamsfilePath); os.IsNotExist(statErr) {
			slog.Debug("no hamsfile for provider, skipping", "provider", name)
			continue
		}

		hf, readErr := hamsfile.Read(hamsfilePath)
		if readErr != nil {
			slog.Error("failed to read hamsfile", "provider", name, "error", readErr)
			continue
		}

		// Load state.
		statePath := filepath.Join(stateDir, name+".state.yaml")
		sf, loadErr := state.Load(statePath)
		if loadErr != nil {
			sf = state.New(name, cfg.MachineID)
		}

		// Compute plan.
		desired := hf.Tags() // Simplified: use tags as resource IDs for now.
		_ = desired
		// TODO: extract actual resource IDs from hamsfile entries.

		apps := extractApps(hf)
		actions := provider.ComputePlan(apps, sf, sf.ConfigHash)

		// Execute.
		result := provider.Execute(ctx, p, actions, sf)
		allResults = append(allResults, result)

		// Save state.
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
		return NewUserError(ExitPartialFailure,
			fmt.Sprintf("%d resources failed", merged.Failed),
			"Run 'hams apply' again to retry failed resources",
			"Use '--debug' for detailed error output",
		)
	}

	return nil
}

func extractApps(hf *hamsfile.File) []string { //nolint:unparam // will return real apps when hamsfile iteration is implemented
	// Extract all app names from all tags in the hamsfile.
	var apps []string
	for _, tag := range hf.Tags() {
		// FindApp iterates items; we need a list method.
		// For now, tags serve as group names — actual app extraction
		// will iterate the yaml.Node tree per tag.
		_ = tag
	}
	return apps
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
func SetupLogging(flags *GlobalFlags) func() {
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

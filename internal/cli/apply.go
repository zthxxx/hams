package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/sudo"
)

func applyCmd(registry *provider.Registry, sudoAcq sudo.Acquirer) *cli.Command {
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
			return runApply(ctx, flags, registry, sudoAcq,
				cmd.String("from-repo"),
				cmd.Bool("no-refresh"),
				cmd.String("only"),
				cmd.String("except"),
			)
		},
	}
}

func runApply(ctx context.Context, flags *provider.GlobalFlags, registry *provider.Registry, sudoAcq sudo.Acquirer, fromRepo string, noRefresh bool, only, except string) error {
	if flags.DryRun {
		fmt.Println("[dry-run] Would apply configurations. No changes will be made.")
	}

	paths := resolvePaths(flags)

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
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Run 'hams apply --from-repo=<user/repo>' to clone a store",
			"Or set store_path in ~/.config/hams/hams.config.yaml",
		)
	}

	cfg, err := config.Load(paths, storePath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Honor --profile flag override.
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
	}

	if cfg.ProfileTag == "" || cfg.MachineID == "" {
		fmt.Println("Not Found Profile in config, init it at first")
		tag, mid, promptErr := promptProfileInit()
		if promptErr != nil {
			return fmt.Errorf("profile init: %w", promptErr)
		}
		cfg.ProfileTag = tag
		cfg.MachineID = mid

		// Persist prompted values so they survive across runs.
		if writeErr := config.WriteConfigKey(paths, storePath, "profile_tag", tag); writeErr != nil {
			slog.Warn("failed to persist profile_tag", "error", writeErr)
		}
		if writeErr := config.WriteConfigKey(paths, storePath, "machine_id", mid); writeErr != nil {
			slog.Warn("failed to persist machine_id", "error", writeErr)
		}
	}

	stateDir := cfg.StateDir()
	lock := state.NewLock(stateDir)
	if !flags.DryRun {
		if lockErr := lock.Acquire("hams apply"); lockErr != nil {
			return hamserr.NewUserError(hamserr.ExitLockError, lockErr.Error(),
				fmt.Sprintf("Remove %s/.lock if the previous run crashed", stateDir),
			)
		}
		defer func() {
			if releaseErr := lock.Release(); releaseErr != nil {
				slog.Error("failed to release lock", "error", releaseErr)
			}
		}()
	}

	defer sudoAcq.Stop()

	// Two-stage provider filter:
	//   Stage 1 — artifact presence: skip providers that have no hamsfile AND
	//   no state file for the active profile/machine. Prevents Bootstrap /
	//   Probe from running for providers whose upstream tool (brew, cargo,
	//   …) may not even be installed on this machine.
	//   Stage 2 — user-supplied --only / --except narrows within stage 1.
	profileDir := cfg.ProfileDir()
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
		fmt.Println("No providers match: no hamsfile or state file present for any registered provider (after --only/--except filtering).")
		return nil
	}

	sorted, dagErr := provider.ResolveDAG(providers)
	if dagErr != nil {
		return fmt.Errorf("resolving provider dependencies: %w", dagErr)
	}

	// Bootstrap providers. Stage 1 guarantees each `sorted` provider has at
	// least a hamsfile OR a state file. Bootstrap failure policy:
	//   - hamsfile present → fatal (user actively declared resources).
	//   - only state present (hamsfile since deleted) → debug log, continue;
	//     subsequent Probe/Plan will simply observe no desired state.
	var bootstrapFailed []string
	for _, p := range sorted {
		bootstrapErr := p.Bootstrap(ctx)
		if bootstrapErr == nil {
			continue
		}
		manifest := p.Manifest()
		filePrefix := manifestFilePrefix(manifest)
		mainPath := filepath.Join(profileDir, filePrefix+".hams.yaml")
		localPath := filepath.Join(profileDir, filePrefix+".hams.local.yaml")
		_, mainErr := os.Stat(mainPath)
		_, localErr := os.Stat(localPath)
		if mainErr == nil || localErr == nil {
			slog.Error("provider bootstrap failed (hamsfile exists, cannot proceed)",
				"provider", manifest.Name, "error", bootstrapErr)
			bootstrapFailed = append(bootstrapFailed, manifest.Name)
		} else {
			slog.Debug("provider bootstrap skipped (no hamsfile)", "provider", manifest.Name, "error", bootstrapErr)
		}
	}
	if len(bootstrapFailed) > 0 {
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("bootstrap failed for providers with hamsfiles: %s", strings.Join(bootstrapFailed, ", ")),
			"Ensure required tools are installed or remove the hamsfile entries",
			"Use '--only' / '--except' to skip specific providers",
		)
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

	// Acquire sudo credentials once before any provider operations (after dry-run check).
	if sudoErr := sudoAcq.Acquire(ctx); sudoErr != nil {
		slog.Warn("sudo acquisition failed; some providers may fail", "error", sudoErr)
	}

	var allResults []provider.ExecuteResult
	var skippedProviders []string
	for _, p := range sorted {
		manifest := p.Manifest()
		name := manifest.Name
		filePrefix := manifestFilePrefix(manifest)
		hamsfilePath := filepath.Join(profileDir, filePrefix+".hams.yaml")
		hamsfileLocalPath := filepath.Join(profileDir, filePrefix+".hams.local.yaml")

		mainExists := true
		if _, statErr := os.Stat(hamsfilePath); os.IsNotExist(statErr) {
			mainExists = false
		}
		localExists := true
		if _, statErr := os.Stat(hamsfileLocalPath); os.IsNotExist(statErr) {
			localExists = false
		}
		if !mainExists && !localExists {
			slog.Debug("no hamsfile for provider, skipping", "provider", name)
			continue
		}

		mergeStrategy := hamsfile.MergeAppend
		if manifest.ResourceClass != provider.ClassPackage {
			mergeStrategy = hamsfile.MergeOverride
		}

		var hf *hamsfile.File
		var readErr error
		if mainExists {
			hf, readErr = hamsfile.ReadMerged(hamsfilePath, hamsfileLocalPath, mergeStrategy)
		} else {
			// Only local file exists — read it directly.
			hf, readErr = hamsfile.Read(hamsfileLocalPath)
		}
		if readErr != nil {
			slog.Error("failed to read hamsfile", "provider", name, "error", readErr)
			skippedProviders = append(skippedProviders, name)
			continue
		}

		statePath := filepath.Join(stateDir, filePrefix+".state.yaml")
		sf, loadErr := state.Load(statePath)
		if loadErr != nil {
			sf = state.New(name, cfg.MachineID)
		}

		actions, planErr := p.Plan(ctx, hf, sf)
		if planErr != nil {
			slog.Error("failed to plan provider actions", "provider", name, "error", planErr)
			skippedProviders = append(skippedProviders, name)
			continue
		}

		// If config content changed, promote skips to updates so edits apply.
		currentHash := configHashForHamsfile(hf)
		if sf.ConfigHash != "" && currentHash != sf.ConfigHash {
			for i := range actions {
				if actions[i].Type == provider.ActionSkip {
					actions[i].Type = provider.ActionUpdate
				}
			}
		}

		result := provider.Execute(ctx, p, actions, sf)
		allResults = append(allResults, result)

		if result.Failed == 0 {
			sf.ConfigHash = currentHash
		}

		if saveErr := sf.Save(statePath); saveErr != nil {
			slog.Error("failed to save state", "provider", name, "error", saveErr)
		}

		slog.Info("provider complete", "provider", name,
			"installed", result.Installed, "failed", result.Failed, "skipped", result.Skipped)
	}

	// Run async enrichment for providers that support it, non-blocking.
	enrichErrors := runEnrichPhase(ctx, sorted, cfg)
	if len(enrichErrors) > 0 {
		for _, enrichErr := range enrichErrors {
			slog.Warn("enrichment error", "error", enrichErr)
		}
	}

	merged := provider.MergeResults(allResults)
	fmt.Printf("\nhams apply complete: %d installed, %d updated, %d removed, %d skipped, %d failed\n",
		merged.Installed, merged.Updated, merged.Removed, merged.Skipped, merged.Failed)

	if len(skippedProviders) > 0 {
		fmt.Printf("Warning: %d provider(s) skipped due to errors: %s\n",
			len(skippedProviders), strings.Join(skippedProviders, ", "))
	}

	if merged.Failed > 0 || len(skippedProviders) > 0 {
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("%d resources failed, %d providers skipped", merged.Failed, len(skippedProviders)),
			"Run 'hams apply' again to retry failed resources",
			"Use '--debug' for detailed error output",
		)
	}

	return nil
}

// runEnrichPhase calls Enrich on providers that implement the Enricher interface.
// This runs after all apply operations and is best-effort (errors are collected, not fatal).
func runEnrichPhase(ctx context.Context, providers []provider.Provider, cfg *config.Config) []error {
	var errs []error
	for _, p := range providers {
		enricher, ok := p.(provider.Enricher)
		if !ok {
			continue
		}

		// Enrich all resources that were just installed/updated.
		name := p.Manifest().Name
		filePrefix := manifestFilePrefix(p.Manifest())
		hamsfilePath := filepath.Join(cfg.ProfileDir(), filePrefix+".hams.yaml")
		hf, readErr := hamsfile.Read(hamsfilePath)
		if readErr != nil {
			continue
		}

		for _, appID := range hf.ListApps() {
			if enrichErr := enricher.Enrich(ctx, appID); enrichErr != nil {
				slog.Debug("enrich failed", "provider", name, "resource", appID, "error", enrichErr)
				errs = append(errs, fmt.Errorf("%s: enrich %s: %w", name, appID, enrichErr))
			}
		}
	}
	return errs
}

func manifestFilePrefix(m provider.Manifest) string { //nolint:gocritic // simple helper, copy is acceptable
	if m.FilePrefix != "" {
		return m.FilePrefix
	}
	return m.Name
}

func configHashForHamsfile(hf *hamsfile.File) string {
	// Hash the full YAML content so config changes (not just ID changes) are detected.
	data, err := yaml.Marshal(hf.Root)
	if err != nil {
		// Fall back to hashing app IDs.
		appIDs := hf.ListApps()
		slices.Sort(appIDs)
		sum := sha256.Sum256([]byte(strings.Join(appIDs, "\n")))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func filterProviders(providers []provider.Provider, only, except string, knownNames []string) ([]provider.Provider, error) {
	if only == "" && except == "" {
		return providers, nil
	}

	if only != "" && except != "" {
		return nil, hamserr.NewUserError(hamserr.ExitUsageError,
			"--only and --except are mutually exclusive",
			"Use --only to include specific providers, or --except to exclude them",
		)
	}

	knownSet := make(map[string]bool)
	for _, n := range knownNames {
		knownSet[strings.ToLower(n)] = true
	}

	if only != "" {
		onlySet := parseCSV(only)
		if err := validateProviderNames(onlySet, knownSet, knownNames); err != nil {
			return nil, err
		}
		var filtered []provider.Provider
		for _, p := range providers {
			if onlySet[strings.ToLower(p.Manifest().Name)] {
				filtered = append(filtered, p)
			}
		}
		return filtered, nil
	}

	exceptSet := parseCSV(except)
	if err := validateProviderNames(exceptSet, knownSet, knownNames); err != nil {
		return nil, err
	}
	var filtered []provider.Provider
	for _, p := range providers {
		if !exceptSet[strings.ToLower(p.Manifest().Name)] {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func validateProviderNames(requested, known map[string]bool, knownNames []string) error {
	var unknown []string
	for name := range requested {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("unknown provider(s): %s", strings.Join(unknown, ", ")),
			fmt.Sprintf("Available providers: %s", strings.Join(knownNames, ", ")),
		)
	}
	return nil
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
func SetupLogging(flags *provider.GlobalFlags) func() {
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

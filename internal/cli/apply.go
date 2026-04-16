package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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
			&cli.BoolFlag{Name: "prune-orphans", Usage: "Process providers that have a state file but no hamsfile by removing every state-tracked resource. Destructive; default off."},
			&cli.BoolFlag{Name: "bootstrap", Usage: "Auto-run provider bootstrap scripts (e.g., Homebrew install.sh) when a prerequisite is missing. Runs remote scripts — opt in explicitly."},
			&cli.BoolFlag{Name: "no-bootstrap", Usage: "Fail fast when a provider prerequisite is missing. Skip the interactive consent prompt that would otherwise show on a TTY."},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
			return runApply(ctx, flags, registry, sudoAcq,
				cmd.String("from-repo"),
				cmd.Bool("no-refresh"),
				cmd.String("only"),
				cmd.String("except"),
				cmd.Bool("prune-orphans"),
				bootstrapMode{Allow: cmd.Bool("bootstrap"), Deny: cmd.Bool("no-bootstrap")},
			)
		},
	}
}

// bootstrapMode carries the user's stated consent for auto-running
// provider bootstrap scripts. Both false = ask on TTY, error off TTY.
type bootstrapMode struct {
	Allow bool // --bootstrap: always run without prompting
	Deny  bool // --no-bootstrap: never run, even on TTY
}

func runApply(ctx context.Context, flags *provider.GlobalFlags, registry *provider.Registry, sudoAcq sudo.Acquirer, fromRepo string, noRefresh bool, only, except string, pruneOrphans bool, boot bootstrapMode) (retErr error) {
	if boot.Allow && boot.Deny {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"--bootstrap and --no-bootstrap are mutually exclusive",
			"Pick one: --bootstrap to auto-run, --no-bootstrap to fail fast",
		)
	}
	// Validate --only/--except exclusion BEFORE loading config — otherwise
	// users with both flags and no store-path get a misleading "no store
	// configured" error first. The real filtering still happens later via
	// filterProviders, but the exclusion check is pure args validation.
	if only != "" && except != "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"--only and --except are mutually exclusive",
			"Use --only to include specific providers, or --except to exclude them",
		)
	}
	if flags.DryRun {
		fmt.Println("[dry-run] Would apply configurations. No changes will be made.")
	}

	paths := resolvePaths(flags)

	storePath := flags.Store
	if storePath == "" {
		cfg, err := config.Load(paths, "")
		if err != nil {
			// Previously this error was swallowed — which meant malformed
			// YAML in ~/.config/hams/hams.config.yaml was demoted into a
			// confusing "no store directory configured" message. Surface
			// the real error so users can fix their config file. Do not
			// double-wrap: config.Load already labels which file failed.
			return err
		}
		if cfg.StorePath != "" {
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

	// OTel session is opt-in via HAMS_OTEL=1 (see internal/cli/otel.go).
	// When enabled, a root "hams.apply" span wraps the entire run and
	// each provider.Execute call records child spans + per-provider
	// metrics via the executor's existing span machinery (executor.go).
	// When disabled, otelSess.Session() returns nil and Execute's
	// nil-session branches skip the tracing machinery entirely.
	otelSess := maybeStartOTelSession(paths.DataHome, "hams.apply")
	defer func() {
		status := otelStatusOK
		if retErr != nil {
			status = otelStatusError
		}
		otelSess.End(context.Background(), status)
	}()

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

	sorted, dagErr := provider.ResolveDAG(providers)
	if dagErr != nil {
		return fmt.Errorf("resolving provider dependencies: %w", dagErr)
	}

	// Attach root-span attributes now that profile + provider count
	// are resolved. Per observability spec, the root span carries
	// hams.profile + hams.providers.count.
	otelSess.AttachRootAttrs(cfg.ProfileTag, len(sorted))

	// Default `hams apply` does NOT touch state-only providers (those with
	// a state file but no hamsfile). Remove them BEFORE refresh so
	// `ProbeAll` cannot mutate their state as a side-effect — without this
	// gate, an orphaned apt resource that was manually uninstalled on the
	// host would have its state silently flipped from `ok` → `failed` even
	// though the user did NOT pass `--prune-orphans`.
	var stateOnlyDropped []provider.Provider
	sorted, stateOnlyDropped = provider.FilterStateOnlyWithoutPrune(sorted, profileDir, stateDir, pruneOrphans)
	for _, p := range stateOnlyDropped {
		slog.Debug("provider skipped (state-only without --prune-orphans)", "provider", p.Manifest().Name)
	}
	if len(sorted) == 0 {
		fmt.Println("No providers match: every selected provider is state-only and --prune-orphans was not given.")
		return nil
	}

	// Bootstrap providers. Stage 1 guarantees each `sorted` provider has at
	// least a hamsfile OR a state file. Bootstrap failure policy:
	//   - hamsfile present + BootstrapRequiredError → run consent flow (flag
	//     or TTY prompt). Consent-granted → delegate to RunBootstrap, retry.
	//   - hamsfile present + other error → fatal.
	//   - only state present (hamsfile since deleted) → debug log, continue;
	//     subsequent Probe/Plan will simply observe no desired state.
	var (
		bootstrapFailed         []string
		bootstrapRequiredDenied []*provider.BootstrapRequiredError
		skipProviders           = map[string]bool{}
	)
	for _, p := range sorted {
		bootstrapErr := p.Bootstrap(ctx)
		if bootstrapErr == nil {
			continue
		}
		manifest := p.Manifest()
		hasHamsfile := hamsfilePresent(profileDir, &manifest)

		var brerr *provider.BootstrapRequiredError
		if errors.As(bootstrapErr, &brerr) && hasHamsfile {
			switch resolveBootstrapConsent(boot, brerr) {
			case bootDecisionRun:
				// Dry-run preserves --bootstrap's INTENT (user consented)
				// without the side effect: print what WOULD run and
				// leave the host untouched. The surrounding printDryRunPlan
				// step still fires to show the rest of the plan.
				if flags.DryRun {
					fmt.Printf("[dry-run] Would bootstrap %s via: %s\n",
						manifest.Name, brerr.Script)
					continue
				}
				if runErr := provider.RunBootstrap(ctx, p, registry); runErr != nil {
					slog.Error("provider bootstrap script failed",
						"provider", manifest.Name, "error", runErr)
					// Capture the structured context so the final
					// UserFacingError surfaces which script was attempted
					// even when the RunBootstrap path failed — otherwise
					// users see a generic error with no breadcrumb back
					// to the install command that just broke.
					bootstrapRequiredDenied = append(bootstrapRequiredDenied, brerr)
					bootstrapFailed = append(bootstrapFailed, manifest.Name)
					continue
				}
				if retryErr := p.Bootstrap(ctx); retryErr != nil {
					slog.Error("provider still unavailable after bootstrap",
						"provider", manifest.Name, "error", retryErr)
					// Same rationale: capture the script context so the
					// final UserFacingError explains what ran (and
					// apparently succeeded per exit code) yet still left
					// the binary missing — typically a PATH-hydration
					// edge case or a non-standard install location.
					bootstrapRequiredDenied = append(bootstrapRequiredDenied, brerr)
					bootstrapFailed = append(bootstrapFailed, manifest.Name)
				}
				continue
			case bootDecisionSkipProvider:
				slog.Info("user opted to skip provider for this run",
					"provider", manifest.Name)
				skipProviders[manifest.Name] = true
				continue
			case bootDecisionDeny:
				// Capture the structured error so the final UserFacingError
				// can surface the binary name + exact install script + the
				// --bootstrap remedy (per builtin-providers spec scenario
				// "Bootstrap emits actionable error when --bootstrap is not
				// set"). Then fall through to the regular-failure path.
				bootstrapRequiredDenied = append(bootstrapRequiredDenied, brerr)
			}
		}

		if hasHamsfile {
			slog.Error("provider bootstrap failed (hamsfile exists, cannot proceed)",
				"provider", manifest.Name, "error", bootstrapErr)
			bootstrapFailed = append(bootstrapFailed, manifest.Name)
		} else {
			slog.Debug("provider bootstrap skipped (no hamsfile)",
				"provider", manifest.Name, "error", bootstrapErr)
		}
	}
	// Transitively propagate skip-provider to any DAG dependents so we
	// don't silently run a provider whose declared prerequisite was just
	// removed from the run. Fixed-point because dependents of dependents
	// must also skip.
	if len(skipProviders) > 0 {
		for {
			added := false
			for _, p := range sorted {
				name := p.Manifest().Name
				if skipProviders[name] {
					continue
				}
				for _, dep := range p.Manifest().DependsOn {
					if skipProviders[dep.Provider] {
						slog.Info("cascading skip to dependent provider",
							"provider", name, "depends_on", dep.Provider)
						skipProviders[name] = true
						added = true
						break
					}
				}
			}
			if !added {
				break
			}
		}
		filtered := make([]provider.Provider, 0, len(sorted))
		for _, p := range sorted {
			if skipProviders[p.Manifest().Name] {
				continue
			}
			filtered = append(filtered, p)
		}
		sorted = filtered
	}
	if len(bootstrapFailed) > 0 {
		suggestions := buildBootstrapFailureSuggestions(bootstrapRequiredDenied)
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("bootstrap failed for providers with hamsfiles: %s", strings.Join(bootstrapFailed, ", ")),
			suggestions...,
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
		fmt.Println("[dry-run] Provider execution order:")
		for i, p := range sorted {
			fmt.Printf("  %d. %s (%s)\n", i+1, p.Manifest().DisplayName, p.Manifest().Name)
		}
		fmt.Println()
	}

	// Acquire sudo credentials once before any provider operations (after dry-run check).
	// Dry-run skips sudo acquisition — no commands will actually run.
	if !flags.DryRun {
		if sudoErr := sudoAcq.Acquire(ctx); sudoErr != nil {
			slog.Warn("sudo acquisition failed; some providers may fail", "error", sudoErr)
		}
	}

	var allResults []provider.ExecuteResult
	var skippedProviders []string
	for _, p := range sorted {
		manifest := p.Manifest()
		name := manifest.Name
		filePrefix := provider.ManifestFilePrefix(manifest)
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

		// State-only provider (no hamsfile, only a state file). Reachable
		// here only when --prune-orphans is set — without the flag, the
		// pre-refresh `FilterStateOnlyWithoutPrune` already dropped them.
		// Synthesize an empty in-memory hamsfile so Plan computes
		// remove-actions against state.
		statePath := filepath.Join(stateDir, filePrefix+".state.yaml")
		stateOnly := !mainExists && !localExists
		if stateOnly {
			slog.Info("--prune-orphans: reconciling against empty desired-state", "provider", name, "state_file", statePath)
		}

		mergeStrategy := hamsfile.MergeAppend
		if manifest.ResourceClass != provider.ClassPackage {
			mergeStrategy = hamsfile.MergeOverride
		}

		var hf *hamsfile.File
		var readErr error
		switch {
		case stateOnly:
			hf = hamsfile.NewEmpty(hamsfilePath)
		case mainExists:
			hf, readErr = hamsfile.ReadMerged(hamsfilePath, hamsfileLocalPath, mergeStrategy)
		default:
			// Only local file exists — read it directly.
			hf, readErr = hamsfile.Read(hamsfileLocalPath)
		}
		if readErr != nil {
			slog.Error("failed to read hamsfile", "provider", name, "error", readErr)
			skippedProviders = append(skippedProviders, name)
			continue
		}

		// state.Load returns a wrapped error for any read/parse failure.
		// Missing file (os.ErrNotExist) is the common first-run case —
		// fall back to an empty state. Any OTHER failure (YAML corruption,
		// permission denied) is destructive to swallow: silently resetting
		// to empty state would (1) lose drift detection for every tracked
		// resource and (2) could re-trigger installs the user already
		// performed. Skip the provider and report it instead so the user
		// can inspect or delete the state file manually.
		sf, loadErr := state.Load(statePath)
		if loadErr != nil {
			if !errors.Is(loadErr, fs.ErrNotExist) {
				slog.Error("failed to load state file (corrupted?)",
					"provider", name, "path", statePath, "error", loadErr)
				skippedProviders = append(skippedProviders, name)
				continue
			}
			sf = state.New(name, cfg.MachineID)
		}

		// ComputePlan only considers state-resources for removal when
		// observed.ConfigHash is non-empty (a guard against accidentally
		// removing pre-existing host state on a fresh machine). The CLI
		// install handlers populate state directly without setting
		// ConfigHash, so a state-only provider in prune mode would see
		// zero remove-actions. Stamp the synthesized empty-doc hash now
		// — semantically correct: in prune mode, the "last applied
		// desired state" IS the empty hamsfile we just synthesized.
		if stateOnly && pruneOrphans && sf.ConfigHash == "" {
			sf.ConfigHash = configHashForHamsfile(hf)
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

		// Dry-run: print the planned actions per provider and skip
		// execution. The user sees exactly which resources would be
		// installed / updated / removed before committing to the real run.
		if flags.DryRun {
			printDryRunActions(name, manifest.DisplayName, actions)
			continue
		}

		result := provider.Execute(ctx, p, actions, sf, otelSess.Session())
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

	// Dry-run: all providers have been planned and printed; skip
	// enrichment and the execute-phase summary. Report skipped providers
	// and return ExitPartialFailure so CI scripts and `hams apply`
	// previews fail fast on broken hamsfiles instead of silently exiting
	// 0 — matching the non-dry-run branch's error semantics.
	if flags.DryRun {
		if len(skippedProviders) > 0 {
			fmt.Printf("Warning: %d provider(s) skipped due to errors: %s\n",
				len(skippedProviders), strings.Join(skippedProviders, ", "))
			return hamserr.NewUserError(hamserr.ExitPartialFailure,
				fmt.Sprintf("[dry-run] %d providers skipped due to errors (see log for details)", len(skippedProviders)),
				"Fix the hamsfile or remove broken provider entries before running apply",
				"Use '--debug' for detailed error output",
			)
		}
		fmt.Println("[dry-run] No changes made.")
		return nil
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
		filePrefix := provider.ManifestFilePrefix(p.Manifest())
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

// printDryRunActions prints the list of planned actions for one
// provider in dry-run mode. Groups by action type so the user can
// quickly scan what install / update / remove operations would run.
func printDryRunActions(name, displayName string, actions []provider.Action) {
	var installs, updates, removes, skips []string
	for _, a := range actions {
		id := a.ID
		if a.Resource != nil {
			if token, ok := a.Resource.(string); ok && token != "" {
				id = token
			}
		}
		switch a.Type {
		case provider.ActionInstall:
			installs = append(installs, id)
		case provider.ActionUpdate:
			updates = append(updates, id)
		case provider.ActionRemove:
			removes = append(removes, id)
		case provider.ActionSkip:
			skips = append(skips, id)
		}
	}
	fmt.Printf("[dry-run] %s (%s):\n", displayName, name)
	if len(installs) == 0 && len(updates) == 0 && len(removes) == 0 {
		fmt.Printf("  no changes (%d resources already at desired state)\n", len(skips))
		return
	}
	for _, id := range installs {
		fmt.Printf("  + install %s\n", id)
	}
	for _, id := range updates {
		fmt.Printf("  ~ update  %s\n", id)
	}
	for _, id := range removes {
		fmt.Printf("  - remove  %s\n", id)
	}
	if len(skips) > 0 {
		fmt.Printf("  (%d resources unchanged)\n", len(skips))
	}
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

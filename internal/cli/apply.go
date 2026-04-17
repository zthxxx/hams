package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/builtin/bash"
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
			// Reject positional args — apply reads all hamsfiles and
			// applies everything by design; a stray positional arg
			// is almost certainly a typo (e.g. `hams apply apt`
			// where the user meant `--only=apt`). Previously these
			// were silently ignored, causing hard-to-debug cases
			// where `hams apply --only=apt pnpm` only filtered to
			// apt while the user thought pnpm was also included.
			if cmd.Args().Len() > 0 {
				return hamserr.NewUserError(hamserr.ExitUsageError,
					fmt.Sprintf("hams apply does not take positional arguments (got %q)", cmd.Args().First()),
					"To filter providers: hams apply --only=<provider1>,<provider2>",
					"To apply everything: hams apply",
				)
			}
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
	// Cycle 238: capture wall-clock start so the user-facing summary
	// (text + JSON) can surface total elapsed time. CI dashboards
	// alert on regression; interactive users debugging slow runs see
	// the duration without grepping slog timestamps. Same field name
	// + units as runRefresh's cycle-238 addition.
	applyStart := time.Now()

	if boot.Allow && boot.Deny {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"--bootstrap and --no-bootstrap are mutually exclusive",
			"Pick one: --bootstrap to auto-run, --no-bootstrap to fail fast",
		)
	}
	// --from-repo clones to `${HAMS_DATA_HOME}/repo/<user>/<repo>/`
	// (see resolveClonePath in bootstrap.go) — there is no way for
	// hams to honor a user-supplied --store location at the same time.
	// Previously --store was silently dropped when --from-repo was
	// also given, leading users to think they'd point the clone at a
	// specific directory; the actual clone landed in data_home. Error
	// loudly so the user picks one or the other.
	if fromRepo != "" && flags.Store != "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"--from-repo and --store are mutually exclusive",
			"--from-repo clones into ${HAMS_DATA_HOME}/repo/<user>/<name>/ — hams cannot honor a custom --store at the same time",
			"Pick one: --from-repo=<user/repo> to clone, OR --store=<path> to use an existing local directory",
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
	if flags.DryRun && !flags.JSON {
		// Cycle 187: suppress prose header in JSON mode. CI scripts
		// running `hams --json --dry-run apply` need machine-parseable
		// output — prose on stdout before the final JSON object breaks
		// `jq` / `json.Unmarshal`. The same guard applies to the
		// per-provider dry-run previews (printDryRunActions) and the
		// "No changes made" / execution-order lines.
		fmt.Println("[dry-run] Would apply configurations. No changes will be made.")
	}

	paths := resolvePaths(flags)

	// Wire session logging to a file under ${HAMS_DATA_HOME}/<YYYY-MM>/
	// per logging.Setup. Previously SetupLogging was defined but never
	// called, so every `hams apply` session ran with the default slog
	// handler writing to stderr only — users had no rolling log file
	// even though the spec + docs reference one. Enabling for apply/
	// refresh only; short commands (`--version`, `config get`) don't
	// need per-invocation log files.
	cleanupLog := SetupLogging(flags)
	defer cleanupLog()

	// Resolve the single CLI tag override upfront so the rest of the
	// function (config.Load, ProfileDir validation, profile-not-found
	// error messages) sees one value regardless of whether the user
	// typed --tag or the legacy --profile alias. Fails fast on the
	// conflict case so it doesn't leak into deeper layers.
	cliTagOverride, err := config.ResolveCLITagOverride(flags.Tag, flags.Profile)
	if err != nil {
		return err
	}

	storePath := flags.Store
	var configuredRepo string
	if storePath == "" {
		cfg, loadErr := config.Load(paths, "", cliTagOverride)
		if loadErr != nil {
			// Previously this error was swallowed — which meant malformed
			// YAML in ~/.config/hams/hams.config.yaml was demoted into a
			// confusing "no store directory configured" message. Surface
			// the real error so users can fix their config file. Do not
			// double-wrap: config.Load already labels which file failed.
			return loadErr
		}
		if cfg.StorePath != "" {
			storePath = cfg.StorePath
		}
		configuredRepo = cfg.StoreRepo
	}

	// Resolution order for where the store lives, from highest to
	// lowest precedence:
	//   1. --from-repo on the command line  (explicit override)
	//   2. --store on the command line       (explicit override, handled above via flags.Store)
	//   3. store_path in config              (handled above via cfg.StorePath)
	//   4. store_repo in config              (auto-clone, per schema-design spec)
	// Step 4 was missing — cfg.StoreRepo was defined, written on
	// `config set store_repo`, and displayed by `config get` but
	// NEVER resolved into an actual store path. Users who configured
	// only store_repo got "no store directory configured" despite
	// the spec calling store_repo a required field.
	effectiveFromRepo := fromRepo
	if effectiveFromRepo == "" && storePath == "" && configuredRepo != "" {
		slog.Info("resolving store from configured store_repo", "store_repo", configuredRepo)
		effectiveFromRepo = configuredRepo
	}
	if effectiveFromRepo != "" {
		slog.Info("from-repo specified", "repo", effectiveFromRepo)
		resolvedPath, done, resolveErr := resolveFromRepoStorePath(ctx, effectiveFromRepo, paths, flags.DryRun)
		if resolveErr != nil {
			return fmt.Errorf("bootstrap from repo: %w", resolveErr)
		}
		if done {
			// Cycle 251: dry-run with --from-repo pointing at a repo
			// that isn't already cloned. The preview message went to
			// stderr (cycle 250), but pre-cycle-251 stdout was empty
			// in JSON mode — `hams --json --dry-run apply
			// --from-repo=<X> | jq .` errored on zero bytes. Emit the
			// dry-run JSON summary shape with zero planned actions so
			// CI consumers see a parseable object that says "nothing
			// to do (would clone, no providers planned yet)".
			if flags.JSON {
				return emitDryRunJSON(nil, nil, nil, time.Since(applyStart).Milliseconds())
			}
			return nil
		}
		storePath = resolvedPath
	}

	if storePath == "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Run 'hams apply --from-repo=<user/repo>' to clone a store",
			"Or set store_path in ~/.config/hams/hams.config.yaml",
		)
	}

	// Validate the configured/supplied store path exists as a directory
	// BEFORE attempting config load / lock acquisition. Without this,
	// a typo or stale path surfaced as "creating lock directory: mkdir
	// /nonexistent: permission denied" — the error pointed at a
	// symptom (the lock-file subpath) instead of the root cause (the
	// store_path itself is wrong). Now users get a direct
	// UserFacingError naming the bad path + remediation.
	if info, statErr := os.Stat(storePath); statErr != nil || !info.IsDir() {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("store_path %q does not exist or is not a directory", storePath),
			"Fix store_path in ~/.config/hams/hams.config.yaml",
			"Or clone a store: hams apply --from-repo=<user/repo>",
			"Or initialize one: hams store init",
		)
	}

	cfg, err := config.Load(paths, storePath, cliTagOverride)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Cycle 219 puts the --profile overlay inside config.Load, so
	// cfg.ProfileTag already reflects the override. Apply still
	// hard-fails when the resulting profile dir doesn't exist
	// (cycle 92's no-silent-typo guarantee), so the validation stays
	// in place — a typo like `--tag=Linux` (vs "linux") used to
	// produce a misleading "No providers match" + exit 0 instead of
	// "profile directory not found".
	if cliTagOverride != "" {
		profileDir := cfg.ProfileDir()
		if info, statErr := os.Stat(profileDir); statErr != nil || !info.IsDir() {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				fmt.Sprintf("profile %q not found at %s", cliTagOverride, profileDir),
				"Check available profiles: ls "+storePath,
				"Or create this profile: mkdir -p "+profileDir,
			)
		}
	}

	if cfg.ProfileTag == "" || cfg.MachineID == "" {
		if err := ensureProfileConfigured(paths, storePath, cfg, flags); err != nil {
			return err
		}
	}

	stateDir := cfg.StateDir()
	// Cycle 223: route through the shared acquireMutationLock seam
	// (commands_seams.go → provider.AcquireMutationLock) instead of
	// inlining state.NewLock + Acquire. Unifies apply's lock path with
	// refresh's (cycle 221), gives the same ExitLockError shape, and
	// makes the acquisition DI-testable via the package seam.
	if !flags.DryRun {
		release, lockErr := acquireMutationLock(stateDir, "hams apply")
		if lockErr != nil {
			return lockErr
		}
		defer release()
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
		// Cycle 247: JSON consumers need a parseable stdout even when
		// the stage-1 filter produces zero providers (e.g. `hams --json
		// apply --only=apt` on a store with no apt artifacts). Pre-
		// cycle-247 this path printed the prose "no providers match"
		// message unconditionally — `hams --json apply ... | jq .` then
		// errored on invalid JSON. Emit the empty-apply summary shape
		// (same fields as a successful zero-work apply) so CI scripts
		// don't need a special-case parser.
		if flags.JSON {
			return emitEmptyApplyJSON(applyStart)
		}
		reportNoProvidersMatch(cfg, profileDir, len(stageOneProviders),
			only, allProviders, stageOneProviders)
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
		// Cycle 247: symmetric with the stage-1 empty branch above —
		// JSON mode returns the empty-apply shape; text mode prints
		// the human-readable reason.
		if flags.JSON {
			return emitEmptyApplyJSON(applyStart)
		}
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
				// step still fires to show the rest of the plan. Cycle
				// 187: suppress the prose in JSON mode so stdout stays
				// parseable.
				if flags.DryRun {
					if !flags.JSON {
						fmt.Printf("[dry-run] Would bootstrap %s via: %s\n",
							manifest.Name, brerr.Script)
					}
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

	// Per-provider failure tracking lists. Declared above the refresh
	// block because the pre-apply probe also uses stateSaveFailures
	// (failures there affect the final summary just like post-install
	// save failures do — same drift consequences).
	var allResults []provider.ExecuteResult
	var skippedProviders []string
	var stateSaveFailures []string
	var failedProviders []string            // cycle 231: per-provider failure attribution
	var probeFailedProviders []string       // cycle 237: symmetric with refresh
	var dryRunActions []dryRunProviderEntry // cycle 244: JSON dry-run planned actions

	if !noRefresh {
		slog.Info("refreshing state")
		probeResults := provider.ProbeAll(ctx, sorted, stateDir, cfg.MachineID)
		// Sort the result-map keys before iterating so save errors and
		// the per-provider slog.Error lines emerge in stable order across
		// runs. Without this, the eventual "failed to persist state"
		// summary listed providers in shuffled order — apply-side
		// parallel of cycle 151's runRefresh fix.
		probeNames := make([]string, 0, len(probeResults))
		for name := range probeResults {
			probeNames = append(probeNames, name)
		}
		sort.Strings(probeNames)
		// Cycle 241: skip sf.Save under --dry-run so the probe phase is
		// side-effect-free. Pre-cycle-241, `hams apply --dry-run` on a
		// fresh store would CREATE `.state/<machine>/<provider>.state.yaml`
		// for every probe-succeeded provider — violating the global
		// --dry-run flag's "no changes" contract and leaving persistent
		// artifacts the user didn't ask for. The fresh probe data is
		// discarded; the next non-dry-run apply will re-probe. Matches
		// runRefresh's cycle-226 "--dry-run skips state write" behavior.
		for _, filePrefix := range probeNames {
			sf := probeResults[filePrefix]
			if flags.DryRun {
				continue
			}
			statePath := filepath.Join(stateDir, filePrefix+".state.yaml")
			if saveErr := sf.Save(statePath); saveErr != nil {
				slog.Error("failed to save probed state", "provider", sf.Provider, "path", statePath, "error", saveErr)
				stateSaveFailures = append(stateSaveFailures, sf.Provider)
			}
		}

		// Cycle 237: compute probe-failed providers (symmetric with
		// runRefresh's probe_failed_providers JSON field). ProbeAll
		// silently drops failing providers from its result map; the
		// per-goroutine slog.Warn is easy to miss in a long apply log.
		// Tracking the set-difference here lets the summary surface
		// how many providers couldn't be probed BEFORE the install
		// phase ran (so the user knows drift detection was incomplete).
		// Apply proceeds either way — stale state is a known-acceptable
		// fallback for best-effort pre-apply refresh — but an explicit
		// aggregated warning primes users to expect potential re-install
		// surprises.
		for _, p := range sorted {
			name := p.Manifest().Name
			prefix := provider.ManifestFilePrefix(p.Manifest())
			if _, ok := probeResults[prefix]; !ok {
				probeFailedProviders = append(probeFailedProviders, name)
			}
		}
		sort.Strings(probeFailedProviders)
		if len(probeFailedProviders) > 0 {
			slog.Warn("pre-apply refresh: some providers failed to probe; drift detection incomplete",
				"count", len(probeFailedProviders), "providers", probeFailedProviders)
		}
	}

	if flags.DryRun && !flags.JSON {
		// Cycle 187: suppress in JSON mode — see the DryRun guard above.
		fmt.Println("[dry-run] Provider execution order:")
		for i, p := range sorted {
			fmt.Printf("  %d. %s (%s)\n", i+1, p.Manifest().DisplayName, p.Manifest().Name)
		}
		fmt.Println()
	}

	// Acquire sudo credentials once before any provider operations (after dry-run check).
	// Dry-run skips sudo acquisition — no commands will actually run.
	//
	// Cycle 227: only prompt when at least one provider in the
	// resolved `sorted` set has Manifest.RequiresSudo == true.
	// Pre-cycle-227 `hams apply` unconditionally prompted for sudo
	// even on profiles containing only cargo / npm / brew / pnpm /
	// uv / git-clone (none of which need sudo) — contradicting the
	// cli-architecture spec scenario "Operations that do not
	// require sudo SHALL NOT prompt for credentials". The check
	// runs after dry-run + filter so `--only=cargo` on an apt+cargo
	// profile correctly suppresses the prompt.
	if !flags.DryRun && providersNeedSudo(sorted, profileDir) {
		if sudoErr := sudoAcq.Acquire(ctx); sudoErr != nil {
			// Cycle 228: cli-architecture/spec.md §"Sudo not granted"
			// scenario mandates exit code 10 (ExitSudoError) when the
			// user cancels the prompt or sudo times out. Pre-cycle-228
			// the failure was downgraded to a slog.Warn and apply kept
			// going — so apt's runner.Install would fail later with a
			// generic provider error, the user got the wrong exit code,
			// and CI scripts couldn't distinguish "user canceled" from
			// "apt-get returned 100". Now: if any provider in the
			// resolved set requires sudo AND we failed to acquire, exit
			// hard with ExitSudoError + the recovery hints.
			return hamserr.NewUserError(hamserr.ExitSudoError,
				fmt.Sprintf("sudo acquisition failed: %v", sudoErr),
				"Re-run and enter the sudo password when prompted",
				"Or arrange passwordless sudo for this user (NOPASSWD entry in sudoers)",
				"Or filter out sudo-requiring providers: hams apply --except=apt",
			)
		}
	}

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

		// Cycle 255: reject hamsfiles that declare the same app under
		// two or more tags per schema-design spec §"Duplicate app
		// identity across groups is rejected". Pre-cycle-255 the
		// duplicate silently folded via ComputePlan's dedup, so the
		// user never learned their edit was ambiguous and drift
		// attribution between tags became meaningless. Skip-and-log
		// so other providers still run, and surface the offending
		// provider through skipped_providers. The log line names the
		// duplicate app and the tag list so a `tail -f` debugger has
		// the full context.
		if dupErr := hf.ValidateNoDuplicateApps(); dupErr != nil {
			slog.Error("hamsfile has duplicate app across tags",
				"provider", name, "error", dupErr)
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
		// Cycle 187: skip the per-provider prose preview in JSON mode
		// — the final JSON summary at the end of the dry-run branch is
		// the machine-readable surface for CI consumers.
		if flags.DryRun {
			if flags.JSON {
				// Cycle 244: collect per-provider planned actions for
				// the JSON summary. Spec §Dry-run apply shows planned
				// actions requires listing each action's type + target.
				// Pre-cycle-244 emitDryRunJSON emitted only aggregates
				// (skipped_providers, success) — CI scripts that
				// wanted to verify "would this apply install htop?"
				// without running it had to parse the prose preview.
				dryRunActions = append(dryRunActions, dryRunProviderEntry{
					Name:        name,
					DisplayName: manifest.DisplayName,
					Actions:     actions,
				})
			} else {
				printDryRunActions(name, manifest.DisplayName, actions)
			}
			continue
		}

		// Execute + save state for this provider in an IIFE so a panic
		// during provider.Execute (buggy provider, OOM in runner, etc.)
		// still flushes the partially-updated state.File to disk before
		// re-panicking. Without this, a panic after installing N of M
		// actions would lose the in-memory tracking — next apply would
		// re-attempt the already-installed actions.
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("provider panicked during Execute; flushing state before unwinding",
						"provider", name, "panic", r, "path", statePath)
					if saveErr := sf.Save(statePath); saveErr != nil {
						slog.Error("failed to save state after provider panic",
							"provider", name, "path", statePath, "error", saveErr)
					}
					panic(r) // re-throw after best-effort flush
				}
			}()

			result := provider.Execute(ctx, p, actions, sf, otelSess.Session())
			allResults = append(allResults, result)

			// Cycle 231: track which provider had any failed action so
			// the final JSON summary can surface failed_providers (a list
			// of names) alongside the existing `failed` count. Pre-cycle
			// 231 the summary said `"failed": N` but didn't name WHICH
			// providers failed — CI scripts couldn't retry just the
			// failed subset, they had to grep slog output.
			if result.Failed > 0 {
				failedProviders = append(failedProviders, name)
			}

			if result.Failed == 0 {
				sf.ConfigHash = currentHash
			}

			if saveErr := sf.Save(statePath); saveErr != nil {
				// State save failure is non-fatal to the install (the install
				// succeeded) but DOES invalidate drift tracking until a
				// successful save. Track it so the final summary surfaces the
				// inconsistency to the user — previously these failures were
				// only logged and scripts couldn't detect them.
				slog.Error("failed to save state", "provider", name, "path", statePath, "error", saveErr)
				stateSaveFailures = append(stateSaveFailures, name)
			}

			slog.Info("provider complete", "provider", name,
				"installed", result.Installed, "failed", result.Failed, "skipped", result.Skipped)
		}()
	}

	// Cycle 254: sort the failure-provider lists alphabetically so
	// both the dry-run and real-run paths emit them in a canonical
	// order. Pre-cycle-254 these slices appended providers in DAG /
	// provider-priority iteration order — stable for a given config
	// but not alphabetical, inconsistent with `probe_failed_providers`
	// (cycle 232) and the text-mode `failedProviders` warning (cycle
	// 235). Sort here so every downstream consumer (dry-run JSON,
	// dry-run text, real-run JSON, real-run text) sees the same
	// alphabetical order.
	sort.Strings(failedProviders)
	sort.Strings(skippedProviders)
	sort.Strings(stateSaveFailures)

	// Dry-run: all providers have been planned and printed; skip
	// enrichment and the execute-phase summary. Report skipped providers
	// and return ExitPartialFailure so CI scripts and `hams apply`
	// previews fail fast on broken hamsfiles instead of silently exiting
	// 0 — matching the non-dry-run branch's error semantics.
	if flags.DryRun {
		// Cycle 187: in JSON mode, emit a pure JSON summary of the
		// dry-run outcome instead of the prose warnings — CI scripts
		// need to parse the result without grepping prose lines.
		if flags.JSON {
			return emitDryRunJSON(skippedProviders, stateSaveFailures, dryRunActions, time.Since(applyStart).Milliseconds())
		}

		if len(skippedProviders) > 0 {
			fmt.Printf("Warning: %d provider(s) skipped due to errors: %s\n",
				len(skippedProviders), strings.Join(skippedProviders, ", "))
			return hamserr.NewUserError(hamserr.ExitPartialFailure,
				fmt.Sprintf("[dry-run] %d providers skipped due to errors (see log for details)", len(skippedProviders)),
				"Fix the hamsfile or remove broken provider entries before running apply",
				"Use '--debug' for detailed error output",
			)
		}
		// Pre-apply refresh state-save failures: surface them in dry-run
		// too. Previously dry-run printed "No changes made" + exit 0 even
		// when every state file was unwriteable — the user had no clue
		// their drift tracking was broken until they ran the real apply.
		// Same class of silent-exit-0 bug as cycle 39 (skipped providers
		// in dry-run). Symmetric with the non-dry-run branch's
		// stateSaveFailures handling at the end of runApply.
		if len(stateSaveFailures) > 0 {
			fmt.Printf("Warning: %d provider(s) failed to persist state during pre-apply refresh: %s\n",
				len(stateSaveFailures), strings.Join(stateSaveFailures, ", "))
			fmt.Println("  Drift tracking is broken for these providers. Fix permissions on the store, then re-run.")
			return hamserr.NewUserError(hamserr.ExitPartialFailure,
				fmt.Sprintf("[dry-run] %d state save failure(s) during refresh", len(stateSaveFailures)),
				"Check filesystem permissions on the store's .state/ directory",
				"Use '--no-refresh' to skip the pre-apply probe if state intentionally read-only",
			)
		}
		// Cycle 239: append elapsed for symmetry with the real-run
		// "hams apply complete: ... (took Xms)" summary.
		fmt.Printf("[dry-run] No changes made (took %dms)\n", time.Since(applyStart).Milliseconds())
		return nil
	}

	// If the user interrupted mid-apply (Ctrl+C → context.Canceled or
	// SIGTERM → context.Canceled via signal.NotifyContext in root.go),
	// the per-provider Execute loop would have bailed early and the
	// outer for-loop kept iterating, producing a misleading
	// "hams apply complete: 0 installed, ..." summary AND a zero exit
	// code. Short-circuit here so the shell sees the interruption, the
	// per-provider state that WAS saved during earlier iterations is
	// still on disk, and no enrichment / summary prints pretend the
	// work completed cleanly.
	if ctx.Err() != nil {
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("hams apply interrupted: %s", ctx.Err().Error()),
			"Partially-applied state has been saved to disk; inspect with `hams refresh`",
			"Re-run `hams apply` to continue installing remaining resources",
		)
	}

	// Run async enrichment for providers that support it, non-blocking.
	enrichErrors := runEnrichPhase(ctx, sorted, cfg)
	if len(enrichErrors) > 0 {
		for _, enrichErr := range enrichErrors {
			slog.Warn("enrichment error", "error", enrichErr)
		}
	}

	merged := provider.MergeResults(allResults)

	// Cycle 183: emit a JSON summary when --json is set. CI scripts
	// that orchestrate multi-machine applies need to detect partial
	// failures programmatically rather than parsing the prose output.
	// Symmetric with cycles 181 (version) / 182 (refresh).
	if flags.JSON {
		data := buildApplyJSONSummary(merged, failedProviders, skippedProviders, stateSaveFailures, probeFailedProviders, time.Since(applyStart).Milliseconds())
		out, mErr := json.MarshalIndent(data, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshaling apply JSON: %w", mErr)
		}
		fmt.Println(string(out))
		if merged.Failed > 0 || len(skippedProviders) > 0 || len(stateSaveFailures) > 0 {
			return hamserr.NewUserError(hamserr.ExitPartialFailure,
				fmt.Sprintf("%d resources failed, %d providers skipped, %d state saves failed",
					merged.Failed, len(skippedProviders), len(stateSaveFailures)),
				"Run 'hams apply' again to retry failed resources",
				"Use '--debug' for detailed error output",
			)
		}
		return nil
	}

	// Cycle 238: append elapsed time to the summary so interactive
	// users can spot slowdowns without scraping slog timestamps.
	fmt.Printf("\nhams apply complete: %d installed, %d updated, %d removed, %d skipped, %d failed (took %dms)\n",
		merged.Installed, merged.Updated, merged.Removed, merged.Skipped, merged.Failed, time.Since(applyStart).Milliseconds())

	// Cycle 235: name the providers whose Apply produced any failed
	// action. Symmetric with cycle 231's JSON failed_providers list
	// and cycle 234's refresh text naming. Pre-cycle-235 the prose
	// summary said `... %d failed` (count only) — interactive users
	// had to grep slog to find WHICH providers failed.
	if len(failedProviders) > 0 {
		// Cycle 254: failedProviders is now sorted upstream (right
		// after merged := provider.MergeResults). Drop the local
		// copy+sort.
		fmt.Printf("Warning: %d provider(s) had failed actions: %s\n",
			len(failedProviders), strings.Join(failedProviders, ", "))
		fmt.Println("  Re-run with --debug for the underlying error from each provider's runner.")
	}
	if len(skippedProviders) > 0 {
		fmt.Printf("Warning: %d provider(s) skipped due to errors: %s\n",
			len(skippedProviders), strings.Join(skippedProviders, ", "))
	}
	if len(stateSaveFailures) > 0 {
		// Install succeeded but persisting the record failed. Surface so
		// the user knows the next apply will re-plan these resources
		// instead of treating them as already-tracked.
		fmt.Printf("Warning: %d provider(s) failed to persist state after apply: %s\n",
			len(stateSaveFailures), strings.Join(stateSaveFailures, ", "))
		fmt.Println("  Next `hams apply` may re-execute these resources. Check permissions on the store.")
	}

	if merged.Failed > 0 || len(skippedProviders) > 0 || len(stateSaveFailures) > 0 {
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("%d resources failed, %d providers skipped, %d state saves failed",
				merged.Failed, len(skippedProviders), len(stateSaveFailures)),
			"Run 'hams apply' again to retry failed resources",
			"Use '--debug' for detailed error output",
		)
	}

	return nil
}

// reportNoProvidersMatch prints the user-facing message when no
// providers survive the two-stage artifact+flag filter. Cycle 194
// distinguishes the profile-mismatch case (configured profile_tag
// doesn't exist in the store) from the genuinely-empty case —
// previously both produced the bare "no providers match" message,
// leaving users who cloned a store without their profile_tag
// confused about what was missing.
//
// Cycle 226: when `--only=X` is set and X is a VALID registered
// provider but has no hamsfile/state for the current profile, the
// old "--only/--except excluded every provider that has artifacts"
// message misled the user — they named the provider explicitly and
// the message suggested the filter removed something the user
// wanted. New message names the artifact-less providers from the
// --only list so the user's attention goes to "X has no artifacts",
// not "my filter is wrong".
func reportNoProvidersMatch(cfg *config.Config, profileDir string, stageOneProvidersLen int,
	only string, allProviders, stageOneProviders []provider.Provider,
) {
	if stageOneProvidersLen > 0 {
		if missing := onlyMissingArtifacts(only, allProviders, stageOneProviders); len(missing) > 0 {
			verb := pluralize(len(missing), "has", "have")
			fmt.Printf("No providers match: %s %s no hamsfile or state file for the current profile — nothing to apply.\n",
				strings.Join(missing, ", "), verb)
			fmt.Printf("  Profile: %s (%s)\n", cfg.ProfileTag, logging.TildePath(profileDir))
			fmt.Printf("  Run 'hams %s install <pkg>' to start tracking, or omit --only to apply every provider with artifacts.\n",
				missing[0])
			return
		}
		fmt.Println("No providers match: --only/--except excluded every provider that has artifacts.")
		return
	}
	if info, statErr := os.Stat(profileDir); statErr == nil && info.IsDir() {
		fmt.Println("No providers match: no hamsfile or state file present for any registered provider.")
		return
	}
	fmt.Printf("No providers match: profile directory %q does not exist (profile_tag=%q).\n",
		logging.TildePath(profileDir), cfg.ProfileTag)
	entries, readErr := os.ReadDir(cfg.StorePath)
	if readErr != nil {
		return
	}
	var available []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			available = append(available, e.Name())
		}
	}
	if len(available) == 0 {
		return
	}
	sort.Strings(available)
	fmt.Printf("  Available profiles in this store: %s\n", strings.Join(available, ", "))
	fmt.Println("  Fix: hams config set profile_tag <profile>  OR  pass --profile=<profile>")
}

// dryRunProviderEntry groups a provider's dry-run planned actions
// for the JSON summary. Cycle 244.
type dryRunProviderEntry struct {
	Name        string
	DisplayName string
	Actions     []provider.Action
}

// marshalDryRunActions converts the planned-actions slice into a
// JSON-friendly shape with stable field names (provider, display_name,
// actions). Each action becomes {type, id, requires_sudo?} so CI
// scripts can drive decisions without parsing prose. Cycle 244.
//
// The `id` field uses Action.ID verbatim (matches text mode's
// printDryRunActions output). `type` is the lowercase action verb
// (install / update / remove / skip). Skipped actions are included
// so consumers see the full plan.
func marshalDryRunActions(entries []dryRunProviderEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		acts := make([]map[string]string, 0, len(e.Actions))
		for _, a := range e.Actions {
			id := a.ID
			if a.Resource != nil {
				if token, ok := a.Resource.(string); ok && token != "" {
					id = token
				}
			}
			acts = append(acts, map[string]string{
				"type": a.Type.String(),
				"id":   id,
			})
		}
		out = append(out, map[string]any{
			"provider":     e.Name,
			"display_name": e.DisplayName,
			"actions":      acts,
		})
	}
	return out
}

// emitEmptyApplyJSON writes the apply summary for the
// "no-providers-match" exit paths (stage-1 empty, or state-only
// dropped). Uses buildApplyJSONSummary with zero-valued inputs so the
// schema is identical to a healthy zero-work apply — CI consumers
// don't need special-case parsing for the empty case. Cycle 247.
func emitEmptyApplyJSON(applyStart time.Time) error {
	data := buildApplyJSONSummary(
		provider.ExecuteResult{},
		nil, nil, nil, nil,
		time.Since(applyStart).Milliseconds(),
	)
	out, mErr := json.MarshalIndent(data, "", "  ")
	if mErr != nil {
		return fmt.Errorf("marshaling empty-apply JSON: %w", mErr)
	}
	fmt.Println(string(out))
	return nil
}

// buildApplyJSONSummary constructs the map used by the `--json`
// real-run branch of runApply. Factored out of the inline block in
// cycle 237 because the growing schema (installed, updated, removed,
// skipped, failed, failed_providers, skipped_providers,
// state_save_errors, probe_failed_providers, success, dry_run) with
// per-field nil-to-empty normalization pushed the surrounding
// `if flags.JSON` block past golangci-lint's nestif threshold.
func buildApplyJSONSummary(merged provider.ExecuteResult, failedProviders, skippedProviders, stateSaveFailures, probeFailedProviders []string, elapsedMs int64) map[string]any {
	// Normalize nil → empty slice so consumers don't need to
	// nil-check before iterating.
	failedNorm := failedProviders
	if failedNorm == nil {
		failedNorm = []string{}
	}
	skippedNorm := skippedProviders
	if skippedNorm == nil {
		skippedNorm = []string{}
	}
	saveFailNorm := stateSaveFailures
	if saveFailNorm == nil {
		saveFailNorm = []string{}
	}
	probeFailNorm := probeFailedProviders
	if probeFailNorm == nil {
		probeFailNorm = []string{}
	}
	// Cycle 230: include dry_run = false so the apply JSON shape
	// matches refresh's (cycle 229). Without this, the only way
	// for a CI consumer to tell apart a dry-run preview from a
	// real run is to check whether `dry_run` is present at all
	// (presence == dry-run because the dry-run path uses
	// emitDryRunJSON). Always-present `dry_run` field gives
	// machine consumers a stable schema across modes.
	// Cycle 238: elapsed_ms surfaces total wall-clock duration so
	// CI dashboards can alert on regression. Same field name +
	// units as runRefresh's cycle-238 addition.
	return map[string]any{
		"installed":              merged.Installed,
		"updated":                merged.Updated,
		"removed":                merged.Removed,
		"skipped":                merged.Skipped,
		"failed":                 merged.Failed,
		"failed_providers":       failedNorm,
		"skipped_providers":      skippedNorm,
		"state_save_errors":      saveFailNorm,
		"probe_failed_providers": probeFailNorm,
		"success":                merged.Failed == 0 && len(skippedProviders) == 0 && len(stateSaveFailures) == 0,
		"dry_run":                false,
		"elapsed_ms":             elapsedMs,
	}
}

// onlyMissingArtifacts returns the subset of `--only=<csv>` names
// that are VALID registered providers but lack a hamsfile/state file
// for the active profile. Empty slice when `only` is empty, when the
// parsed CSV is empty, or when every named provider already has
// artifacts.
//
// Cycle 226: powers the user-facing "X has no hamsfile/state" message
// so the user sees the actual reason their --only filter produced
// zero matches. Names preserved in --only input order so output is
// predictable for users and scripts.
func onlyMissingArtifacts(only string, allProviders, stageOneProviders []provider.Provider) []string {
	if only == "" {
		return nil
	}
	parsed := parseCSV(only)
	if len(parsed) == 0 {
		return nil
	}

	allNames := make(map[string]bool, len(allProviders))
	for _, p := range allProviders {
		allNames[strings.ToLower(p.Manifest().Name)] = true
	}
	haveArtifacts := make(map[string]bool, len(stageOneProviders))
	for _, p := range stageOneProviders {
		haveArtifacts[strings.ToLower(p.Manifest().Name)] = true
	}

	var missing []string
	for raw := range strings.SplitSeq(only, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if !allNames[name] {
			continue // unknown name is caught upstream; don't surface here
		}
		if haveArtifacts[name] {
			continue
		}
		missing = append(missing, name)
	}
	return missing
}

// emitDryRunJSON writes a pure JSON summary of a dry-run to stdout
// and returns the matching ExitPartialFailure when appropriate. The
// JSON shape mirrors the full-apply JSON (cycle 183) with a
// `dry_run: true` marker so CI scripts can distinguish both modes.
func emitDryRunJSON(skippedProviders, stateSaveFailures []string, plannedActions []dryRunProviderEntry, elapsedMs int64) error {
	skippedNorm := skippedProviders
	if skippedNorm == nil {
		skippedNorm = []string{}
	}
	saveFailNorm := stateSaveFailures
	if saveFailNorm == nil {
		saveFailNorm = []string{}
	}
	// Cycle 244: planned_actions lists the would-do actions each
	// provider planned (per the spec §"Dry-run apply shows planned
	// actions"). Pre-cycle-244 JSON dry-run only emitted aggregates
	// — CI scripts had to grep the prose preview to know WHAT hams
	// would install. Now a direct array consumers can iterate.
	plannedNorm := plannedActions
	if plannedNorm == nil {
		plannedNorm = []dryRunProviderEntry{}
	}
	// Cycle 240: include elapsed_ms so dry-run JSON shape matches the
	// real-run shape (cycle 238). CI dashboards that diff dry-run
	// previews against real applies need the same field set on both
	// sides; without elapsed_ms on dry-run, regression tests had to
	// special-case the field as "may be missing on the dry-run path".
	data := map[string]any{
		"dry_run":           true,
		"planned_actions":   marshalDryRunActions(plannedNorm),
		"skipped_providers": skippedNorm,
		"state_save_errors": saveFailNorm,
		"success":           len(skippedProviders) == 0 && len(stateSaveFailures) == 0,
		"elapsed_ms":        elapsedMs,
	}
	out, mErr := json.MarshalIndent(data, "", "  ")
	if mErr != nil {
		return fmt.Errorf("marshaling dry-run JSON: %w", mErr)
	}
	fmt.Println(string(out))
	if len(skippedProviders) > 0 {
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("[dry-run] %d providers skipped due to errors (see log for details)", len(skippedProviders)),
			"Fix the hamsfile or remove broken provider entries before running apply",
			"Use '--debug' for detailed error output",
		)
	}
	if len(stateSaveFailures) > 0 {
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("[dry-run] %d state save failure(s) during refresh", len(stateSaveFailures)),
			"Check filesystem permissions on the store's .state/ directory",
			"Use '--no-refresh' to skip the pre-apply probe if state intentionally read-only",
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
		// Whitespace-only input (e.g., `--only="   "` or `--only=,,`)
		// parses to an empty set. Without this guard we'd silently filter
		// to zero providers — indistinguishable from "no providers match"
		// the user saw on a fresh store, masking the user's typo.
		if len(onlySet) == 0 {
			return nil, hamserr.NewUserError(hamserr.ExitUsageError,
				"--only value is empty after trimming whitespace",
				fmt.Sprintf("Pass a comma-separated list: --only=%s", strings.Join(knownNames, ",")),
			)
		}
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
	// Mirror of the --only empty-after-trim guard. A whitespace-only
	// --except would otherwise be a silent no-op (keeps every provider)
	// and mask a typo where the user MEANT to exclude something.
	if len(exceptSet) == 0 {
		return nil, hamserr.NewUserError(hamserr.ExitUsageError,
			"--except value is empty after trimming whitespace",
			fmt.Sprintf("Pass a comma-separated list: --except=%s", strings.Join(knownNames, ",")),
		)
	}
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
		// Sort the unknown list before formatting so a user typing
		// `--only=foo,bar,baz` with all three being typos sees the
		// same error message across runs. Without the sort, Go's
		// non-deterministic map iteration shuffled the order on
		// every invocation. Symmetric with cycles 148-151.
		sort.Strings(unknown)
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
// providersNeedSudo reports whether any provider in the slice has
// `Manifest.RequiresSudo == true`, OR (cycle 232) is a bash provider
// whose hamsfile contains at least one `sudo: true` entry.
//
// Cycle 227 wired Manifest.RequiresSudo into runApply's startup path
// so the password prompt only fires for apt-bearing profiles. But bash
// can't declare RequiresSudo statically — each bash hamsfile entry
// optionally sets `sudo: true`, and that's not knowable until the
// hamsfile is read. Pre-cycle-232: a bash-only profile with sudo: true
// scripts silently skipped sudoAcq.Acquire, then prompted for
// password EACH TIME a sudo script ran during Execute. Cycle 232:
// detect sudo-using bash hamsfiles via bash.HamsfileHasSudoEntries
// and surface them as a sudo-needing provider so the upfront
// acquire fires ONCE.
//
// profileDir is used to resolve the bash hamsfile path when a bash
// provider is in the slice.
func providersNeedSudo(providers []provider.Provider, profileDir string) bool {
	for _, p := range providers {
		if p.Manifest().RequiresSudo {
			return true
		}
		if p.Manifest().Name == "bash" {
			bashPath := filepath.Join(profileDir, "bash.hams.yaml")
			if bash.HamsfileHasSudoEntries(bashPath) {
				return true
			}
		}
	}
	return false
}

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
		fmt.Printf("  no changes (%d %s already at desired state)\n",
			len(skips), pluralize(len(skips), "resource", "resources"))
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
		fmt.Printf("  (%d %s unchanged)\n",
			len(skips), pluralize(len(skips), "resource", "resources"))
	}
}

// ensureProfileConfigured fills in any missing profile_tag / machine_id
// fields on cfg. Three paths, in priority order:
//
//  1. First-run auto-init (NEW): when `${HAMS_CONFIG_HOME}/hams.config.yaml`
//     does not exist on disk AND the user supplied `--tag` (or the
//     legacy `--profile` alias), hams seeds the config with the
//     supplied tag + a derived machine_id (env or hostname, via
//     config.DeriveMachineID), then continues. This is the "fresh
//     machine" workflow: one-shot `hams apply --from-repo=X --tag=Y`
//     with no manual config editing. Fires regardless of TTY —
//     explicit CLI input is sufficient consent.
//  2. TTY prompt: interactive terminals get the legacy
//     promptProfileInit flow, identical to pre-cycle behavior.
//  3. Non-TTY failure: surface a UserFacingError naming the missing
//     keys + concrete remediation instead of reading EOF from a
//     pipe.
//
// Auto-init is intentionally scoped to the "no global config exists"
// case: if a user deliberately wrote a config with `profile_tag:
// macOS` but left `machine_id:` blank, that is a declarative choice
// and hams still surfaces the missing-machine_id error (path 3).
// The auto-init is for pristine machines only.
//
// Previously this logic lived inline in runApply and violated
// golangci-lint nestif (complexity 8). Extracted for testability and
// clarity — any future "apply on a fresh machine" UX change should
// touch only this helper.
func ensureProfileConfigured(paths config.Paths, storePath string, cfg *config.Config, flags *provider.GlobalFlags) error {
	cliTag, _ := config.ResolveCLITagOverride(flags.Tag, flags.Profile) //nolint:errcheck // ambiguity already checked upstream
	globalConfigPresent, _ := statFile(paths.GlobalConfigPath())
	if cliTag != "" && !globalConfigPresent {
		cfg.ProfileTag = cliTag
		if writeErr := config.WriteConfigKey(paths, storePath, "profile_tag", cliTag); writeErr != nil {
			slog.Warn("failed to persist profile_tag", "error", writeErr)
		}
		mid := config.DeriveMachineID()
		cfg.MachineID = mid
		if writeErr := config.WriteConfigKey(paths, storePath, "machine_id", mid); writeErr != nil {
			slog.Warn("failed to persist machine_id", "error", writeErr)
		}
		slog.Info("auto-initialized global config", "profile_tag", cliTag, "machine_id", mid)
		return nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // Fd() returns uintptr that fits in int on all supported platforms
		// Cycle 252: diagnostic notice goes to stderr, symmetric with
		// promptProfileInit's stderr prompts. Keeps stdout reserved
		// for the primary output (apply summary / JSON).
		fmt.Fprintln(os.Stderr, "Not Found Profile in config, init it at first")
		tag, mid, promptErr := promptProfileInit()
		if promptErr != nil {
			return fmt.Errorf("profile init: %w", promptErr)
		}
		cfg.ProfileTag = tag
		cfg.MachineID = mid
		if writeErr := config.WriteConfigKey(paths, storePath, "profile_tag", tag); writeErr != nil {
			slog.Warn("failed to persist profile_tag", "error", writeErr)
		}
		if writeErr := config.WriteConfigKey(paths, storePath, "machine_id", mid); writeErr != nil {
			slog.Warn("failed to persist machine_id", "error", writeErr)
		}
		return nil
	}

	missing := make([]string, 0, 2)
	if cfg.ProfileTag == "" {
		missing = append(missing, "profile_tag")
	}
	if cfg.MachineID == "" {
		missing = append(missing, "machine_id")
	}
	return hamserr.NewUserError(hamserr.ExitUsageError,
		fmt.Sprintf("%s not configured and stdin is not a terminal", strings.Join(missing, " and ")),
		"Set them explicitly (example):",
		"  hams config set profile_tag macOS",
		"  hams config set machine_id $(hostname)",
		"Or pass --tag=<tag> on the command line — hams will auto-derive machine_id",
	)
}

// statFile returns (exists, path) for a regular-file check. Used by
// ensureProfileConfigured to detect "first run" state. Directories
// and permission-denied errors are conservatively treated as "exists"
// so auto-init never fires when the path is present-but-unreadable.
func statFile(path string) (exists bool, checkedPath string) {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return true, path
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, path
	}
	// Any other error (permission denied, broken symlink, etc.) →
	// treat as present so we don't auto-init over it.
	return true, path
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

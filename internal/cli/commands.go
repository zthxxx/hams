package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/selfupdate"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/version"
)

// versionCmd exposes the detailed build info (semver, commit, date, OS/arch)
// via `hams version`. Complements `--version` which returns the brief form;
// users filing bug reports want the full string. Previously `version.Info()`
// was defined + unit-tested but had zero callers.
//
// Cycle 181: honor the global --json flag. CI scripts that want to
// machine-extract the running version (e.g. for compatibility gates,
// bug-report templates) need a parseable shape — the text form
// "hams 1.0.0 (abc123) built 2026-04-17 linux/amd64" is awkward to
// regex-parse and brittle across format changes.
func versionCmd() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print detailed version information",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := globalFlags(cmd)
			if flags.JSON {
				data := map[string]string{
					"version": version.Version(),
					"commit":  version.Commit(),
					"date":    version.Date(),
					"goos":    runtime.GOOS,
					"goarch":  runtime.GOARCH,
				}
				out, mErr := json.MarshalIndent(data, "", "  ")
				if mErr != nil {
					return fmt.Errorf("marshaling version JSON: %w", mErr)
				}
				fmt.Println(string(out))
				return nil
			}
			fmt.Println(version.Info())
			return nil
		},
	}
}

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
			// Same positional-args guard as `hams apply` (see apply.go
			// for the rationale): refresh reads all hamsfiles/state
			// by design, so a stray positional is almost certainly a
			// typo (e.g. `hams refresh apt` instead of `--only=apt`).
			if cmd.Args().Len() > 0 {
				return hamserr.NewUserError(hamserr.ExitUsageError,
					fmt.Sprintf("hams refresh does not take positional arguments (got %q)", cmd.Args().First()),
					"To filter providers: hams refresh --only=<provider1>,<provider2>",
					"To refresh everything: hams refresh",
				)
			}
			flags := globalFlags(cmd)
			return runRefresh(ctx, flags, registry, cmd.String("only"), cmd.String("except"))
		},
	}
}

func runRefresh(ctx context.Context, flags *provider.GlobalFlags, registry *provider.Registry, only, except string) (retErr error) {
	// Same --only/--except exclusion as runApply — check before config
	// load so a misconfigured store doesn't mask the args error.
	if only != "" && except != "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"--only and --except are mutually exclusive",
			"Use --only to include specific providers, or --except to exclude them",
		)
	}
	paths := resolvePaths(flags)

	// Mirror runApply: persist session logs to ${HAMS_DATA_HOME}/<YYYY-MM>/
	// for refresh too, since it's equally long-running when many
	// providers are probed in parallel.
	cleanupLog := SetupLogging(flags)
	defer cleanupLog()

	cfg, err := config.Load(paths, flags.Store, flags.Profile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if flags.Profile != "" {
		// Cycle 219 puts the --profile overlay inside config.Load, so
		// cfg.ProfileTag already reflects the override. Refresh still
		// hard-fails when the resulting profile dir doesn't exist
		// (cycle 93's no-silent-typo guarantee), so the validation
		// stays in place.
		profileDir := cfg.ProfileDir()
		if info, statErr := os.Stat(profileDir); statErr != nil || !info.IsDir() {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				fmt.Sprintf("profile %q not found at %s", flags.Profile, profileDir),
				"Check available profiles: ls "+cfg.StorePath,
				"Or create this profile: mkdir -p "+profileDir,
			)
		}
	}
	// NOTE: cycle 90 originally patched `cfg.StorePath = flags.Store`
	// here; cycle 91 promoted that guarantee into `config.Load` itself
	// (explicitStoreOverride), so the override now fires for every
	// config.Load caller (apply / refresh / list / store-status /
	// config-* / register) without duplication. Cycle 219 did the
	// same for `--profile`.

	// Validate the configured/supplied store path exists as a directory.
	// Without this, refresh against a typo'd store_path silently reported
	// "No providers match" + exit 0 — the user couldn't tell whether
	// there genuinely were no providers or their config was misaimed.
	// Symmetric with the same check in runApply (cycle 87).
	if cfg.StorePath != "" {
		if info, statErr := os.Stat(cfg.StorePath); statErr != nil || !info.IsDir() {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				fmt.Sprintf("store_path %q does not exist or is not a directory", cfg.StorePath),
				"Fix store_path in ~/.config/hams/hams.config.yaml",
				"Or clone a store: hams apply --from-repo=<user/repo>",
				"Or initialize one: hams store init",
			)
		}
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

	// Cycle 221: refresh writes state files (one per probed provider),
	// so per the cli-architecture spec it MUST acquire the
	// single-writer lock for the duration. Pre-cycle-221 only runApply
	// did this, so a `hams refresh` could race with an in-flight
	// `hams apply` (or another refresh) and clobber state. Skip the
	// lock under --dry-run because dry-run has zero side effects;
	// taking the lock would itself write the .lock file. The lock
	// must come AFTER store_path/profile validation so a typo'd flag
	// surfaces as a usage error rather than a confusing lock-file
	// touch on a non-existent stateDir.
	//
	// Indirected through `acquireMutationLock` (see commands_seams.go)
	// so DI-isolated tests that exercise sub-paths (e.g., the
	// save-failure-ordering test) can inject a no-op lock without
	// touching the real .lock file under the test's tempdir.
	if !flags.DryRun {
		release, lockErr := acquireMutationLock(stateDir, "hams refresh")
		if lockErr != nil {
			return lockErr
		}
		defer release()
	}

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
	// Sort the result-map keys before iterating so save errors and the
	// per-provider slog.Error lines emerge in stable, alphabetical order
	// across runs. Without this, the printed "failed to save" warning
	// listed providers in shuffled order on each invocation, breaking
	// log-grep / diff workflows. Symmetric with cycles 148/149/150.
	probeNames := make([]string, 0, len(probeResults))
	for name := range probeResults {
		probeNames = append(probeNames, name)
	}
	sort.Strings(probeNames)
	var saveFailures []string
	for _, name := range probeNames {
		sf := probeResults[name]
		statePath := filepath.Join(stateDir, name+".state.yaml")
		if saveErr := sf.Save(statePath); saveErr != nil {
			slog.Error("failed to save probed state", "provider", name, "path", statePath, "error", saveErr)
			saveFailures = append(saveFailures, name)
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
	providersNoun := pluralize(planned, "provider", "providers")

	// Cycle 209: if ctx was canceled (Ctrl+C / SIGTERM), distinguish
	// the user interruption from genuine probe errors. Without this,
	// a canceled refresh reported "Refresh complete: 0/N providers
	// probed (N probe error(s); see log for details)" — misleading
	// because (a) "Refresh complete" is wrong, and (b) the N "probe
	// error(s)" are all the same ctx.Canceled, not separate failures.
	// Matches runApply's cycle-84 behavior (which also surfaces a
	// distinct "interrupted" message on ctx cancellation).
	if ctx.Err() != nil {
		if flags.JSON {
			data := map[string]any{
				"probed":      probed,
				"planned":     planned,
				"interrupted": true,
				"success":     false,
			}
			out, mErr := json.MarshalIndent(data, "", "  ")
			if mErr != nil {
				return fmt.Errorf("marshaling refresh JSON: %w", mErr)
			}
			fmt.Println(string(out))
		} else {
			fmt.Printf("Refresh interrupted: %d/%d %s probed before cancellation\n",
				probed, planned, providersNoun)
		}
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("refresh interrupted by signal (%v) after probing %d/%d providers",
				ctx.Err(), probed, planned),
			"Re-run 'hams refresh' to complete the probe",
		)
	}

	// Cycle 182: emit a JSON summary when --json is set. CI scripts
	// that run `hams refresh` in a loop need to detect partial
	// failures programmatically rather than parsing the prose output.
	// The non-JSON branches print the same fields in human form.
	if flags.JSON {
		data := map[string]any{
			"probed":         probed,
			"planned":        planned,
			"save_failures":  saveFailures,
			"probe_failures": planned - probed,
			"success":        probed == planned && len(saveFailures) == 0,
		}
		if saveFailures == nil {
			data["save_failures"] = []string{}
		}
		out, mErr := json.MarshalIndent(data, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshaling refresh JSON: %w", mErr)
		}
		fmt.Println(string(out))
		if probed == planned && len(saveFailures) == 0 {
			return nil
		}
		return hamserr.NewUserError(hamserr.ExitPartialFailure,
			fmt.Sprintf("%d of %d providers failed to probe, %d state saves failed",
				planned-probed, planned, len(saveFailures)),
			"Check slog output above for the specific error(s)",
			"Use '--debug' for detailed error output",
		)
	}

	if probed == planned && len(saveFailures) == 0 {
		fmt.Printf("Refresh complete: %d %s probed\n", planned, providersNoun)
		return nil
	}
	// Partial failure: some providers couldn't probe or their state
	// file couldn't be saved. Return ExitPartialFailure so scripts
	// detect the anomaly; previously refresh returned nil (exit 0)
	// despite the log line warning of errors — a silent-exit-0 UX
	// bug matching the apply --dry-run drift fixed in cycle 39.
	if probed == planned {
		fmt.Printf("Refresh complete: %d %s probed, but %d state file(s) failed to save: %s\n",
			planned, providersNoun, len(saveFailures), strings.Join(saveFailures, ", "))
	} else {
		fmt.Printf("Refresh complete: %d/%d %s probed (%d probe error(s); see log for details)\n",
			probed, planned, providersNoun, planned-probed)
	}
	if len(saveFailures) > 0 {
		fmt.Printf("Warning: %d state save failure(s): %s — next run may re-probe these\n",
			len(saveFailures), strings.Join(saveFailures, ", "))
	}
	return hamserr.NewUserError(hamserr.ExitPartialFailure,
		fmt.Sprintf("%d of %d providers failed to probe, %d state saves failed",
			planned-probed, planned, len(saveFailures)),
		"Check slog output above for the specific error(s)",
		"Use '--debug' for detailed error output",
	)
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
					cfg, loadErr := config.Load(paths, flags.Store, flags.Profile)
					if loadErr != nil {
						return fmt.Errorf("loading config: %w", loadErr)
					}
					// Point users at their local overrides file — sensitive
					// values (`hams config set notification.bark_token ...`)
					// land there, and `hams config list` otherwise has no
					// way to surface arbitrary sensitive keys.
					localPath := localConfigPath(paths, cfg.StorePath)

					if flags.JSON {
						// Per cli-architecture spec §"Global flags" —
						// --json is for machine parsing across commands.
						// Text output stays as the default; JSON is opt-in.
						data := map[string]any{
							"config_home":       paths.ConfigHome,
							"data_home":         paths.DataHome,
							"global_config":     paths.GlobalConfigPath(),
							"local_overrides":   localPath,
							"profile_tag":       cfg.ProfileTag,
							"machine_id":        cfg.MachineID,
							"store_path":        cfg.StorePath,
							"store_repo":        cfg.StoreRepo,
							"llm_cli":           cfg.LLMCLI,
							"provider_priority": cfg.ProviderPriority,
						}
						out, mErr := json.MarshalIndent(data, "", "  ")
						if mErr != nil {
							return fmt.Errorf("marshaling config list JSON: %w", mErr)
						}
						fmt.Println(string(out))
						return nil
					}

					fmt.Printf("Config home:       %s\n", logging.TildePath(paths.ConfigHome))
					fmt.Printf("Data home:         %s\n", logging.TildePath(paths.DataHome))
					fmt.Printf("Global config:     %s\n", logging.TildePath(paths.GlobalConfigPath()))
					if localPath != "" {
						fmt.Printf("Local overrides:   %s\n", logging.TildePath(localPath))
					}
					fmt.Printf("Profile tag:       %s\n", cfg.ProfileTag)
					fmt.Printf("Machine ID:        %s\n", cfg.MachineID)
					fmt.Printf("Store path:        %s\n", logging.TildePath(cfg.StorePath))
					if cfg.StoreRepo != "" {
						fmt.Printf("Store repo:        %s\n", cfg.StoreRepo)
					}
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
					// Strict arg count — without this, `hams config get
					// profile_tag extra_arg` silently dropped extra_arg
					// (the user might have meant "extra_arg" as the key,
					// or had a typo in their script). Surface the
					// mismatch immediately.
					if cmd.Args().Len() != 1 {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							fmt.Sprintf("config get requires exactly one key (got %d args)", cmd.Args().Len()),
							"Usage: hams config get <key>",
						)
					}
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, loadErr := config.Load(paths, flags.Store, flags.Profile)
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
					// Strict arg count. The previous `< 2` check accepted
					// 2 OR MORE args and silently dropped the rest — so
					// `hams config set notification.bark_token abc def ghi`
					// (user forgot to quote a token containing spaces)
					// silently stored only "abc". Far worse than a typo:
					// users believed their token was set correctly. Now:
					// surface the mismatch with a hint about quoting.
					if cmd.Args().Len() != 2 { //nolint:mnd // exactly 2 args required: key and value
						return hamserr.NewUserError(hamserr.ExitUsageError,
							fmt.Sprintf("config set requires exactly one key and one value (got %d args)", cmd.Args().Len()),
							"Usage: hams config set <key> <value>",
							"Quote values containing spaces: hams config set <key> \"<value with spaces>\"",
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
					target := "global config"
					if config.IsSensitiveKey(key) {
						target = "local config"
					}
					// --dry-run: preview the target routing + value
					// without mutating the YAML file. Previously
					// `hams --dry-run config set ...` performed a real
					// WriteConfigKey, contradicting the global flag's
					// "no changes" contract. Same fix pattern as
					// cycles 143/144 (store push/pull/init).
					if flags.DryRun {
						fmt.Printf("[dry-run] Would set %s = %s (in %s)\n", key, value, target)
						return nil
					}
					if err := config.WriteConfigKey(paths, flags.Store, key, value); err != nil {
						return fmt.Errorf("writing config: %w", err)
					}
					fmt.Printf("Set %s = %s (in %s)\n", key, value, target)
					return nil
				},
			},
			{
				Name:      "unset",
				Usage:     "Remove a configuration value",
				ArgsUsage: "<key>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					// Strict arg count — same rationale as `config get`
					// and `config set`: silent excess-arg drop hides
					// typos and quoting mistakes.
					if cmd.Args().Len() != 1 {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							fmt.Sprintf("config unset requires exactly one key (got %d args)", cmd.Args().Len()),
							"Usage: hams config unset <key>",
							"Valid keys: "+strings.Join(config.ValidConfigKeys, ", "),
						)
					}
					key := cmd.Args().Get(0)
					// Accept whitelisted keys OR sensitive-pattern keys —
					// mirrors `config set`'s gate so users can unset any
					// key they could previously set.
					if !config.IsValidConfigKey(key) && !config.IsSensitiveKey(key) {
						return hamserr.NewUserError(hamserr.ExitUsageError,
							fmt.Sprintf("unknown config key %q", key),
							"Valid keys: "+strings.Join(config.ValidConfigKeys, ", "),
							"Or use a key containing token/key/secret/password/credential",
						)
					}
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					target := "global config"
					if config.IsSensitiveKey(key) {
						target = "local config"
					}
					// Mirror cycle 145's set dry-run guard: preview
					// without mutating.
					if flags.DryRun {
						fmt.Printf("[dry-run] Would unset %s (from %s)\n", key, target)
						return nil
					}
					if err := config.UnsetConfigKey(paths, flags.Store, key); err != nil {
						return fmt.Errorf("unsetting config: %w", err)
					}
					fmt.Printf("Unset %s (from %s)\n", key, target)
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

					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = os.Getenv("VISUAL")
					}
					if editor == "" {
						editor = "vi"
					}
					// EDITOR can carry args (e.g. "code -w", "emacs -nw",
					// "nvim -p"). Splitting on whitespace lets us exec the
					// binary as the first field and forward the remaining
					// fields as args. Without this, the whole string
					// reached exec.CommandContext as a single binary path
					// and the user got "executable file not found" for
					// any non-bare $EDITOR. Quoted values with spaces in
					// PATH (rare) are out of scope; users with such paths
					// can wrap their editor in a script.
					editorParts := strings.Fields(editor)
					if len(editorParts) == 0 {
						editorParts = []string{"vi"}
					}

					// --dry-run: preview the target path + editor
					// without creating any directory/file or exec-ing
					// the editor. Completes the dry-run consistency
					// sweep (cycles 143-146) across all destructive
					// commands: apply, refresh, store push/pull/init,
					// config set/unset, and now config edit.
					if flags.DryRun {
						fmt.Printf("[dry-run] Would open %s in %s\n",
							logging.TildePath(configPath), editor)
						if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
							fmt.Printf("[dry-run]   (file does not exist; would be created with a stub header)\n")
						}
						return nil
					}

					// Ensure the config file exists.
					if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
						if mkdirErr := os.MkdirAll(filepath.Dir(configPath), 0o750); mkdirErr != nil {
							return fmt.Errorf("creating config directory: %w", mkdirErr)
						}
						if writeErr := os.WriteFile(configPath, []byte("# hams global configuration\n"), 0o600); writeErr != nil {
							return fmt.Errorf("creating config file: %w", writeErr)
						}
					}

					editorArgs := make([]string, 0, len(editorParts))
					editorArgs = append(editorArgs, editorParts[1:]...)
					editorArgs = append(editorArgs, configPath)
					editorCmd := exec.CommandContext(ctx, editorParts[0], editorArgs...) //nolint:gosec // editor is user-specified via $EDITOR
					editorCmd.Stdin = os.Stdin
					editorCmd.Stdout = os.Stdout
					editorCmd.Stderr = os.Stderr
					return editorCmd.Run()
				},
			},
		},
	}
}

// ensureStoreIsGitRepo returns a user-facing error when the store is
// not a git repository — the generic git fatal-output plus
// "exit status 128" wrapping otherwise surfaces as a confusing error.
// Actionable: point users at `git init` or `hams apply --from-repo=`.
func ensureStoreIsGitRepo(storePath string) error {
	gitDir := filepath.Join(storePath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil
	}
	// Also accept bare repos (HEAD at the store root).
	if _, err := os.Stat(filepath.Join(storePath, "HEAD")); err == nil {
		return nil
	}
	return hamserr.NewUserError(hamserr.ExitUsageError,
		fmt.Sprintf("store at %s is not a git repository", logging.TildePath(storePath)),
		"Initialize git in the store: cd "+storePath+" && git init",
		"Or clone an existing store: hams apply --from-repo=<user/repo>",
	)
}

// storePushRunner is the exec seam for runStorePush. Production wires
// realStorePushRunner (shells out to the host's git); tests inject a
// fake that records calls and simulates git status / commit / push
// outcomes. Keeps `hams store push` tests host-safe.
type storePushRunner interface {
	// Status runs `git -C <store> status --porcelain` and returns the
	// trimmed stdout. An empty string means there is nothing to
	// commit — the caller skips commit+push instead of surfacing
	// "nothing to commit, working tree clean" as a confusing error.
	Status(ctx context.Context, storePath string) (string, error)
	// AddAll runs `git -C <store> add -A`.
	AddAll(ctx context.Context, storePath string) error
	// Commit runs `git -C <store> commit -m <msg>`.
	Commit(ctx context.Context, storePath, message string) error
	// Push runs `git -C <store> push`.
	Push(ctx context.Context, storePath string) error
}

// realStorePushRunner shells out to the host git binary.
type realStorePushRunner struct{}

func (realStorePushRunner) Status(ctx context.Context, storePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", storePath, "status", "--porcelain") //nolint:gosec // storePath is user-configured
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func (realStorePushRunner) AddAll(ctx context.Context, storePath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", storePath, "add", "-A") //nolint:gosec // storePath is user-configured
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (realStorePushRunner) Commit(ctx context.Context, storePath, message string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", storePath, "commit", "-m", message) //nolint:gosec // storePath is user-configured
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (realStorePushRunner) Push(ctx context.Context, storePath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", storePath, "push") //nolint:gosec // storePath is user-configured
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// pushRunner is the package-level store-push runner. Overridable in
// tests so `hams store push` can be exercised without shelling out
// to a real git binary.
var pushRunner storePushRunner = realStorePushRunner{}

// runStorePush stages, commits, and pushes the store with the given
// message. A clean working tree short-circuits to a friendly
// "nothing to commit" message and exits zero — running `hams store
// push` right after `hams refresh` (which only mutates state files
// under .state/ which is .gitignored) previously errored with
// "nothing to commit, working tree clean" bubbled through as an
// exec-exit-1 failure.
func runStorePush(ctx context.Context, storePath, commitMsg string) error {
	status, err := pushRunner.Status(ctx, storePath)
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if status == "" {
		fmt.Println("Nothing to commit — the store is clean. Skipping commit+push.")
		return nil
	}

	if err := pushRunner.AddAll(ctx, storePath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := pushRunner.Commit(ctx, storePath, commitMsg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	if err := pushRunner.Push(ctx, storePath); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

// localConfigPath returns the effective path for hams.config.local.yaml
// — store-scoped when a store is active, otherwise the global fallback
// in ConfigHome. Mirrors the routing in config.WriteConfigKey and
// config.ReadRawConfigKey so `hams config list` points users at the
// same file that sensitive-key writes land in.
func localConfigPath(paths config.Paths, storePath string) string {
	if storePath == "" {
		return filepath.Join(paths.ConfigHome, "hams.config.local.yaml")
	}
	return filepath.Join(storePath, "hams.config.local.yaml")
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
	storeStatusAction := func(ctx context.Context, cmd *cli.Command) error {
		flags := globalFlags(cmd)
		paths := resolvePaths(flags)
		cfg, err := config.Load(paths, flags.Store, flags.Profile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Cycle 219 makes `--profile` overlay live inside config.Load,
		// so the value is already on cfg.ProfileTag. Status does NOT
		// fail hard when the overridden profile dir is absent — a
		// missing profile dir is a legitimate status observation
		// (fresh store) and the hamsfiles count sentinel (-1) already
		// surfaces "(profile dir not found)" in JSON + text output.
		storePath := cfg.StorePath
		if storePath == "" {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"no store directory configured",
				"Set store_path in ~/.config/hams/hams.config.yaml",
				"Or use 'hams apply --from-repo=<user/repo>' to set up a store",
			)
		}

		// If the configured store_path points at a non-existent
		// directory, distinguish that from "store exists but profile
		// subdir missing" — the latter is normal for a fresh store,
		// the former means the user's config is pointing at nothing.
		storeDirExists := true
		if info, statErr := os.Stat(storePath); statErr != nil || !info.IsDir() {
			storeDirExists = false
		}

		// Per cli-architecture spec §"Store command", status SHALL
		// display store path, active profile tag, machine-id, and any
		// uncommitted changes to Hamsfiles.
		profileDir := cfg.ProfileDir()

		// Count hamsfiles (profile-dir missing → sentinel -1 so JSON
		// output can represent the same semantic as the text "(profile
		// dir not found)" message).
		hamsfiles := -1
		if entries, readErr := os.ReadDir(profileDir); readErr == nil {
			hamsfiles = 0
			for _, e := range entries {
				if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
					hamsfiles++
				}
			}
		}

		// Git status: probe only when the store is actually a git repo.
		// Non-git stores leave gitStatus empty; JSON consumers can
		// distinguish "not a git repo" from "clean".
		gitStatus := ""
		gitChanges := 0
		if _, err := os.Stat(filepath.Join(storePath, ".git")); err == nil {
			// Derive from request ctx (not Background) so SIGINT/SIGTERM
			// cancels the probe immediately. Without this, Ctrl+C during
			// `hams store status` had to wait up to the 5s timeout
			// because the cancel signal was never wired.
			cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			gs := exec.CommandContext(cmdCtx, "git", "-C", storePath, "status", "--short") //nolint:gosec // storePath is user-configured
			out, gsErr := gs.Output()
			switch {
			case gsErr != nil:
				gitStatus = fmt.Sprintf("command failed: %v", gsErr)
			case len(out) == 0:
				gitStatus = "clean"
			default:
				gitChanges = strings.Count(string(out), "\n")
				gitStatus = fmt.Sprintf("%d uncommitted change(s)", gitChanges)
			}
		}

		if flags.JSON {
			data := map[string]any{
				"store_path":        storePath,
				"store_path_exists": storeDirExists,
				"profile_tag":       cfg.ProfileTag,
				"machine_id":        cfg.MachineID,
				"profile_dir":       cfg.ProfileDir(),
				"state_dir":         cfg.StateDir(),
				"hamsfiles":         hamsfiles,
				"git_status":        gitStatus,
				"git_changes":       gitChanges,
			}
			out, mErr := json.MarshalIndent(data, "", "  ")
			if mErr != nil {
				return fmt.Errorf("marshaling store status JSON: %w", mErr)
			}
			fmt.Println(string(out))
			return nil
		}

		if !storeDirExists {
			fmt.Printf("Store path:    %s  (does NOT exist)\n", logging.TildePath(storePath))
			fmt.Println("  The configured store_path points at a missing directory.")
			fmt.Println("  Run 'hams store init' to create it, or fix store_path in hams.config.yaml.")
			return nil
		}

		fmt.Printf("Store path:    %s\n", logging.TildePath(storePath))
		fmt.Printf("Profile tag:   %s\n", cfg.ProfileTag)
		fmt.Printf("Machine ID:    %s\n", cfg.MachineID)
		fmt.Printf("Profile dir:   %s\n", logging.TildePath(cfg.ProfileDir()))
		fmt.Printf("State dir:     %s\n", logging.TildePath(cfg.StateDir()))
		if hamsfiles >= 0 {
			fmt.Printf("Hamsfiles:     %d\n", hamsfiles)
		} else {
			fmt.Printf("Hamsfiles:     (profile dir not found)\n")
		}
		if gitStatus != "" {
			fmt.Printf("Git status:    %s\n", gitStatus)
		}

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
					cfg, err := config.Load(paths, flags.Store, flags.Profile)
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

					// --dry-run must skip all side effects per the global
					// flag contract: no TTY prompt (prompt's answer would
					// be persisted via config.WriteConfigKey), no mkdir,
					// no YAML/gitignore writes. Print the intent-level
					// preview showing what WOULD be created. Previously
					// `hams --dry-run store init` performed the real
					// init — matches cycle 143's push/pull fix.
					if flags.DryRun {
						fmt.Printf("[dry-run] Would initialize store at %s\n", logging.TildePath(storePath))
						fmt.Printf("  Would create profile dir:    %s\n", logging.TildePath(cfg.ProfileDir()))
						fmt.Printf("  Would create state dir:      %s\n", logging.TildePath(cfg.StateDir()))
						fmt.Printf("  Would create %s/hams.config.yaml (if missing)\n",
							logging.TildePath(storePath))
						fmt.Printf("  Would create %s/.gitignore (if missing)\n",
							logging.TildePath(storePath))
						if cfg.ProfileTag == "" && term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // Fd() returns uintptr that fits in int on all supported platforms
							fmt.Println("  Would prompt for profile_tag + machine_id (TTY detected)")
						}
						return nil
					}

					// Prompt for profile tag when missing AND stdin is a
					// TTY, per schema-design spec §"Initialize a new store".
					// Non-TTY (CI, tests) falls back to the default so init
					// stays scriptable. Persist the answer to the global
					// config so subsequent `hams apply` doesn't re-prompt.
					if cfg.ProfileTag == "" && term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // Fd() returns uintptr that fits in int on all supported platforms
						tag, mid, promptErr := promptProfileInit()
						if promptErr != nil {
							return fmt.Errorf("profile init: %w", promptErr)
						}
						cfg.ProfileTag = tag
						cfg.MachineID = mid
						if writeErr := config.WriteConfigKey(paths, "", "profile_tag", tag); writeErr != nil {
							slog.Warn("failed to persist profile_tag", "error", writeErr)
						}
						if writeErr := config.WriteConfigKey(paths, "", "machine_id", mid); writeErr != nil {
							slog.Warn("failed to persist machine_id", "error", writeErr)
						}
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
					// Store-level config MUST NOT contain machine-scoped
					// fields (profile_tag, machine_id) — the caller's
					// Config carries these from the global layer, but
					// writing them into the store file would fail
					// validateStoreScope on the next load. Write a
					// minimal placeholder and let the user populate it.
					storeConfigPath := filepath.Join(storePath, "hams.config.yaml")
					if _, statErr := os.Stat(storeConfigPath); os.IsNotExist(statErr) {
						const initialYAML = "# hams store project-level config\n" +
							"# Machine-scoped fields (profile_tag, machine_id) MUST NOT appear here.\n" +
							"# Set them via 'hams config set' in the global config instead.\n" +
							"# See openspec/specs/schema-design/spec.md §Project-Level Config Schema.\n"
						if writeErr := os.WriteFile(storeConfigPath, []byte(initialYAML), 0o600); writeErr != nil {
							return fmt.Errorf("writing initial config: %w", writeErr)
						}
					}

					// Create .gitignore per schema-design spec: hide state
					// files and *.local.* overrides from git. Idempotent —
					// skip if user already has one.
					gitignorePath := filepath.Join(storePath, ".gitignore")
					if _, statErr := os.Stat(gitignorePath); os.IsNotExist(statErr) {
						const gi = "# hams store .gitignore — keep private state + local overrides out of the repo\n" +
							".state/\n" +
							"*.local.yaml\n" +
							"*.local.*\n"
						if writeErr := os.WriteFile(gitignorePath, []byte(gi), 0o600); writeErr != nil {
							return fmt.Errorf("writing .gitignore: %w", writeErr)
						}
					}

					fmt.Printf("Store initialized at %s\n", logging.TildePath(storePath))
					fmt.Printf("  Profile dir: %s\n", logging.TildePath(profileDir))
					fmt.Printf("  State dir:   %s\n", logging.TildePath(stateDir))
					fmt.Printf("  .gitignore:  %s\n", logging.TildePath(gitignorePath))
					return nil
				},
			},
			{
				Name:  "push",
				Usage: "Commit and push store changes to the remote repository",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "message",
						Aliases: []string{"m"},
						Usage:   "Commit message (default: \"hams: update store\")",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, err := config.Load(paths, flags.Store, flags.Profile)
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

					if err := ensureStoreIsGitRepo(storePath); err != nil {
						return err
					}

					commitMsg := cmd.String("message")
					if commitMsg == "" {
						commitMsg = "hams: update store"
					}

					// --dry-run means "preview without mutating" — for
					// push, the mutation is a commit + network push. Skip
					// both so the user gets the intent-level feedback
					// ("this is what would happen") without any
					// side effects. Previously `hams --dry-run store push`
					// performed the real commit+push, contradicting the
					// global flag's documented contract ("Show what would
					// be done without making changes").
					if flags.DryRun {
						fmt.Printf("[dry-run] Would commit changes in %s with message %q and push to origin\n",
							logging.TildePath(storePath), commitMsg)
						return nil
					}

					return runStorePush(ctx, storePath, commitMsg)
				},
			},
			{
				Name:  "pull",
				Usage: "Pull latest store changes from the remote repository",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					flags := globalFlags(cmd)
					paths := resolvePaths(flags)
					cfg, err := config.Load(paths, flags.Store, flags.Profile)
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

					if err := ensureStoreIsGitRepo(storePath); err != nil {
						return err
					}

					// Mirror the push dry-run guard (cycle 143): pull is
					// a network operation that mutates the working tree
					// via rebase. Skip it under --dry-run so the global
					// flag's "no changes" contract holds.
					if flags.DryRun {
						fmt.Printf("[dry-run] Would run: git -C %s pull --rebase\n",
							logging.TildePath(storePath))
						return nil
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

// shortName extracts the human-facing resource name from an ID that
// may be a full URN (urn:hams:<provider>:<name>) or a bare name.
// Used by list --json's `name` field per the cli-architecture spec:
// consumers that don't care about URN namespacing get just "htop"
// from "urn:hams:apt:htop".
// pluralize picks singular/plural based on count. Small helper to
// avoid the `1 providers probed` / `5 providers probed` grammar
// bug. Keeps CLI summary output correct regardless of count.
func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func shortName(id string) string {
	const prefix = "urn:hams:"
	if !strings.HasPrefix(id, prefix) {
		return id
	}
	// urn:hams:<provider>:<name> — split on the 3rd colon.
	rest := strings.TrimPrefix(id, prefix)
	_, name, ok := strings.Cut(rest, ":")
	if !ok {
		return id
	}
	return name
}

// listResource is used for JSON serialization of list output.
// Field names follow the cli-architecture spec §"List in JSON format":
// each element contains `provider`, `name`, `status`, `version` + extras.
// `name` is the short resource identifier (e.g., "htop") extracted from
// the URN; `id` is the full URN (e.g., "urn:hams:apt:htop") retained
// for scripts that need a globally-unique handle. Both fields carry
// the same info but at different granularity — consumers pick what
// matches their schema. `version` is populated for Package-class
// resources; `value` for KV-Config-class (defaults/duti/git-config);
// `last_error` when the resource's state is `failed` or
// `hook-failed`. All three use `omitempty` so a KV-Config entry
// doesn't emit an empty `version`, and a Package entry doesn't
// emit an empty `value`.
type listResource struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	Version     string `json:"version,omitempty"`
	Value       string `json:"value,omitempty"`
	LastError   string `json:"last_error,omitempty"`
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
			// Same positional-args guard as `hams apply` / `hams refresh`:
			// list reads all state by design. Stray positional args are
			// almost certainly typos like `hams list apt` instead of
			// `hams list --only=apt`.
			if cmd.Args().Len() > 0 {
				return hamserr.NewUserError(hamserr.ExitUsageError,
					fmt.Sprintf("hams list does not take positional arguments (got %q)", cmd.Args().First()),
					"To filter providers: hams list --only=<provider1>,<provider2>",
					"To list everything: hams list",
				)
			}
			flags := globalFlags(cmd)
			paths := resolvePaths(flags)
			cfg, err := config.Load(paths, flags.Store, flags.Profile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Cycle 217 fixed the silent --profile drop in list; cycle
			// 219 promoted the overlay into config.Load itself, so
			// cfg.ProfileTag already reflects the override here. Keep
			// the per-command stat: list still hard-fails on a
			// missing/typo'd --profile so users don't see the
			// misleading "No managed resources found" fallback.
			if flags.Profile != "" {
				profileDir := cfg.ProfileDir()
				if info, statErr := os.Stat(profileDir); statErr != nil || !info.IsDir() {
					return hamserr.NewUserError(hamserr.ExitUsageError,
						fmt.Sprintf("profile %q not found at %s", flags.Profile, profileDir),
						"Check available profiles: ls "+cfg.StorePath,
						"Or create this profile: mkdir -p "+profileDir,
					)
				}
			}

			// Cycle 211: symmetric with cycle 87 (apply) and cycle 88
			// (refresh) — when store_path is missing or not a directory,
			// list previously printed "No managed resources found. Run
			// 'hams <provider> install <package>' ..." which incorrectly
			// pointed the user at installing packages when the real
			// issue was a misaimed store_path. Now: surface the typo'd
			// path with the same recovery hints apply/refresh show.
			// Empty store_path is still allowed — some users may run
			// `hams list` before a store is set up and we don't want
			// to block that exploratory case; the empty-state message
			// below still fires.
			if cfg.StorePath != "" {
				if info, statErr := os.Stat(cfg.StorePath); statErr != nil || !info.IsDir() {
					return hamserr.NewUserError(hamserr.ExitUsageError,
						fmt.Sprintf("store_path %q does not exist or is not a directory", cfg.StorePath),
						"Fix store_path in ~/.config/hams/hams.config.yaml",
						"Or clone a store: hams apply --from-repo=<user/repo>",
						"Or initialize one: hams store init",
					)
				}
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
				// Validate against the defined ResourceState values so a
				// typo like --status=failled doesn't silently produce
				// "no managed resources found" — previously the typo
				// matched zero resources and looked identical to an empty
				// store to the user.
				validStates := map[string]bool{
					"ok": true, "failed": true, "pending": true,
					"removed": true, "hook-failed": true,
				}
				statusSet = make(map[string]bool)
				var unknown []string
				for s := range strings.SplitSeq(statusFilter, ",") {
					trimmed := strings.TrimSpace(s)
					if trimmed == "" {
						continue
					}
					if !validStates[trimmed] {
						unknown = append(unknown, trimmed)
					}
					statusSet[trimmed] = true
				}
				if len(unknown) > 0 {
					return hamserr.NewUserError(hamserr.ExitUsageError,
						fmt.Sprintf("unknown status value(s): %s", strings.Join(unknown, ", ")),
						"Valid statuses: ok, failed, pending, removed, hook-failed",
					)
				}
			}

			stateDir := cfg.StateDir()
			var jsonResults []listResource
			printedAny := false
			hadAnyResources := false // any provider had >0 pre-filter resources

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
				hadAnyResources = true

				// Collect filtered resources for this provider, then sort
				// by ID. Without the sort, Go's non-deterministic map
				// iteration shuffles per-provider rows on every `hams list`
				// invocation — breaks grep/diff/snapshot workflows over
				// both text and --json output. Symmetric with cycle 148's
				// fix in DiffDesiredVsState.
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
				sort.Strings(filteredIDs)

				if flags.JSON {
					for _, id := range filteredIDs {
						r := sf.Resources[id]
						jsonResults = append(jsonResults, listResource{
							Provider:    manifest.Name,
							DisplayName: displayName,
							Name:        shortName(id),
							ID:          id,
							Status:      string(r.State),
							Version:     r.Version,
							Value:       r.Value,
							LastError:   r.LastError,
						})
					}
				} else {
					noun := pluralize(len(filteredIDs), "resource", "resources")
					fmt.Printf("\n%s (%d %s):\n", displayName, len(filteredIDs), noun)
					for _, id := range filteredIDs {
						r := sf.Resources[id]
						status := string(r.State)
						// Show version OR value (mutually exclusive by
						// resource class — Package sets Version, KV-Config
						// sets Value). Without this, `hams list` on a
						// git-config store showed `user.name=zthxxx ok`
						// with no way to see the actual value without
						// reading the state YAML.
						extra := ""
						switch {
						case r.Version != "":
							extra = " " + r.Version
						case r.Value != "":
							extra = " = " + r.Value
						}
						// Surface LastError for failed / hook-failed rows
						// so a user debugging a broken apply doesn't have
						// to run `hams list --json` or read state YAML
						// to see WHY something failed. The (error: ...)
						// suffix is distinctive and scriptable by sed/awk.
						if r.LastError != "" {
							extra += fmt.Sprintf(" (error: %s)", r.LastError)
						}
						fmt.Printf("  %-30s %s%s\n", id, status, extra)
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
				// Distinguish truly-empty state (no resources anywhere)
				// from filter-matched-nothing (resources exist but a
				// valid --status/--only/--except excluded them all).
				// The original "no managed resources" message was
				// misleading in the latter case.
				switch {
				case hadAnyResources:
					fmt.Println("No resources match the current filter.")
					if statusFilter != "" {
						fmt.Printf("  --status=%q matched zero entries. Try without it or widen the set.\n", statusFilter)
					}
				default:
					fmt.Println("No managed resources found.")
					fmt.Println("Run 'hams <provider> install <package>' to start managing resources,")
					fmt.Println("or 'hams apply' to replay configurations from the store.")
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSelfUpgrade(ctx, globalFlags(cmd))
		},
	}
}

func runSelfUpgrade(ctx context.Context, flags *provider.GlobalFlags) error {
	paths := config.ResolvePaths()
	channel := selfupdate.DetectChannel(paths)

	switch channel {
	case selfupdate.ChannelHomebrew:
		return runHomebrewUpgrade(ctx, flags)
	case selfupdate.ChannelBinary:
		return runBinaryUpgrade(ctx, flags)
	default:
		return fmt.Errorf("unknown install channel %q", channel)
	}
}

func runHomebrewUpgrade(ctx context.Context, flags *provider.GlobalFlags) error {
	if flags.DryRun {
		fmt.Println("[dry-run] Would run: brew upgrade zthxxx/tap/hams")
		return nil
	}
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

func runBinaryUpgrade(ctx context.Context, flags *provider.GlobalFlags) error {
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

	if flags.DryRun {
		fmt.Printf("[dry-run] Would download %s and upgrade hams from v%s to v%s\n", wantName, current, release.Version)
		fmt.Printf("[dry-run]   asset URL: %s\n", downloadURL)
		return nil
	}

	// Resolve the expected SHA256 from the release's checksums.txt
	// manifest BEFORE downloading the binary. Without this, the binary
	// integrity check was skipped entirely (ReplaceBinary was called
	// with expectedSHA256 = "" — see selfupdate.ReplaceBinary line:
	// "if expectedSHA256 != ''"). HTTPS catches transport tampering
	// but not a hostile origin or a swapped CDN object — the
	// checksums file (published by .github/workflows/release.yml)
	// is the integrity anchor.
	expectedSHA, err := updater.LookupChecksum(ctx, release.Assets, wantName)
	if err != nil {
		return fmt.Errorf("verifying release integrity: %w", err)
	}
	if expectedSHA == "" {
		// Older releases pre-date the checksums.txt manifest. Warn
		// loudly so the user can opt to wait for a newer release that
		// ships verified checksums.
		slog.Warn("release does not publish checksums.txt; binary integrity will NOT be verified",
			"release", release.Version, "asset", wantName)
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

	if err := selfupdate.ReplaceBinary(exePath, body, expectedSHA); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	fmt.Printf("Successfully upgraded hams from v%s to v%s\n", current, release.Version)
	return nil
}

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
func versionCmd() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print detailed version information",
		Action: func(_ context.Context, _ *cli.Command) error {
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

	cfg, err := config.Load(paths, flags.Store)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
		// Symmetric to cycle 92: when --profile is explicit, validate
		// the profile dir exists. Same "silent no-op on typo" problem
		// that cycle 92 fixed for apply.
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
	// config-* / register) without duplication.

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
	var saveFailures []string
	for name, sf := range probeResults {
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
	if probed == planned && len(saveFailures) == 0 {
		fmt.Printf("Refresh complete: %d providers probed\n", planned)
		return nil
	}
	// Partial failure: some providers couldn't probe or their state
	// file couldn't be saved. Return ExitPartialFailure so scripts
	// detect the anomaly; previously refresh returned nil (exit 0)
	// despite the log line warning of errors — a silent-exit-0 UX
	// bug matching the apply --dry-run drift fixed in cycle 39.
	if probed == planned {
		fmt.Printf("Refresh complete: %d providers probed, but %d state file(s) failed to save: %s\n",
			planned, len(saveFailures), strings.Join(saveFailures, ", "))
	} else {
		fmt.Printf("Refresh complete: %d/%d providers probed (%d probe error(s); see log for details)\n",
			probed, planned, planned-probed)
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
					cfg, loadErr := config.Load(paths, flags.Store)
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
			cmdCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

					if err := ensureStoreIsGitRepo(storePath); err != nil {
						return err
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

					if err := ensureStoreIsGitRepo(storePath); err != nil {
						return err
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
// matches their schema.
type listResource struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
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
							Name:        shortName(id),
							ID:          id,
							Status:      string(r.State),
							Version:     r.Version,
						})
					}
				} else {
					noun := "resources"
					if len(filteredIDs) == 1 {
						noun = "resource"
					}
					fmt.Printf("\n%s (%d %s):\n", displayName, len(filteredIDs), noun)
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

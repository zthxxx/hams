package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/storeinit"
)

// fallbackMachineID is used when os.Hostname fails or returns a value that
// would not satisfy config.IsValidPathSegment (e.g. contains a `/` or `..`).
const fallbackMachineID = "unknown"

// envTrue values that switch HAMS_NO_AUTO_INIT (and similar boolean env
// vars) to "enabled". Centralized so the parser does not duplicate the
// list across helpers.
var envTrue = map[string]bool{"1": true, "true": true, "yes": true}

// DefaultAutoStoreSubdir is the directory under HAMS_DATA_HOME used as the
// auto-initialized store on a fresh machine. The store ends up at
// `${HAMS_DATA_HOME}/store/` (default: `~/.local/share/hams/store/`).
const DefaultAutoStoreSubdir = "store"

// DefaultTag is the fallback profile tag when the user has not set one.
// Aligns with CLAUDE.md's task-list wording: "default `tag: default`".
const DefaultTag = "default"

// isDryRun reports whether flags requests a side-effect-free preview.
// Nil-safe so call-sites can pass `flags` without guarding.
func isDryRun(flags *provider.GlobalFlags) bool {
	return flags != nil && flags.DryRun
}

// stderrSink returns the configured stderr writer for flags, or the
// process stderr when flags is nil. Keeps status + dry-run preview
// lines test-injectable via flags.Err without forcing callers to
// construct a GlobalFlags just to get plain stderr behavior.
func stderrSink(flags *provider.GlobalFlags) io.Writer {
	if flags != nil {
		return flags.Stderr()
	}
	return os.Stderr
}

// EnsureGlobalConfig writes a minimal default global config when
// paths.GlobalConfigPath() does not yet exist. Called at the start of any
// command that needs a config so first-run users do not see "config file
// missing" errors.
//
// The auto-created config is intentionally minimal — only `tag: default`
// and `machine_id: <hostname>` are set. `store_path` is added later by
// EnsureStoreReady once the auto-init store is created (so the two writes
// are decoupled and tests can exercise them independently).
//
// When flags.DryRun is set, EnsureGlobalConfig emits a
// `[dry-run] Would ...` preview line to flags.Stderr() and returns nil
// without touching the filesystem. Honors CLAUDE.md's "first principle"
// of isolated verification: `--dry-run` promises no side effects, and
// that promise MUST hold even on the first-run auto-init path.
//
// Idempotent: if the file exists, EnsureGlobalConfig is a no-op.
func EnsureGlobalConfig(paths config.Paths, flags *provider.GlobalFlags) error {
	target := paths.GlobalConfigPath()
	if _, err := os.Stat(target); err == nil {
		return nil
	}

	if isDryRun(flags) {
		// Preview only — skip the mkdir, skip the write, skip the
		// hostname derivation (the derived value is inherently a
		// side-effect at the caller's level via config.WriteConfigKey
		// calls downstream; here we only want the user to see where the
		// config WOULD land).
		fmt.Fprintln(stderrSink(flags), i18n.Tf(i18n.AutoInitDryRunGlobalConfig, map[string]any{
			"Path": logging.TildePath(target),
		}))
		return nil
	}

	if mkErr := os.MkdirAll(filepath.Dir(target), 0o750); mkErr != nil {
		return fmt.Errorf("creating config home: %w", mkErr)
	}

	machineID := defaultMachineID()
	body := strings.Builder{}
	body.WriteString("# hams global config — auto-generated on first run.\n")
	body.WriteString("# Edit this file to override the auto-detected machine settings.\n")
	body.WriteString("# Per-store settings live in <store_path>/hams.config.yaml.\n")
	body.WriteString("\n")
	body.WriteString("tag: " + DefaultTag + "\n")
	body.WriteString("machine_id: " + machineID + "\n")

	if writeErr := os.WriteFile(target, []byte(body.String()), 0o600); writeErr != nil {
		return fmt.Errorf("writing default global config %s: %w", target, writeErr)
	}
	// i18n user-facing line: surfaces to TTY users so the auto-init is
	// visible. slog still records structured fields for log scraping.
	fmt.Fprintln(stderrSink(flags), i18n.Tf(i18n.AutoInitGlobalConfigCreated, map[string]any{
		"Path":      logging.TildePath(target),
		"Tag":       DefaultTag,
		"MachineID": machineID,
	}))
	slog.Info("auto-created hams global config", "path", logging.TildePath(target),
		"tag", DefaultTag, "machine_id", machineID)
	return nil
}

// EnsureStoreReady resolves the store path the rest of the command should
// use. Resolution order:
//  1. cliOverride (highest, from --store)
//  2. cfg.StorePath (configured)
//  3. Auto-init at `${HAMS_DATA_HOME}/${DefaultAutoStoreSubdir}` and
//     persist the path back into the global config.
//
// When flags.DryRun is set AND the auto-init path would otherwise fire,
// EnsureStoreReady emits a `[dry-run] Would ...` preview line to
// flags.Stderr() and returns (targetPath, false, nil) WITHOUT calling
// storeinit.Bootstrap or persisting store_path. Side-effect-free by
// construction, matching the global-config dry-run semantics above.
//
// Returns ("", false, err) when auto-init fails.
//
// Returns (storePath, autoInited=true, nil) when this call performed
// the auto-init — callers may surface a one-time message to the user.
//
// Returns (storePath, autoInited=false, nil) when the path was already
// configured.
func EnsureStoreReady(paths config.Paths, cfg *config.Config, cliOverride string, flags *provider.GlobalFlags) (storePath string, autoInited bool, err error) {
	if cliOverride != "" {
		return cliOverride, false, nil
	}
	if cfg != nil && cfg.StorePath != "" {
		return cfg.StorePath, false, nil
	}

	target := filepath.Join(paths.DataHome, DefaultAutoStoreSubdir)
	if storeinit.Bootstrapped(target) {
		if cfg != nil {
			cfg.StorePath = target
		}
		if isDryRun(flags) {
			// Already-bootstrapped store is a no-op even outside
			// dry-run, but we MUST NOT call WriteConfigKey here in
			// dry-run mode — that would mutate ~/.config/hams/
			// hams.config.yaml. Surface a preview line only.
			fmt.Fprintln(stderrSink(flags), i18n.Tf(i18n.AutoInitDryRunStore, map[string]any{
				"Path": logging.TildePath(target),
			}))
			return target, false, nil
		}
		// Persist for next-time discoverability even if the in-memory
		// cfg already had it (cheap; no-op when value matches).
		if err := config.WriteConfigKey(paths, "", "store_path", target); err != nil {
			slog.Warn("auto-init: failed to persist store_path to global config",
				"error", err, "path", target)
		}
		return target, false, nil
	}

	if isDryRun(flags) {
		// Fresh target that WOULD be scaffolded. Emit the preview line
		// so the user sees where the store is headed, but create
		// nothing — no directory, no git init, no template files, no
		// global-config mutation. Honors CLAUDE.md's "dry-run is
		// lossless" guarantee on the first-run path.
		fmt.Fprintln(stderrSink(flags), i18n.Tf(i18n.AutoInitDryRunStore, map[string]any{
			"Path": logging.TildePath(target),
		}))
		return target, false, nil
	}

	if err := storeinit.Bootstrap(target); err != nil {
		return "", false, fmt.Errorf("auto-init store at %s: %w", target, err)
	}

	if writeErr := config.WriteConfigKey(paths, "", "store_path", target); writeErr != nil {
		// Best-effort persist — auto-init can still complete without the
		// config write, and the next call will re-find via Bootstrapped.
		slog.Warn("auto-init: failed to persist store_path to global config",
			"error", writeErr, "path", target)
	}

	// Seed machine-scoped identity (profile_tag + machine_id) so the
	// user does not see the "profile_tag is empty / machine_id is
	// empty" nudges on every subsequent `hams <provider>` invocation.
	// This makes the first-time onboarding loop single-shot: `hams
	// brew install htop` produces a complete, committable config in
	// one command. seedIdentityIfMissing is a no-op when the user has
	// already set either key (e.g. via `hams config set profile_tag
	// macOS` before the first provider install).
	seedIdentityIfMissing(paths)

	if cfg != nil {
		cfg.StorePath = target
	}

	// i18n user-facing line so first-run users see WHERE the store
	// landed without scraping slog records.
	fmt.Fprintln(stderrSink(flags), i18n.Tf(i18n.AutoInitStoreCreated, map[string]any{
		"Path": logging.TildePath(target),
	}))
	slog.Info("auto-initialized hams store",
		"path", logging.TildePath(target),
		"hint", "edit ~/.config/hams/hams.config.yaml to relocate")
	return target, true, nil
}

// seedIdentityIfMissing writes `profile_tag` + `machine_id` to the global
// config IFF those keys are currently empty. Respects user-supplied
// values: if the user ran `hams config set profile_tag macOS` before
// their first provider install, the explicit choice is preserved.
//
// Failures are logged (slog.Warn) but not propagated — identity seeding
// is best-effort and the surrounding auto-init path already succeeded;
// a failure here does not invalidate the store itself.
func seedIdentityIfMissing(paths config.Paths) {
	cfg, err := config.Load(paths, "", "")
	if err != nil {
		slog.Warn("auto-init: failed to load config for identity seeding", "error", err)
		return
	}
	seeds := []struct {
		key   string
		empty bool
		value string
	}{
		{"profile_tag", cfg.ProfileTag == "", config.DefaultProfileTag},
		{"machine_id", cfg.MachineID == "", config.DeriveMachineID()},
	}
	for _, s := range seeds {
		if !s.empty || s.value == "" {
			continue
		}
		if wErr := config.WriteConfigKey(paths, "", s.key, s.value); wErr != nil {
			slog.Warn("auto-init: failed to seed identity key",
				"key", s.key, "error", wErr)
		}
	}
}

// defaultMachineID picks a reasonable default for the machine_id field
// when auto-creating the global config. Falls back to "unknown" when
// hostname lookup fails so the file stays writable in odd environments
// (containers without /etc/hostname, sandbox runners, …).
func defaultMachineID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return fallbackMachineID
	}
	// Sanitize: machine_id sits in a filesystem path component, so it
	// MUST satisfy config.IsValidPathSegment. A typical hostname like
	// "my-laptop.local" already satisfies this, but a hostname with
	// a slash or dot-traversal would be rejected by WriteConfigKey
	// downstream. Preempt that by collapsing to "unknown".
	if !config.IsValidPathSegment(host) {
		return fallbackMachineID
	}
	return host
}

// IsAutoInitDisabled reports whether the user has opted out of auto-init
// via the HAMS_NO_AUTO_INIT env var. Tests that want to keep the legacy
// "no store directory configured" behavior set this to "1" so they can
// assert the negative path without touching $HOME.
func IsAutoInitDisabled() bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv("HAMS_NO_AUTO_INIT")))
	return envTrue[val]
}

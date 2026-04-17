package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
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
// Idempotent: if the file exists, EnsureGlobalConfig is a no-op.
func EnsureGlobalConfig(paths config.Paths) error {
	target := paths.GlobalConfigPath()
	if _, err := os.Stat(target); err == nil {
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
// Returns ("", false, err) when auto-init fails.
//
// Returns (storePath, autoInited=true, nil) when this call performed
// the auto-init — callers may surface a one-time message to the user.
//
// Returns (storePath, autoInited=false, nil) when the path was already
// configured.
func EnsureStoreReady(paths config.Paths, cfg *config.Config, cliOverride string) (storePath string, autoInited bool, err error) {
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
		// Persist for next-time discoverability even if the in-memory
		// cfg already had it (cheap; no-op when value matches).
		if err := config.WriteConfigKey(paths, "", "store_path", target); err != nil {
			slog.Warn("auto-init: failed to persist store_path to global config",
				"error", err, "path", target)
		}
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

	if cfg != nil {
		cfg.StorePath = target
	}

	slog.Info("auto-initialized hams store",
		"path", logging.TildePath(target),
		"hint", "edit ~/.config/hams/hams.config.yaml to relocate")
	return target, true, nil
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

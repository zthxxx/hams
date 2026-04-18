package cli

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
)

// storeTemplateFS holds the embedded files that seed a new store
// directory. Keeping the templates as real files inside the binary
// (rather than inlined string literals) means anyone modifying the
// scaffolded content only needs to edit the file — no code change,
// no re-release of the surrounding helper.
//
// File naming note: `gitignore` (not `.gitignore`) is intentional
// because Go's embed tooling skips dotfiles unless an `all:` prefix
// is used. The scaffolder renames it to `.gitignore` at write time.
//
//go:embed template/store
var storeTemplateFS embed.FS

// gitInitExec is the DI seam for running `git init <dir>`. Tests
// swap this to a fake that records the invocation without touching
// the real git binary. Production value shells out to git.
var gitInitExec = func(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "init", "--quiet", dir) //nolint:gosec // dir is a trusted, in-process path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// defaultStoreLocation returns the path hams picks when the user
// invokes a provider wrap on a machine that has no store configured
// at all. Resolution order:
//
//  1. `$HAMS_STORE` env var (explicit user override).
//  2. `${HAMS_DATA_HOME}/store` (under the app's data directory so
//     the scaffolded directory follows the same placement rules as
//     logs and OTel traces).
//  3. `~/.local/share/hams/store` (same as #2 once XDG defaults fire).
//
// The returned path is NOT created here; the caller runs MkdirAll.
func defaultStoreLocation(paths config.Paths) string {
	if env := os.Getenv("HAMS_STORE"); env != "" {
		expanded, _ := config.ExpandHome(env) //nolint:errcheck // best-effort; returns input unchanged on error
		return expanded
	}
	return filepath.Join(paths.DataHome, "store")
}

// EnsureStoreScaffolded guarantees that a usable store directory
// exists for the current invocation — if one doesn't, it scaffolds a
// minimal repo at the default location and persists the pointer so
// every subsequent `hams apply` / `hams <provider> …` works with no
// flags.
//
// Scaffolding is the "first-time setup" variant of `hams store init`:
//
//  1. Pick a store path (flags.Store > cfg.StorePath >
//     defaultStoreLocation).
//  2. mkdir -p <store-path>.
//  3. `git init <store-path>` if `.git` is absent — required because
//     the whole point of the store is to be a git-tracked repo that
//     the user syncs between machines.
//  4. Write `<store>/.gitignore` from the embedded template if the
//     file does not already exist. Idempotent.
//  5. Write `<store>/hams.config.yaml` from the embedded template if
//     missing.
//  6. Write the resolved store path back to the global config's
//     `store_path` so the next invocation finds it automatically.
//
// Returns the scaffolded store path (which callers set on
// flags.Store so downstream config.Load picks it up) and nil error
// on success.
//
// When a store_path is ALREADY configured (either via flag or
// config) AND the directory already exists, this is a no-op — the
// guarantee is that after calling this function, the user's store
// is on disk, not that a fresh one is created every time.
func EnsureStoreScaffolded(ctx context.Context, paths config.Paths, flags *provider.GlobalFlags) (string, error) {
	storePath := flags.Store
	if storePath == "" {
		cfg, loadErr := config.Load(paths, "", "")
		if loadErr == nil && cfg.StorePath != "" {
			storePath = cfg.StorePath
		}
	}
	if storePath == "" {
		storePath = defaultStoreLocation(paths)
	}

	existsAsDir, err := storeDirExists(storePath)
	if err != nil {
		return "", fmt.Errorf("stat store path %s: %w", storePath, err)
	}
	if !existsAsDir {
		if flags.DryRun {
			fmt.Fprintf(flags.Stderr(), "[dry-run] Would scaffold store at %s\n",
				logging.TildePath(storePath))
			return storePath, nil
		}
		if mkErr := os.MkdirAll(storePath, 0o750); mkErr != nil {
			return "", fmt.Errorf("creating store directory: %w", mkErr)
		}
	}

	if flags.DryRun {
		// Subsequent side effects are scaffolding details; the top-
		// level preview message covers the intent. Skip the git init
		// and file writes so --dry-run stays truly side-effect-free.
		return storePath, nil
	}

	if scaffErr := scaffoldStoreFiles(ctx, storePath); scaffErr != nil {
		return "", scaffErr
	}

	// Persist the resolved store path so the next invocation of hams
	// doesn't need to re-scaffold or re-resolve. Best-effort —
	// WriteConfigKey logs on failure but doesn't block the current
	// command (the store still works for THIS invocation because
	// flags.Store is populated in-memory).
	if persistErr := config.WriteConfigKey(paths, "", "store_path", storePath); persistErr != nil {
		slog.Warn("failed to persist store_path after scaffold", "error", persistErr)
	}

	// Seed machine-scoped identity (profile_tag + machine_id) so the
	// user does not see the "using 'default'/'unknown'" nudge on every
	// subsequent `hams <provider>` invocation. This makes the
	// first-time onboarding loop single-shot: `hams brew install htop`
	// produces a complete, silent, committable config in one command.
	//
	// Only seed keys that are still empty — respects a user who ran
	// `hams config set profile_tag macOS` before their first provider
	// install. WriteConfigKey is keychain-aware but these two keys
	// are plain identifiers (not secrets), so they land in the
	// global config file, not a keychain entry.
	seedIfMissing(paths, "profile_tag", func() string { return config.DefaultProfileTag })
	seedIfMissing(paths, "machine_id", config.DeriveMachineID)

	return storePath, nil
}

// seedIfMissing writes `value` to the global config file under `key`
// when and only when that key is currently empty (or the config file
// does not yet exist). Failures are logged but not propagated — the
// scaffold path is best-effort and the surrounding command should not
// fail because an optional identity key could not be written.
func seedIfMissing(paths config.Paths, key string, value func() string) {
	cfg, err := config.Load(paths, "", "")
	if err == nil {
		switch key {
		case "profile_tag":
			if cfg.ProfileTag != "" {
				return
			}
		case "machine_id":
			if cfg.MachineID != "" {
				return
			}
		}
	}
	v := value()
	if v == "" {
		return
	}
	if wErr := config.WriteConfigKey(paths, "", key, v); wErr != nil {
		slog.Warn("failed to seed config key after scaffold", "key", key, "error", wErr)
	}
}

// scaffoldStoreFiles runs `git init` + writes the embedded template
// files into storePath, both idempotently. Extracted from
// EnsureStoreScaffolded to keep the helper's control flow shallow.
func scaffoldStoreFiles(ctx context.Context, storePath string) error {
	gitDir := filepath.Join(storePath, ".git")
	if _, err := os.Stat(gitDir); errors.Is(err, fs.ErrNotExist) {
		// Time-box git init so a hung network hook (rare but real on
		// corporate boxes that wire global hooks) cannot wedge the
		// first-time setup path.
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if initErr := gitInitExec(initCtx, storePath); initErr != nil {
			return fmt.Errorf("git init %s: %w", storePath, initErr)
		}
		slog.Info("initialized store git repo", "path", storePath)
	}

	// Write-if-missing for each bundled template file. The embed FS
	// entries are listed with their source names ("gitignore",
	// "hams.config.yaml"); the on-disk destination renames gitignore
	// to the dotted form.
	templates := []struct{ src, dst string }{
		{"template/store/gitignore", ".gitignore"},
		{"template/store/hams.config.yaml", "hams.config.yaml"},
	}
	for _, t := range templates {
		dstPath := filepath.Join(storePath, t.dst)
		if _, statErr := os.Stat(dstPath); statErr == nil {
			continue // already exists — do not clobber the user's edits.
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", dstPath, statErr)
		}
		body, readErr := storeTemplateFS.ReadFile(t.src)
		if readErr != nil {
			return fmt.Errorf("reading embedded template %s: %w", t.src, readErr)
		}
		if writeErr := os.WriteFile(dstPath, body, 0o600); writeErr != nil {
			return fmt.Errorf("writing %s: %w", dstPath, writeErr)
		}
		slog.Info("scaffolded store file", "path", dstPath)
	}

	return nil
}

// storeDirExists returns (true, nil) when storePath is a directory,
// (false, nil) when the path does not exist, and propagates stat
// errors for other cases (e.g. permission denied — we should not
// silently paper over those).
func storeDirExists(storePath string) (bool, error) {
	info, err := os.Stat(storePath)
	if err == nil && info.IsDir() {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return false, fmt.Errorf("%s exists but is not a directory", storePath)
}

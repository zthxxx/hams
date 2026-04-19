package storeinit

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

	gogit "github.com/go-git/go-git/v5"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
	"github.com/zthxxx/hams/internal/provider"
)

// templateFS holds the embedded files that seed a new store directory.
// Keeping templates as real files inside the binary (rather than inlined
// string literals) means anyone modifying the scaffolded content only
// needs to edit the file — no code change, no re-release of the
// surrounding helper.
//
// Filename note: `gitignore` (not `.gitignore`) is intentional because
// Go's embed tooling skips dotfiles unless an `all:` prefix is used.
// Bootstrap renames it to `.gitignore` at write time.
//
//go:embed template
var templateFS embed.FS

// GitInitTimeout bounds the shell-out to `git init` so a hung corporate
// `core.hooksPath` cannot wedge the first-run path. go-git's PlainInit
// runs in-process and does not need this guard. Exposed as a var so
// tests can shrink it without touching the production timeout.
//
//nolint:gochecknoglobals // DI seam; immutable after init except in tests.
var GitInitTimeout = 30 * time.Second

// LookPathGit is the DI seam that resolves the `git` binary on PATH.
// Tests rebind this to return exec.ErrNotFound to force the go-git
// fallback branch in defaultExecGitInit, without having to swap the
// whole ExecGitInit function.
//
//nolint:gochecknoglobals // DI seam; immutable after init except in tests.
var LookPathGit = func() (string, error) { return exec.LookPath("git") }

// ExecCommandContext is the DI seam for spawning `git init`. Tests
// rebind this to record invocations or simulate spawn failure without
// having to swap the whole ExecGitInit function.
//
//nolint:gochecknoglobals // DI seam; immutable after init except in tests.
var ExecCommandContext = exec.CommandContext

// ExecGitInit is the function-level DI seam that shells out to the
// system `git` when it is on PATH. Tests rebind this to a fake that
// records the call without forking a real child. The default
// implementation is composed of the fine-grained LookPathGit /
// ExecCommandContext / GitInitTimeout seams above; rebind those for
// targeted slice-of-pipeline mocking, or rebind ExecGitInit itself
// for whole-function replacement.
//
//nolint:gochecknoglobals // DI seam; immutable after init except in tests.
var ExecGitInit = defaultExecGitInit

// GoGitInit is the DI seam that falls back to the bundled go-git
// library when the system `git` is absent. Tests rebind this to assert
// the fallback branch is (or is not) taken. Production value calls
// `gogit.PlainInit(dir, false)` to produce a non-bare repository with
// the same `.git/HEAD` + `.git/config` layout the CLI would.
//
//nolint:gochecknoglobals // DI seam; immutable after init except in tests.
var GoGitInit = defaultGoGitInit

func defaultExecGitInit(ctx context.Context, dir string) error {
	gitBin, lookErr := LookPathGit()
	if lookErr != nil {
		// Signal "git is not available" so Bootstrap's caller triggers
		// the go-git fallback. Wrap so callers can match via errors.Is.
		return fmt.Errorf("git not on PATH: %w", errors.Join(lookErr, exec.ErrNotFound))
	}
	initCtx, cancel := context.WithTimeout(ctx, GitInitTimeout)
	defer cancel()
	// gitBin comes from LookPathGit (resolved via exec.LookPath in
	// production); dir comes from a trusted in-process value. Safe.
	cmd := ExecCommandContext(initCtx, gitBin, "init", "--quiet", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultGoGitInit(_ context.Context, dir string) error {
	if _, err := gogit.PlainInit(dir, false); err != nil {
		return fmt.Errorf("go-git PlainInit: %w", err)
	}
	return nil
}

// DefaultLocation returns the path hams picks when the user invokes a
// provider wrap on a machine that has no store configured at all.
// Resolution order:
//
//  1. `$HAMS_STORE` env var (explicit user override).
//  2. `${HAMS_DATA_HOME}/store` (under the app's data directory so the
//     scaffolded directory follows the same placement rules as logs
//     and OTel traces).
//  3. `~/.local/share/hams/store` (same as #2 once XDG defaults fire).
//
// The returned path is NOT created here; Bootstrap runs MkdirAll.
func DefaultLocation(paths config.Paths) string {
	if env := os.Getenv("HAMS_STORE"); env != "" {
		expanded, _ := config.ExpandHome(env) //nolint:errcheck // best-effort; returns input unchanged on error.
		return expanded
	}
	return filepath.Join(paths.DataHome, "store")
}

// Bootstrapped reports whether dir already looks like a hams-initialized
// store. Useful when callers want to distinguish "no-op second call"
// from "first-time scaffold". Returns true when the directory contains
// BOTH a `.git` marker AND a `hams.config.yaml` template.
func Bootstrapped(dir string) bool {
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "hams.config.yaml")); err != nil {
		return false
	}
	return true
}

// Bootstrap guarantees that a usable store directory exists for the
// current invocation. It is the one entry point the CLI layer calls
// when it discovers the store is missing; everything (flag-pick,
// dry-run short-circuit, `git init`, go-git fallback, template writes,
// `store_path` persistence, identity seeding) happens inside.
//
// Resolution order for the store path:
//
//  1. flags.Store (highest, from --store).
//  2. Loaded config's StorePath.
//  3. DefaultLocation(paths).
//
// Scaffolding side effects (skipped entirely when flags.DryRun is set):
//
//   - mkdir -p <store>
//   - `git init <store>` via ExecGitInit, OR go-git PlainInit via
//     GoGitInit when ExecGitInit returns exec.ErrNotFound.
//   - Write .gitignore + hams.config.yaml from the embedded templates,
//     idempotently (do not clobber user edits).
//   - Persist `store_path` to the global config.
//   - Seed `profile_tag` + `machine_id` via seedIfMissing so the next
//     CLI invocation is silent.
//
// Returns the resolved store path. When Bootstrapped(path) was already
// true on entry, the function is a no-op for the filesystem but still
// re-persists `store_path` so a stale global config heals itself.
func Bootstrap(ctx context.Context, paths config.Paths, flags *provider.GlobalFlags) (string, error) {
	storePath := resolveStorePath(paths, flags)

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
		// level preview covers the intent. Skip `git init`, template
		// writes, and identity seeding so --dry-run stays truly
		// side-effect-free.
		return storePath, nil
	}

	if scaffErr := scaffoldFiles(ctx, storePath); scaffErr != nil {
		return "", scaffErr
	}

	// Persist the resolved store path so the next invocation of hams
	// does not need to re-resolve. Best-effort — WriteConfigKey logs
	// on failure but the current invocation still works because
	// flags.Store gets populated by the caller.
	if persistErr := config.WriteConfigKey(paths, "", "store_path", storePath); persistErr != nil {
		slog.Warn("storeinit: failed to persist store_path", "error", persistErr)
	}

	// Seed machine-scoped identity (profile_tag + machine_id) so the
	// user does not see the "using 'default'/'unknown'" nudge on every
	// subsequent `hams <provider>` invocation. Respects a user who
	// pre-set `profile_tag` via `hams config set`.
	seedIfMissing(paths, "profile_tag", func() string { return config.DefaultProfileTag })
	seedIfMissing(paths, "machine_id", config.DeriveMachineID)

	return storePath, nil
}

// resolveStorePath picks the final store path using the precedence
// documented on Bootstrap. Extracted for readability; not exported.
func resolveStorePath(paths config.Paths, flags *provider.GlobalFlags) string {
	if flags != nil && flags.Store != "" {
		return flags.Store
	}
	cfg, loadErr := config.Load(paths, "", "")
	if loadErr == nil && cfg.StorePath != "" {
		return cfg.StorePath
	}
	return DefaultLocation(paths)
}

// scaffoldFiles runs `git init` (exec or fallback) and writes the
// embedded template files into storePath, both idempotently.
func scaffoldFiles(ctx context.Context, storePath string) error {
	if err := ensureGitRepo(ctx, storePath); err != nil {
		return err
	}

	// Write-if-missing for each bundled template file. Source names
	// carry no leading dot (Go embed quirk); destination names add
	// the dot where appropriate.
	templates := []struct{ src, dst string }{
		{"template/gitignore", ".gitignore"},
		{"template/hams.config.yaml", "hams.config.yaml"},
	}
	for _, t := range templates {
		dstPath := filepath.Join(storePath, t.dst)
		if _, statErr := os.Stat(dstPath); statErr == nil {
			continue // already exists — do not clobber the user's edits.
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", dstPath, statErr)
		}
		body, readErr := templateFS.ReadFile(t.src)
		if readErr != nil {
			return fmt.Errorf("reading embedded template %s: %w", t.src, readErr)
		}
		if writeErr := os.WriteFile(dstPath, body, 0o600); writeErr != nil {
			return fmt.Errorf("writing %s: %w", dstPath, writeErr)
		}
		slog.Info("storeinit: scaffolded store file", "path", dstPath)
	}

	return nil
}

// ensureGitRepo runs `git init` on storePath when `.git` is absent.
// Prefers the system `git` via ExecGitInit and falls back to the
// bundled go-git via GoGitInit when ExecGitInit reports that git is
// not available. Any other exec failure propagates unchanged — the
// fallback is strictly for "git missing", not "git misconfigured".
func ensureGitRepo(ctx context.Context, storePath string) error {
	gitDir := filepath.Join(storePath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil // already an initialized repo
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", gitDir, err)
	}

	err := ExecGitInit(ctx, storePath)
	switch {
	case err == nil:
		slog.Info("storeinit: initialized store git repo", "path", storePath, "via", "exec")
		return nil
	case errors.Is(err, exec.ErrNotFound):
		// System git missing — fall through to the bundled go-git
		// fallback. This is the `project-structure/spec.md:686-699`
		// scenario: "The go-git dependency SHALL be used as a fallback
		// when system `git` is not available."
		slog.Info("storeinit: used bundled go-git fallback for git init",
			"path", storePath, "reason", "system git not on PATH")
		if goErr := GoGitInit(ctx, storePath); goErr != nil {
			return fmt.Errorf("go-git PlainInit %s: %w", storePath, goErr)
		}
		return nil
	default:
		// Any other error (permission denied, hook crash, hung hook)
		// is a real problem that the fallback would only hide. Let
		// the operator see the underlying failure.
		return fmt.Errorf("git init %s: %w", storePath, err)
	}
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
		slog.Warn("storeinit: failed to seed config key", "key", key, "error", wErr)
	}
}

// storeDirExists returns (true, nil) when storePath is a directory,
// (false, nil) when the path does not exist, and propagates stat
// errors for other cases (e.g., permission denied — we should not
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

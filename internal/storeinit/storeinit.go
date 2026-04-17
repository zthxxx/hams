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

	gogit "github.com/go-git/go-git/v5"
)

//go:embed template/*
var templateFS embed.FS

// Bootstrap prepares dir as a hams store: create the directory tree,
// run `git init`, then materialize every file under the embedded template
// (idempotent — files that already exist are NOT overwritten so users
// can hand-edit the auto-generated config without losing changes on the
// next call).
//
// Idempotent across re-runs: a directory that is already a valid store
// returns nil without re-initing git. Use [Bootstrapped] to check whether
// dir was a fresh init vs an already-initialized store.
//
// Falls back to in-process go-git when the `git` CLI is not on PATH so
// the path also works inside fresh-machine containers and on systems
// where users have not installed git yet (per the hams "fresh machine"
// design constraint — the binary bundles go-git for exactly this case).
func Bootstrap(dir string) error {
	return BootstrapContext(context.Background(), dir)
}

// BootstrapContext is the context-aware version of [Bootstrap]. The
// supplied ctx is forwarded to the `git init` exec invocation so a
// canceled CLI run does not leave a half-initialized directory behind.
func BootstrapContext(ctx context.Context, dir string) error {
	if dir == "" {
		return errors.New("storeinit: dir must not be empty")
	}

	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		return fmt.Errorf("storeinit: creating store directory: %w", mkErr)
	}

	if !isGitRepo(dir) {
		if initErr := initGitRepo(ctx, dir); initErr != nil {
			return fmt.Errorf("storeinit: git init: %w", initErr)
		}
		slog.Info("storeinit: initialized git repo", "path", dir)
	} else {
		slog.Debug("storeinit: directory already a git repo, skipping init", "path", dir)
	}

	if writeErr := writeTemplate(dir); writeErr != nil {
		return fmt.Errorf("storeinit: materializing template: %w", writeErr)
	}

	defaultProfile := filepath.Join(dir, "default")
	if mkErr := os.MkdirAll(defaultProfile, 0o750); mkErr != nil {
		return fmt.Errorf("storeinit: creating default profile dir: %w", mkErr)
	}

	return nil
}

// Bootstrapped returns true when dir already looks like a hams-initialized
// store (has .git AND hams.config.yaml). Useful for tests that want to
// distinguish "freshly bootstrapped" from "no-op call".
func Bootstrapped(dir string) bool {
	if !isGitRepo(dir) {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "hams.config.yaml")); err != nil {
		return false
	}
	return true
}

func isGitRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		return true
	}
	return false
}

func initGitRepo(ctx context.Context, dir string) error {
	if gitBin, err := exec.LookPath("git"); err == nil {
		// gitBin came from LookPath, dir came from os.MkdirAll above —
		// neither is user-input directly tainted at this point.
		cmd := exec.CommandContext(ctx, gitBin, "init", "--quiet", dir) //nolint:gosec // gitBin resolved via LookPath; dir validated by caller
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			return fmt.Errorf("git init failed: %w (output: %s)", runErr, out)
		}
		return nil
	}
	if _, err := gogit.PlainInit(dir, false); err != nil {
		return fmt.Errorf("go-git PlainInit failed: %w", err)
	}
	return nil
}

func writeTemplate(dir string) error {
	return fs.WalkDir(templateFS, "template", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "template" {
			return nil
		}

		rel, relErr := filepath.Rel("template", path)
		if relErr != nil {
			return relErr
		}
		dest := filepath.Join(dir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o750)
		}

		if _, err := os.Stat(dest); err == nil {
			slog.Debug("storeinit: file already exists, skipping", "path", dest)
			return nil
		}

		data, readErr := templateFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading embedded template %s: %w", path, readErr)
		}

		if writeErr := os.WriteFile(dest, data, 0o600); writeErr != nil {
			return fmt.Errorf("writing template file %s: %w", dest, writeErr)
		}
		slog.Info("storeinit: wrote template file", "path", dest)
		return nil
	})
}

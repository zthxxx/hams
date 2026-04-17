package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/logging"
)

// bootstrapFromRepo resolves a store repo (local path or remote URL) and returns
// the local store path. Local paths are resolved first; if the path exists as a
// directory with a .git folder, it is used directly. Otherwise, it is treated as
// a remote URL (with GitHub shorthand expansion) and cloned.
//
// Input that clearly looks like a local path attempt (starts with `/`, `~/`,
// `./`, `../`, or points at an existing directory) does NOT fall through to
// remote cloning when resolveLocalRepo rejects it — otherwise a typo in the
// local path turned into a confusing "authentication required" error against
// https://github.com//<path>. Local-looking inputs now surface the real local
// error (e.g., "not a git repository").
func bootstrapFromRepo(ctx context.Context, repo string, paths config.Paths) (string, error) {
	// Priority 1: check if repo is a local path.
	localPath, err := resolveLocalRepo(repo)
	if err == nil {
		slog.Info("using local store repo", "path", logging.TildePath(localPath))
		return localPath, nil
	}
	if isLocalPathAttempt(repo) {
		return "", fmt.Errorf("resolving local store repo: %w", err)
	}

	// Priority 2: treat as remote URL (expand GitHub shorthand).
	return cloneRemoteRepo(ctx, repo, paths)
}

// resolveFromRepoStorePath picks the store path to use for a given
// --from-repo (or configured store_repo) input. In normal mode it
// clones/pulls on demand. In dry-run mode it refuses to touch the
// network or filesystem: if a local copy exists (direct local path
// or cached clone) it's reused; otherwise the caller is told
// "Would clone" and `done=true` signals it should return nil
// immediately. Symmetric with the --dry-run --bootstrap branch
// (cycle `6f8cbeb`).
//
// Returns (path, done, err):
//   - path != "", done=false, err=nil — caller proceeds with path
//   - path == "", done=true, err=nil  — caller returns nil (dry-run
//     would clone)
//   - path == "", done=false, err!=nil — caller propagates err
func resolveFromRepoStorePath(ctx context.Context, repo string, paths config.Paths, dryRun bool) (storePath string, done bool, err error) {
	if !dryRun {
		path, cloneErr := bootstrapFromRepo(ctx, repo, paths)
		return path, false, cloneErr
	}
	if preview, ok := previewExistingStoreFromRepo(repo, paths); ok {
		return preview, false, nil
	}
	// Cycle 250: dry-run preview text goes to stderr, not stdout.
	// Symmetric with the real-clone progress (above) — stdout stays
	// reserved for the apply's primary output (the JSON summary in
	// --json mode). CI scripts running `hams --json --dry-run apply
	// --from-repo=X | jq .` would otherwise see this prose on stdout
	// before the summary, breaking JSON parsing.
	fmt.Fprintf(os.Stderr, "[dry-run] Would clone %s. Re-run without --dry-run to clone and preview the plan.\n", repo)
	return "", true, nil
}

// previewExistingStoreFromRepo returns the local store path only if
// the input resolves to something already on disk — either a valid
// local git repo or a prior clone cached under `${HAMS_DATA_HOME}/repo/`.
// Returns ("", false) when the repo would need to be cloned.
//
// Called exclusively by the `--dry-run --from-repo=X` branch so the
// preview path can skip the network + disk write and report "Would
// clone X" when no local copy exists yet.
func previewExistingStoreFromRepo(repo string, paths config.Paths) (string, bool) {
	if localPath, err := resolveLocalRepo(repo); err == nil {
		return localPath, true
	}
	clonePath := resolveClonePath(repo, paths)
	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
		return clonePath, true
	}
	return "", false
}

// isLocalPathAttempt reports whether the input string unambiguously looks
// like a local filesystem path — either prefixed with an explicit path
// separator marker (`/`, `~/`, `./`, `../`) or naming an existing
// filesystem entry. Ambiguous bare names like "user/repo" are NOT
// considered local (they fall through to the GitHub-shorthand path).
func isLocalPathAttempt(repo string) bool {
	if strings.HasPrefix(repo, "/") ||
		strings.HasPrefix(repo, "~/") ||
		strings.HasPrefix(repo, "./") ||
		strings.HasPrefix(repo, "../") {
		return true
	}
	// Last resort: stat it. `user/repo` (GitHub shorthand) won't exist
	// on disk so this check won't misfire for the common remote case.
	if _, err := os.Stat(repo); err == nil {
		return true
	}
	return false
}

// resolveLocalRepo checks if the given path is a local git repository.
// Expands ~ prefix. Returns the absolute path if valid, error otherwise.
func resolveLocalRepo(repo string) (string, error) {
	path := repo

	// Expand ~ to home directory.
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Check if it's an absolute or relative path that exists.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	// Must be a directory with .git (non-bare) or HEAD (bare repo).
	info, statErr := os.Stat(absPath)
	if statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("not a local directory: %s", absPath)
	}

	gitDir := filepath.Join(absPath, ".git")
	headFile := filepath.Join(absPath, "HEAD")
	if _, gitErr := os.Stat(gitDir); gitErr != nil {
		if _, headErr := os.Stat(headFile); headErr != nil {
			return "", fmt.Errorf("not a git repository (no .git or HEAD): %s", absPath)
		}
	}

	return absPath, nil
}

// cloneRemoteRepo clones a remote git repository into the data home.
func cloneRemoteRepo(ctx context.Context, repo string, paths config.Paths) (string, error) {
	// Expand to a full URL based on input shape.
	repoURL := expandRepoShorthand(repo)

	// Determine clone path from repo identifier.
	clonePath := resolveClonePath(repo, paths)

	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
		// Already cloned — pull latest. PullContext (not Pull) so Ctrl+C
		// aborts a hanging network fetch instead of waiting for go-git's
		// default timeout (can be minutes).
		slog.Info("pulling latest changes", "path", logging.TildePath(clonePath))
		r, openErr := gogit.PlainOpen(clonePath)
		if openErr != nil {
			return clonePath, fmt.Errorf("opening repo %s: %w", clonePath, openErr)
		}
		w, wtErr := r.Worktree()
		if wtErr != nil {
			return clonePath, fmt.Errorf("getting worktree: %w", wtErr)
		}
		if pullErr := w.PullContext(ctx, &gogit.PullOptions{}); pullErr != nil && !errors.Is(pullErr, gogit.NoErrAlreadyUpToDate) {
			slog.Warn("pull failed, using existing state", "error", pullErr)
		}
		return clonePath, nil
	}

	// Clone. PlainCloneContext (not PlainClone) so Ctrl+C during the
	// initial clone aborts promptly instead of waiting for network
	// timeout. Previously users saw hams appear hung during
	// --from-repo clones even after SIGINT.
	//
	// Cycle 250: progress / status messages go to stderr, not stdout.
	// Pre-cycle-250 `fmt.Printf(...)` and `Progress: os.Stdout`
	// interleaved clone progress into stdout — which made
	// `hams --json apply --from-repo=<X> | jq .` fail on invalid
	// JSON (progress lines appeared before the final summary object).
	// Standard UNIX convention: stdout is for the command's primary
	// output (the JSON summary), stderr is for diagnostics / progress
	// (status messages, git transfer progress). git itself writes
	// progress to stderr. This also makes `hams apply --from-repo=X
	// > out.log` leave progress on the user's terminal while routing
	// the final output to the file.
	fmt.Fprintf(os.Stderr, "Downloading Hams Store to %s\n", logging.TildePath(clonePath))
	_, err := gogit.PlainCloneContext(ctx, clonePath, false, &gogit.CloneOptions{
		URL:      repoURL,
		Progress: os.Stderr,
	})
	if err != nil {
		return "", transformCloneError(repoURL, err)
	}

	fmt.Fprintf(os.Stderr, "Download Hams Store success\n")
	fmt.Fprintf(os.Stderr, "Profile Store is %s now\n\n", logging.TildePath(clonePath))
	return clonePath, nil
}

// transformCloneError re-phrases go-git error messages that would
// confuse users. Currently catches the "Repository not found"
// signature (which go-git wraps as "authentication required: ...")
// and returns a UserFacingError with actionable suggestions. Other
// errors propagate verbatim with a short "cloning <url>:" prefix.
// Extracted from cloneRemoteRepo so the error-transform branch is
// directly unit-testable without network.
func transformCloneError(repoURL string, err error) error {
	if strings.Contains(err.Error(), "Repository not found") {
		return hamserr.NewUserError(hamserr.ExitGeneralError,
			fmt.Sprintf("repository %s not found or not accessible", repoURL),
			"Verify the URL is correct (case-sensitive on GitHub)",
			"For private repos: configure a git credential helper or use SSH URL",
			"For local paths: use an absolute path starting with / or ~/",
		)
	}
	return fmt.Errorf("cloning %s: %w", repoURL, err)
}

// expandRepoShorthand mirrors `resolveClonePath`'s host-detection
// heuristic so that `--from-repo` accepts the same range of shapes
// uniformly. Without this, a user typing `--from-repo=gitlab.com/x/y`
// pre-cycle-225 had it silently rewritten to
// `https://github.com/gitlab.com/x/y` — go-git then returned a
// confusing "Repository not found" against the wrong host.
//
// Recognized shapes (must align with resolveClonePath):
//   - `scheme://host/path`        → returned verbatim
//   - `git@host:path`             → returned verbatim
//   - `host.tld/user/repo`        → `https://host.tld/user/repo`
//     (heuristic: first segment contains a dot, signaling a host)
//   - `user/repo` (no dot)        → `https://github.com/user/repo`
//     (GitHub shorthand, the documented common case)
//   - anything else (one segment) → returned verbatim; clone will
//     fail with go-git's URL-format error so the user sees a real
//     diagnostic rather than a silent github.com prefix.
func expandRepoShorthand(repo string) string {
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 && strings.Contains(parts[0], ".") {
		return "https://" + repo
	}
	if len(parts) == 2 {
		return "https://github.com/" + repo
	}
	return repo
}

// resolveClonePath returns the local cache directory for a remote
// `--from-repo`. The path includes the HOST component so that two
// repos with the same `<user>/<repo>` on different forges don't
// collide. Without the host scoping, `--from-repo=github.com/x/y`
// and `--from-repo=gitlab.com/x/y` would both resolve to
// `${DataHome}/repo/x/y` — the second clone would silently
// inherit the first's `.git` and pull from the wrong origin
// (or fail with confusing remote errors).
//
// Recognized forms:
//   - `user/repo` shorthand → assumed github.com → `repo/github.com/user/repo`
//   - `git@host:user/repo[.git]` → `repo/host/user/repo`
//   - `https://host/user/repo[.git]` → `repo/host/user/repo`
//   - anything else (defensive fallback): use the trimmed input
//     verbatim under `repo/`.
func resolveClonePath(repo string, paths config.Paths) string {
	repoName := strings.TrimSuffix(repo, ".git")

	// SSH form: git@host:user/repo
	if rest, ok := strings.CutPrefix(repoName, "git@"); ok {
		if host, path, found := strings.Cut(rest, ":"); found {
			return filepath.Join(paths.DataHome, "repo", host, path)
		}
	}

	// URL form: scheme://host/user/repo
	if idx := strings.Index(repoName, "://"); idx >= 0 {
		afterScheme := repoName[idx+len("://"):]
		host, path, found := strings.Cut(afterScheme, "/")
		if found && host != "" && path != "" {
			return filepath.Join(paths.DataHome, "repo", host, path)
		}
	}

	// Shorthand `user/repo` — assume github.com (no dot in first segment).
	parts := strings.Split(repoName, "/")
	if len(parts) == 2 && !strings.Contains(parts[0], ".") {
		return filepath.Join(paths.DataHome, "repo", "github.com", parts[0], parts[1])
	}

	// Cycle 225: host-prefixed shorthand `host.tld/user/repo` (or
	// deeper, e.g. `gitea.example/group/sub/repo`) — first segment
	// contains a dot, so treat it as a host and keep the rest as-is.
	// Pre-cycle-225 inputs like `gitlab.com/team/repo` fell into the
	// defensive 2-segment fallback, producing `/data/hams/repo/team/repo`
	// (no host scoping → forge collisions).
	if len(parts) >= 2 && strings.Contains(parts[0], ".") {
		return filepath.Join(paths.DataHome, "repo", parts[0], filepath.Join(parts[1:]...))
	}

	// Defensive fallback: use the input verbatim. Same behavior as
	// the pre-cycle-168 code, modulo the host-prefix scoping.
	if len(parts) >= 2 {
		repoName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return filepath.Join(paths.DataHome, "repo", repoName)
}

// promptProfileInit asks the user for profile tag and machine ID.
//
// Cycle 198: validates non-empty input via isValidConfigSegment. The
// cycle 195 sanitizer silently collapses invalid values to fallback
// at runtime; cycle 197 rejects them at config.WriteConfigKey; here
// we reject at the prompt so the user gets an immediate error
// instead of a confusing "typed-but-not-stored" discrepancy (the
// in-memory cfg.ProfileTag would hold the invalid value while the
// persisted YAML rejected the write and defaulted).
func promptProfileInit() (tag, machineID string, err error) {
	reader := bufio.NewReader(os.Stdin)

	// Cycle 252: prompts go to stderr, not stdout. Pre-cycle-252
	// `fmt.Print("Profile tag: ")` wrote to stdout — so an
	// interactive `hams --json apply` on a fresh machine (TTY
	// stdin, profile missing) interleaved "Profile tag: " into the
	// primary JSON output surface. Stderr is the conventional
	// channel for prompts / diagnostics; interactive users still
	// see them on the terminal, and CI consumers redirecting stdout
	// no longer get prose mixed with JSON.
	fmt.Fprint(os.Stderr, "Profile tag: ")
	tag, err = reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("reading profile tag: %w", err)
	}
	tag = strings.TrimSpace(tag)

	fmt.Fprint(os.Stderr, "Profile Machine-ID: ")
	machineID, err = reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("reading machine ID: %w", err)
	}
	machineID = strings.TrimSpace(machineID)

	if tag == "" {
		tag = "default"
	}
	if machineID == "" {
		machineID = "unknown"
	}

	if !config.IsValidPathSegment(tag) {
		return "", "", fmt.Errorf("invalid profile tag %q: must be a simple identifier (letters, digits, '.', '-', '_' — no path separators or '..')", tag)
	}
	if !config.IsValidPathSegment(machineID) {
		return "", "", fmt.Errorf("invalid machine ID %q: must be a simple identifier (letters, digits, '.', '-', '_' — no path separators or '..')", machineID)
	}

	return tag, machineID, nil
}

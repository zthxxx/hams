package cli

import (
	"bufio"
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
func bootstrapFromRepo(repo string, paths config.Paths) (string, error) {
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
	return cloneRemoteRepo(repo, paths)
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
func cloneRemoteRepo(repo string, paths config.Paths) (string, error) {
	// Expand GitHub shorthand.
	repoURL := repo
	if !strings.Contains(repo, "://") && !strings.HasPrefix(repo, "git@") {
		repoURL = "https://github.com/" + repo
	}

	// Determine clone path from repo identifier.
	clonePath := resolveClonePath(repo, paths)

	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
		// Already cloned — pull latest.
		slog.Info("pulling latest changes", "path", logging.TildePath(clonePath))
		r, openErr := gogit.PlainOpen(clonePath)
		if openErr != nil {
			return clonePath, fmt.Errorf("opening repo %s: %w", clonePath, openErr)
		}
		w, wtErr := r.Worktree()
		if wtErr != nil {
			return clonePath, fmt.Errorf("getting worktree: %w", wtErr)
		}
		if pullErr := w.Pull(&gogit.PullOptions{}); pullErr != nil && !errors.Is(pullErr, gogit.NoErrAlreadyUpToDate) {
			slog.Warn("pull failed, using existing state", "error", pullErr)
		}
		return clonePath, nil
	}

	// Clone.
	fmt.Printf("Downloading Hams Store to %s\n", logging.TildePath(clonePath))
	_, err := gogit.PlainClone(clonePath, false, &gogit.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		return "", transformCloneError(repoURL, err)
	}

	fmt.Printf("Download Hams Store success\n")
	fmt.Printf("Profile Store is %s now\n\n", logging.TildePath(clonePath))
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

func resolveClonePath(repo string, paths config.Paths) string {
	repoName := strings.TrimSuffix(repo, ".git")
	parts := strings.Split(repoName, "/")
	if len(parts) >= 2 {
		repoName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return filepath.Join(paths.DataHome, "repo", repoName)
}

// promptProfileInit asks the user for profile tag and machine ID.
func promptProfileInit() (tag, machineID string, err error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Profile tag: ")
	tag, err = reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("reading profile tag: %w", err)
	}
	tag = strings.TrimSpace(tag)

	fmt.Print("Profile Machine-ID: ")
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

	return tag, machineID, nil
}

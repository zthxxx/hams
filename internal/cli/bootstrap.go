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
	"github.com/zthxxx/hams/internal/logging"
)

// bootstrapFromRepo resolves a store repo (local path or remote URL) and returns
// the local store path. Local paths are resolved first; if the path exists as a
// directory with a .git folder, it is used directly. Otherwise, it is treated as
// a remote URL (with GitHub shorthand expansion) and cloned.
func bootstrapFromRepo(repo string, paths config.Paths) (string, error) {
	// Priority 1: check if repo is a local path.
	if localPath, err := resolveLocalRepo(repo); err == nil {
		slog.Info("using local store repo", "path", logging.TildePath(localPath))
		return localPath, nil
	}

	// Priority 2: treat as remote URL (expand GitHub shorthand).
	return cloneRemoteRepo(repo, paths)
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

	// Must be a directory with .git.
	info, statErr := os.Stat(absPath)
	if statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("not a local directory: %s", absPath)
	}

	gitDir := filepath.Join(absPath, ".git")
	if _, gitErr := os.Stat(gitDir); gitErr != nil {
		return "", fmt.Errorf("no .git directory in %s", absPath)
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
		return "", fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	fmt.Printf("Download Hams Store success\n")
	fmt.Printf("Profile Store is %s now\n\n", logging.TildePath(clonePath))
	return clonePath, nil
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

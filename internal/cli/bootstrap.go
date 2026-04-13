package cli

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/logging"
)

// bootstrapFromRepo clones a store repo and sets up the local config.
func bootstrapFromRepo(repo string, paths config.Paths) (string, error) {
	// Expand GitHub shorthand.
	repoURL := repo
	if !strings.Contains(repo, "://") && !strings.HasPrefix(repo, "git@") {
		repoURL = "https://github.com/" + repo
	}

	// Determine clone path.
	repoName := repo
	if idx := strings.LastIndex(repoName, "/"); idx >= 0 {
		// Use owner/repo as directory structure.
		parts := strings.Split(strings.TrimSuffix(repo, ".git"), "/")
		if len(parts) >= 2 {
			repoName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}
	clonePath := filepath.Join(paths.DataHome, "repo", repoName)

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
		if pullErr := w.Pull(&gogit.PullOptions{}); pullErr != nil && pullErr.Error() != "already up-to-date" {
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

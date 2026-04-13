package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// CloneProvider implements the git clone filesystem provider.
type CloneProvider struct{}

// NewCloneProvider creates a new git clone provider.
func NewCloneProvider() *CloneProvider { return &CloneProvider{} }

// Manifest returns the git clone provider metadata.
func (p *CloneProvider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "git-clone",
		DisplayName:   "git clone",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassFilesystem,
		FilePrefix:    "git-clone",
	}
}

// Bootstrap checks if git is available.
func (p *CloneProvider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}
	return nil
}

// Probe checks if the local path exists for each tracked clone.
func (p *CloneProvider) Probe(_ context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		// For git clone, the resource ID format is "remote -> local-path".
		localPath := extractLocalPath(id)
		if localPath == "" {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: "invalid format"})
			continue
		}

		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK})
		}
	}
	return results, nil
}

// Plan computes actions for git clone entries.
func (p *CloneProvider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply clones a repository to the specified path.
func (p *CloneProvider) Apply(ctx context.Context, action provider.Action) error {
	remote, localPath, branch := parseCloneResource(action.ID)
	if remote == "" || localPath == "" {
		return fmt.Errorf("git-clone: resource must be 'remote -> local-path [branch]'")
	}

	slog.Info("git clone", "remote", remote, "path", localPath)
	args := []string{"clone", remote, localPath}
	if branch != "" {
		args = append(args, "--branch", branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // git clone args from hamsfile declarations
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Remove is a no-op — we don't delete cloned repos.
func (p *CloneProvider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("git-clone: remove is a no-op (won't delete directories)", "resource", resourceID)
	return nil
}

// List returns cloned repos with status.
func (p *CloneProvider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-60s %s\n", id, r.State)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for git clone.
func (p *CloneProvider) HandleCommand(args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	if len(args) < 2 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"git-clone requires a remote URL and local path",
			"Usage: hams git-clone <remote-url> <local-path> [--branch=<branch>]",
		)
	}

	remote := args[0]
	localPath := args[1]

	if flags.DryRun {
		fmt.Printf("[dry-run] Would clone: git clone %s %s\n", remote, localPath)
		return nil
	}

	cmd := exec.CommandContext(context.Background(), "git", "clone", remote, localPath) //nolint:gosec // git clone from CLI input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Name returns the CLI name.
func (p *CloneProvider) Name() string { return "git-clone" }

// DisplayName returns the display name.
func (p *CloneProvider) DisplayName() string { return "git clone" }

func extractLocalPath(resourceID string) string {
	_, localPath, _ := parseCloneResource(resourceID)
	return localPath
}

func parseCloneResource(id string) (remote, localPath, branch string) {
	// Format: "remote -> local-path" or "remote -> local-path branch"
	parts := strings.SplitN(id, " -> ", 2)
	if len(parts) != 2 {
		return "", "", ""
	}
	remote = strings.TrimSpace(parts[0])
	rest := strings.TrimSpace(parts[1])

	spaceIdx := strings.LastIndex(rest, " ")
	if spaceIdx > 0 {
		localPath = rest[:spaceIdx]
		branch = rest[spaceIdx+1:]
	} else {
		localPath = rest
	}

	return remote, localPath, branch
}

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
)

func TestResolveLocalRepo_ValidGitDir(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	path, err := resolveLocalRepo(dir)
	if err != nil {
		t.Fatalf("resolveLocalRepo error: %v", err)
	}
	if path != dir {
		t.Errorf("path = %q, want %q", path, dir)
	}
}

func TestResolveLocalRepo_NoGitDir(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveLocalRepo(dir)
	if err == nil {
		t.Error("expected error for directory without .git")
	}
}

func TestResolveLocalRepo_NonExistent(t *testing.T) {
	_, err := resolveLocalRepo("/nonexistent/path/foo")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestResolveLocalRepo_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Create a temp dir inside home to test ~ expansion.
	testDir := filepath.Join(home, ".hams-test-resolve-local")
	gitDir := filepath.Join(testDir, ".git")
	if mkErr := os.MkdirAll(gitDir, 0o750); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	defer os.RemoveAll(testDir) //nolint:errcheck // test cleanup

	path, resolveErr := resolveLocalRepo("~/.hams-test-resolve-local")
	if resolveErr != nil {
		t.Fatalf("resolveLocalRepo with ~: %v", resolveErr)
	}
	if path != testDir {
		t.Errorf("path = %q, want %q", path, testDir)
	}
}

func TestResolveClonePath(t *testing.T) {
	paths := config.Paths{DataHome: "/data/hams"}

	tests := []struct {
		repo string
		want string
	}{
		{"zthxxx/hams-store", "/data/hams/repo/zthxxx/hams-store"},
		{"zthxxx/hams-store.git", "/data/hams/repo/zthxxx/hams-store"},
		{"single-name", "/data/hams/repo/single-name"},
	}

	for _, tt := range tests {
		got := resolveClonePath(tt.repo, paths)
		if got != tt.want {
			t.Errorf("resolveClonePath(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestBootstrapFromRepo_LocalPath(t *testing.T) {
	// Create a local git repo for testing.
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "test-store")
	if err := os.MkdirAll(repoDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Initialize a real git repo.
	cmd := exec.CommandContext(t.Context(), "git", "init", repoDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	paths := config.Paths{DataHome: filepath.Join(dir, "data")}
	storePath, err := bootstrapFromRepo(repoDir, paths)
	if err != nil {
		t.Fatalf("bootstrapFromRepo local: %v", err)
	}
	if storePath != repoDir {
		t.Errorf("storePath = %q, want %q (local path should be used directly)", storePath, repoDir)
	}
}

func TestBootstrapFromRepo_LocalPathPriority(t *testing.T) {
	// Local path should take priority over GitHub shorthand expansion.
	dir := t.TempDir()
	localDir := filepath.Join(dir, "zthxxx", "hams-store")
	gitDir := filepath.Join(localDir, ".git")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths := config.Paths{DataHome: filepath.Join(dir, "data")}
	storePath, err := bootstrapFromRepo(localDir, paths)
	if err != nil {
		t.Fatalf("bootstrapFromRepo: %v", err)
	}

	// Should resolve to local path, NOT try to clone from GitHub.
	if storePath != localDir {
		t.Errorf("storePath = %q, want %q", storePath, localDir)
	}
}

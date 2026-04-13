//go:build integration

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
)

// TestIntegration_BootstrapFromRepo_LocalGitRepo creates a real local git repository
// (using git init + git commit) and verifies that bootstrapFromRepo resolves it correctly.
func TestIntegration_BootstrapFromRepo_LocalGitRepo(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "test-store")
	if err := os.MkdirAll(repoDir, 0o750); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}

	// Initialize a real git repository.
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test")

	// Create a hams.config.yaml inside the repo.
	configContent := "profile_tag: macOS\nmachine_id: integration-test\n"
	configPath := filepath.Join(repoDir, "hams.config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o640); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	// Create a profile directory with a dummy hamsfile.
	profileDir := filepath.Join(repoDir, "macOS")
	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("MkdirAll profile: %v", err)
	}
	hamsfileContent := "packages:\n  - app: git\n"
	if err := os.WriteFile(filepath.Join(profileDir, "Homebrew.hams.yaml"), []byte(hamsfileContent), 0o640); err != nil {
		t.Fatalf("WriteFile hamsfile: %v", err)
	}

	// Commit all files.
	runGit(t, repoDir, "add", "-A")
	runGit(t, repoDir, "commit", "-m", "initial commit")

	// Call bootstrapFromRepo with the local path.
	paths := config.Paths{
		ConfigHome: filepath.Join(root, "config"),
		DataHome:   filepath.Join(root, "data"),
	}

	storePath, err := bootstrapFromRepo(repoDir, paths)
	if err != nil {
		t.Fatalf("bootstrapFromRepo: %v", err)
	}

	// The local path should be used directly.
	if storePath != repoDir {
		t.Errorf("storePath = %q, want %q", storePath, repoDir)
	}

	// Verify the store config can be loaded.
	cfg, err := config.Load(paths, storePath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.ProfileTag != "macOS" {
		t.Errorf("cfg.ProfileTag = %q, want %q", cfg.ProfileTag, "macOS")
	}
	if cfg.MachineID != "integration-test" {
		t.Errorf("cfg.MachineID = %q, want %q", cfg.MachineID, "integration-test")
	}

	// Verify profile directory structure.
	entries, err := os.ReadDir(cfg.ProfileDir())
	if err != nil {
		t.Fatalf("ReadDir profile: %v", err)
	}
	foundHamsfile := false
	for _, e := range entries {
		if e.Name() == "Homebrew.hams.yaml" {
			foundHamsfile = true
			break
		}
	}
	if !foundHamsfile {
		t.Error("Homebrew.hams.yaml not found in profile directory")
	}
}

// TestIntegration_BootstrapFromRepo_CloneLocal creates a bare git repo and clones it
// via bootstrapFromRepo to verify the clone path is set up correctly.
func TestIntegration_BootstrapFromRepo_CloneLocal(t *testing.T) {
	root := t.TempDir()

	// Create a source repo with a commit.
	srcDir := filepath.Join(root, "source-store")
	if err := os.MkdirAll(srcDir, 0o750); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}

	runGit(t, srcDir, "init")
	runGit(t, srcDir, "config", "user.email", "test@example.com")
	runGit(t, srcDir, "config", "user.name", "Test")

	configContent := "profile_tag: linux\nmachine_id: clone-test\n"
	if err := os.WriteFile(filepath.Join(srcDir, "hams.config.yaml"), []byte(configContent), 0o640); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	runGit(t, srcDir, "add", "-A")
	runGit(t, srcDir, "commit", "-m", "initial")

	// Create a bare clone to simulate a "remote".
	bareDir := filepath.Join(root, "bare-store.git")
	runGit(t, root, "clone", "--bare", srcDir, bareDir)

	// Now use bootstrapFromRepo with the bare repo path as a "remote" URL.
	// Since the bare repo path won't have a .git directory as a subdirectory
	// (it IS the git directory), bootstrapFromRepo will treat it as remote and clone.
	paths := config.Paths{
		ConfigHome: filepath.Join(root, "config"),
		DataHome:   filepath.Join(root, "data"),
	}

	storePath, err := bootstrapFromRepo("file://"+bareDir, paths)
	if err != nil {
		t.Fatalf("bootstrapFromRepo bare: %v", err)
	}

	// Verify the clone destination exists and has a .git directory.
	gitDir := filepath.Join(storePath, ".git")
	if _, statErr := os.Stat(gitDir); statErr != nil {
		t.Fatalf(".git directory not found at %q: %v", gitDir, statErr)
	}

	// Verify config can be loaded from the cloned store.
	cfg, err := config.Load(paths, storePath)
	if err != nil {
		t.Fatalf("config.Load from clone: %v", err)
	}
	if cfg.ProfileTag != "linux" {
		t.Errorf("cfg.ProfileTag = %q, want %q", cfg.ProfileTag, "linux")
	}
}

// runGit executes a git command in the given directory, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
}

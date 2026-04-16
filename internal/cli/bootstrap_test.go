package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
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
	// Use a fake HOME so we never touch the real home directory.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	testDir := filepath.Join(fakeHome, ".hams-test-resolve-local")
	gitDir := filepath.Join(testDir, ".git")
	if mkErr := os.MkdirAll(gitDir, 0o750); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}

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

	// Create a .git directory — bootstrapFromRepo only checks for its existence.
	if err := os.Mkdir(filepath.Join(repoDir, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir .git: %v", err)
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

// TestRunApply_AutoResolvesStoreFromConfigRepo asserts cycle-49: when
// neither --from-repo nor --store nor store_path is configured but
// store_repo IS configured (per schema-design spec), runApply uses
// store_repo as the effective --from-repo and clones/resolves the
// store automatically. Uses a local bare-repo to avoid network.
func TestRunApply_AutoResolvesStoreFromConfigRepo(t *testing.T) {
	// Prepare a local bare repo that bootstrapFromRepo will accept as
	// a valid local path.
	fakeStore := filepath.Join(t.TempDir(), "fake-store.git")
	if err := os.MkdirAll(fakeStore, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fakeStore, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	// Config with store_repo but no store_path.
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: test\nmachine_id: mach\nstore_repo: "+fakeStore+"\n")

	registry := provider.NewRegistry()
	flags := &provider.GlobalFlags{}
	// runApply with no --from-repo / no --store. Should still succeed
	// (no providers match because the bare repo has no profile dir,
	// but the point is: no "no store configured" error).
	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err != nil {
		t.Fatalf("runApply should auto-resolve store_repo; got: %v", err)
	}
}

// TestBootstrapFromRepo_LocalAttemptSurfacesError asserts that when the
// user passes a clearly-local-looking path (`/tmp/foo`, `./foo`, `~/foo`)
// that doesn't exist or isn't a git repo, the error message names the
// local path — NOT a confusing "https://github.com//<path> not found".
// Regression for cycle 35.
func TestBootstrapFromRepo_LocalAttemptSurfacesError(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"absolute nonexistent", "/nonexistent-path-12345"},
		{"absolute dir without .git", "/tmp"}, // /tmp exists but no .git
		{"tilde nonexistent", "~/nonexistent-hams-path-xyz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths := config.Paths{DataHome: t.TempDir()}
			_, err := bootstrapFromRepo(tc.input, paths)
			if err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			if strings.Contains(err.Error(), "github.com") {
				t.Errorf("local-looking input %q should not mention github.com in error; got %q", tc.input, err.Error())
			}
		})
	}
}

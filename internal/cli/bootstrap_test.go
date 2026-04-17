package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
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

	// Cycle 168: clone paths now include the HOST so that two repos
	// with the same `<user>/<repo>` on different forges don't
	// collide on disk (github.com/x/y vs gitlab.com/x/y previously
	// shared `/data/hams/repo/x/y`).
	tests := []struct {
		repo string
		want string
	}{
		// Shorthand `user/repo` defaults to github.com.
		{"zthxxx/hams-store", "/data/hams/repo/github.com/zthxxx/hams-store"},
		{"zthxxx/hams-store.git", "/data/hams/repo/github.com/zthxxx/hams-store"},
		// HTTPS URLs preserve the host.
		{"https://github.com/zthxxx/hams-store.git", "/data/hams/repo/github.com/zthxxx/hams-store"},
		{"https://gitlab.com/zthxxx/hams-store.git", "/data/hams/repo/gitlab.com/zthxxx/hams-store"},
		{"https://bitbucket.org/team/repo", "/data/hams/repo/bitbucket.org/team/repo"},
		// SSH URLs (`git@host:user/repo`) preserve the host.
		{"git@github.com:zthxxx/hams-store.git", "/data/hams/repo/github.com/zthxxx/hams-store"},
		{"git@gitlab.com:team/project.git", "/data/hams/repo/gitlab.com/team/project"},
		// Defensive fallback: malformed input still returns SOME path
		// (the legacy last-2-segments behavior) so tests don't crash.
		{"single-name", "/data/hams/repo/single-name"},
	}

	for _, tt := range tests {
		got := resolveClonePath(tt.repo, paths)
		if got != tt.want {
			t.Errorf("resolveClonePath(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

// TestExpandRepoShorthand — cycle 225 guard. The URL-expansion path
// pre-cycle-225 hardcoded `https://github.com/<input>` for any input
// without a scheme/git@ prefix, silently misrouting things like
// `gitlab.com/team/repo` to `https://github.com/gitlab.com/team/repo`
// (which then surfaced as "Repository not found" against the wrong
// host). The fix mirrors resolveClonePath's host-detection heuristic
// so the URL and the on-disk cache path agree on which forge the
// repo lives at.
func TestExpandRepoShorthand(t *testing.T) {
	tests := []struct {
		repo string
		want string
	}{
		// GitHub shorthand: no dot in first segment → assume github.com.
		{"zthxxx/hams-store", "https://github.com/zthxxx/hams-store"},
		{"zthxxx/hams-store.git", "https://github.com/zthxxx/hams-store.git"},
		// Host-prefixed shorthand: first segment has a dot → use as-is.
		{"gitlab.com/team/project", "https://gitlab.com/team/project"},
		{"bitbucket.org/team/repo", "https://bitbucket.org/team/repo"},
		{"git.example.com/x/y", "https://git.example.com/x/y"},
		{"codeberg.org/u/r", "https://codeberg.org/u/r"},
		// Full URLs: returned verbatim, no expansion.
		{"https://github.com/zthxxx/hams-store", "https://github.com/zthxxx/hams-store"},
		{"https://gitlab.com/team/repo.git", "https://gitlab.com/team/repo.git"},
		{"http://internal/x/y", "http://internal/x/y"},
		{"ssh://git@host/x/y", "ssh://git@host/x/y"},
		// SSH (`git@host:user/repo`): returned verbatim.
		{"git@github.com:zthxxx/hams-store.git", "git@github.com:zthxxx/hams-store.git"},
		{"git@gitlab.com:team/project.git", "git@gitlab.com:team/project.git"},
		// Single segment: defensive fallback (no expansion). go-git will
		// fail with a real URL-format error rather than silently
		// prefixing github.com.
		{"single-name", "single-name"},
	}

	for _, tt := range tests {
		got := expandRepoShorthand(tt.repo)
		if got != tt.want {
			t.Errorf("expandRepoShorthand(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

// TestExpandRepoShorthand_AgreesWithResolveClonePath asserts the
// post-cycle-225 invariant that `expandRepoShorthand` and
// `resolveClonePath` pick the same forge host for any input. If they
// drift, a user's `--from-repo=gitlab.com/x/y` would land on disk
// under `repo/gitlab.com/x/y` (correct path) but be cloned from
// `https://github.com/gitlab.com/x/y` (wrong URL) — exactly the
// pre-fix bug.
func TestExpandRepoShorthand_AgreesWithResolveClonePath(t *testing.T) {
	paths := config.Paths{DataHome: "/data/hams"}
	repos := []string{
		"zthxxx/hams-store",
		"gitlab.com/team/project",
		"bitbucket.org/team/repo",
		"https://github.com/zthxxx/hams-store",
		"https://gitlab.com/team/repo.git",
		"git@github.com:zthxxx/hams-store.git",
		"git@gitlab.com:team/project.git",
	}

	for _, repo := range repos {
		urlForm := expandRepoShorthand(repo)
		clonePath := resolveClonePath(repo, paths)
		// Extract the host segment from the URL form. For SSH the host
		// sits between "git@" and ":"; for HTTPS it sits between "://"
		// and the next "/". Build a tiny extractor inline.
		host := ""
		switch {
		case strings.HasPrefix(urlForm, "git@"):
			rest := strings.TrimPrefix(urlForm, "git@")
			if h, _, found := strings.Cut(rest, ":"); found {
				host = h
			}
		case strings.Contains(urlForm, "://"):
			_, rest, _ := strings.Cut(urlForm, "://")
			if h, _, found := strings.Cut(rest, "/"); found {
				host = h
			}
		}
		if host == "" {
			t.Errorf("could not extract host from urlForm %q (input: %q)", urlForm, repo)
			continue
		}
		if !strings.Contains(clonePath, "/repo/"+host+"/") {
			t.Errorf("URL host %q does not appear in clone path %q (input: %q)", host, clonePath, repo)
		}
	}
}

// TestResolveClonePath_NoCollisionAcrossForges asserts the cycle-168
// invariant that github.com/X/Y and gitlab.com/X/Y resolve to
// DIFFERENT clone paths. Without the host scoping, the second clone
// would silently inherit the first's `.git` and pull from the
// wrong origin.
func TestResolveClonePath_NoCollisionAcrossForges(t *testing.T) {
	paths := config.Paths{DataHome: "/data/hams"}

	githubPath := resolveClonePath("https://github.com/team/repo", paths)
	gitlabPath := resolveClonePath("https://gitlab.com/team/repo", paths)
	if githubPath == gitlabPath {
		t.Errorf("github + gitlab repos collide at %q", githubPath)
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
	storePath, err := bootstrapFromRepo(context.Background(), repoDir, paths)
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
	storePath, err := bootstrapFromRepo(context.Background(), localDir, paths)
	if err != nil {
		t.Fatalf("bootstrapFromRepo: %v", err)
	}

	// Should resolve to local path, NOT try to clone from GitHub.
	if storePath != localDir {
		t.Errorf("storePath = %q, want %q", storePath, localDir)
	}
}

// TestTransformCloneError_RepositoryNotFound locks in cycle-72:
// go-git's "authentication required: Repository not found" error
// is rewritten as a UserFacingError that doesn't mislead users
// into chasing credential issues when the repo simply doesn't
// exist.
func TestTransformCloneError_RepositoryNotFound(t *testing.T) {
	t.Parallel()
	goGitErr := errors.New("authentication required: Repository not found")
	got := transformCloneError("https://github.com/nope/nope", goGitErr)

	var ufe *hamserr.UserFacingError
	if !errors.As(got, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", got, got)
	}
	if ufe.Code != hamserr.ExitGeneralError {
		t.Errorf("Code = %d, want ExitGeneralError", ufe.Code)
	}
	// Message MUST NOT mention authentication (that's the whole point).
	if strings.Contains(ufe.Message, "authentication") {
		t.Errorf("message leaks 'authentication' confusion: %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "not found or not accessible") {
		t.Errorf("message should explain the real cause; got %q", ufe.Message)
	}
	if len(ufe.Suggestions) != 3 {
		t.Errorf("want 3 suggestions (URL / private-repo / local-path); got %d: %v",
			len(ufe.Suggestions), ufe.Suggestions)
	}
}

// TestTransformCloneError_OtherErrorsPassthrough asserts that
// non-"Repository not found" errors keep the library's original
// message prefixed with "cloning <url>:" rather than being wrapped
// in a misleading UserFacingError. Network timeouts, invalid refs,
// permission denied, etc.
func TestTransformCloneError_OtherErrorsPassthrough(t *testing.T) {
	t.Parallel()
	netErr := errors.New("dial tcp: connection refused")
	got := transformCloneError("https://example.test/foo", netErr)

	var ufe *hamserr.UserFacingError
	if errors.As(got, &ufe) {
		t.Errorf("network errors should NOT be wrapped as UserFacingError; got %T", got)
	}
	if !strings.Contains(got.Error(), "connection refused") {
		t.Errorf("underlying error should propagate; got %q", got.Error())
	}
	if !strings.Contains(got.Error(), "https://example.test/foo") {
		t.Errorf("wrapped message should include URL for context; got %q", got.Error())
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
			_, err := bootstrapFromRepo(context.Background(), tc.input, paths)
			if err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			if strings.Contains(err.Error(), "github.com") {
				t.Errorf("local-looking input %q should not mention github.com in error; got %q", tc.input, err.Error())
			}
		})
	}
}

// TestPreviewExistingStoreFromRepo_LocalPath: when the input names an
// existing local git repo, preview returns it immediately (no network,
// no clone-path lookup).
func TestPreviewExistingStoreFromRepo_LocalPath(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "local-store")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, ok := previewExistingStoreFromRepo(repoDir, config.Paths{DataHome: t.TempDir()})
	if !ok {
		t.Fatal("expected ok=true for existing local repo")
	}
	if got != repoDir {
		t.Errorf("path = %q, want %q", got, repoDir)
	}
}

// TestPreviewExistingStoreFromRepo_PriorClone: when a remote-shorthand
// has already been cloned into `${DataHome}/repo/<user>/<name>`,
// preview reuses it without re-network.
func TestPreviewExistingStoreFromRepo_PriorClone(t *testing.T) {
	dataHome := t.TempDir()
	// Cycle 168: clone paths now include the host so shorthand
	// `user/repo` lands at `repo/github.com/user/repo` not
	// `repo/user/repo`.
	clonePath := filepath.Join(dataHome, "repo", "github.com", "zthxxx", "hams-store")
	if err := os.MkdirAll(filepath.Join(clonePath, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, ok := previewExistingStoreFromRepo("zthxxx/hams-store", config.Paths{DataHome: dataHome})
	if !ok {
		t.Fatal("expected ok=true for pre-cloned repo")
	}
	if got != clonePath {
		t.Errorf("path = %q, want %q", got, clonePath)
	}
}

// TestPreviewExistingStoreFromRepo_NotClonedYet: remote-shorthand with
// no local copy → returns false so runApply's --dry-run branch knows
// to print "Would clone" and exit without touching the network.
func TestPreviewExistingStoreFromRepo_NotClonedYet(t *testing.T) {
	_, ok := previewExistingStoreFromRepo("fresh-user/never-cloned", config.Paths{DataHome: t.TempDir()})
	if ok {
		t.Error("expected ok=false for never-cloned repo")
	}
}

// TestRunApply_DryRunFromRepoSkipsCloneWhenNotCached asserts cycle 86:
// `hams apply --dry-run --from-repo=X` MUST NOT clone when no prior
// clone exists. Previously the clone fired before the dry-run check
// so users got a real disk write + network call. Fix: preview
// recognizes no-prior-clone and prints "Would clone X" + exits.
func TestRunApply_DryRunFromRepoSkipsCloneWhenNotCached(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\nmachine_id: mid1\n")

	flags := &provider.GlobalFlags{DryRun: true}
	registry := provider.NewRegistry()

	// Use a repo shorthand that's never been cloned. Because we set
	// DryRun=true and the preview returns (_, false), runApply's
	// dry-run branch SHOULD print "Would clone" and return nil
	// without ever invoking go-git (which would otherwise either
	// hit the network or create a directory in dataHome/repo/).
	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "never-cloned-user/never-cloned-repo", true, "", "", false, bootstrapMode{})
	if err != nil {
		t.Fatalf("dry-run should exit cleanly when repo not cached; got: %v", err)
	}

	// CRITICAL: no filesystem side effect — the data_home/repo
	// directory must NOT have been created.
	repoPath := filepath.Join(dataHome, "repo", "never-cloned-user", "never-cloned-repo")
	if _, statErr := os.Stat(repoPath); statErr == nil {
		t.Errorf("dry-run should not have created clone dir at %q", repoPath)
	}
}

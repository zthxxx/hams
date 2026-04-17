package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// TestCloneProvider_Manifest asserts the shipped manifest fields.
// Pinned constants because external callers (apt-style integration
// tests, docs generators) hand-write strings against these.
func TestCloneProvider_Manifest(t *testing.T) {
	t.Parallel()
	p := NewCloneProvider(nil)
	m := p.Manifest()
	if m.Name != "git-clone" {
		t.Errorf("Name = %q, want git-clone", m.Name)
	}
	if m.DisplayName != "git clone" {
		t.Errorf("DisplayName = %q, want \"git clone\"", m.DisplayName)
	}
	if m.FilePrefix != "git-clone" {
		t.Errorf("FilePrefix = %q, want git-clone", m.FilePrefix)
	}
}

// TestProbe_StateOKWhenLocalPathExists asserts that Probe reads the
// local-path component of each resource ID and reports StateOK if the
// directory exists, StateFailed if not. This is the entire git-clone
// probe contract — checking filesystem presence.
func TestProbe_StateOKWhenLocalPathExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := NewCloneProvider(nil)

	existing := filepath.Join(dir, "exists")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Cycle 135: Probe now requires a `.git` entry (or bare-repo `HEAD`)
	// to report StateOK. Seed `.git` so this test represents a
	// legitimately-cloned repo.
	if err := os.Mkdir(filepath.Join(existing, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	missing := filepath.Join(dir, "absent")

	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/exists -> "+existing, state.StateOK)
	sf.SetResource("git@github.com:foo/absent -> "+missing, state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Probe returned %d results, want 2", len(results))
	}

	byPath := map[string]state.ResourceState{}
	for _, r := range results {
		byPath[r.ID] = r.State
	}
	if byPath["git@github.com:foo/exists -> "+existing] != state.StateOK {
		t.Errorf("existing path: state = %v, want StateOK", byPath["git@github.com:foo/exists -> "+existing])
	}
	if byPath["git@github.com:foo/absent -> "+missing] != state.StateFailed {
		t.Errorf("missing path: state = %v, want StateFailed", byPath["git@github.com:foo/absent -> "+missing])
	}
}

// TestProbe_ExpandsTildeInLocalPath asserts Probe expands a leading
// `~/` in the stored local path before os.Stat. Without this, a
// hamsfile recording `path: ~/repos/foo` would always report
// StateFailed on any machine lacking a literal `~` subdirectory —
// which breaks the core "share one hamsfile across machines,
// each user's $HOME resolves per-invocation" promise.
//
// Scenario: set HOME to a tempdir that contains /repos/foo. A
// resource id "...remote... -> ~/repos/foo" must resolve to
// $HOME/repos/foo on each machine.
func TestProbe_ExpandsTildeInLocalPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoDir := filepath.Join(home, "repos", "foo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	// Cycle 135: Probe requires `.git` to see the directory as a
	// valid git repo. Seed it alongside the tilde-expansion fixture.
	if err := os.Mkdir(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	p := NewCloneProvider(nil)
	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/bar -> ~/repos/foo", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateOK {
		t.Errorf("tilde-prefixed existing path: state = %v, want StateOK", results[0].State)
	}
}

// TestProbe_TildeStillFailsWhenDirectoryMissing gates the other side:
// an ~/-prefixed id whose expanded path doesn't exist must produce
// StateFailed (not accidentally pass just because the ~/ expansion
// hit some unrelated directory).
func TestProbe_TildeStillFailsWhenDirectoryMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // empty home — nothing under ~/

	p := NewCloneProvider(nil)
	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/absent -> ~/repos/absent", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 || results[0].State != state.StateFailed {
		t.Errorf("missing tilde path: state = %v, want StateFailed", results[0].State)
	}
}

// TestProbe_PathExistsButNotGitRepoFlagsFailed locks in cycle 135:
// if the user manually deleted `.git/` (or never fully cloned), the
// directory exists on disk but is NOT a git repo. Probe MUST flip
// to StateFailed so the next apply re-clones — previously the
// probe called os.Stat() only and reported StateOK, so apply would
// see no drift and skip, leaving the user with a broken-repo
// directory forever.
func TestProbe_PathExistsButNotGitRepoFlagsFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := NewCloneProvider(nil)

	// Directory exists but has no .git and no HEAD — not a git repo.
	brokenRepo := filepath.Join(dir, "broken")
	if err := os.Mkdir(brokenRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/broken -> "+brokenRepo, state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 || results[0].State != state.StateFailed {
		t.Errorf("dir-without-.git: state = %v, want StateFailed", results[0].State)
	}
	// Cycle 239: ErrorMsg must distinguish "path exists but not a
	// git repo" from "path missing entirely" so users can tell
	// "I accidentally rm -rf'd .git/" apart from "I deleted the
	// whole repo directory".
	if results[0].ErrorMsg == "" {
		t.Error("ErrorMsg should explain why StateFailed; got empty string")
	}
	if !strings.Contains(results[0].ErrorMsg, ".git") && !strings.Contains(results[0].ErrorMsg, "HEAD") {
		t.Errorf("ErrorMsg should mention .git/HEAD marker; got %q", results[0].ErrorMsg)
	}
	if !strings.Contains(results[0].ErrorMsg, brokenRepo) {
		t.Errorf("ErrorMsg should name the path; got %q", results[0].ErrorMsg)
	}
}

// TestProbe_PathMissingEmitsDistinctErrorMsg — cycle 239: the
// ErrorMsg for a fully-missing local path ("local path missing: X")
// distinguishes from the "dir exists but not a repo" case. Users
// reading `hams list --json` can triage accordingly.
func TestProbe_PathMissingEmitsDistinctErrorMsg(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := NewCloneProvider(nil)

	missingRepo := filepath.Join(dir, "never-existed") // NOT created
	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/missing -> "+missingRepo, state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 || results[0].State != state.StateFailed {
		t.Errorf("missing-path: state = %v, want StateFailed", results[0].State)
	}
	if !strings.Contains(results[0].ErrorMsg, "local path missing") {
		t.Errorf("ErrorMsg should say 'local path missing'; got %q", results[0].ErrorMsg)
	}
	// Must NOT claim the dir "still exists" when it doesn't.
	if strings.Contains(results[0].ErrorMsg, "still exists") {
		t.Errorf("ErrorMsg should not claim dir 'still exists'; got %q", results[0].ErrorMsg)
	}
}

// TestProbe_BareRepoHEADFileTreatedAsValid asserts a bare repo
// (HEAD file at the path root, no .git subdir) is still StateOK.
// Mirrors ensureStoreIsGitRepo's logic at the CLI layer.
func TestProbe_BareRepoHEADFileTreatedAsValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := NewCloneProvider(nil)

	bareRepo := filepath.Join(dir, "bare")
	if err := os.Mkdir(bareRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Bare-repo marker: HEAD file at the root (no .git subdir).
	if err := os.WriteFile(filepath.Join(bareRepo, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/bare -> "+bareRepo, state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 || results[0].State != state.StateOK {
		t.Errorf("bare-repo HEAD: state = %v, want StateOK", results[0].State)
	}
}

// TestApply_NonGitDirSurfacesActionableError locks in cycle 136:
// when the target path exists but has no .git (e.g., user deleted
// it manually), Apply must surface a clear UserFacingError with
// remediation hints — NOT shell out to `git clone` and let git
// complain cryptically about "destination already exists and is
// not an empty directory".
func TestApply_NonGitDirSurfacesActionableError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.Mkdir(broken, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// No .git seeded — directory exists but isn't a repo.

	p := NewCloneProvider(nil)
	action := provider.Action{
		ID: "git@github.com:foo/bar -> " + broken,
		Resource: cloneResource{
			Remote: "git@github.com:foo/bar",
			Path:   broken,
		},
		Type: provider.ActionInstall,
	}

	err := p.Apply(context.Background(), action)
	if err == nil {
		t.Fatalf("Apply should error on non-git target dir, got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
	}
	// Message should name the bad path AND suggestions should mention rm.
	if !strings.Contains(ufe.Message, broken) {
		t.Errorf("error message should name the path %q, got: %q", broken, ufe.Message)
	}
	joined := strings.Join(ufe.Suggestions, " | ")
	if !strings.Contains(joined, "rm -rf") {
		t.Errorf("suggestions should include `rm -rf` remedy, got: %q", joined)
	}
	if !strings.Contains(joined, "git init") {
		t.Errorf("suggestions should include `git init` alternative, got: %q", joined)
	}
}

// TestApply_ExistingGitRepoIsIdempotent asserts that an Apply
// against a target path that ALREADY contains a valid git repo
// is a no-op (returns nil without re-cloning). Mirrors the
// idempotency expected of other providers — `hams apply` on a
// correctly-cloned repo does NOT churn.
func TestApply_ExistingGitRepoIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo := filepath.Join(dir, "already-cloned")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Seed .git to make it look like a valid clone.
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	p := NewCloneProvider(nil)
	action := provider.Action{
		ID: "git@github.com:foo/bar -> " + repo,
		Resource: cloneResource{
			Remote: "git@github.com:foo/bar",
			Path:   repo,
		},
		Type: provider.ActionInstall,
	}

	if err := p.Apply(context.Background(), action); err != nil {
		t.Errorf("Apply on existing git repo should be no-op, got: %v", err)
	}
}

// TestProbe_SkipsRemovedResources asserts that resources marked
// StateRemoved do NOT appear in Probe output. This is critical: a
// removed resource that re-emits as StateFailed would trip a re-clone
// on the next apply.
func TestProbe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := NewCloneProvider(nil)
	sf := state.New("git-clone", "test-machine")
	sf.SetResource("git@github.com:foo/removed -> /tmp/removed", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved resource, want 0", len(results))
	}
}

// TestRemove_NoOp asserts that the Remove method does not delete the
// local clone directory. Documented at clone.go:132 — this is a
// deliberate safety choice (never `rm -rf` user data) and a
// regression here would be catastrophic.
func TestRemove_NoOp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clonePath := filepath.Join(dir, "must-survive")
	if err := os.MkdirAll(filepath.Join(clonePath, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	p := NewCloneProvider(nil)
	if err := p.Remove(context.Background(), "git@github.com:foo/repo -> "+clonePath); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	if _, err := os.Stat(clonePath); os.IsNotExist(err) {
		t.Fatalf("Remove deleted the local directory %q — must be a no-op", clonePath)
	}
}

// TestProperty_ParseCloneResource_NoPanic asserts the parser never
// panics on arbitrary input shapes — the resource ID format is
// "remote -> local-path" but legacy entries may carry malformed
// strings (e.g., ones that survived a hand-edit error).
func TestProperty_ParseCloneResource_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		_, _ = parseCloneResource(input)
	})
}

// TestProperty_ParseCloneResource_RoundtripWellFormedInput verifies
// that any synthesized "remote -> path" string parses back to the
// originals (modulo whitespace trimming, which is documented).
func TestProperty_ParseCloneResource_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Synthesize remotes and paths that don't contain " -> " or
		// leading/trailing whitespace (the parser strips edges).
		remote := rapid.StringMatching(`[a-z][a-z0-9./:_@-]{4,30}`).Draw(t, "remote")
		path := rapid.StringMatching(`/[a-z][a-z0-9/_-]{4,30}`).Draw(t, "path")

		gotRemote, gotPath := parseCloneResource(remote + " -> " + path)
		if gotRemote != remote {
			t.Fatalf("remote: got %q, want %q", gotRemote, remote)
		}
		if gotPath != path {
			t.Fatalf("path: got %q, want %q", gotPath, path)
		}
	})
}

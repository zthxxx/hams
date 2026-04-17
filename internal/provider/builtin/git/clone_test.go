package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"

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

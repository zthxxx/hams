package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// captureStdoutForClone swaps os.Stdout for a pipe, runs fn, and
// returns the captured text. Mirrors internal/cli/captureStdout but
// package-private to avoid cross-package imports. Serialized so two
// parallel tests don't race on the global os.Stdout.
var captureStdoutForCloneMu sync.Mutex

func captureStdoutForClone(t *testing.T, fn func()) string {
	t.Helper()
	captureStdoutForCloneMu.Lock()
	defer captureStdoutForCloneMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe: %v", closeErr)
	}
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("read pipe: %v", copyErr)
	}
	return buf.String()
}

// newCloneHarness builds a CloneProvider pointed at a tempdir store
// so handleList can load real state/hamsfile files without touching
// the host.
func newCloneHarness(t *testing.T) (*CloneProvider, *provider.GlobalFlags, string) {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	machineID := "test-machine"
	stateDir := filepath.Join(storeDir, ".state", machineID)
	for _, d := range []string{profileDir, stateDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: profileTag, MachineID: machineID}
	return NewCloneProvider(cfg), &provider.GlobalFlags{Store: storeDir, Profile: profileTag}, stateDir
}

// TestHandleCommand_List_EmptyStateShowsHint asserts the empty-state
// branch: no tracked repos → the list command prints the header AND
// an actionable hint pointing at `git-clone add`. Before this fix
// the list command printed only the header and exited silently,
// indistinguishable from a hung command.
func TestHandleCommand_List_EmptyStateShowsHint(t *testing.T) {
	t.Parallel()
	p, flags, _ := newCloneHarness(t)

	out := captureStdoutForClone(t, func() {
		if err := p.HandleCommand(context.Background(), []string{"list"}, nil, flags); err != nil {
			t.Fatalf("HandleCommand list: %v", err)
		}
	})

	if !strings.Contains(out, "git clone managed repositories:") {
		t.Errorf("missing header; got:\n%s", out)
	}
	if !strings.Contains(out, "no clones tracked yet") {
		t.Errorf("empty-state hint missing; got:\n%s", out)
	}
	if !strings.Contains(out, "hams git-clone add") {
		t.Errorf("empty-state hint should point at 'hams git-clone add'; got:\n%s", out)
	}
}

// TestHandleCommand_List_PopulatedStateEnumeratesResources asserts
// that when the state file has tracked repos, HandleCommand list
// prints each resource-id + state. Regression gate against the pre-fix
// bug where list always printed only the header, never the entries.
func TestHandleCommand_List_PopulatedStateEnumeratesResources(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	sf := state.New(p.Manifest().Name, "test-machine")
	sf.SetResource("git@github.com:foo/bar -> /tmp/bar", state.StateOK)
	sf.SetResource("git@github.com:baz/qux -> /tmp/qux", state.StateOK)
	if err := sf.Save(filepath.Join(stateDir, "git-clone.state.yaml")); err != nil {
		t.Fatalf("save state: %v", err)
	}

	out := captureStdoutForClone(t, func() {
		if err := p.HandleCommand(context.Background(), []string{"list"}, nil, flags); err != nil {
			t.Fatalf("HandleCommand list: %v", err)
		}
	})

	if !strings.Contains(out, "git clone managed repositories:") {
		t.Errorf("missing header; got:\n%s", out)
	}
	if !strings.Contains(out, "git@github.com:foo/bar -> /tmp/bar") {
		t.Errorf("missing foo/bar entry; got:\n%s", out)
	}
	if !strings.Contains(out, "git@github.com:baz/qux -> /tmp/qux") {
		t.Errorf("missing baz/qux entry; got:\n%s", out)
	}
	// Empty-state hint MUST NOT appear when resources exist.
	if strings.Contains(out, "no clones tracked yet") {
		t.Errorf("empty-state hint should not appear when resources exist; got:\n%s", out)
	}
}

// TestHandleCommand_Remove_MarksStateAsRemoved asserts `hams
// git-clone remove <id>` updates BOTH the hamsfile (entry deleted)
// AND the state file (resource marked StateRemoved). Previously only
// the hamsfile was updated — the state resource stayed at its prior
// value (typically StateOK), so `hams list` and the next apply's
// drift-detection saw a phantom resource that was actually
// user-removed. Mirrors the symmetric fix git-config's doRemove
// already satisfies (cycle 104).
func TestHandleCommand_Remove_MarksStateAsRemoved(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	// Seed: pre-existing state with the resource as StateOK (as if a
	// prior apply cloned it successfully) AND a hamsfile entry.
	resourceID := "git@github.com:foo/bar -> /tmp/bar"
	sf := state.New(p.Manifest().Name, "test-machine")
	sf.SetResource(resourceID, state.StateOK)
	if err := sf.Save(filepath.Join(stateDir, "git-clone.state.yaml")); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	// Force hamsfile creation by running handleAdd via HandleCommand —
	// but handleAdd would shell out to real git, so shortcut via
	// loadOrCreateHamsfile + AddApp directly.
	hf, err := p.loadOrCreateHamsfile(nil, flags)
	if err != nil {
		t.Fatalf("load hamsfile: %v", err)
	}
	hf.AddApp("repos", resourceID, "")
	if writeErr := hf.Write(); writeErr != nil {
		t.Fatalf("write hamsfile: %v", writeErr)
	}

	err = p.HandleCommand(context.Background(), []string{"remove", resourceID}, nil, flags)
	if err != nil {
		t.Fatalf("HandleCommand remove: %v", err)
	}

	// State file should now show StateRemoved for the resource.
	sfAfter, err := state.Load(filepath.Join(stateDir, "git-clone.state.yaml"))
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	r, ok := sfAfter.Resources[resourceID]
	if !ok {
		t.Fatalf("resource %q missing from state after remove — expected StateRemoved tombstone", resourceID)
	}
	if r.State != state.StateRemoved {
		t.Errorf("state = %v, want StateRemoved", r.State)
	}

	// Hamsfile should also have the entry removed.
	hfAfter, err := p.loadOrCreateHamsfile(nil, flags)
	if err != nil {
		t.Fatalf("reload hamsfile: %v", err)
	}
	for _, app := range hfAfter.ListApps() {
		if app == resourceID {
			t.Errorf("hamsfile still contains removed entry %q", resourceID)
		}
	}
}

// TestRecordAdd_WritesBothHamsfileAndState asserts recordAdd (the
// extracted post-clone bookkeeping from handleAdd) persists to
// hamsfile AND state. Before the CP-1 auto-record fix, handleAdd
// only wrote the hamsfile — `hams list` showed nothing for the
// just-cloned repo until the user ran `hams refresh` separately.
// This gates against regression of that contract: the state file
// MUST have the resource as StateOK right after a successful add.
func TestRecordAdd_WritesBothHamsfileAndState(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	remote := "git@github.com:foo/new"
	localPath := "/tmp/new-clone"
	if err := p.recordAdd(remote, localPath, nil, flags); err != nil {
		t.Fatalf("recordAdd: %v", err)
	}

	// Hamsfile should have the entry.
	hf, err := p.loadOrCreateHamsfile(nil, flags)
	if err != nil {
		t.Fatalf("load hamsfile: %v", err)
	}
	resourceID := remote + " -> " + localPath
	if !slices.Contains(hf.ListApps(), resourceID) {
		t.Errorf("hamsfile missing resource %q — got %v", resourceID, hf.ListApps())
	}

	// State file should have the resource at StateOK.
	sf, err := state.Load(filepath.Join(stateDir, "git-clone.state.yaml"))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	r, ok := sf.Resources[resourceID]
	if !ok {
		t.Fatalf("state missing resource %q — got keys %v", resourceID, keysOf(sf.Resources))
	}
	if r.State != state.StateOK {
		t.Errorf("state = %v, want StateOK", r.State)
	}
}

// TestHandleAdd_StrictArgCount locks in cycle 165: the pre-cycle-165
// implementation only used args[0] of `hams git-clone add …` and
// silently dropped extra positional args. Common typo: the user
// remembers `git clone <remote> <path>` syntax and types
// `hams git-clone add <remote> <path> --hams-path=<X>` thinking
// `<path>` was forwarded. The actual path came from --hams-path;
// `<path>` was silently lost. Now: too-many positional args returns
// a usage error that points the user at --hams-path.
func TestHandleAdd_StrictArgCount(t *testing.T) {
	t.Parallel()
	p, flags, _ := newCloneHarness(t)

	err := p.HandleCommand(context.Background(),
		[]string{"add", "git@github.com:foo/bar", "extra-positional-arg"},
		map[string]string{"path": "/tmp/bar"},
		flags)
	if err == nil {
		t.Fatalf("git-clone add with extra positional should error")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Message, "exactly one") {
		t.Errorf("error should say 'exactly one'; got %q", ufe.Message)
	}
	joined := strings.Join(ufe.Suggestions, " | ")
	if !strings.Contains(joined, "--hams-path") {
		t.Errorf("suggestions should mention --hams-path; got %q", joined)
	}
}

// TestHandleAdd_ExistingNonGitDirErrors locks in cycle 137:
// `hams git-clone add <remote> --hams-path=<existing-non-git>`
// surfaces an actionable UserFacingError instead of shelling
// out to git and letting it fail cryptically with "destination
// path already exists and is not an empty directory". Mirror of
// cycle 136's declarative-apply fix applied to the CLI path.
func TestHandleAdd_ExistingNonGitDirErrors(t *testing.T) {
	t.Parallel()
	p, flags, _ := newCloneHarness(t)
	broken := t.TempDir() // exists, no .git

	err := p.HandleCommand(context.Background(),
		[]string{"add", "git@github.com:foo/bar"},
		map[string]string{"path": broken},
		flags)
	if err == nil {
		t.Fatalf("git-clone add on non-git existing dir should error")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Message, broken) {
		t.Errorf("error should name the path %q, got: %q", broken, ufe.Message)
	}
	joined := strings.Join(ufe.Suggestions, " | ")
	if !strings.Contains(joined, "rm -rf") {
		t.Errorf("suggestions should include `rm -rf` remedy, got: %q", joined)
	}
}

// TestHandleAdd_ExistingValidRepoRecordsWithoutCloning locks in
// the cycle 137 idempotency behavior: `hams git-clone add` on a
// target path that already contains a valid `.git` records the
// resource in the hamsfile WITHOUT re-cloning (no duplicate
// network call, no `destination already exists` error). Common
// scenario: user manually cloned then realized they want hams
// to track it.
func TestHandleAdd_ExistingValidRepoRecordsWithoutCloning(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	// Seed: a directory with a `.git` subdir (looks like a valid
	// repo without needing a real git binary).
	targetDir := t.TempDir()
	clonePath := filepath.Join(targetDir, "already")
	if err := os.Mkdir(clonePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(clonePath, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	remote := "git@github.com:foo/already"
	err := p.HandleCommand(context.Background(),
		[]string{"add", remote},
		map[string]string{"path": clonePath},
		flags)
	if err != nil {
		t.Errorf("git-clone add on valid repo should be no-op, got: %v", err)
	}

	// The hamsfile MUST have the resource recorded — the user's
	// intent to track was captured even though no clone happened.
	hf, err := p.loadOrCreateHamsfile(nil, flags)
	if err != nil {
		t.Fatalf("load hamsfile: %v", err)
	}
	wantID := remote + " -> " + clonePath
	if !slices.Contains(hf.ListApps(), wantID) {
		t.Errorf("hamsfile missing recorded resource %q — got %v", wantID, hf.ListApps())
	}
	// State file should also have the resource at StateOK.
	sf, err := state.Load(filepath.Join(stateDir, "git-clone.state.yaml"))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if r, ok := sf.Resources[wantID]; !ok || r.State != state.StateOK {
		t.Errorf("state missing or wrong state for %q: ok=%v, state=%v", wantID, ok, r)
	}
}

// TestHandleCommand_List_DeterministicOrder locks in cycle 155:
// `hams git-clone list` previously iterated sf.Resources directly
// (Go map iteration is non-deterministic), so each invocation
// shuffled the rows of tracked repos. Now: sort.Strings before
// emit so the output is byte-identical across runs and IDs appear
// alphabetically. Symmetric with cycles 148/149.
func TestHandleCommand_List_DeterministicOrder(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	// 5 tracked resources whose alphabetical order is unrelated to
	// insertion order.
	sf := state.New(p.Manifest().Name, "test-machine")
	insertOrder := []string{
		"git@github.com:zeta/repo -> /tmp/zeta",
		"git@github.com:alpha/repo -> /tmp/alpha",
		"git@github.com:mu/repo -> /tmp/mu",
		"git@github.com:beta/repo -> /tmp/beta",
		"git@github.com:omega/repo -> /tmp/omega",
	}
	for _, id := range insertOrder {
		sf.SetResource(id, state.StateOK)
	}
	if err := sf.Save(filepath.Join(stateDir, "git-clone.state.yaml")); err != nil {
		t.Fatalf("save state: %v", err)
	}

	runOnce := func() string {
		return captureStdoutForClone(t, func() {
			if err := p.HandleCommand(context.Background(), []string{"list"}, nil, flags); err != nil {
				t.Fatalf("HandleCommand list: %v", err)
			}
		})
	}

	first := runOnce()
	for range 20 {
		got := runOnce()
		if got != first {
			t.Errorf("output differs across runs:\nfirst:\n%s\n\nlater:\n%s", first, got)
			break
		}
	}

	// Assert alphabetical: alpha < beta < mu < omega < zeta.
	wantOrder := []string{"alpha", "beta", "mu", "omega", "zeta"}
	last := -1
	for _, name := range wantOrder {
		idx := strings.Index(first, "github.com:"+name)
		if idx < 0 {
			t.Errorf("output missing %q; got:\n%s", name, first)
			continue
		}
		if idx <= last {
			t.Errorf("output not alphabetical: %q at idx %d should come after previous (idx %d)", name, idx, last)
		}
		last = idx
	}
}

// TestHandleRemove_UnknownIDErrors_TrackedListIsAlphabetical locks in
// cycle 155's second fix: when `hams git-clone remove <typo>` errors
// with "no tracked resource", the suggestion text "Tracked IDs: …"
// must list the IDs in stable alphabetical order. Previously the
// iteration over sf.Resources to merge orphan IDs was non-
// deterministic, so each typo-retry showed the IDs in different order.
func TestHandleRemove_UnknownIDErrors_TrackedListIsAlphabetical(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	// Seed 4 tracked resources whose alphabetical order ≠ insertion order.
	sf := state.New(p.Manifest().Name, "test-machine")
	for _, id := range []string{
		"git@github.com:zeta/repo -> /tmp/zeta",
		"git@github.com:alpha/repo -> /tmp/alpha",
		"git@github.com:mu/repo -> /tmp/mu",
		"git@github.com:beta/repo -> /tmp/beta",
	} {
		sf.SetResource(id, state.StateOK)
	}
	if err := sf.Save(filepath.Join(stateDir, "git-clone.state.yaml")); err != nil {
		t.Fatalf("save state: %v", err)
	}

	runOnce := func() string {
		err := p.HandleCommand(context.Background(),
			[]string{"remove", "git@github.com:nonexistent/repo -> /tmp/nope"},
			nil, flags)
		if err == nil {
			t.Fatal("expected error for nonexistent ID")
		}
		var ufe *hamserr.UserFacingError
		if !errors.As(err, &ufe) {
			t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
		}
		return strings.Join(ufe.Suggestions, " ")
	}

	first := runOnce()
	for range 10 {
		if got := runOnce(); got != first {
			t.Errorf("Tracked IDs list differs across runs:\nfirst: %q\nlater: %q", first, got)
			break
		}
	}

	// Assert alphabetical positioning of the 4 tracked names.
	wantOrder := []string{"alpha", "beta", "mu", "zeta"}
	last := -1
	for _, name := range wantOrder {
		idx := strings.Index(first, "github.com:"+name)
		if idx < 0 {
			t.Errorf("suggestion missing %q; got: %q", name, first)
			continue
		}
		if idx <= last {
			t.Errorf("Tracked IDs not alphabetical: %q at idx %d should come after previous (idx %d)", name, idx, last)
		}
		last = idx
	}
}

// TestHandleRemove_UnknownIDErrors locks in cycle 140: `hams
// git-clone remove <typo>` against a tracked-ID set that doesn't
// contain the typo surfaces a UserFacingError instead of silently
// writing a StateRemoved tombstone for the nonexistent entry. The
// error lists the tracked IDs so the user can retry with a valid
// value.
func TestHandleRemove_UnknownIDErrors(t *testing.T) {
	t.Parallel()
	p, flags, stateDir := newCloneHarness(t)

	// Seed one valid tracked resource.
	validID := "git@github.com:foo/bar -> /tmp/bar"
	sf := state.New(p.Manifest().Name, "test-machine")
	sf.SetResource(validID, state.StateOK)
	if err := sf.Save(filepath.Join(stateDir, "git-clone.state.yaml")); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	// User types a typo'd ID that doesn't exist.
	err := p.HandleCommand(context.Background(),
		[]string{"remove", "git@github.com:foo/br -> /tmp/br"}, // typo: br vs bar
		nil, flags)
	if err == nil {
		t.Fatalf("remove on nonexistent ID should error")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Message, "no tracked resource") {
		t.Errorf("error message should mention 'no tracked resource', got: %q", ufe.Message)
	}
	// Suggestions must include the valid tracked IDs so user can
	// retry.
	joined := strings.Join(ufe.Suggestions, " | ")
	if !strings.Contains(joined, validID) {
		t.Errorf("suggestions should list valid tracked IDs, got: %q", joined)
	}

	// State MUST NOT have a tombstone for the typo'd ID.
	sf2, err := state.Load(filepath.Join(stateDir, "git-clone.state.yaml"))
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if _, ok := sf2.Resources["git@github.com:foo/br -> /tmp/br"]; ok {
		t.Errorf("typo'd ID should NOT have a state entry, but found one")
	}
	// The valid ID should still be StateOK (not accidentally touched).
	if r, ok := sf2.Resources[validID]; !ok || r.State != state.StateOK {
		t.Errorf("valid ID should still be StateOK, got ok=%v state=%v", ok, r)
	}
}

// keysOf is a tiny helper for the state-assertion error message.
func keysOf(m map[string]*state.Resource) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestHandleCommand_List_NoStoreConfiguredErrors asserts list fails
// fast with a UserFacingError when no store is configured — we need
// a store path to locate the hamsfile/state. Without this guard
// LoadOrCreateEmpty would create an empty file at some unexpected
// location.
func TestHandleCommand_List_NoStoreConfiguredErrors(t *testing.T) {
	t.Parallel()
	p := NewCloneProvider(nil)
	flags := &provider.GlobalFlags{}

	err := p.HandleCommand(context.Background(), []string{"list"}, nil, flags)
	if err == nil {
		t.Fatal("expected error when no store configured")
	}
	if !strings.Contains(err.Error(), "store") {
		t.Errorf("error should mention 'store'; got: %v", err)
	}
}

package git

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zthxxx/hams/internal/config"
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

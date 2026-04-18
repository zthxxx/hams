package git

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
)

// newUnifiedHarness builds a UnifiedProvider wired to DI-fake
// runners for both sub-providers. Store lives in a tempdir.
type unifiedHarness struct {
	t      *testing.T
	flags  *provider.GlobalFlags
	cfg    *config.Config
	u      *UnifiedProvider
	runner *FakeCmdRunner
}

func newUnifiedHarness(t *testing.T) *unifiedHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	stateDir := filepath.Join(storeDir, ".state", "test-machine")
	for _, d := range []string{profileDir, stateDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}
	cfg := &config.Config{
		StorePath:  storeDir,
		ProfileTag: profileTag,
		MachineID:  "test-machine",
	}
	flags := &provider.GlobalFlags{Store: storeDir, Profile: profileTag}

	u := NewUnifiedProvider(cfg)
	runner := NewFakeCmdRunner()
	u.config.WithRunner(runner)

	return &unifiedHarness{
		t:      t,
		flags:  flags,
		cfg:    cfg,
		u:      u,
		runner: runner,
	}
}

// TestUnifiedProvider_ConfigSetRoutesToConfigProvider asserts that
// `hams git config <key> <value>` invokes the config sub-provider
// (which in turn records to the hamsfile via the fake runner).
// Regression gate for the CLAUDE.md bullet: "The git-clone and
// git-config providers should be merged into a unified git
// provider, exposing only the `hams git` entry point".
func TestUnifiedProvider_ConfigSetRoutesToConfigProvider(t *testing.T) {
	h := newUnifiedHarness(t)

	args := []string{"config", "set", "user.name", "zthxxx"}
	if err := h.u.HandleCommand(context.Background(), args, nil, h.flags); err != nil {
		t.Fatalf("config set: %v", err)
	}

	if len(h.runner.SetCalls) == 0 {
		t.Fatal("expected at least one SetGlobal call; got none")
	}
	last := h.runner.SetCalls[len(h.runner.SetCalls)-1]
	if last.Key != "user.name" {
		t.Errorf("runner.SetGlobal key = %q, want user.name", last.Key)
	}
	if last.Value != "zthxxx" {
		t.Errorf("runner.SetGlobal value = %q, want zthxxx", last.Value)
	}
}

// TestUnifiedProvider_CloneRequiresRemote asserts that `hams git
// clone` without args surfaces a UsageError naming the missing
// remote. Mirrors the strict-arg-count pattern every other
// provider follows.
func TestUnifiedProvider_CloneRequiresRemote(t *testing.T) {
	h := newUnifiedHarness(t)

	err := h.u.HandleCommand(context.Background(), []string{"clone"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected usage error for `hams git clone` with no args")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
}

// TestUnifiedProvider_CloneTranslatesPositionalPath asserts the
// key ergonomic win of the merge: a user types `hams git clone
// <remote> <path>` (matching real git's shape) and the unified
// provider translates that into the CloneProvider's internal
// `add <remote> --hams-path=<path>` form so auto-record still fires
// without the user needing to know about --hams-path at all.
func TestUnifiedProvider_CloneTranslatesPositionalPath(t *testing.T) {
	h := newUnifiedHarness(t)

	err := h.u.HandleCommand(context.Background(),
		[]string{"clone", "https://github.com/zthxxx/test-store.hams", "/tmp/test-store"},
		map[string]string{}, h.flags)
	// We expect the clone attempt to fail because we haven't swapped
	// the real `git` binary in this test — that's fine. What we care
	// about is that the UsageError from "missing --hams-path" does
	// NOT fire, which would mean translation didn't happen.
	if err != nil {
		var ufe *hamserr.UserFacingError
		if errors.As(err, &ufe) && strings.Contains(ufe.Message, "--hams-path") {
			t.Errorf("positional <path> was not translated to --hams-path: %v", err)
		}
	}
}

// TestUnifiedProvider_ConfigRemoveForwards asserts that the second
// config verb (`remove`) also reaches the sub-provider.
func TestUnifiedProvider_ConfigRemoveForwards(t *testing.T) {
	h := newUnifiedHarness(t)

	// First set so there is something to remove.
	if err := h.u.HandleCommand(context.Background(),
		[]string{"config", "set", "user.email", "ci@example.test"},
		nil, h.flags); err != nil {
		t.Fatalf("setup set: %v", err)
	}

	if err := h.u.HandleCommand(context.Background(),
		[]string{"config", "remove", "user.email"},
		nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if len(h.runner.UnsetCalls) == 0 {
		t.Fatal("expected at least one UnsetGlobal call; got none")
	}
	lastUnset := h.runner.UnsetCalls[len(h.runner.UnsetCalls)-1]
	if lastUnset.Key != "user.email" {
		t.Errorf("runner.UnsetGlobal key = %q, want user.email", lastUnset.Key)
	}
}

// TestUnifiedProvider_BareSubcommandShowsUsage asserts that `hams
// git` with no args surfaces the usage listing rather than silently
// succeeding.
func TestUnifiedProvider_BareSubcommandShowsUsage(t *testing.T) {
	h := newUnifiedHarness(t)
	err := h.u.HandleCommand(context.Background(), []string{}, nil, h.flags)
	if err == nil {
		t.Fatal("expected usage error for bare `hams git`")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
}

// TestUnifiedProvider_DryRunPassthroughSkipsExec asserts that an
// unrecognized subcommand with --dry-run prints the "Would run"
// line and returns nil without invoking the real git binary.
// Regression gate for the CLAUDE.md principle that --dry-run is a
// preview-only contract across every provider.
func TestUnifiedProvider_DryRunPassthroughSkipsExec(t *testing.T) {
	h := newUnifiedHarness(t)
	h.flags.DryRun = true
	outBuf := new(strings.Builder)
	h.flags.Out = outBuf

	// "status" is a real git subcommand that hams does NOT intercept.
	err := h.u.HandleCommand(context.Background(), []string{"status"}, nil, h.flags)
	if err != nil {
		t.Fatalf("dry-run passthrough: %v", err)
	}
	out := outBuf.String()
	if !strings.Contains(out, "[dry-run]") || !strings.Contains(out, "git status") {
		t.Errorf("dry-run output missing expected preview; got %q", out)
	}
}

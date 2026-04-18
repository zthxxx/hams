package git

import (
	"bytes"
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

func newUnifiedTestEnv(t *testing.T) (*UnifiedHandler, *FakeCmdRunner, string) {
	t.Helper()
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "default"), 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	cfg := &config.Config{StorePath: store, ProfileTag: "default", MachineID: "u1"}
	cfgP := NewConfigProvider(cfg)
	cloneP := NewCloneProvider(cfg)
	fr := NewFakeCmdRunner()
	cfgP.WithRunner(fr)
	return NewUnifiedHandler(cfgP, cloneP), fr, store
}

func TestUnifiedHandler_NameAndDisplay(t *testing.T) {
	t.Parallel()
	u := NewUnifiedHandler(nil, nil)
	if u.Name() != "git" {
		t.Errorf("Name = %q, want git", u.Name())
	}
	if u.DisplayName() != "git" {
		t.Errorf("DisplayName = %q, want git", u.DisplayName())
	}
}

func TestUnifiedHandler_RoutesConfigBareForm(t *testing.T) {
	t.Parallel()
	u, runner, store := newUnifiedTestEnv(t)

	args := []string{"config", "user.email", "alice@example.com"}
	flags := &provider.GlobalFlags{Store: store, Profile: "default"}
	if err := u.HandleCommand(context.Background(), args, nil, flags); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(runner.SetCalls) != 1 {
		t.Fatalf("expected 1 SetGlobal call, got %d", len(runner.SetCalls))
	}
	if runner.SetCalls[0].Key != "user.email" || runner.SetCalls[0].Value != "alice@example.com" {
		t.Errorf("runner.SetGlobal got %+v, want (user.email, alice@example.com)", runner.SetCalls[0])
	}
}

func TestUnifiedHandler_RoutesConfigSetVerb(t *testing.T) {
	t.Parallel()
	u, runner, store := newUnifiedTestEnv(t)

	args := []string{"config", "set", "user.name", "zthxxx"}
	flags := &provider.GlobalFlags{Store: store, Profile: "default"}
	if err := u.HandleCommand(context.Background(), args, nil, flags); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(runner.SetCalls) != 1 {
		t.Fatalf("expected 1 SetGlobal call, got %d", len(runner.SetCalls))
	}
	if runner.SetCalls[0].Key != "user.name" || runner.SetCalls[0].Value != "zthxxx" {
		t.Errorf("runner.SetGlobal got %+v, want (user.name, zthxxx)", runner.SetCalls[0])
	}
}

func TestUnifiedHandler_RejectsEmptyArgs(t *testing.T) {
	t.Parallel()
	u := NewUnifiedHandler(nil, nil)
	err := u.HandleCommand(context.Background(), nil, nil, &provider.GlobalFlags{})
	if err == nil {
		t.Fatal("expected error on empty args")
	}
	var ufe *hamserr.UserFacingError
	if !errorsAs(err, &ufe) {
		t.Fatalf("expected UserFacingError; got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "subcommand") {
		t.Errorf("error should mention subcommand; got %q", err)
	}
}

// TestUnifiedHandler_PassesUnknownSubcommandThroughToGit locks in the
// spec-required "wrapped commands behave like the original" contract:
// `hams git <anything>` that hams does not intercept MUST invoke the
// real `git <anything>` binary. Uses the passthroughExec DI seam so
// the test does not spawn a real process.
func TestUnifiedHandler_PassesUnknownSubcommandThroughToGit(t *testing.T) {
	origExec := passthroughExec
	t.Cleanup(func() { passthroughExec = origExec })

	var recorded []string
	passthroughExec = func(_ context.Context, args []string) error {
		recorded = append([]string{}, args...)
		return nil
	}

	u := NewUnifiedHandler(nil, nil)
	args := []string{"pull", "--rebase", "origin", "main"}
	if err := u.HandleCommand(context.Background(), args, nil, &provider.GlobalFlags{}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(recorded) != len(args) {
		t.Fatalf("passthrough received %d args, want %d: %v", len(recorded), len(args), recorded)
	}
	for i, want := range args {
		if recorded[i] != want {
			t.Errorf("arg[%d] = %q, want %q", i, recorded[i], want)
		}
	}
}

// TestUnifiedHandler_PassthroughPropagatesExecError asserts that a
// non-zero exit from the real git call bubbles up unchanged. Use a
// sentinel error via the DI seam.
func TestUnifiedHandler_PassthroughPropagatesExecError(t *testing.T) {
	origExec := passthroughExec
	t.Cleanup(func() { passthroughExec = origExec })

	sentinel := errors.New("git exit 128")
	passthroughExec = func(_ context.Context, _ []string) error { return sentinel }

	u := NewUnifiedHandler(nil, nil)
	err := u.HandleCommand(context.Background(), []string{"push"}, nil, &provider.GlobalFlags{})
	if !errors.Is(err, sentinel) {
		t.Errorf("passthrough error not propagated; got %v", err)
	}
}

// TestUnifiedHandler_PassthroughDryRunSkipsExec asserts that dry-run
// mode prints a preview line and does NOT invoke the exec seam.
func TestUnifiedHandler_PassthroughDryRunSkipsExec(t *testing.T) {
	origExec := passthroughExec
	t.Cleanup(func() { passthroughExec = origExec })

	var called bool
	passthroughExec = func(_ context.Context, _ []string) error {
		called = true
		return nil
	}

	var stdout bytes.Buffer
	u := NewUnifiedHandler(nil, nil)
	flags := &provider.GlobalFlags{DryRun: true, Out: &stdout}
	if err := u.HandleCommand(context.Background(), []string{"log", "--oneline"}, nil, flags); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if called {
		t.Error("passthroughExec should not be invoked when flags.DryRun is set")
	}
	if !strings.Contains(stdout.String(), "[dry-run] Would run: git log --oneline") {
		t.Errorf("dry-run preview not printed; stdout=%q", stdout.String())
	}
}

// TestUnifiedHandler_CloneNaturalFormTranslatesPath asserts
// `hams git clone <url> <path>` folds the positional path into
// hamsFlags["path"] before delegating to the CloneProvider's add verb.
// Not parallelized: siblings in this file mutate the package-level
// passthroughExec DI seam, and go-test race detection is strict about
// any concurrent exercise of the git package when those globals swap.
func TestUnifiedHandler_CloneNaturalFormTranslatesPath(t *testing.T) {
	u, _, store := newUnifiedTestEnv(t)

	hamsFlags := map[string]string{}
	args := []string{"clone", "https://github.com/example/repo.git", "/tmp/repo"}
	flags := &provider.GlobalFlags{Store: store, Profile: "default", DryRun: true}

	// Dry-run is fine; the CloneProvider's own handler honors it so we
	// don't actually fetch the repo. We're only checking the flag fold.
	// Any error below is a downstream CloneProvider problem (not under
	// test here); we just assert the flag made it.
	if err := u.HandleCommand(context.Background(), args, hamsFlags, flags); err != nil {
		t.Logf("clone handler returned %v (may be unrelated to the flag-fold assertion)", err)
	}
	if hamsFlags["path"] != "/tmp/repo" {
		t.Errorf("hamsFlags[path] = %q, want /tmp/repo", hamsFlags["path"])
	}
}

// TestUnifiedHandler_CloneRejectsUnknownGitFlag asserts that unforwarded
// git flags (--depth, --branch, --recurse-submodules, …) surface a
// loud error rather than silently dropping. Serialized for the same
// reason as CloneNaturalFormTranslatesPath.
func TestUnifiedHandler_CloneRejectsUnknownGitFlag(t *testing.T) {
	u := NewUnifiedHandler(nil, nil)
	err := u.HandleCommand(context.Background(),
		[]string{"clone", "https://github.com/example/repo.git", "--depth=1"},
		map[string]string{}, &provider.GlobalFlags{})
	if err == nil {
		t.Fatal("expected error on unforwarded git flag")
	}
	if !strings.Contains(err.Error(), "--depth") {
		t.Errorf("error should name the rejected flag; got %q", err)
	}
}

// TestUnifiedHandler_CloneRequiresRemote asserts the bare `hams git clone`
// form emits a usage error. Serialized for the same reason as
// CloneNaturalFormTranslatesPath.
func TestUnifiedHandler_CloneRequiresRemote(t *testing.T) {
	u := NewUnifiedHandler(nil, nil)
	err := u.HandleCommand(context.Background(), []string{"clone"}, nil, &provider.GlobalFlags{})
	if err == nil {
		t.Fatal("expected error on bare `hams git clone`")
	}
	var ufe *hamserr.UserFacingError
	if !errorsAs(err, &ufe) {
		t.Fatalf("expected UserFacingError; got %T (%v)", err, err)
	}
}

// errorsAs delegates to errors.As — kept as a tiny wrapper so the
// caller stays terse and the linter does not flag a bare type
// assertion on an error value (errorlint).
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}

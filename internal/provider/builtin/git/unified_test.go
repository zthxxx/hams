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

func TestUnifiedHandler_RejectsUnknownSubcommand(t *testing.T) {
	t.Parallel()
	u := NewUnifiedHandler(nil, nil)
	err := u.HandleCommand(context.Background(), []string{"merge", "--no-ff"}, nil, &provider.GlobalFlags{})
	if err == nil {
		t.Fatal("expected error on unknown subcommand")
	}
	if !strings.Contains(err.Error(), "merge") {
		t.Errorf("error should mention the rejected subcommand; got %q", err)
	}
}

// errorsAs delegates to errors.As — kept as a tiny wrapper so the
// caller stays terse and the linter does not flag a bare type
// assertion on an error value (errorlint).
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}

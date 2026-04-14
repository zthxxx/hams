package provider

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/state"
)

func TestRunPreInstallHooks_Success(t *testing.T) {
	hooks := []Hook{
		{Type: HookPreInstall, Command: "true"},
	}
	err := RunPreInstallHooks(context.Background(), hooks, "htop")
	if err != nil {
		t.Fatalf("RunPreInstallHooks error: %v", err)
	}
}

func TestRunPreInstallHooks_Failure(t *testing.T) {
	hooks := []Hook{
		{Type: HookPreInstall, Command: "false"},
	}
	err := RunPreInstallHooks(context.Background(), hooks, "htop")
	if err == nil {
		t.Fatal("expected pre-install hook failure")
	}
}

func TestRunPreInstallHooks_SkipsDeferred(t *testing.T) {
	hooks := []Hook{
		{Type: HookPreInstall, Command: "false", Defer: true}, // Should be skipped.
	}
	err := RunPreInstallHooks(context.Background(), hooks, "htop")
	if err != nil {
		t.Fatalf("deferred hook should be skipped: %v", err)
	}
}

func TestRunPostInstallHooks_FailureRecordsState(t *testing.T) {
	sf := state.New("test", "machine")
	sf.SetResource("htop", state.StateOK)

	hooks := []Hook{
		{Type: HookPostInstall, Command: "false"},
	}
	err := RunPostInstallHooks(context.Background(), hooks, "htop", sf)
	if err == nil {
		t.Fatal("expected post-install hook failure")
	}
	if sf.Resources["htop"].State != state.StateHookFailed {
		t.Errorf("state = %q, want hook-failed", sf.Resources["htop"].State)
	}
}

func TestCollectDeferredHooks(t *testing.T) {
	hooks := []Hook{
		{Type: HookPostInstall, Command: "echo 1", Defer: false},
		{Type: HookPostInstall, Command: "echo 2", Defer: true},
		{Type: HookPostInstall, Command: "echo 3", Defer: true},
	}
	deferred := CollectDeferredHooks(hooks)
	if len(deferred) != 2 {
		t.Fatalf("deferred = %d, want 2", len(deferred))
	}
}

func TestRunPreUpdateHooks_Success(t *testing.T) {
	hooks := []Hook{
		{Type: HookPreUpdate, Command: "true"},
	}
	err := RunPreUpdateHooks(context.Background(), hooks, "htop")
	if err != nil {
		t.Fatalf("RunPreUpdateHooks error: %v", err)
	}
}

func TestRunPreUpdateHooks_Failure(t *testing.T) {
	hooks := []Hook{
		{Type: HookPreUpdate, Command: "false"},
	}
	err := RunPreUpdateHooks(context.Background(), hooks, "htop")
	if err == nil {
		t.Fatal("expected pre-update hook failure")
	}
}

func TestRunPreUpdateHooks_SkipsDeferred(t *testing.T) {
	hooks := []Hook{
		{Type: HookPreUpdate, Command: "false", Defer: true}, // Should be skipped.
	}
	err := RunPreUpdateHooks(context.Background(), hooks, "htop")
	if err != nil {
		t.Fatalf("deferred hook should be skipped: %v", err)
	}
}

func TestRunPostUpdateHooks_FailureRecordsState(t *testing.T) {
	sf := state.New("test", "machine")
	sf.SetResource("htop", state.StateOK)

	hooks := []Hook{
		{Type: HookPostUpdate, Command: "false"},
	}
	err := RunPostUpdateHooks(context.Background(), hooks, "htop", sf)
	if err == nil {
		t.Fatal("expected post-update hook failure")
	}
	if sf.Resources["htop"].State != state.StateHookFailed {
		t.Errorf("state = %q, want hook-failed", sf.Resources["htop"].State)
	}
}

func TestRunDeferredHooks_MixedResults(t *testing.T) {
	sf := state.New("test", "machine")
	sf.SetResource("htop", state.StateOK)
	sf.SetResource("jq", state.StateOK)

	deferred := []DeferredHook{
		{Hook: Hook{Command: "true"}, ResourceID: "htop"},
		{Hook: Hook{Command: "false"}, ResourceID: "jq"},
	}

	errs := RunDeferredHooks(context.Background(), deferred, sf)
	if len(errs) != 1 {
		t.Fatalf("errors = %d, want 1", len(errs))
	}
	if sf.Resources["jq"].State != state.StateHookFailed {
		t.Errorf("jq state = %q, want hook-failed", sf.Resources["jq"].State)
	}
	// htop should remain OK (its hook succeeded).
	if sf.Resources["htop"].State != state.StateOK {
		t.Errorf("htop state = %q, want ok", sf.Resources["htop"].State)
	}
}

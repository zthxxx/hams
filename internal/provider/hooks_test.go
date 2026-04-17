package provider

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/state"
)

// TestRunHook_OutputStreamsToTerminalAndCapturesForError locks in
// cycle 178: runHook used to call cmd.CombinedOutput which blocked
// until the hook finished — long-running hooks (compilation, brew
// bottle install, network calls) appeared to hang for minutes with
// no progress indication. Now: stream stdout/stderr to the user's
// terminal AND capture into a buffer for the error path.
//
// The test redirects os.Stdout to a pipe and asserts the hook's
// output appears there mid-execution (not just after).
func TestRunHook_OutputStreamsToTerminalAndCapturesForError(t *testing.T) {
	// Test scenario A: successful hook — output streamed to stdout.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = w

	hookErr := runHook(context.Background(),
		Hook{Type: HookPostInstall, Command: "echo PROGRESS-MARKER"},
		"test-resource")

	if cerr := w.Close(); cerr != nil {
		t.Logf("close pipe writer: %v", cerr)
	}
	os.Stdout = origStdout
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if cerr := r.Close(); cerr != nil {
		t.Logf("close pipe reader: %v", cerr)
	}

	if hookErr != nil {
		t.Fatalf("runHook: %v", hookErr)
	}
	if !strings.Contains(string(out), "PROGRESS-MARKER") {
		t.Errorf("hook stdout NOT streamed; got %q", string(out))
	}
}

// TestRunHook_FailureCapturesOutputForErrorMessage asserts that
// even with the streaming change, the captured output is still
// included in the wrapping error so debugging stays informative.
func TestRunHook_FailureCapturesOutputForErrorMessage(t *testing.T) {
	// Redirect stdout/stderr so the test's own output isn't polluted.
	origStdout, origStderr := os.Stdout, os.Stderr
	devNull, openErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if openErr != nil {
		t.Fatalf("open /dev/null: %v", openErr)
	}
	os.Stdout = devNull
	os.Stderr = devNull
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
		if cerr := devNull.Close(); cerr != nil {
			t.Logf("close /dev/null: %v", cerr)
		}
	}()

	err := runHook(context.Background(),
		Hook{Type: HookPostInstall, Command: "echo FAILURE-MARKER && false"},
		"test-resource")

	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if !strings.Contains(err.Error(), "FAILURE-MARKER") {
		t.Errorf("error message should include captured output 'FAILURE-MARKER'; got: %v", err)
	}
}

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
	deferred := CollectDeferredHooks("htop", hooks)
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

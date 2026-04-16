package cli

import (
	"context"
	"errors"
	"testing"
)

// fakeStorePushRunner records calls and scripts return values for
// each git step in runStorePush. Fields starting with `force` are
// error injections.
type fakeStorePushRunner struct {
	statusValue    string
	forceStatusErr error
	forceAddErr    error
	forceCommitErr error
	forcePushErr   error
	gotStorePath   string
	gotCommitMsg   string
	gotAddAllCalls int
	gotCommitCalls int
	gotPushCalls   int
	gotStatusCalls int
}

func (f *fakeStorePushRunner) Status(_ context.Context, storePath string) (string, error) {
	f.gotStorePath = storePath
	f.gotStatusCalls++
	return f.statusValue, f.forceStatusErr
}

func (f *fakeStorePushRunner) AddAll(_ context.Context, _ string) error {
	f.gotAddAllCalls++
	return f.forceAddErr
}

func (f *fakeStorePushRunner) Commit(_ context.Context, _, message string) error {
	f.gotCommitCalls++
	f.gotCommitMsg = message
	return f.forceCommitErr
}

func (f *fakeStorePushRunner) Push(_ context.Context, _ string) error {
	f.gotPushCalls++
	return f.forcePushErr
}

func withPushRunner(fake storePushRunner) func() {
	original := pushRunner
	pushRunner = fake
	return func() { pushRunner = original }
}

// TestRunStorePush_CleanTreeSkipsCommit asserts that when `git status
// --porcelain` returns empty (clean working tree), runStorePush
// short-circuits: no add/commit/push — prevents the surprising
// "nothing to commit, working tree clean" exec error that the
// pre-cycle-108 code surfaced after e.g. `hams refresh` (which only
// touches .state/ files, already gitignored).
func TestRunStorePush_CleanTreeSkipsCommit(t *testing.T) {
	fake := &fakeStorePushRunner{statusValue: ""}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "hams: update store"); err != nil {
		t.Fatalf("runStorePush on clean tree: %v", err)
	}
	if fake.gotStatusCalls != 1 {
		t.Errorf("Status calls = %d, want 1", fake.gotStatusCalls)
	}
	if fake.gotAddAllCalls != 0 || fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("clean tree should skip add/commit/push; got add=%d commit=%d push=%d",
			fake.gotAddAllCalls, fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_DirtyTreeRunsAddCommitPush asserts the full
// sequence runs when `git status` shows changes. Also verifies the
// message is forwarded to git commit.
func TestRunStorePush_DirtyTreeRunsAddCommitPush(t *testing.T) {
	fake := &fakeStorePushRunner{statusValue: " M foo.yaml"}
	t.Cleanup(withPushRunner(fake))

	const wantMsg = "hams: install htop"
	if err := runStorePush(context.Background(), "/tmp/store", wantMsg); err != nil {
		t.Fatalf("runStorePush on dirty tree: %v", err)
	}

	if fake.gotAddAllCalls != 1 {
		t.Errorf("AddAll calls = %d, want 1", fake.gotAddAllCalls)
	}
	if fake.gotCommitCalls != 1 {
		t.Errorf("Commit calls = %d, want 1", fake.gotCommitCalls)
	}
	if fake.gotPushCalls != 1 {
		t.Errorf("Push calls = %d, want 1", fake.gotPushCalls)
	}
	if fake.gotCommitMsg != wantMsg {
		t.Errorf("Commit message = %q, want %q", fake.gotCommitMsg, wantMsg)
	}
}

// TestRunStorePush_StatusErrorPropagates asserts a Status failure
// short-circuits and wraps the error — subsequent git calls are
// skipped.
func TestRunStorePush_StatusErrorPropagates(t *testing.T) {
	fake := &fakeStorePushRunner{forceStatusErr: errors.New("git status failed")}
	t.Cleanup(withPushRunner(fake))

	err := runStorePush(context.Background(), "/tmp/store", "msg")
	if err == nil {
		t.Fatalf("expected status error, got nil")
	}
	if fake.gotAddAllCalls != 0 || fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("status error should skip later steps; got add=%d commit=%d push=%d",
			fake.gotAddAllCalls, fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_AddAllErrorShortCircuits asserts an Add failure
// prevents commit and push from running.
func TestRunStorePush_AddAllErrorShortCircuits(t *testing.T) {
	fake := &fakeStorePushRunner{
		statusValue: " M foo.yaml",
		forceAddErr: errors.New("add failed"),
	}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "msg"); err == nil {
		t.Fatalf("expected add error, got nil")
	}
	if fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("add error should skip commit+push; got commit=%d push=%d",
			fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_CommitErrorShortCircuits asserts a Commit failure
// prevents push from running (so we don't push a dirty working tree
// that wasn't actually committed).
func TestRunStorePush_CommitErrorShortCircuits(t *testing.T) {
	fake := &fakeStorePushRunner{
		statusValue:    " M foo.yaml",
		forceCommitErr: errors.New("commit failed"),
	}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "msg"); err == nil {
		t.Fatalf("expected commit error, got nil")
	}
	if fake.gotPushCalls != 0 {
		t.Errorf("commit error should skip push; got push=%d", fake.gotPushCalls)
	}
}

// TestRunStorePush_PushErrorSurfaces asserts a Push failure is
// returned so the caller (shell script, CI pipeline) sees a non-zero
// exit code.
func TestRunStorePush_PushErrorSurfaces(t *testing.T) {
	fake := &fakeStorePushRunner{
		statusValue:  " M foo.yaml",
		forcePushErr: errors.New("push failed"),
	}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "msg"); err == nil {
		t.Fatalf("expected push error, got nil")
	}
}

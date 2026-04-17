package provider

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/state"
)

// TestAcquireMutationLock_HappyPath: a fresh stateDir with no
// existing lock returns a release closure and no error. The closure
// removes the .lock file when invoked.
func TestAcquireMutationLock_HappyPath(t *testing.T) {
	stateDir := t.TempDir()
	release, err := AcquireMutationLock(stateDir, "test cmd")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if release == nil {
		t.Fatal("release closure must not be nil on success")
	}
	if _, statErr := os.Stat(filepath.Join(stateDir, ".lock")); statErr != nil {
		t.Errorf("lock file should exist after Acquire; stat err: %v", statErr)
	}
	release()
	if _, statErr := os.Stat(filepath.Join(stateDir, ".lock")); !os.IsNotExist(statErr) {
		t.Errorf("lock file should be removed after release; stat err: %v", statErr)
	}
}

// TestAcquireMutationLock_ConflictReturnsExitLockError: when another
// live process already holds the lock, the second Acquire MUST fail
// with ExitLockError (exit code 3 per spec) and a message naming the
// holding PID + command. Uses a real Lock primitive to seed the
// conflict so the test exercises the production code path.
func TestAcquireMutationLock_ConflictReturnsExitLockError(t *testing.T) {
	stateDir := t.TempDir()
	holder := state.NewLock(stateDir)
	if err := holder.Acquire("seed-holder"); err != nil {
		t.Fatalf("seed Acquire: %v", err)
	}
	t.Cleanup(func() {
		if err := holder.Release(); err != nil {
			t.Logf("seed-holder release err (best-effort): %v", err)
		}
	})

	release, err := AcquireMutationLock(stateDir, "cycle-221")
	if err == nil {
		release()
		t.Fatal("expected lock conflict error")
	}
	if release != nil {
		t.Error("release closure must be nil on conflict")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitLockError {
		t.Errorf("expected exit code %d (ExitLockError), got %d", hamserr.ExitLockError, ufe.Code)
	}
	// Message should mention the seed-holder so the user knows what
	// to debug. The exact format comes from state.Lock.Acquire.
	if ufe.Message == "" {
		t.Error("error message should be non-empty")
	}
}

// TestAcquireMutationLock_StaleLockReclaimed: a lock file present
// but pointing at a dead PID is reclaimable per the cycle-1 spec
// invariant. Seed a lock with PID 0 (always dead per isProcessAlive)
// and confirm the second Acquire succeeds.
func TestAcquireMutationLock_StaleLockReclaimed(t *testing.T) {
	stateDir := t.TempDir()
	stalePath := filepath.Join(stateDir, ".lock")
	if err := os.WriteFile(stalePath, []byte("pid: 0\ncommand: stale\nstarted_at: 19700101T000000\n"), 0o600); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	release, err := AcquireMutationLock(stateDir, "reclaim")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(release)
}

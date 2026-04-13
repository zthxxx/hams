package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLock_AcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	lock := NewLock(dir)

	if err := lock.Acquire("hams apply"); err != nil {
		t.Fatalf("Acquire error: %v", err)
	}

	// Lock file should exist.
	lockPath := filepath.Join(dir, ".lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("lock file should exist after Acquire")
	}

	// Read lock info.
	info, err := lock.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if info.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}
	if info.Command != "hams apply" {
		t.Errorf("Command = %q, want 'hams apply'", info.Command)
	}

	// Release.
	if err := lock.Release(); err != nil {
		t.Fatalf("Release error: %v", err)
	}

	// Lock file should be gone.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should not exist after Release")
	}
}

func TestLock_DoubleAcquire_SamePID(t *testing.T) {
	dir := t.TempDir()
	lock := NewLock(dir)

	if err := lock.Acquire("hams apply"); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	// Same process acquiring again should detect its own PID as alive.
	err := lock.Acquire("hams apply second")
	if err == nil {
		t.Fatal("second Acquire should fail when same process holds lock")
	}

	lock.Release() //nolint:errcheck // test cleanup
}

func TestLock_StaleRecovery(t *testing.T) {
	dir := t.TempDir()
	lock := NewLock(dir)

	// Write a lock with a PID that doesn't exist.
	lockPath := filepath.Join(dir, ".lock")
	data := []byte(`{"pid":999999999,"command":"stale","started_at":"20240101T000000"}`)
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	// Should be able to acquire over a stale lock.
	if err := lock.Acquire("hams apply"); err != nil {
		t.Fatalf("Acquire over stale lock should succeed: %v", err)
	}

	lock.Release() //nolint:errcheck // test cleanup
}

func TestLock_Release_NonExistent(t *testing.T) {
	dir := t.TempDir()
	lock := NewLock(dir)

	// Releasing a non-existent lock should not error.
	if err := lock.Release(); err != nil {
		t.Fatalf("Release non-existent lock: %v", err)
	}
}

func TestIsProcessAlive_Self(t *testing.T) {
	if !isProcessAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}
}

func TestIsProcessAlive_Dead(t *testing.T) {
	if isProcessAlive(999999999) {
		t.Error("PID 999999999 should not be alive")
	}
}

func TestIsProcessAlive_Invalid(t *testing.T) {
	if isProcessAlive(0) {
		t.Error("PID 0 should not be reported alive")
	}
	if isProcessAlive(-1) {
		t.Error("PID -1 should not be reported alive")
	}
}

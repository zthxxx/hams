package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LockInfo describes the current lock holder.
type LockInfo struct {
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	StartedAt string `json:"started_at"`
}

// Lock represents a single-writer lock file.
type Lock struct {
	path string
}

// NewLock creates a lock for the given state directory.
func NewLock(stateDir string) *Lock {
	return &Lock{path: filepath.Join(stateDir, ".lock")}
}

// Acquire attempts to acquire the lock.
// Returns an error if the lock is already held by a live process.
func (l *Lock) Acquire(command string) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o750); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}

	// Check for existing lock.
	if info, err := l.Read(); err == nil {
		if isProcessAlive(info.PID) {
			return fmt.Errorf("hams is already running (PID %d, command: %s, started: %s). "+
				"If this is stale, remove %s",
				info.PID, info.Command, info.StartedAt, l.path)
		}
		// Stale lock — process is dead, reclaim it.
	}

	info := LockInfo{
		PID:       os.Getpid(),
		Command:   command,
		StartedAt: time.Now().UTC().Format("20060102T150405"),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshaling lock info: %w", err)
	}

	if err := os.WriteFile(l.path, data, 0o600); err != nil {
		return fmt.Errorf("writing lock file: %w", err)
	}

	return nil
}

// Release removes the lock file.
func (l *Lock) Release() error {
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing lock file: %w", err)
	}
	return nil
}

// Read reads the current lock info.
func (l *Lock) Read() (*LockInfo, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parsing lock file: %w", err)
	}

	return &info, nil
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check liveness.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// FormatPID returns a human-readable PID with process name if available.
func FormatPID(pid int) string {
	// Try to read /proc/<pid>/cmdline on Linux.
	procPath := fmt.Sprintf("/proc/%d/cmdline", pid)
	cmdline, err := os.ReadFile(procPath) //nolint:gosec // system proc path is constructed from integer PID, not user input
	if err == nil {
		parts := strings.SplitN(string(cmdline), "\x00", 2)
		if len(parts) > 0 && parts[0] != "" {
			return fmt.Sprintf("%d (%s)", pid, filepath.Base(parts[0]))
		}
	}
	return strconv.Itoa(pid)
}

// Package sudo manages one-time credential acquisition and periodic heartbeat for privileged operations.
package sudo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/zthxxx/hams/internal/i18n"
)

const heartbeatInterval = 4 * time.Minute

// Acquirer manages one-time sudo credential acquisition and keepalive.
// Unit tests inject NoopAcquirer; production uses Manager.
type Acquirer interface {
	Acquire(ctx context.Context) error
	Stop()
}

// CmdBuilder constructs exec.Cmd instances with optional sudo wrapping.
// Unit tests inject DirectBuilder; production uses Builder.
type CmdBuilder interface {
	Command(ctx context.Context, name string, args ...string) *exec.Cmd
}

// isRoot reports whether the current process runs as uid 0.
// Overridable in tests to verify both branches without requiring actual root.
var isRoot = func() bool { return os.Getuid() == 0 }

// Manager handles sudo credential acquisition and keepalive.
type Manager struct {
	acquired bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
}

// NewManager creates a new sudo manager.
func NewManager() *Manager {
	return &Manager{}
}

// Acquire prompts the user for sudo credentials if not already acquired.
// Call this once at the start of `hams apply`.
// When running as root (uid 0), sudo is unnecessary — acquisition and
// heartbeat are skipped entirely.
func (m *Manager) Acquire(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.acquired {
		return nil
	}

	// Running as root — sudo is unnecessary, heartbeat not needed.
	if isRoot() {
		m.acquired = true
		return nil
	}

	// Check if we already have sudo cached.
	if checkSudo() {
		m.acquired = true
		m.startHeartbeat(ctx)
		return nil
	}

	fmt.Fprintln(os.Stderr, i18n.T(i18n.SudoPrompt))
	cmd := exec.CommandContext(ctx, "sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo credential acquisition failed: %w", err)
	}

	m.acquired = true
	m.startHeartbeat(ctx)
	return nil
}

// IsAcquired returns whether sudo credentials have been acquired.
func (m *Manager) IsAcquired() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acquired
}

// Stop cancels the heartbeat goroutine and waits for it to finish.
func (m *Manager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// startHeartbeat runs `sudo -v` periodically to keep credentials fresh.
func (m *Manager) startHeartbeat(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	m.cancel = cancel

	m.wg.Go(func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := exec.CommandContext(ctx, "sudo", "-v").Run(); err != nil {
					slog.Warn("sudo heartbeat failed", "error", err)
				}
			}
		}
	})
}

// Builder wraps commands with sudo when not running as root.
type Builder struct{}

// Command returns an exec.Cmd, prepending sudo if not root.
func (s *Builder) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if isRoot() {
		return exec.CommandContext(ctx, name, args...) //nolint:gosec // root-skip path; args from hamsfile declarations
	}
	sudoArgs := make([]string, 0, len(args)+1)
	sudoArgs = append(sudoArgs, name)
	sudoArgs = append(sudoArgs, args...)
	return exec.CommandContext(ctx, "sudo", sudoArgs...) //nolint:gosec // sudo wrapping is intentional; args come from provider declarations not user input
}

// checkSudo tests if sudo credentials are already cached.
func checkSudo() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	return cmd.Run() == nil
}

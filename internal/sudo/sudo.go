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
)

const heartbeatInterval = 4 * time.Minute

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
func (m *Manager) Acquire(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.acquired {
		return nil
	}

	// Check if we already have sudo cached.
	if checkSudo() {
		m.acquired = true
		m.startHeartbeat(ctx)
		return nil
	}

	fmt.Fprintln(os.Stderr, "hams needs sudo for some operations.")
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

// RunWithSudo creates a command that runs with sudo.
func RunWithSudo(ctx context.Context, name string, args ...string) *exec.Cmd {
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

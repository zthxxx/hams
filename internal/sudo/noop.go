package sudo

import (
	"context"
	"os/exec"
)

// NoopAcquirer is a no-op Acquirer for unit tests.
// It never prompts for sudo and always succeeds.
type NoopAcquirer struct{}

// Acquire is a no-op that always returns nil.
func (NoopAcquirer) Acquire(context.Context) error { return nil }

// Stop is a no-op.
func (NoopAcquirer) Stop() {}

// DirectBuilder runs commands directly without sudo wrapping.
// Used in unit tests to avoid privilege escalation.
type DirectBuilder struct{}

// Command returns an exec.Cmd without sudo wrapping.
func (DirectBuilder) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...) //nolint:gosec // test-only direct execution
}

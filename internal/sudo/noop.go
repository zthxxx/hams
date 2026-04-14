package sudo

import (
	"context"
	"os/exec"
	"sync"
)

// NoopAcquirer is a no-op Acquirer for unit tests.
type NoopAcquirer struct{}

// Acquire is a no-op.
func (NoopAcquirer) Acquire(context.Context) error { return nil }

// Stop is a no-op.
func (NoopAcquirer) Stop() {}

// SpyAcquirer records Acquire and Stop calls for test assertions.
type SpyAcquirer struct {
	mu           sync.Mutex
	AcquireCalls int
	StopCalls    int
}

// Acquire records the call and returns nil.
func (s *SpyAcquirer) Acquire(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AcquireCalls++
	return nil
}

// Stop records the call.
func (s *SpyAcquirer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StopCalls++
}

// DirectBuilder runs commands directly without sudo wrapping.
// Used in sudo package tests to verify command construction (args only, not execution).
type DirectBuilder struct{}

// Command returns an exec.Cmd without sudo wrapping.
func (DirectBuilder) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...) //nolint:gosec // test-only direct execution
}

// RecordingCall records a single CmdBuilder.Command invocation.
type RecordingCall struct {
	Name string
	Args []string
}

// RecordingBuilder records Command calls and returns a harmless /bin/true command.
// Use this in provider unit tests to avoid executing real commands.
type RecordingBuilder struct {
	mu    sync.Mutex
	Calls []RecordingCall
}

// Command records the call and returns a harmless /bin/true command.
func (r *RecordingBuilder) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = append(r.Calls, RecordingCall{Name: name, Args: append([]string(nil), args...)})
	return exec.CommandContext(ctx, "true")
}

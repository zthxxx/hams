//go:build sudo

package sudo

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests run inside Docker containers where sudo is available.
// Build tag "sudo" ensures they never run in normal `go test ./...`.

func TestAcquire_AsRoot_Succeeds(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root")
	}

	m := NewManager()
	defer m.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Acquire(ctx); err != nil {
		t.Fatalf("Acquire as root should succeed: %v", err)
	}
	if !m.IsAcquired() {
		t.Error("expected acquired = true after Acquire as root")
	}
}

func TestAcquire_AsNonRoot_WithNOPASSWD_Succeeds(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user with NOPASSWD sudoers")
	}

	m := NewManager()
	defer m.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Acquire(ctx); err != nil {
		t.Fatalf("Acquire with NOPASSWD should succeed: %v", err)
	}
	if !m.IsAcquired() {
		t.Error("expected acquired = true")
	}
}

func TestBuilder_AsRoot_SkipsSudo(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root")
	}

	sb := &Builder{}
	cmd := sb.Command(context.Background(), "id", "-u")
	args := cmd.Args
	// As root, should NOT prepend sudo.
	if args[0] == "sudo" {
		t.Errorf("expected no sudo prefix when running as root, got %v", args)
	}

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if string(out) == "" {
		t.Error("expected output from id -u")
	}
}

func TestBuilder_AsNonRoot_PrependsSudo(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	sb := &Builder{}
	cmd := sb.Command(context.Background(), "id", "-u")
	args := cmd.Args
	if args[0] != "sudo" {
		t.Errorf("expected sudo prefix when non-root, got %v", args)
	}

	// With NOPASSWD, this should actually execute.
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sudo command failed (NOPASSWD not configured?): %v", err)
	}
	// sudo id -u should return "0".
	if got := string(out); got != "0\n" {
		t.Errorf("sudo id -u = %q, want %q", got, "0\n")
	}
}

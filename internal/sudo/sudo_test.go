package sudo

import (
	"context"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m.IsAcquired() {
		t.Error("new manager should not have acquired sudo")
	}
}

func TestRunWithSudo_CreatesCommand(t *testing.T) {
	cmd := RunWithSudo(context.Background(), "ls", "-la")
	if cmd.Path == "" {
		t.Error("RunWithSudo should create a command")
	}
	// Should have sudo as the actual command.
	args := cmd.Args
	if len(args) < 3 || args[0] != "sudo" || args[1] != "ls" || args[2] != "-la" {
		t.Errorf("Args = %v, want [sudo ls -la]", args)
	}
}

func TestStop_NoAcquire(_ *testing.T) {
	m := NewManager()
	// Stop without Acquire should not panic.
	m.Stop()
}

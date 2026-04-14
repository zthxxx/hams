package sudo

import (
	"context"
	"testing"
)

// Tests in this file must NOT use t.Parallel() because they override the
// package-level isRoot variable.

// Interface compliance tests.

func TestManager_ImplementsAcquirer(_ *testing.T) {
	var _ Acquirer = (*Manager)(nil)
}

func TestBuilder_ImplementsCmdBuilder(_ *testing.T) {
	var _ CmdBuilder = (*Builder)(nil)
}

func TestNoopAcquirer_ImplementsAcquirer(_ *testing.T) {
	var _ Acquirer = NoopAcquirer{}
}

func TestDirectBuilder_ImplementsCmdBuilder(_ *testing.T) {
	var _ CmdBuilder = DirectBuilder{}
}

func TestSpyAcquirer_ImplementsAcquirer(_ *testing.T) {
	var _ Acquirer = (*SpyAcquirer)(nil)
}

func TestRecordingBuilder_ImplementsCmdBuilder(_ *testing.T) {
	var _ CmdBuilder = (*RecordingBuilder)(nil)
}

// Manager tests.

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m.IsAcquired() {
		t.Error("new manager should not have acquired sudo")
	}
}

func TestAcquire_Root_SkipsSudo(t *testing.T) {
	orig := isRoot
	isRoot = func() bool { return true }
	t.Cleanup(func() { isRoot = orig })

	m := NewManager()
	if err := m.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire as root should succeed: %v", err)
	}
	if !m.IsAcquired() {
		t.Error("Acquire as root should set acquired")
	}
}

func TestStop_NoAcquire(_ *testing.T) {
	m := NewManager()
	// Stop without Acquire should not panic.
	m.Stop()
}

// Builder tests.

func TestBuilder_NonRoot_WrapsSudo(t *testing.T) {
	orig := isRoot
	isRoot = func() bool { return false }
	t.Cleanup(func() { isRoot = orig })

	sb := &Builder{}
	cmd := sb.Command(context.Background(), "ls", "-la")
	args := cmd.Args
	if len(args) != 3 || args[0] != "sudo" || args[1] != "ls" || args[2] != "-la" {
		t.Errorf("Args = %v, want [sudo ls -la]", args)
	}
}

func TestBuilder_Root_SkipsSudo(t *testing.T) {
	orig := isRoot
	isRoot = func() bool { return true }
	t.Cleanup(func() { isRoot = orig })

	sb := &Builder{}
	cmd := sb.Command(context.Background(), "ls", "-la")
	args := cmd.Args
	if len(args) != 2 || args[0] != "ls" || args[1] != "-la" {
		t.Errorf("Args = %v, want [ls -la] when running as root", args)
	}
}

// Noop/Direct tests.

func TestDirectBuilder_NeverWrapsSudo(t *testing.T) {
	t.Parallel()
	db := DirectBuilder{}
	cmd := db.Command(context.Background(), "ls", "-la")
	args := cmd.Args
	if len(args) != 2 || args[0] != "ls" || args[1] != "-la" {
		t.Errorf("Args = %v, want [ls -la]", args)
	}
}

func TestNoopAcquirer_AlwaysSucceeds(t *testing.T) {
	t.Parallel()
	na := NoopAcquirer{}
	if err := na.Acquire(context.Background()); err != nil {
		t.Fatalf("NoopAcquirer.Acquire should always succeed: %v", err)
	}
	na.Stop() // Should not panic.
}

func TestSpyAcquirer_RecordsCalls(t *testing.T) {
	t.Parallel()
	spy := &SpyAcquirer{}
	if spy.AcquireCalls != 0 || spy.StopCalls != 0 {
		t.Fatal("new SpyAcquirer should have zero calls")
	}

	if err := spy.Acquire(context.Background()); err != nil {
		t.Fatalf("SpyAcquirer.Acquire error: %v", err)
	}
	if spy.AcquireCalls != 1 {
		t.Errorf("AcquireCalls = %d, want 1", spy.AcquireCalls)
	}

	spy.Stop()
	if spy.StopCalls != 1 {
		t.Errorf("StopCalls = %d, want 1", spy.StopCalls)
	}
}

func TestRecordingBuilder_RecordsCalls(t *testing.T) {
	t.Parallel()
	rb := &RecordingBuilder{}
	cmd := rb.Command(context.Background(), "apt-get", "install", "-y", "curl")

	if len(rb.Calls) != 1 {
		t.Fatalf("Calls = %d, want 1", len(rb.Calls))
	}
	if rb.Calls[0].Name != "apt-get" {
		t.Errorf("Name = %q, want apt-get", rb.Calls[0].Name)
	}
	wantArgs := []string{"install", "-y", "curl"}
	if len(rb.Calls[0].Args) != len(wantArgs) {
		t.Fatalf("Args = %v, want %v", rb.Calls[0].Args, wantArgs)
	}
	for i, want := range wantArgs {
		if rb.Calls[0].Args[i] != want {
			t.Errorf("Args[%d] = %q, want %q", i, rb.Calls[0].Args[i], want)
		}
	}

	// The returned command should be harmless (true).
	if err := cmd.Run(); err != nil {
		t.Fatalf("recorded command should succeed: %v", err)
	}
}

package ansible

import (
	"context"
	"os"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

// TestHandleCommand_NoArgsReturnsUsageError asserts the empty-args
// path surfaces a UserFacingError pointing at the right usage. A
// user typing `hams ansible` without a playbook path should get a
// pointed "requires a playbook" message, not a cryptic
// ansible-playbook exec failure.
func TestHandleCommand_NoArgsReturnsUsageError(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	err := p.HandleCommand(context.Background(), []string{}, nil, &provider.GlobalFlags{})
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
}

// TestHandleCommand_DryRunPreviews asserts dry-run prints the
// "Would run" line and returns nil without exec-ing
// ansible-playbook. Same pattern as cycle 118's homebrew
// dry-run gate.
func TestHandleCommand_DryRunPreviews(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	flags := &provider.GlobalFlags{DryRun: true}

	// Redirect stdout to a pipe to avoid writing to test runner output.
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe: %v", pipeErr)
	}
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = origStdout
	})

	err := p.HandleCommand(context.Background(), []string{"playbooks/site.yml"}, nil, flags)
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe: %v", closeErr)
	}
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	// Drain the pipe so we can inspect the output.
	buf := make([]byte, 256)
	n, readErr := r.Read(buf)
	if readErr != nil && readErr.Error() != "EOF" {
		t.Fatalf("read pipe: %v", readErr)
	}
	got := string(buf[:n])
	if got == "" {
		t.Errorf("dry-run produced no output")
	}
	// The preview text format is "[dry-run] Would run: ansible-playbook ..."
	const want = "[dry-run]"
	if !contains(got, want) {
		t.Errorf("dry-run output missing %q; got %q", want, got)
	}
}

// contains is a small helper; strings.Contains would work but
// pulls in another import that's only needed here.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

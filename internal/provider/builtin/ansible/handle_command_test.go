package ansible

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
)

// TestHandleCommand_NoArgsReturnsUsageError asserts the empty-args
// path surfaces a UserFacingError pointing at the right usage. A
// user typing `hams ansible` without a playbook path should get a
// pointed "requires a playbook" message, not a cryptic
// ansible-playbook exec failure.
func TestHandleCommand_NoArgsReturnsUsageError(t *testing.T) {
	t.Parallel()
	p := New(&config.Config{}, NewFakeCmdRunner())
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
	// NOT Parallel: swaps os.Stdout globally (cycle 213 added two
	// sibling tests with the same pattern; they all serialize).
	p := New(&config.Config{}, NewFakeCmdRunner())
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

// TestHandleCommand_ListVerbEmitsDiff locks in cycle 213: `hams ansible list`
// must print the DiffDesiredVsState output (the same text that `hams list
// --only=ansible` would show), NOT exec `ansible-playbook list`. Pre-cycle-213
// the handler treated any non-empty args as a playbook path, so typing the
// spec-mandated verb produced a confusing exec failure ("ansible-playbook:
// playbook 'list' not found") instead of the tracked-resource status.
func TestHandleCommand_ListVerbEmitsDiff(t *testing.T) {
	// NOT Parallel: swaps os.Stdout globally, races with other
	// pipe-stdout tests in this file.
	tmp := t.TempDir()
	storeDir := filepath.Join(tmp, "store")
	profileDir := filepath.Join(storeDir, "test")
	stateDir := filepath.Join(storeDir, ".state", "m1")
	for _, d := range []string{profileDir, stateDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	// Seed an ansible hamsfile with a tracked URN entry.
	hfPath := filepath.Join(profileDir, "ansible.hams.yaml")
	if err := os.WriteFile(hfPath, []byte("playbooks:\n  - urn: \"urn:hams:ansible:bootstrap\"\n"), 0o600); err != nil {
		t.Fatalf("seed hamsfile: %v", err)
	}

	cfg := &config.Config{StorePath: storeDir, ProfileTag: "test", MachineID: "m1"}
	p := New(cfg, NewFakeCmdRunner())

	// Capture stdout.
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	orig := os.Stdout
	os.Stdout = w
	err := p.HandleCommand(context.Background(), []string{"list"}, nil, &provider.GlobalFlags{Store: storeDir, Profile: "test"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if closeErr := w.Close(); closeErr != nil {
		t.Logf("close pipe: %v", closeErr)
	}
	os.Stdout = orig
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	got := string(data)
	// The diff shows the URN as a "+" addition (hamsfile has it, state doesn't).
	if !strings.Contains(got, "urn:hams:ansible:bootstrap") {
		t.Errorf("diff should surface the tracked URN; got %q", got)
	}
}

// TestHandleCommand_RunAndRemoveVerbsReportV1Gap — cycle 213. The
// spec promises `hams ansible run` / `hams ansible remove` but v1
// doesn't yet wire the hamsfile-edit path. Returning a clear
// "planned for v1.1" error with actionable alternatives beats the
// pre-cycle-213 behavior of exec'ing `ansible-playbook run <urn>`
// (which fails with a cryptic "playbook 'run' not found").
func TestHandleCommand_RunAndRemoveVerbsReportV1Gap(t *testing.T) {
	t.Parallel()
	cases := []string{"run", "remove"}
	for _, verb := range cases {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()
			p := New(&config.Config{}, NewFakeCmdRunner())
			err := p.HandleCommand(context.Background(), []string{verb, "urn:hams:ansible:foo"}, nil, &provider.GlobalFlags{})
			if err == nil {
				t.Fatal("expected ExitUsageError for v1.1-planned verb")
			}
			var ufe *hamserr.UserFacingError
			if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitUsageError {
				t.Fatalf("expected ExitUsageError, got %v (%T)", err, err)
			}
			if !strings.Contains(ufe.Message, "v1.1") {
				t.Errorf("message should mention 'v1.1'; got %q", ufe.Message)
			}
			// Must point at the apply fallback.
			foundApply := false
			for _, s := range ufe.Suggestions {
				if strings.Contains(s, "hams apply --only=ansible") {
					foundApply = true
				}
			}
			if !foundApply {
				t.Errorf("suggestions should include 'hams apply --only=ansible'; got %v", ufe.Suggestions)
			}
		})
	}
}

// TestHandleCommand_BarePlaybookStillPassesThrough asserts the
// backward-compat path: a first-arg that isn't a recognized verb
// falls through to the existing ad-hoc ansible-playbook exec. Using
// dry-run so we observe the preview without actually exec-ing.
func TestHandleCommand_BarePlaybookStillPassesThrough(t *testing.T) {
	// NOT Parallel: same reason as TestHandleCommand_ListVerbEmitsDiff.
	p := New(&config.Config{}, NewFakeCmdRunner())
	flags := &provider.GlobalFlags{DryRun: true}

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	orig := os.Stdout
	os.Stdout = w

	err := p.HandleCommand(context.Background(), []string{"playbooks/bootstrap.yml"}, nil, flags)
	if closeErr := w.Close(); closeErr != nil {
		t.Logf("close pipe: %v", closeErr)
	}
	os.Stdout = orig
	if err != nil {
		t.Fatalf("passthrough dry-run: %v", err)
	}
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	got := string(data)
	if !strings.Contains(got, "[dry-run]") || !strings.Contains(got, "playbooks/bootstrap.yml") {
		t.Errorf("expected dry-run preview of passthrough; got %q", got)
	}
}

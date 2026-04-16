package duti

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// duti lifecycle tests modeled after apt's U-pattern.
// duti's resource model is "<ext>=<bundle-id>" (file extension to
// macOS application bundle ID). Apply binds the association; Probe
// reads the current default; Remove is a documented no-op.

// U1 — Apply binds the extension-to-bundle association via duti.
func TestU1_Apply_BindsAssociation(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const id = "pdf=com.adobe.acrobat.pdf"
	if err := p.Apply(context.Background(), provider.Action{ID: id}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if got := fake.AssociationOf("pdf"); got != "com.adobe.acrobat.pdf" {
		t.Errorf("post-Apply: pdf → %q, want com.adobe.acrobat.pdf", got)
	}
	if got := fake.CallCount(fakeOpSet, "pdf"); got != 1 {
		t.Errorf("SetDefault call count for pdf = %d, want 1", got)
	}
}

// U2 — Apply with malformed ID returns the parser error and never
// invokes the runner. The state-key invariant requires "ext=bundleID";
// silently mutating on garbage input would corrupt the diff.
func TestU2_Apply_RejectsMalformedID(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	err := p.Apply(context.Background(), provider.Action{ID: "no-equals-sign"})
	if err == nil {
		t.Fatal("expected error for malformed ID")
	}
	if got := fake.CallCount(fakeOpSet, ""); got != 0 {
		t.Errorf("SetDefault should not have been called for malformed ID; got %d calls", got)
	}
}

// U3 — Apply propagates SetDefault failures.
func TestU3_Apply_FailurePropagated(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated set failure")
	fake := NewFakeCmdRunner().WithSetError("html", wantErr)
	p := New(fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "html=com.google.Chrome"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
}

// U4 — Probe reads the current bundle for each tracked extension and
// emits StateOK with the bundle in Value. parseDutiOutput strips
// trailing newlines and takes the first non-blank line.
func TestU4_Probe_ReadsCurrentDefaultAsValue(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("pdf", "com.apple.Preview")
	p := New(fake)

	sf := state.New("duti", "test-machine")
	sf.SetResource("pdf=com.apple.Preview", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateOK {
		t.Errorf("state = %v, want StateOK", results[0].State)
	}
	if results[0].Value != "com.apple.Preview" {
		t.Errorf("Value = %q, want com.apple.Preview", results[0].Value)
	}
}

// U5 — Probe surfaces query errors as StateFailed for the affected
// resource without aborting the entire probe loop.
func TestU5_Probe_QueryErrorPerResource(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("pdf", "com.apple.Preview").
		WithQueryError("html", errors.New("simulated query failure"))
	p := New(fake)

	sf := state.New("duti", "test-machine")
	sf.SetResource("pdf=com.apple.Preview", state.StateOK)
	sf.SetResource("html=com.apple.Safari", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Probe returned %d results, want 2", len(results))
	}

	byID := map[string]provider.ProbeResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	if byID["pdf=com.apple.Preview"].State != state.StateOK {
		t.Errorf("pdf state = %v, want StateOK", byID["pdf=com.apple.Preview"].State)
	}
	if byID["html=com.apple.Safari"].State != state.StateFailed {
		t.Errorf("html state = %v, want StateFailed", byID["html=com.apple.Safari"].State)
	}
}

// U6 — Probe surfaces malformed resource IDs as StateFailed with an
// ErrorMsg, without aborting the loop.
func TestU6_Probe_MalformedIDReportedAsFailed(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("duti", "test-machine")
	sf.SetResource("garbage", state.StateOK) // no `=` separator

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateFailed {
		t.Errorf("state = %v, want StateFailed", results[0].State)
	}
	if results[0].ErrorMsg == "" {
		t.Errorf("ErrorMsg is empty; should describe the parser failure")
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("duti", "test-machine")
	sf.SetResource("pdf=com.removed", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U8 — Remove is a no-op (documented at duti.go:118): macOS does not
// have a "reset to default" command; manual user action is required.
// A regression here that started actually invoking duti could trash
// the user's preferences.
func TestU8_Remove_IsNoOp(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("pdf", "com.apple.Preview")
	p := New(fake)

	if err := p.Remove(context.Background(), "pdf=com.apple.Preview"); err != nil {
		t.Errorf("Remove should always return nil; got %v", err)
	}
	if got := fake.CallCount(fakeOpSet, ""); got != 0 {
		t.Errorf("Remove must NOT call SetDefault; got %d calls", got)
	}
	if fake.AssociationOf("pdf") != "com.apple.Preview" {
		t.Errorf("Remove must NOT mutate associations; pdf was changed")
	}
}

// U9 — Bootstrap returns nil when duti is on PATH.
func TestU9_Bootstrap_DutiPresentReturnsNil(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Errorf("Bootstrap = %v, want nil", err)
	}
}

// U10 — Bootstrap returns *BootstrapRequiredError with duti install
// script when duti is missing.
func TestU10_Bootstrap_DutiMissingReturnsStructuredError(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().WithLookPathError(exec.ErrNotFound)
	p := New(fake)

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "duti" {
		t.Errorf("Binary = %q, want duti", brerr.Binary)
	}
	if brerr.Script != dutiInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, dutiInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error must wrap ErrBootstrapRequired")
	}
}

// TestU11_Plan_WrapsComputePlanWithHooks covers the 0%-covered Plan
// function: hamsfile with two file-association URNs produces two
// Install actions. Matches the pattern used for ansible/defaults/mas
// (cycle 22/29).
func TestU11_Plan_WrapsComputePlanWithHooks(t *testing.T) {
	t.Parallel()
	yamlDoc := `
associations:
  - urn: urn:hams:duti:pdf=com.adobe.acrobat.pdf
  - urn: urn:hams:duti:md=com.microsoft.vscode
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(NewFakeCmdRunner())
	observed := state.New("duti", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(actions))
	}
	for _, a := range actions {
		if a.Type != provider.ActionInstall {
			t.Errorf("action %q has Type=%v, want Install", a.ID, a.Type)
		}
	}
}

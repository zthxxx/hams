package ansible

import (
	"context"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// ansible lifecycle tests modeled after apt's U-pattern.

// U1 — Apply runs the playbook via runner.RunPlaybook.
func TestU1_Apply_RunsPlaybook(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const playbook = "/etc/playbooks/site.yml"
	action := provider.Action{ID: playbook, Resource: playbook}
	if err := p.Apply(context.Background(), action); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.WasPlaybookRun(playbook) {
		t.Errorf("post-Apply: %q should be marked run", playbook)
	}
	if got := fake.CallCount(fakeOpRunPlaybook, playbook); got != 1 {
		t.Errorf("RunPlaybook count = %d, want 1", got)
	}
}

// U2 — Apply with non-string Resource returns an error and never
// invokes the runner. Defensive coding for the Resource→playbookPath
// type assertion.
func TestU2_Apply_RejectsNonStringResource(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	err := p.Apply(context.Background(), provider.Action{ID: "x", Resource: 42})
	if err == nil {
		t.Fatal("expected error when Resource is not a string")
	}
	if got := fake.CallCount(fakeOpRunPlaybook, ""); got != 0 {
		t.Errorf("RunPlaybook should not be called for invalid Resource; got %d", got)
	}
}

// U3 — Apply propagates RunPlaybook errors.
func TestU3_Apply_FailurePropagated(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated playbook failure")
	const playbook = "/etc/playbooks/flaky.yml"
	fake := NewFakeCmdRunner().WithPlaybookError(playbook, wantErr)
	p := New(fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: playbook, Resource: playbook})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
}

// U4 — Remove is a no-op (documented at ansible.go:106). A regression
// here that started actually invoking ansible could re-run a playbook
// in destructive mode by accident.
func TestU4_Remove_IsNoOp(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	if err := p.Remove(context.Background(), "/etc/playbooks/site.yml"); err != nil {
		t.Errorf("Remove should always return nil; got %v", err)
	}
	if got := fake.CallCount(fakeOpRunPlaybook, ""); got != 0 {
		t.Errorf("Remove must NOT call RunPlaybook; got %d calls", got)
	}
}

// U5 — Probe reflects state.File entries verbatim (ansible doesn't
// have a "is this playbook applied" probe; the state IS the truth).
func TestU5_Probe_ReflectsStateVerbatim(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("ansible", "test-machine")
	sf.SetResource("/play1.yml", state.StateOK)
	sf.SetResource("/play2.yml", state.StateFailed)

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
	if byID["/play1.yml"].State != state.StateOK {
		t.Errorf("/play1.yml: state=%v, want StateOK", byID["/play1.yml"].State)
	}
	if byID["/play2.yml"].State != state.StateFailed {
		t.Errorf("/play2.yml: state=%v, want StateFailed", byID["/play2.yml"].State)
	}
}

// U6 — Probe skips StateRemoved.
func TestU6_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("ansible", "test-machine")
	sf.SetResource("/removed.yml", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U7 — Bootstrap returns nil when ansible-playbook is on PATH.
func TestU7_Bootstrap_AnsiblePresentReturnsNil(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Errorf("Bootstrap = %v, want nil", err)
	}
}

// U9 — Plan attaches each playbook URN (action.ID) as the action
// Resource so Apply's Resource→string type assertion succeeds.
// Regression guard for a previously 0% branch.
func TestU9_Plan_AttachesPlaybookPathAsResource(t *testing.T) {
	yamlDoc := `
playbooks:
  - urn: urn:hams:ansible:site.yml
  - urn: urn:hams:ansible:deploy/db.yml
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(NewFakeCmdRunner())
	observed := state.New("ansible", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(actions))
	}
	for _, a := range actions {
		got, ok := a.Resource.(string)
		if !ok || got != a.ID {
			t.Errorf("action %q: Resource = %v, want string == ID", a.ID, a.Resource)
		}
	}
}

// U8 — Bootstrap returns *BootstrapRequiredError with the pipx
// install script when ansible-playbook is missing.
func TestU8_Bootstrap_AnsibleMissingReturnsStructuredError(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().WithLookPathError(errors.New("not found"))
	p := New(fake)

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "ansible-playbook" {
		t.Errorf("Binary = %q, want ansible-playbook", brerr.Binary)
	}
	if brerr.Script != ansibleInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, ansibleInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error must wrap ErrBootstrapRequired")
	}
}

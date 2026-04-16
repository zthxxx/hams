package mas

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

// mas lifecycle tests modeled after apt's U-pattern. Each U-test
// uses FakeCmdRunner so the host's real mas is never invoked.

// U1 — Apply on a missing app triggers `mas install` once.
func TestU1_Apply_InstallsMissingApp(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const appID = "497799835"
	if err := p.Apply(context.Background(), provider.Action{ID: appID}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsInstalled(appID) {
		t.Errorf("post-Apply: %q should be installed", appID)
	}
	if got := fake.CallCount(fakeOpInstall, appID); got != 1 {
		t.Errorf("Install call count for %q = %d, want 1", appID, got)
	}
}

// U2 — Apply is idempotent at the runner level.
func TestU2_Apply_IdempotentOnReapply(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const appID = "409183694"
	for range 3 {
		if err := p.Apply(context.Background(), provider.Action{ID: appID}); err != nil {
			t.Fatalf("Apply error: %v", err)
		}
	}
	if !fake.IsInstalled(appID) {
		t.Errorf("post-Apply: %q should be installed", appID)
	}
	if got := fake.CallCount(fakeOpInstall, appID); got != 3 {
		t.Errorf("Install call count for %q = %d, want 3", appID, got)
	}
}

// U3 — Apply propagates install failure as the returned error AND
// leaves the app uninstalled.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	fake := NewFakeCmdRunner().WithInstallError("999999999", wantErr)
	p := New(fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "999999999"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
	if fake.IsInstalled("999999999") {
		t.Errorf("failed app should NOT be installed; was")
	}
}

// U4 — Remove on an installed app triggers `mas uninstall`.
func TestU4_Remove_UninstallsApp(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("497799835", "15.2")
	p := New(fake)

	if err := p.Remove(context.Background(), "497799835"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if fake.IsInstalled("497799835") {
		t.Errorf("post-Remove: 497799835 should NOT be installed")
	}
	if got := fake.CallCount(fakeOpUninstall, "497799835"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
}

// U5 — Remove failure leaves app installed.
func TestU5_Remove_FailureLeavesAppInstalled(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated remove failure")
	fake := NewFakeCmdRunner().Seed("409183694", "13.2").WithUninstallError("409183694", wantErr)
	p := New(fake)

	gotErr := p.Remove(context.Background(), "409183694")
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if !fake.IsInstalled("409183694") {
		t.Errorf("on Remove failure, 409183694 should remain installed")
	}
}

// U6 — Probe classifies StateOK/StateFailed and recovers version
// from the synthesized list output.
func TestU6_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("497799835", "15.2").
		Seed("409183694", "13.2")
	p := New(fake)

	sf := state.New("mas", "test-machine")
	sf.SetResource("497799835", state.StateOK)
	sf.SetResource("409183694", state.StateOK)
	sf.SetResource("999999999", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("Probe returned %d results, want 3", len(results))
	}

	byID := map[string]provider.ProbeResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	if byID["497799835"].State != state.StateOK || byID["497799835"].Version != "15.2" {
		t.Errorf("497799835: state=%v version=%q, want StateOK 15.2", byID["497799835"].State, byID["497799835"].Version)
	}
	if byID["999999999"].State != state.StateFailed {
		t.Errorf("999999999: state=%v, want StateFailed", byID["999999999"].State)
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("mas", "test-machine")
	sf.SetResource("removed-app", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U8 — Probe surfaces runner.List errors.
func TestU8_Probe_PropagatesRunnerError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("mas not initialized")
	fake := &erroringFake{listErr: wantErr}
	p := New(fake)

	sf := state.New("mas", "test-machine")
	sf.SetResource("anything", state.StateOK)
	if _, err := p.Probe(context.Background(), sf); !errors.Is(err, wantErr) {
		t.Errorf("Probe error = %v, want %v", err, wantErr)
	}
}

// U9 — Bootstrap returns nil when mas is on PATH.
func TestU9_Bootstrap_MasPresentReturnsNil(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Errorf("Bootstrap = %v, want nil", err)
	}
}

// U10 — Bootstrap returns *BootstrapRequiredError with mas script
// when mas is missing.
func TestU10_Bootstrap_MasMissingReturnsStructuredError(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().WithLookPathError(exec.ErrNotFound)
	p := New(fake)

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "mas" {
		t.Errorf("Binary = %q, want mas", brerr.Binary)
	}
	if brerr.Script != masInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, masInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error must wrap ErrBootstrapRequired")
	}
}

// TestU11_Plan_WrapsComputePlanWithHooks exercises the previously
// 0%-covered Plan function: hamsfile with two app URNs should produce
// two Install actions (observed state is empty). Regression guard
// against accidental short-circuiting of ComputePlan or
// PopulateActionHooks.
func TestU11_Plan_WrapsComputePlanWithHooks(t *testing.T) {
	t.Parallel()
	yamlDoc := `
apps:
  - urn: urn:hams:mas:497799835
  - urn: urn:hams:mas:441258766
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(NewFakeCmdRunner())
	observed := state.New("mas", "test")
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

// erroringFake overrides only List for List-error simulation.
type erroringFake struct {
	*FakeCmdRunner
	listErr error
}

func (e *erroringFake) List(_ context.Context) (string, error) {
	return "", e.listErr
}

func (e *erroringFake) Install(_ context.Context, _ string) error   { return nil }
func (e *erroringFake) Uninstall(_ context.Context, _ string) error { return nil }
func (e *erroringFake) LookPath() error                             { return nil }

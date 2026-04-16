package pnpm

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// pnpm lifecycle tests modeled after apt's U-pattern. Each U-test
// uses FakeCmdRunner so the host's real pnpm is never invoked.

// U1 — Apply on a missing package triggers `pnpm add` once.
func TestU1_Apply_InstallsMissingPackage(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	const pkg = "typescript"
	if err := p.Apply(context.Background(), provider.Action{ID: pkg}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsInstalled(pkg) {
		t.Errorf("post-Apply: %q should be installed", pkg)
	}
	if got := fake.CallCount(fakeOpInstall, pkg); got != 1 {
		t.Errorf("Install call count for %q = %d, want 1", pkg, got)
	}
}

// U2 — Apply is idempotent at the runner level (each Apply still
// invokes the runner; the fake just re-marks installed).
func TestU2_Apply_IdempotentOnReapply(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	const pkg = "serve"
	for range 3 {
		if err := p.Apply(context.Background(), provider.Action{ID: pkg}); err != nil {
			t.Fatalf("Apply error: %v", err)
		}
	}
	if !fake.IsInstalled(pkg) {
		t.Errorf("post-Apply: %q should be installed", pkg)
	}
	if got := fake.CallCount(fakeOpInstall, pkg); got != 3 {
		t.Errorf("Install call count for %q = %d, want 3", pkg, got)
	}
}

// U3 — Apply propagates install failure as the returned error AND
// leaves the package uninstalled.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	fake := NewFakeCmdRunner().WithInstallError("flaky-pkg", wantErr)
	p := New(nil, fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "flaky-pkg"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
	if fake.IsInstalled("flaky-pkg") {
		t.Errorf("failed pkg should NOT be installed; was")
	}
}

// U4 — Remove on an installed package triggers `pnpm remove`.
func TestU4_Remove_UninstallsPackage(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("typescript", "5.3.3")
	p := New(nil, fake)

	if err := p.Remove(context.Background(), "typescript"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if fake.IsInstalled("typescript") {
		t.Errorf("post-Remove: typescript should NOT be installed")
	}
	if got := fake.CallCount(fakeOpUninstall, "typescript"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
}

// U5 — Remove failure is propagated; package stays installed.
func TestU5_Remove_FailureLeavesPackageInstalled(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated remove failure")
	fake := NewFakeCmdRunner().Seed("serve", "14.2.0").WithUninstallError("serve", wantErr)
	p := New(nil, fake)

	gotErr := p.Remove(context.Background(), "serve")
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if !fake.IsInstalled("serve") {
		t.Errorf("on Remove failure, serve should remain installed")
	}
}

// U6 — Probe classifies StateOK/StateFailed correctly per the
// installed-set in the fake.
func TestU6_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("typescript", "5.3.3").
		Seed("serve", "14.2.0")
	p := New(nil, fake)

	sf := state.New("pnpm", "test-machine")
	sf.SetResource("typescript", state.StateOK)
	sf.SetResource("serve", state.StateOK)
	sf.SetResource("missing", state.StateOK)

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
	if byID["typescript"].State != state.StateOK {
		t.Errorf("typescript: state=%v, want StateOK", byID["typescript"].State)
	}
	if byID["serve"].State != state.StateOK {
		t.Errorf("serve: state=%v, want StateOK", byID["serve"].State)
	}
	if byID["missing"].State != state.StateFailed {
		t.Errorf("missing: state=%v, want StateFailed", byID["missing"].State)
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	sf := state.New("pnpm", "test-machine")
	sf.SetResource("removed-pkg", state.StateRemoved)

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
	wantErr := errors.New("pnpm not initialized")
	fake := &erroringFake{listErr: wantErr}
	p := New(nil, fake)

	sf := state.New("pnpm", "test-machine")
	sf.SetResource("anything", state.StateOK)
	if _, err := p.Probe(context.Background(), sf); !errors.Is(err, wantErr) {
		t.Errorf("Probe error = %v, want %v", err, wantErr)
	}
}

// U9 — Bootstrap returns nil when pnpm is on PATH (LookPath success).
func TestU9_Bootstrap_PnpmPresentReturnsNil(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner()) // no LookPath error
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Errorf("Bootstrap = %v, want nil", err)
	}
}

// U10 — Bootstrap returns BootstrapRequiredError (with the install
// script) when pnpm is missing. Verifies the wrap-into-typed-error
// behavior, not just the underlying LookPath result.
func TestU10_Bootstrap_PnpmMissingReturnsStructuredError(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().WithLookPathError(exec.ErrNotFound)
	p := New(nil, fake)

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T (%v)", err, err)
	}
	if brerr.Binary != "pnpm" {
		t.Errorf("Binary = %q, want pnpm", brerr.Binary)
	}
	if brerr.Script != pnpmInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, pnpmInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error must wrap ErrBootstrapRequired")
	}
}

// erroringFake overrides only List so we can simulate List-error paths.
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

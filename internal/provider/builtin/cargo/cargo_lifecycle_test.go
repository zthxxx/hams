package cargo

import (
	"context"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Cargo lifecycle tests modeled after apt's U-pattern. Each U-test
// exercises one verb with the FakeCmdRunner so the host's real cargo
// is never invoked. The fake captures every call for post-condition
// assertion.

// U1 — Apply on a missing crate triggers `cargo install` once.
func TestU1_Apply_InstallsMissingCrate(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	const crate = "ripgrep"
	if err := p.Apply(context.Background(), provider.Action{ID: crate}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsInstalled(crate) {
		t.Errorf("post-Apply: %q should be installed", crate)
	}
	if got := fake.CallCount(fakeOpInstall, crate); got != 1 {
		t.Errorf("Install call count for %q = %d, want 1", crate, got)
	}
}

// U2 — Apply is idempotent: a second Apply on the same crate produces
// no change in the installed set (the fake just re-marks installed).
func TestU2_Apply_IdempotentOnReapply(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	const crate = "bat"
	for range 3 {
		if err := p.Apply(context.Background(), provider.Action{ID: crate}); err != nil {
			t.Fatalf("Apply error: %v", err)
		}
	}
	if !fake.IsInstalled(crate) {
		t.Errorf("post-Apply: %q should be installed", crate)
	}
	if got := fake.CallCount(fakeOpInstall, crate); got != 3 {
		t.Errorf("Install call count for %q = %d, want 3 (each Apply hits cargo)", crate, got)
	}
}

// U3 — Apply propagates install failure as the returned error AND
// leaves the crate uninstalled. Mirrors apt's transactional contract.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	fake := NewFakeCmdRunner().WithInstallError("flaky-crate", wantErr)
	p := New(nil, fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "flaky-crate"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
	if fake.IsInstalled("flaky-crate") {
		t.Errorf("failed crate should NOT be installed; was")
	}
}

// U4 — Remove on an installed crate triggers `cargo uninstall` and
// the crate disappears from the installed set.
func TestU4_Remove_UninstallsCrate(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("fd-find", "10.1.0")
	p := New(nil, fake)

	if err := p.Remove(context.Background(), "fd-find"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if fake.IsInstalled("fd-find") {
		t.Errorf("post-Remove: fd-find should NOT be installed")
	}
	if got := fake.CallCount(fakeOpUninstall, "fd-find"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
}

// U5 — Remove failure is propagated; the crate is NOT removed from
// the installed set when the runner errors.
func TestU5_Remove_FailureLeavesCrateInstalled(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated remove failure")
	fake := NewFakeCmdRunner().Seed("tokei", "12.1.2").WithUninstallError("tokei", wantErr)
	p := New(nil, fake)

	gotErr := p.Remove(context.Background(), "tokei")
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if !fake.IsInstalled("tokei") {
		t.Errorf("on Remove failure, tokei should remain installed")
	}
}

// U6 — Probe queries the runner once and emits StateOK for every
// state-tracked resource that's installed, StateFailed for ones that
// aren't.
func TestU6_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("ripgrep", "14.1.0").
		Seed("bat", "0.24.0")
	p := New(nil, fake)

	sf := state.New("cargo", "test-machine")
	sf.SetResource("ripgrep", state.StateOK) // installed
	sf.SetResource("bat", state.StateOK)     // installed
	sf.SetResource("missing", state.StateOK) // NOT installed

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
	if byID["ripgrep"].State != state.StateOK || byID["ripgrep"].Version != "14.1.0" {
		t.Errorf("ripgrep: state=%v version=%q, want StateOK 14.1.0", byID["ripgrep"].State, byID["ripgrep"].Version)
	}
	if byID["bat"].State != state.StateOK || byID["bat"].Version != "0.24.0" {
		t.Errorf("bat: state=%v version=%q, want StateOK 0.24.0", byID["bat"].State, byID["bat"].Version)
	}
	if byID["missing"].State != state.StateFailed {
		t.Errorf("missing: state=%v, want StateFailed", byID["missing"].State)
	}
}

// U7 — Probe skips resources marked StateRemoved (they shouldn't
// appear in the output at all). Without this skip, removed resources
// would re-emit as StateFailed and trip a re-install on next apply.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	sf := state.New("cargo", "test-machine")
	sf.SetResource("removed-crate", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved entry, want 0", len(results))
	}
}

// U8 — Probe surfaces runner.List errors as a fatal error to the
// caller. Apply-time failures of the upstream cargo binary must never
// be silently swallowed.
func TestU8_Probe_PropagatesRunnerError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("cargo not initialized")
	fake := &erroringFake{listErr: wantErr}
	p := New(nil, fake)

	sf := state.New("cargo", "test-machine")
	sf.SetResource("anything", state.StateOK)
	if _, err := p.Probe(context.Background(), sf); !errors.Is(err, wantErr) {
		t.Errorf("Probe error = %v, want %v", err, wantErr)
	}
}

// U9 — Bootstrap delegates to runner.LookPath; success → nil,
// configured failure → propagated error.
func TestU9_Bootstrap_DelegatesLookPath(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		p := New(nil, NewFakeCmdRunner())
		if err := p.Bootstrap(context.Background()); err != nil {
			t.Errorf("Bootstrap error = %v, want nil", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		want := errors.New("cargo not on PATH")
		p := New(nil, NewFakeCmdRunner().WithLookPathError(want))
		if err := p.Bootstrap(context.Background()); !errors.Is(err, want) {
			t.Errorf("Bootstrap error = %v, want %v", err, want)
		}
	})
}

// erroringFake is a CmdRunner whose List returns a configured error
// — used by TestU8 because the standard FakeCmdRunner has no
// per-method configurable List error (List always succeeds with the
// synthesized installed-set output).
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

// TestU10_Plan_WrapsComputePlanWithHooks covers the previously 0%
// Plan function: two-URN hamsfile produces two Install actions on
// an empty observed state. Same pattern as the ansible/defaults/mas/
// duti Plan tests from cycles 22/29/30.
func TestU10_Plan_WrapsComputePlanWithHooks(t *testing.T) {
	t.Parallel()
	yamlDoc := `
crates:
  - urn: urn:hams:cargo:ripgrep
  - urn: urn:hams:cargo:fd-find
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(nil, NewFakeCmdRunner())
	observed := state.New("cargo", "test")
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

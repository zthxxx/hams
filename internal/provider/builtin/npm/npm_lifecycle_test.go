package npm

import (
	"context"
	"errors"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// npm lifecycle tests modeled after apt's U-pattern. Each U-test uses
// FakeCmdRunner so the host's real npm is never invoked.

// U1 — Apply on a missing package triggers install once.
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
// invokes the runner; the fake just re-marks installed). This matches
// apt's contract: apply doesn't pre-check; the executor does.
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

// U4 — Remove on an installed package triggers uninstall and the
// package disappears from the installed set.
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
// installed-set in the fake. Note: parseNpmList drops version info
// (only names matter for the diff), so probe results have empty
// version strings even when the fake records a version.
func TestU6_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("typescript", "5.3.3").
		Seed("serve", "14.2.0")
	p := New(nil, fake)

	sf := state.New("npm", "test-machine")
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

// TestProbe_MatchesPinnedVersionIDs locks in cycle 189: state IDs
// with `@version` pins strip the suffix before the installed-map
// lookup. Scoped packages (`@scope/bar`) preserve the leading `@`.
// Pre-cycle-189 a state entry like "typescript@5.3.3" would never
// match the installed map (keyed on bare name) and always return
// StateFailed, breaking drift detection for any CLI-pinned install.
func TestProbe_MatchesPinnedVersionIDs(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("typescript", "5.3.3").
		Seed("@scope/tool", "1.0.0")
	p := New(nil, fake)

	sf := state.New("npm", "test-machine")
	sf.SetResource("typescript@5.3.3", state.StateOK)
	sf.SetResource("@scope/tool@1.0.0", state.StateOK)
	sf.SetResource("@scope/bare", state.StateOK) // scoped without pin

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	byID := map[string]provider.ProbeResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	if byID["typescript@5.3.3"].State != state.StateOK {
		t.Errorf("pinned typescript: state=%v, want StateOK", byID["typescript@5.3.3"].State)
	}
	if byID["@scope/tool@1.0.0"].State != state.StateOK {
		t.Errorf("pinned scoped: state=%v, want StateOK", byID["@scope/tool@1.0.0"].State)
	}
	// Scoped without pin: NOT in fake's installed → StateFailed (still bare vs "@scope/bare").
	if byID["@scope/bare"].State != state.StateFailed {
		t.Errorf("@scope/bare (absent): state=%v, want StateFailed", byID["@scope/bare"].State)
	}
}

// TestStripNpmVersionPin covers the pure-helper invariant.
func TestStripNpmVersionPin(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"foo":              "foo",
		"foo@1.2.3":        "foo",
		"@scope/bar":       "@scope/bar",
		"@scope/bar@1.2.3": "@scope/bar",
		"@scope/bar@^1.0":  "@scope/bar",
		"":                 "",
		"@":                "@",
	}
	for in, want := range cases {
		if got := stripNpmVersionPin(in); got != want {
			t.Errorf("stripNpmVersionPin(%q) = %q, want %q", in, got, want)
		}
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	sf := state.New("npm", "test-machine")
	sf.SetResource("removed-pkg", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U8 — Probe surfaces runner.List errors as a fatal error to the
// caller. npm-list failures (e.g., missing node_modules dir) must
// never be silently swallowed.
func TestU8_Probe_PropagatesRunnerError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("npm not initialized")
	fake := &erroringFake{listErr: wantErr}
	p := New(nil, fake)

	sf := state.New("npm", "test-machine")
	sf.SetResource("anything", state.StateOK)
	if _, err := p.Probe(context.Background(), sf); !errors.Is(err, wantErr) {
		t.Errorf("Probe error = %v, want %v", err, wantErr)
	}
}

// U9 — Bootstrap delegates to runner.LookPath; both paths verified.
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
		want := errors.New("npm not on PATH")
		p := New(nil, NewFakeCmdRunner().WithLookPathError(want))
		if err := p.Bootstrap(context.Background()); !errors.Is(err, want) {
			t.Errorf("Bootstrap error = %v, want %v", err, want)
		}
	})
}

// erroringFake overrides only List so we can simulate List-error paths
// without breaking the rest of the FakeCmdRunner contract.
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

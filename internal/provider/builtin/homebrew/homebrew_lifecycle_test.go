package homebrew

import (
	"context"
	"errors"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// homebrew lifecycle tests modeled after apt's U-pattern, plus
// brew-specific tests for cask handling.

// U1 — Apply on a missing formula triggers `brew install` once.
func TestU1_Apply_InstallsMissingFormula(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(&config.Config{}, fake)

	const pkg = "ripgrep"
	if err := p.Apply(context.Background(), provider.Action{ID: pkg}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsFormulaeInstalled(pkg) {
		t.Errorf("post-Apply: %q should be in formulae set", pkg)
	}
	if got := fake.CallCount(fakeOpInstall, pkg); got != 1 {
		t.Errorf("Install call count for %q = %d, want 1", pkg, got)
	}
	if isCask, _ := fake.LastInstallIsCask(); isCask {
		t.Errorf("LastInstall was cask, expected formula")
	}
}

// U2 — Apply with action.Resource=BrewResource{IsCask:true} routes to
// the cask cellar via --cask flag (modeled by fake.IsCaskInstalled).
func TestU2_Apply_CaskRoutesToCaskCellar(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(&config.Config{}, fake)

	const cask = "visual-studio-code"
	action := provider.Action{ID: cask, Resource: BrewResource{IsCask: true}}
	if err := p.Apply(context.Background(), action); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsCaskInstalled(cask) {
		t.Errorf("post-Apply with IsCask=true: %q should be in casks set", cask)
	}
	if fake.IsFormulaeInstalled(cask) {
		t.Errorf("post-Apply with IsCask=true: %q should NOT be in formulae set", cask)
	}
	isCask, ok := fake.LastInstallIsCask()
	if !ok || !isCask {
		t.Errorf("LastInstallIsCask = (%v, %v), want (true, true)", isCask, ok)
	}
}

// U3 — Apply propagates install errors AND leaves the package out of
// the installed sets.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	fake := NewFakeCmdRunner().WithInstallError("flaky-pkg", wantErr)
	p := New(&config.Config{}, fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "flaky-pkg"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
	if fake.IsFormulaeInstalled("flaky-pkg") || fake.IsCaskInstalled("flaky-pkg") {
		t.Errorf("failed pkg should NOT be installed in any set")
	}
}

// U4 — Remove on an installed formula triggers `brew uninstall`.
func TestU4_Remove_UninstallsPackage(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().SeedFormula("ripgrep", "14.1.0")
	p := New(&config.Config{}, fake)

	if err := p.Remove(context.Background(), "ripgrep"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if fake.IsFormulaeInstalled("ripgrep") {
		t.Errorf("post-Remove: ripgrep should NOT be in formulae set")
	}
	if got := fake.CallCount(fakeOpUninstall, "ripgrep"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
}

// U5 — Remove failure leaves package installed.
func TestU5_Remove_FailureLeavesPackageInstalled(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated remove failure")
	fake := NewFakeCmdRunner().
		SeedFormula("git", "2.42.0").
		WithUninstallError("git", wantErr)
	p := New(&config.Config{}, fake)

	gotErr := p.Remove(context.Background(), "git")
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if !fake.IsFormulaeInstalled("git") {
		t.Errorf("on Remove failure, git should remain installed")
	}
}

// U6 — Probe merges formulae + casks + taps and classifies StateOK
// per installed-set, with version recovery for formulae and casks.
func TestU6_Probe_MergesFormulaeAndCasksAndTaps(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		SeedFormula("ripgrep", "14.1.0").
		SeedCask("visual-studio-code", "1.85.0").
		SeedTap("homebrew/cask-fonts")
	p := New(&config.Config{}, fake)

	sf := state.New("brew", "test-machine")
	sf.SetResource("ripgrep", state.StateOK)
	sf.SetResource("visual-studio-code", state.StateOK)
	sf.SetResource("homebrew/cask-fonts", state.StateOK)
	sf.SetResource("missing-pkg", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("Probe returned %d results, want 4", len(results))
	}

	byID := map[string]provider.ProbeResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	if byID["ripgrep"].State != state.StateOK || byID["ripgrep"].Version != "14.1.0" {
		t.Errorf("ripgrep: state=%v version=%q, want StateOK 14.1.0", byID["ripgrep"].State, byID["ripgrep"].Version)
	}
	if byID["visual-studio-code"].State != state.StateOK || byID["visual-studio-code"].Version != "1.85.0" {
		t.Errorf("vsc: state=%v version=%q, want StateOK 1.85.0", byID["visual-studio-code"].State, byID["visual-studio-code"].Version)
	}
	if byID["homebrew/cask-fonts"].State != state.StateOK {
		t.Errorf("tap: state=%v, want StateOK (taps probe with empty version)", byID["homebrew/cask-fonts"].State)
	}
	if byID["missing-pkg"].State != state.StateFailed {
		t.Errorf("missing-pkg: state=%v, want StateFailed", byID["missing-pkg"].State)
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(&config.Config{}, NewFakeCmdRunner())
	sf := state.New("brew", "test-machine")
	sf.SetResource("removed-pkg", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U8 — Probe propagates ListFormulae errors as a hard error
// (formulae listing failure is fatal; cask-listing failure is not —
// see U9 for the documented swallow).
func TestU8_Probe_PropagatesFormulaeListError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("brew not initialized")
	fake := NewFakeCmdRunner().WithFormulaeListError(wantErr)
	p := New(&config.Config{}, fake)

	sf := state.New("brew", "test-machine")
	sf.SetResource("anything", state.StateOK)
	if _, err := p.Probe(context.Background(), sf); !errors.Is(err, wantErr) {
		t.Errorf("Probe error = %v, want wrapping %v", err, wantErr)
	}
}

// U9 — Probe SWALLOWS cask-listing errors. Real brew exits non-zero
// when zero casks are installed; the provider must not surface this
// as a hard error (would block apply on a casks-free machine). The
// test asserts probe succeeds and reports formulae normally.
func TestU9_Probe_SwallowsCaskListError(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		SeedFormula("ripgrep", "14.1.0").
		WithCasksListError(errors.New("no casks installed (brew exit 1)"))
	p := New(&config.Config{}, fake)

	sf := state.New("brew", "test-machine")
	sf.SetResource("ripgrep", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe should succeed despite cask error; got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateOK || results[0].Version != "14.1.0" {
		t.Errorf("ripgrep result = %+v, want StateOK 14.1.0", results[0])
	}
}

// U11 — Remove on a tap-format ID (user/repo, no formula suffix)
// routes to `brew untap`, NOT `brew uninstall`. Otherwise brew
// returns "no installed keg or cask" and the tap stays registered.
func TestU11_Remove_TapFormatID_RoutesToUntap(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().SeedTap("homebrew/cask-fonts")
	p := New(&config.Config{}, fake)

	if err := p.Remove(context.Background(), "homebrew/cask-fonts"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := fake.CallCount(fakeOpUntap, "homebrew/cask-fonts"); got != 1 {
		t.Errorf("Untap call count = %d, want 1", got)
	}
	if got := fake.CallCount(fakeOpUninstall, "homebrew/cask-fonts"); got != 0 {
		t.Errorf("Uninstall must not be called for tap; got %d", got)
	}
	// Verify tap was actually removed from the fake.
	if fake.IsTapRegistered("homebrew/cask-fonts") {
		t.Error("tap should be removed from state")
	}
}

// U12 — Remove on a package-format ID still goes through Uninstall
// (regression guard: ensure the tap-routing branch doesn't misfire).
func TestU12_Remove_PackageID_RoutesToUninstall(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().SeedFormula("htop", "3.3.0")
	p := New(&config.Config{}, fake)

	if err := p.Remove(context.Background(), "htop"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := fake.CallCount(fakeOpUninstall, "htop"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
	if got := fake.CallCount(fakeOpUntap, "htop"); got != 0 {
		t.Errorf("Untap must not be called for package; got %d", got)
	}
}

// U10 — Probe SWALLOWS tap-listing errors (taps are auxiliary;
// failure should not block formulae/cask diff).
func TestU10_Probe_SwallowsTapListError(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		SeedFormula("ripgrep", "14.1.0").
		WithTapsListError(errors.New("brew tap failed"))
	p := New(&config.Config{}, fake)

	sf := state.New("brew", "test-machine")
	sf.SetResource("ripgrep", state.StateOK)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe should succeed despite tap error; got %v", err)
	}
	if len(results) != 1 || results[0].State != state.StateOK {
		t.Errorf("Probe results = %+v, want one StateOK", results)
	}
}

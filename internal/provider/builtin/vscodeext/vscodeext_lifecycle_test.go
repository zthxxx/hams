package vscodeext

import (
	"context"
	"errors"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// vscodeext lifecycle tests modeled after apt's U-pattern.
// VS Code extension IDs are case-insensitive on the marketplace; the
// fake mirrors this by lowercasing keys on Seed/Install.

// U1 — Apply on a missing extension triggers `code --install-extension`.
func TestU1_Apply_InstallsMissingExtension(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	const ext = "ms-python.python"
	if err := p.Apply(context.Background(), provider.Action{ID: ext}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsInstalled(ext) {
		t.Errorf("post-Apply: %q should be installed", ext)
	}
	if got := fake.CallCount(fakeOpInstall, ext); got != 1 {
		t.Errorf("Install call count for %q = %d, want 1", ext, got)
	}
}

// U2 — Apply is idempotent at the runner level.
func TestU2_Apply_IdempotentOnReapply(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(nil, fake)

	const ext = "esbenp.prettier-vscode"
	for range 3 {
		if err := p.Apply(context.Background(), provider.Action{ID: ext}); err != nil {
			t.Fatalf("Apply error: %v", err)
		}
	}
	if !fake.IsInstalled(ext) {
		t.Errorf("post-Apply: %q should be installed", ext)
	}
	if got := fake.CallCount(fakeOpInstall, ext); got != 3 {
		t.Errorf("Install call count for %q = %d, want 3", ext, got)
	}
}

// U3 — Apply propagates install errors AND leaves extension uninstalled.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	fake := NewFakeCmdRunner().WithInstallError("flaky.ext", wantErr)
	p := New(nil, fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "flaky.ext"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
	if fake.IsInstalled("flaky.ext") {
		t.Errorf("failed extension should NOT be installed")
	}
}

// U4 — Remove on an installed extension triggers `code --uninstall-extension`.
func TestU4_Remove_UninstallsExtension(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("ms-python.python", "2024.2.0")
	p := New(nil, fake)

	if err := p.Remove(context.Background(), "ms-python.python"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if fake.IsInstalled("ms-python.python") {
		t.Errorf("post-Remove: ms-python.python should NOT be installed")
	}
	if got := fake.CallCount(fakeOpUninstall, "ms-python.python"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
}

// U5 — Remove failure leaves extension installed.
func TestU5_Remove_FailureLeavesExtensionInstalled(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated remove failure")
	fake := NewFakeCmdRunner().
		Seed("dbaeumer.vscode-eslint", "3.0.5").
		WithUninstallError("dbaeumer.vscode-eslint", wantErr)
	p := New(nil, fake)

	gotErr := p.Remove(context.Background(), "dbaeumer.vscode-eslint")
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if !fake.IsInstalled("dbaeumer.vscode-eslint") {
		t.Errorf("on Remove failure, extension should remain installed")
	}
}

// U6 — Probe classifies StateOK with version, StateFailed when absent.
// Crucially: probe does case-insensitive lookup, matching production
// (extension IDs from `code --list-extensions` are always lowercased
// but the user's hamsfile entry may be mixed-case).
func TestU6_Probe_OKAndFailedClassification_CaseInsensitive(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("ms-python.python", "2024.2.0").
		Seed("dbaeumer.vscode-eslint", "3.0.5")
	p := New(nil, fake)

	sf := state.New("code", "test-machine")
	// Mixed-case in the hamsfile entry — should still match because
	// the parser lowercases names AND probe does ToLower(id) lookup.
	sf.SetResource("MS-Python.Python", state.StateOK)
	sf.SetResource("dbaeumer.vscode-eslint", state.StateOK)
	sf.SetResource("missing.ext", state.StateOK)

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
	if byID["MS-Python.Python"].State != state.StateOK || byID["MS-Python.Python"].Version != "2024.2.0" {
		t.Errorf("Mixed-case lookup failed: %+v, want StateOK 2024.2.0", byID["MS-Python.Python"])
	}
	if byID["dbaeumer.vscode-eslint"].State != state.StateOK || byID["dbaeumer.vscode-eslint"].Version != "3.0.5" {
		t.Errorf("dbaeumer.vscode-eslint: %+v, want StateOK 3.0.5", byID["dbaeumer.vscode-eslint"])
	}
	if byID["missing.ext"].State != state.StateFailed {
		t.Errorf("missing.ext: state=%v, want StateFailed", byID["missing.ext"].State)
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	sf := state.New("code", "test-machine")
	sf.SetResource("removed.ext", state.StateRemoved)

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
	wantErr := errors.New("code CLI hung")
	fake := &erroringFake{listErr: wantErr}
	p := New(nil, fake)

	sf := state.New("code", "test-machine")
	sf.SetResource("anything", state.StateOK)
	if _, err := p.Probe(context.Background(), sf); !errors.Is(err, wantErr) {
		t.Errorf("Probe error = %v, want %v", err, wantErr)
	}
}

// U9 — Bootstrap delegates to runner.LookPath.
func TestU9_Bootstrap_DelegatesLookPath(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		p := New(nil, NewFakeCmdRunner())
		if err := p.Bootstrap(context.Background()); err != nil {
			t.Errorf("Bootstrap = %v, want nil", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		want := errors.New("code CLI not found in PATH")
		p := New(nil, NewFakeCmdRunner().WithLookPathError(want))
		if err := p.Bootstrap(context.Background()); !errors.Is(err, want) {
			t.Errorf("Bootstrap error = %v, want %v", err, want)
		}
	})
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

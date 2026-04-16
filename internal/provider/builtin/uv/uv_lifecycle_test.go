package uv

import (
	"context"
	"errors"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// uv lifecycle tests modeled after apt's U-pattern. Each U-test uses
// FakeCmdRunner so the host's real uv is never invoked.

// U1 — Apply on a missing tool triggers `uv tool install` once.
func TestU1_Apply_InstallsMissingTool(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const tool = "ruff"
	if err := p.Apply(context.Background(), provider.Action{ID: tool}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !fake.IsInstalled(tool) {
		t.Errorf("post-Apply: %q should be installed", tool)
	}
	if got := fake.CallCount(fakeOpInstall, tool); got != 1 {
		t.Errorf("Install call count for %q = %d, want 1", tool, got)
	}
}

// U2 — Apply is idempotent at the runner level.
func TestU2_Apply_IdempotentOnReapply(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const tool = "black"
	for range 3 {
		if err := p.Apply(context.Background(), provider.Action{ID: tool}); err != nil {
			t.Fatalf("Apply error: %v", err)
		}
	}
	if !fake.IsInstalled(tool) {
		t.Errorf("post-Apply: %q should be installed", tool)
	}
	if got := fake.CallCount(fakeOpInstall, tool); got != 3 {
		t.Errorf("Install call count for %q = %d, want 3", tool, got)
	}
}

// U3 — Apply propagates install failure as the returned error AND
// leaves the tool uninstalled.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	fake := NewFakeCmdRunner().WithInstallError("flaky-tool", wantErr)
	p := New(fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: "flaky-tool"})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
	if fake.IsInstalled("flaky-tool") {
		t.Errorf("failed tool should NOT be installed; was")
	}
}

// U4 — Remove on an installed tool triggers `uv tool uninstall`.
func TestU4_Remove_UninstallsTool(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("ruff", "0.3.0")
	p := New(fake)

	if err := p.Remove(context.Background(), "ruff"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if fake.IsInstalled("ruff") {
		t.Errorf("post-Remove: ruff should NOT be installed")
	}
	if got := fake.CallCount(fakeOpUninstall, "ruff"); got != 1 {
		t.Errorf("Uninstall call count = %d, want 1", got)
	}
}

// U5 — Remove failure is propagated; tool stays installed.
func TestU5_Remove_FailureLeavesToolInstalled(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated remove failure")
	fake := NewFakeCmdRunner().Seed("mypy", "1.8.0").WithUninstallError("mypy", wantErr)
	p := New(fake)

	gotErr := p.Remove(context.Background(), "mypy")
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Remove error = %v, want %v", gotErr, wantErr)
	}
	if !fake.IsInstalled("mypy") {
		t.Errorf("on Remove failure, mypy should remain installed")
	}
}

// U6 — Probe classifies StateOK/StateFailed and recovers version
// from the synthesized list output.
func TestU6_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().
		Seed("ruff", "0.3.0").
		Seed("black", "24.2.0")
	p := New(fake)

	sf := state.New("uv", "test-machine")
	sf.SetResource("ruff", state.StateOK)
	sf.SetResource("black", state.StateOK)
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
	if byID["ruff"].State != state.StateOK || byID["ruff"].Version != "0.3.0" {
		t.Errorf("ruff: state=%v version=%q, want StateOK 0.3.0", byID["ruff"].State, byID["ruff"].Version)
	}
	if byID["black"].State != state.StateOK || byID["black"].Version != "24.2.0" {
		t.Errorf("black: state=%v version=%q, want StateOK 24.2.0", byID["black"].State, byID["black"].Version)
	}
	if byID["missing"].State != state.StateFailed {
		t.Errorf("missing: state=%v, want StateFailed", byID["missing"].State)
	}
}

// U7 — Probe skips StateRemoved entries.
func TestU7_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("uv", "test-machine")
	sf.SetResource("removed-tool", state.StateRemoved)

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
	wantErr := errors.New("uv not initialized")
	fake := &erroringFake{listErr: wantErr}
	p := New(fake)

	sf := state.New("uv", "test-machine")
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
		p := New(NewFakeCmdRunner())
		if err := p.Bootstrap(context.Background()); err != nil {
			t.Errorf("Bootstrap error = %v, want nil", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		want := errors.New("uv not on PATH")
		p := New(NewFakeCmdRunner().WithLookPathError(want))
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

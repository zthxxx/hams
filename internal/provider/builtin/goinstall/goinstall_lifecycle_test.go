package goinstall

import (
	"context"
	"errors"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// goinstall lifecycle tests modeled after apt's U-pattern, with two
// goinstall-specific properties:
//   - Apply auto-injects @latest when the user-supplied id has no version
//   - Remove is a documented no-op (no `go uninstall` exists)

// U1 — Apply with no @ in ID auto-injects @latest before invoking go.
func TestU1_Apply_AutoInjectsLatest(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const bare = "github.com/example/tool"
	if err := p.Apply(context.Background(), provider.Action{ID: bare}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	// Fake records the augmented form (bare + "@latest").
	if got := fake.CallCount(fakeOpInstall, bare+"@latest"); got != 1 {
		t.Errorf("Install was not invoked with @latest auto-injected; want 1 call to %q, got %d", bare+"@latest", got)
	}
}

// U2 — Apply with explicit @version preserves the pin (no @latest reinjection).
func TestU2_Apply_PreservesExplicitVersion(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner()
	p := New(fake)

	const pinned = "github.com/example/tool@v1.2.3"
	if err := p.Apply(context.Background(), provider.Action{ID: pinned}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if got := fake.CallCount(fakeOpInstall, pinned); got != 1 {
		t.Errorf("Install pin not preserved; want 1 call to %q, got %d", pinned, got)
	}
	if got := fake.CallCount(fakeOpInstall, pinned+"@latest"); got != 0 {
		t.Errorf("explicit version should NOT have @latest re-injected")
	}
}

// U3 — Apply propagates install failure as the returned error.
func TestU3_Apply_FailureNotRecorded(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated install failure")
	const pkg = "github.com/example/flaky"
	fake := NewFakeCmdRunner().WithInstallError(pkg+"@latest", wantErr)
	p := New(fake)

	gotErr := p.Apply(context.Background(), provider.Action{ID: pkg})
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("Apply error = %v, want %v", gotErr, wantErr)
	}
}

// U4 — Remove returns nil and warns. Documented no-op (go install has
// no native uninstall command).
func TestU4_Remove_IsNoOp(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	if err := p.Remove(context.Background(), "github.com/example/tool"); err != nil {
		t.Errorf("Remove should always return nil; got %v", err)
	}
}

// U5 — Probe classifies StateOK when the runner reports the binary
// installed, StateFailed otherwise.
func TestU5_Probe_OKAndFailedClassification(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("github.com/example/installed-tool")
	p := New(fake)

	sf := state.New("goinstall", "test-machine")
	sf.SetResource("github.com/example/installed-tool", state.StateOK)
	sf.SetResource("github.com/example/missing-tool", state.StateOK)

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
	if byID["github.com/example/installed-tool"].State != state.StateOK {
		t.Errorf("installed-tool state = %v, want StateOK", byID["github.com/example/installed-tool"].State)
	}
	if byID["github.com/example/missing-tool"].State != state.StateFailed {
		t.Errorf("missing-tool state = %v, want StateFailed", byID["github.com/example/missing-tool"].State)
	}
}

// U6 — Probe skips StateRemoved entries.
func TestU6_Probe_SkipsRemovedResources(t *testing.T) {
	t.Parallel()
	p := New(NewFakeCmdRunner())
	sf := state.New("goinstall", "test-machine")
	sf.SetResource("github.com/example/removed", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Probe returned %d results for StateRemoved, want 0", len(results))
	}
}

// U7 — Probe queries the runner once per state-tracked resource (the
// runner-internal multi-step probe is collapsed behind the seam).
func TestU7_Probe_OneRunnerCallPerResource(t *testing.T) {
	t.Parallel()
	fake := NewFakeCmdRunner().Seed("github.com/a/b").Seed("github.com/c/d")
	p := New(fake)

	sf := state.New("goinstall", "test-machine")
	sf.SetResource("github.com/a/b", state.StateOK)
	sf.SetResource("github.com/c/d", state.StateOK)
	sf.SetResource("github.com/missing/x", state.StateOK)

	if _, err := p.Probe(context.Background(), sf); err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if got := fake.CallCount(fakeOpProbe, ""); got != 3 {
		t.Errorf("IsBinaryInstalled call count = %d, want 3 (one per non-removed resource)", got)
	}
}

// U8 — Bootstrap delegates to runner.LookPath; both paths verified.
func TestU8_Bootstrap_DelegatesLookPath(t *testing.T) {
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
		want := errors.New("go not on PATH")
		p := New(NewFakeCmdRunner().WithLookPathError(want))
		if err := p.Bootstrap(context.Background()); !errors.Is(err, want) {
			t.Errorf("Bootstrap error = %v, want %v", err, want)
		}
	})
}

// TestBinaryNameFromPkg covers the production helper that turns a Go
// module path into the install-time binary name. Edge cases here would
// silently corrupt the probe path's gopath/bin/<name> lookup.
func TestBinaryNameFromPkg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pkg  string
		want string
	}{
		{"github.com/example/tool", "tool"},
		{"github.com/example/cmd/binary", "binary"},
		{"github.com/example/tool@latest", "tool"},
		{"github.com/example/tool@v1.2.3", "tool"},
		{"single", "single"},
		{"single@latest", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := binaryNameFromPkg(tt.pkg)
		if got != tt.want {
			t.Errorf("binaryNameFromPkg(%q) = %q, want %q", tt.pkg, got, tt.want)
		}
	}
}

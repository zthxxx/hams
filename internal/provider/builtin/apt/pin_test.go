package apt

import (
	"testing"

	"github.com/zthxxx/hams/internal/state"
)

// TestPinStateOpts_VersionPin locks in the "pkg=version" detection
// branch: a token of the form `nginx=1.24.0` produces a
// WithRequestedVersion option that SetResource uses to record the
// pin in state. A regression here would drop the pin from state so
// the next apply couldn't tell "installed from apt stable" vs
// "installed with explicit version pin".
func TestPinStateOpts_VersionPin(t *testing.T) {
	t.Parallel()
	opts := pinStateOpts("nginx", "nginx=1.24.0")
	if len(opts) != 1 {
		t.Fatalf("opts = %d, want 1 (WithRequestedVersion)", len(opts))
	}
	r := &state.Resource{}
	opts[0](r)
	if r.RequestedVersion != "1.24.0" {
		t.Errorf("RequestedVersion = %q, want 1.24.0", r.RequestedVersion)
	}
	if r.RequestedSource != "" {
		t.Errorf("RequestedSource = %q, want empty", r.RequestedSource)
	}
}

// TestPinStateOpts_SourcePin locks in the "pkg/source" detection:
// `nginx/bookworm-backports` produces a WithRequestedSource option.
func TestPinStateOpts_SourcePin(t *testing.T) {
	t.Parallel()
	opts := pinStateOpts("nginx", "nginx/bookworm-backports")
	if len(opts) != 1 {
		t.Fatalf("opts = %d, want 1 (WithRequestedSource)", len(opts))
	}
	r := &state.Resource{}
	opts[0](r)
	if r.RequestedSource != "bookworm-backports" {
		t.Errorf("RequestedSource = %q, want bookworm-backports", r.RequestedSource)
	}
	if r.RequestedVersion != "" {
		t.Errorf("RequestedVersion = %q, want empty", r.RequestedVersion)
	}
}

// TestPinStateOpts_BareReturnsNil locks in the no-pin branch: a
// token that matches neither form returns nil opts so SetResource
// doesn't attempt to stamp a stale pin.
func TestPinStateOpts_BareReturnsNil(t *testing.T) {
	t.Parallel()
	if opts := pinStateOpts("nginx", "nginx"); opts != nil {
		t.Errorf("opts = %v, want nil for bare token", opts)
	}
	// Prefix mismatch also returns nil.
	if opts := pinStateOpts("nginx", "apache=2.4.0"); opts != nil {
		t.Errorf("opts = %v, want nil for prefix mismatch", opts)
	}
}

// TestPinStateOpts_EmptyVersionValue asserts that `nginx=` (pin
// with empty value) STILL returns a WithRequestedVersion opt,
// preserving the user's intent to clear the pin explicitly. This
// matches the CutPrefix semantics and is the only way to represent
// "explicitly unpin" in the state file.
func TestPinStateOpts_EmptyVersionValue(t *testing.T) {
	t.Parallel()
	opts := pinStateOpts("nginx", "nginx=")
	if len(opts) != 1 {
		t.Fatalf("opts = %d, want 1", len(opts))
	}
	r := &state.Resource{}
	opts[0](r)
	if r.RequestedVersion != "" {
		t.Errorf("RequestedVersion = %q, want empty string", r.RequestedVersion)
	}
}

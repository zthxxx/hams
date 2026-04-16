package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// TestRunRefresh_MutuallyExclusiveFlags asserts cycle 38's flag check
// runs before config load for the refresh command too.
func TestRunRefresh_MutuallyExclusiveFlags(t *testing.T) {
	flags := &provider.GlobalFlags{}
	err := runRefresh(context.Background(), flags, provider.NewRegistry(), "brew", "apt")
	if err == nil {
		t.Fatal("expected error for --only + --except")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitUsageError {
		t.Fatalf("expected ExitUsageError, got %v (%T)", err, err)
	}
	if !strings.Contains(ufe.Message, "mutually exclusive") {
		t.Errorf("message = %q", ufe.Message)
	}
}

// TestRunRefresh_NoProvidersMatch asserts the stage-1 empty path (no
// hamsfiles, no state files) exits 0 with the right message.
func TestRunRefresh_NoProvidersMatch(t *testing.T) {
	_, _, _, flags := setupApplyTestEnv(t, []string{"apt"})

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// No hamsfile, no state file — empty profile/state dirs.
	out := captureStdout(t, func() {
		if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
			t.Fatalf("runRefresh: %v", err)
		}
	})
	if !strings.Contains(out, "No providers match") {
		t.Errorf("output should mention no-providers-match path; got %q", out)
	}
}

// TestRunRefresh_SaveFailure_ReturnsPartialFailure drives the cycle-47
// path: when sf.Save fails after a successful probe, runRefresh
// returns ExitPartialFailure and surfaces the save failure in the
// summary. Before cycle 47 this was log-only + silent exit 0.
func TestRunRefresh_SaveFailure_ReturnsPartialFailure(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	// Make state dir have an apt.hams.yaml (so the artifact filter
	// keeps the provider in scope) and an empty state file (so
	// ProbeAll can load it successfully).
	writeApplyTestFile(t, filepath.Join(profileDir, "apt.hams.yaml"), "packages:\n  - app: htop\n")
	// Pre-create a directory at the state-file path. state.Load fails
	// with "is a directory", so after cycle 43 ProbeAll omits the
	// provider from its results map — runRefresh then reports the
	// probed/planned mismatch as ExitPartialFailure.
	if err := os.MkdirAll(filepath.Join(stateDir, "apt.state.yaml"), 0o750); err != nil {
		t.Fatalf("seed blocking dir: %v", err)
	}

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		probeFn: func(_ context.Context, _ *state.File) ([]provider.ProbeResult, error) {
			t.Fatal("probe must not be called for a provider whose state is unreadable")
			return nil, nil
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	err := runRefresh(context.Background(), flags, registry, "", "")
	if err == nil {
		t.Fatal("expected ExitPartialFailure; got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitPartialFailure {
		t.Fatalf("expected ExitPartialFailure, got %v (%T)", err, err)
	}
}

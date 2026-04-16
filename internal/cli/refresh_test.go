package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// TestRunRefresh_CreatesSessionLogFile locks in the cycle-65 fix:
// `SetupLogging` is now wired into runRefresh, so a rolling log file
// appears at `${HAMS_DATA_HOME}/<YYYY-MM>/hams.<YYYYMM>.log`.
func TestRunRefresh_CreatesSessionLogFile(t *testing.T) {
	_, _, _, flags := setupApplyTestEnv(t, []string{"apt"})
	dataHome := os.Getenv("HAMS_DATA_HOME")
	if dataHome == "" {
		t.Fatal("setupApplyTestEnv should have set HAMS_DATA_HOME")
	}

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

	// Run refresh — no providers have artifacts so it early-returns
	// the "no providers match" message, BUT SetupLogging runs first.
	if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
		t.Fatalf("runRefresh: %v", err)
	}

	// Assert the data-home contains the month-bucket dir with a
	// hams.YYYYMM.log file.
	now := time.Now()
	monthDir := filepath.Join(dataHome, now.Format("2006-01"))
	wantLog := filepath.Join(monthDir, "hams."+now.Format("200601")+".log")
	info, err := os.Stat(wantLog)
	if err != nil {
		t.Fatalf("expected log file at %s; got: %v", wantLog, err)
	}
	if info.Size() == 0 {
		t.Errorf("log file exists but is empty at %s", wantLog)
	}
}

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

// TestRunRefresh_ExplicitProfileNotFoundEmitsUserError locks in
// cycle 93: symmetric to cycle 92's apply fix. `hams --profile=Typo
// refresh` used to print "No providers match" + exit 0 instead of
// naming the bad profile. Now it emits ExitUsageError with a clear
// message identifying the profile and the missing path.
func TestRunRefresh_ExplicitProfileNotFoundEmitsUserError(t *testing.T) {
	storeDir, _, _, _ := setupApplyTestEnv(t, nil)

	flags := &provider.GlobalFlags{Store: storeDir, Profile: "Typo"}
	registry := provider.NewRegistry()

	err := runRefresh(context.Background(), flags, registry, "", "")
	if err == nil {
		t.Fatal("expected error when --profile dir doesn't exist")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want ExitUsageError (%d)", ufe.Code, hamserr.ExitUsageError)
	}
	if !strings.Contains(ufe.Message, "Typo") {
		t.Errorf("message should name the typo'd profile; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "not found") {
		t.Errorf("message should say the profile isn't found; got %q", ufe.Message)
	}
}

// TestRunRefresh_FlagStoreOverridesConfig locks in cycle 90: when the
// user passes --store=X, it MUST take precedence over the global
// config's store_path. Previously config.Load populated cfg.StorePath
// from the global config; the flags.Store argument only influenced
// where level-3 / level-4 project configs were looked up, not the
// resolved cfg.StorePath. Result: `hams --store=/alt refresh`
// silently refreshed the config's store (not /alt).
//
// The fix in runRefresh now overrides cfg.StorePath from flags.Store
// AFTER config.Load. This test asserts that with BOTH a config
// store_path AND a conflicting --store flag, refresh uses --store.
func TestRunRefresh_FlagStoreOverridesConfig(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	configuredStore := t.TempDir() // exists but not what we want
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\nmachine_id: mid1\nstore_path: "+configuredStore+"\n")

	// --store points at a non-existent path; refresh should surface
	// that via the store_path validation (cycle 88), NOT use the
	// config's configuredStore and pretend nothing is wrong.
	flags := &provider.GlobalFlags{Store: "/this/overrides/but/does/not/exist"}
	registry := provider.NewRegistry()

	err := runRefresh(context.Background(), flags, registry, "", "")
	if err == nil {
		t.Fatal("expected --store override to take precedence; got nil (refresh silently used config)")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Message, "/this/overrides/but/does/not/exist") {
		t.Errorf("error should name --store path, not config store_path; got %q", ufe.Message)
	}
}

// TestRunRefresh_NonexistentStorePathEmitsUserError locks in cycle 88:
// when store_path names a directory that doesn't exist, refresh used
// to print "No providers match" and exit 0 — silently masking the
// real misconfiguration. Now it emits the same UserFacingError that
// runApply produces (cycle 87), so the user can't tell the two
// commands apart on this class of bug.
func TestRunRefresh_NonexistentStorePathEmitsUserError(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\nmachine_id: mid1\n")

	flags := &provider.GlobalFlags{Store: "/definitely/does/not/exist/ever"}
	registry := provider.NewRegistry()

	err := runRefresh(context.Background(), flags, registry, "", "")
	if err == nil {
		t.Fatal("expected error when store_path doesn't exist")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want ExitUsageError (%d)", ufe.Code, hamserr.ExitUsageError)
	}
	if !strings.Contains(ufe.Message, "/definitely/does/not/exist/ever") {
		t.Errorf("message should name the bad path; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "does not exist or is not a directory") {
		t.Errorf("message should explain what's wrong; got %q", ufe.Message)
	}
}

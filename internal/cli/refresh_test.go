package cli

import (
	"context"
	"encoding/json"
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

// TestRunRefresh_SingularProviderNoun locks in cycle 125: a single-
// provider refresh output says "1 provider probed" (singular), not
// "1 providers probed". The pluralize helper is consistent across
// the refresh summary's three message variants.
func TestRunRefresh_SingularProviderNoun(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	// Seed an apt hamsfile so the artifact filter keeps the provider
	// in scope, and a valid empty state file so ProbeAll can load it.
	writeApplyTestFile(t, filepath.Join(profileDir, "apt.hams.yaml"), "cli:\n  - app: htop\n")
	writeApplyTestFile(t, filepath.Join(stateDir, "apt.state.yaml"),
		"provider: apt\nmachine_id: test-machine\nresources: {}\n")

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

	out := captureStdout(t, func() {
		if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
			t.Fatalf("runRefresh: %v", err)
		}
	})
	if !strings.Contains(out, "1 provider probed") {
		t.Errorf("expected singular 'provider'; got %q", out)
	}
	if strings.Contains(out, "1 providers probed") {
		t.Errorf("should NOT use plural 'providers' for count=1; got %q", out)
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

// TestRunRefresh_JSONOutput locks in cycle 182: `hams --json
// refresh` previously printed the same prose summary as the
// non-JSON path, ignoring the global --json flag. CI scripts that
// run `hams refresh` in a loop need a parseable shape to detect
// partial failures programmatically.
func TestRunRefresh_JSONOutput(t *testing.T) {
	storeDir, profileDir, _, flags := setupApplyTestEnv(t, []string{"alpha"})
	flags.JSON = true

	writeApplyTestFile(t, filepath.Join(profileDir, "alpha.hams.yaml"),
		"packages:\n  - app: pkg-a\n")

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "alpha", DisplayName: "alpha", FilePrefix: "alpha",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		probeFn: func(_ context.Context, _ *state.File) ([]provider.ProbeResult, error) {
			return []provider.ProbeResult{{ID: "pkg-a", State: state.StateOK}}, nil
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
			t.Fatalf("refresh: %v", err)
		}
	})

	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\nraw: %q", err, out)
	}

	for _, key := range []string{"probed", "planned", "save_failures", "probe_failures", "success"} {
		if _, ok := data[key]; !ok {
			t.Errorf("JSON missing required key %q; got: %v", key, data)
		}
	}
	if data["success"] != true {
		t.Errorf("success = %v, want true", data["success"])
	}
	// save_failures should be an empty array (NOT null) so consumers
	// can iterate without nil-checking.
	if sf, ok := data["save_failures"].([]any); !ok || len(sf) != 0 {
		t.Errorf("save_failures = %v, want []", data["save_failures"])
	}

	_ = storeDir
}

// TestRunRefresh_SaveFailureListIsAlphabetical locks in cycle 151:
// when multiple providers fail to save their probed state, the
// printed warning ("N state save failure(s): X, Y, Z") MUST list
// the names alphabetically. Previously runRefresh iterated the
// probeResults map (Go map iteration is non-deterministic), so
// the warning showed providers in shuffled order on every run —
// broke log-grep / diff tooling that compared two refresh runs.
// Symmetric with cycles 148/149/150.
func TestRunRefresh_SaveFailureListIsAlphabetical(t *testing.T) {
	storeDir, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"zeta", "alpha", "mu"})

	// Seed each provider's hamsfile so the artifact filter keeps them
	// in scope. Probe will succeed (they each return one StateOK),
	// then sf.Save will fail because we make the state dir read-only.
	for _, name := range []string{"zeta", "alpha", "mu"} {
		writeApplyTestFile(t, filepath.Join(profileDir, name+".hams.yaml"),
			"packages:\n  - app: pkg-a\n")
	}

	registry := provider.NewRegistry()
	for _, name := range []string{"zeta", "alpha", "mu"} {
		nameCopy := name
		p := &applyTestProvider{
			manifest: provider.Manifest{
				Name: nameCopy, DisplayName: nameCopy, FilePrefix: nameCopy,
				Platforms: []provider.Platform{provider.PlatformAll},
			},
			probeFn: func(_ context.Context, _ *state.File) ([]provider.ProbeResult, error) {
				return []provider.ProbeResult{{ID: "pkg-a", State: state.StateOK}}, nil
			},
		}
		if err := registry.Register(p); err != nil {
			t.Fatalf("Register %s: %v", nameCopy, err)
		}
	}

	// State dir needs to exist for the chmod to take effect — and
	// must be read-only so AtomicWrite's CreateTemp + Rename both fail
	// with EACCES → sf.Save returns an error → the provider lands in
	// the saveFailures slice.
	//
	// Cycle 221 added single-writer lock acquisition to runRefresh,
	// which would also fail on a read-only stateDir and short-circuit
	// the test before it reaches the save-failure branch under test.
	// Stub the package-level acquireMutationLock seam to a no-op so
	// the read-only chmod still selectively breaks Save while the
	// "lock" succeeds. The lock semantics themselves are covered by
	// TestAcquireMutationLock_* in internal/provider.
	originalLock := acquireMutationLock
	t.Cleanup(func() { acquireMutationLock = originalLock })
	acquireMutationLock = func(_, _ string) (func(), error) {
		return func() {}, nil
	}

	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir stateDir: %v", err)
	}
	if err := os.Chmod(stateDir, 0o500); err != nil {
		t.Fatalf("chmod stateDir read-only: %v", err)
	}
	t.Cleanup(func() {
		// Restore writable bit so t.TempDir cleanup can remove the dir.
		if err := os.Chmod(stateDir, 0o700); err != nil {
			t.Logf("restore stateDir perms: %v", err)
		}
	})

	out := captureStdout(t, func() {
		err := runRefresh(context.Background(), flags, registry, "", "")
		if err == nil {
			t.Fatal("expected ExitPartialFailure (3 save failures)")
		}
	})

	// Assert the warning lists the 3 providers in alphabetical order.
	// alpha, mu, zeta (NOT registration / map order).
	wantOrder := []string{"alpha", "mu", "zeta"}
	last := -1
	for _, name := range wantOrder {
		idx := strings.Index(out, name)
		if idx < 0 {
			t.Errorf("save-failure warning missing %q; got:\n%s", name, out)
			continue
		}
		if idx <= last {
			t.Errorf("save failure list not alphabetical: %q at idx %d should come after previous (idx %d); got:\n%s",
				name, idx, last, out)
		}
		last = idx
	}

	// Run 10 more times; assert byte-for-byte stability of the
	// warning output across runs.
	for range 10 {
		got := captureStdout(t, func() {
			if err := runRefresh(context.Background(), flags, registry, "", ""); err == nil {
				t.Fatal("expected ExitPartialFailure on every run")
			}
		})
		// Compare the lines that mention the failing providers.
		extractFailureLine := func(s string) string {
			for line := range strings.SplitSeq(s, "\n") {
				if strings.Contains(line, "alpha") && strings.Contains(line, "mu") && strings.Contains(line, "zeta") {
					return line
				}
			}
			return ""
		}
		if extractFailureLine(got) != extractFailureLine(out) {
			t.Errorf("save-failure line differs across runs:\nfirst:\n%s\nlater:\n%s",
				extractFailureLine(out), extractFailureLine(got))
			break
		}
	}

	// Silence unused-var warnings on storeDir.
	_ = storeDir
}

// TestRunRefresh_AcquiresMutationLock — cycle 221 guard. Per the
// cli-architecture spec §"Lock file for single-writer enforcement",
// refresh MUST acquire the single-writer lock for the duration of
// its mutation. Pre-cycle-221 only runApply did this. We use the
// acquireMutationLock seam to record that exactly one acquire +
// matching release happened, with the canonical "hams refresh"
// command label going into the lock file.
func TestRunRefresh_AcquiresMutationLock(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	writeApplyTestFile(t, filepath.Join(profileDir, "apt.hams.yaml"), "cli:\n  - app: htop\n")

	var acquireCount, releaseCount int
	var gotCmd string
	originalLock := acquireMutationLock
	t.Cleanup(func() { acquireMutationLock = originalLock })
	acquireMutationLock = func(_, cmd string) (func(), error) {
		acquireCount++
		gotCmd = cmd
		return func() { releaseCount++ }, nil
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

	if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
		t.Fatalf("runRefresh: %v", err)
	}
	if acquireCount != 1 {
		t.Errorf("acquireCount = %d, want 1", acquireCount)
	}
	if releaseCount != 1 {
		t.Errorf("releaseCount = %d, want 1 (deferred release must fire)", releaseCount)
	}
	if gotCmd != "hams refresh" {
		t.Errorf("lock cmd label = %q, want %q", gotCmd, "hams refresh")
	}
}

// TestRunRefresh_DryRunSkipsStateWrites — cycle 226 guard. `hams
// refresh --dry-run` was unconditionally calling sf.Save on every
// probed provider before this fix, mutating state files and bumping
// timestamps despite the user's explicit --dry-run. Asserts:
//
//  1. probeFn IS called (refresh still surfaces the would-be plan).
//  2. NO state file ends up on disk afterward.
//  3. Stdout contains the "[dry-run] Would write state" preview.
func TestRunRefresh_DryRunSkipsStateWrites(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"alpha"})
	flags.DryRun = true
	writeApplyTestFile(t, filepath.Join(profileDir, "alpha.hams.yaml"), "cli:\n  - app: pkg-a\n")

	registry := provider.NewRegistry()
	probeCalls := 0
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "alpha", DisplayName: "alpha", FilePrefix: "alpha",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		probeFn: func(_ context.Context, _ *state.File) ([]provider.ProbeResult, error) {
			probeCalls++
			return []provider.ProbeResult{{ID: "pkg-a", State: state.StateOK}}, nil
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
			t.Fatalf("runRefresh: %v", err)
		}
	})

	if probeCalls == 0 {
		t.Error("dry-run refresh should still call Probe (preview the plan)")
	}
	statePath := filepath.Join(stateDir, "alpha.state.yaml")
	if _, err := os.Stat(statePath); err == nil {
		t.Errorf("dry-run refresh wrote state file %q; must be a pure preview", statePath)
	}
	if !strings.Contains(out, "[dry-run] Would write state") {
		t.Errorf("dry-run output missing 'Would write state' preview; got %q", out)
	}
}

// TestRunRefresh_DryRunSkipsLock — cycle 221 invariant: dry-run is
// a pure preview per the global flag's "no side effects" contract,
// so refresh MUST NOT acquire the lock under --dry-run (acquiring
// would itself write the .lock file).
func TestRunRefresh_DryRunSkipsLock(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	flags.DryRun = true
	writeApplyTestFile(t, filepath.Join(profileDir, "apt.hams.yaml"), "cli:\n  - app: htop\n")

	var acquireCount int
	originalLock := acquireMutationLock
	t.Cleanup(func() { acquireMutationLock = originalLock })
	acquireMutationLock = func(_, _ string) (func(), error) {
		acquireCount++
		return func() {}, nil
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

	if err := runRefresh(context.Background(), flags, registry, "", ""); err != nil {
		t.Fatalf("runRefresh dry-run: %v", err)
	}
	if acquireCount != 0 {
		t.Errorf("dry-run must NOT acquire the lock; acquireCount = %d", acquireCount)
	}
}

// TestRunRefresh_InterruptedContextReportsExplicitly locks in cycle
// 209: when the root context is canceled mid-refresh (user pressed
// Ctrl+C), the summary output MUST say "Refresh interrupted" — not
// the misleading "Refresh complete: 0/N providers probed (N probe
// error(s); see log for details)" which made it look like N
// independent probe failures instead of one user cancellation.
// Matches cycle 84's behavior for runApply.
func TestRunRefresh_InterruptedContextReportsExplicitly(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	writeApplyTestFile(t, filepath.Join(profileDir, "apt.hams.yaml"), "cli:\n  - app: htop\n")

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		probeFn: func(ctx context.Context, _ *state.File) ([]provider.ProbeResult, error) {
			return nil, ctx.Err()
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := captureStdout(t, func() {
		err := runRefresh(ctx, flags, registry, "", "")
		if err == nil {
			t.Fatal("expected ExitPartialFailure for interrupted ctx")
		}
		var ufe *hamserr.UserFacingError
		if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitPartialFailure {
			t.Fatalf("expected ExitPartialFailure, got %v (%T)", err, err)
		}
		if !strings.Contains(ufe.Message, "interrupted") {
			t.Errorf("error message should mention 'interrupted'; got %q", ufe.Message)
		}
	})
	if !strings.Contains(out, "Refresh interrupted") {
		t.Errorf("expected 'Refresh interrupted' in stdout; got %q", out)
	}
	if strings.Contains(out, "Refresh complete") {
		t.Errorf("stdout should NOT say 'Refresh complete' on ctx cancellation; got %q", out)
	}
}

// TestRunRefresh_InterruptedContextEmitsJSONFlag asserts the JSON
// variant of cycle 209. `hams --json refresh` with a canceled ctx
// produces {"interrupted": true, "success": false} instead of the
// probe_failures/save_failures shape, so CI scripts can branch on
// the interrupted flag rather than parsing text.
func TestRunRefresh_InterruptedContextEmitsJSONFlag(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	flags.JSON = true
	writeApplyTestFile(t, filepath.Join(profileDir, "apt.hams.yaml"), "cli:\n  - app: htop\n")

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		probeFn: func(ctx context.Context, _ *state.File) ([]provider.ProbeResult, error) {
			return nil, ctx.Err()
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := captureStdout(t, func() {
		if err := runRefresh(ctx, flags, registry, "", ""); err == nil {
			t.Fatal("expected ExitPartialFailure for interrupted ctx")
		}
	})
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\nraw: %q", err, out)
	}
	if got, ok := data["interrupted"].(bool); !ok || !got {
		t.Errorf("JSON['interrupted'] = %v (ok=%v), want true; got: %v", data["interrupted"], ok, data)
	}
	if got, ok := data["success"].(bool); ok && got {
		t.Errorf("JSON['success'] = %v, want false or missing; got: %v", data["success"], data)
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

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/sudo"
)

type applyTestProvider struct {
	manifest    provider.Manifest
	bootstrapFn func(context.Context) error
	probeFn     func(context.Context, *state.File) ([]provider.ProbeResult, error)
	planFn      func(context.Context, *hamsfile.File, *state.File) ([]provider.Action, error)
	applyFn     func(context.Context, provider.Action) error
	removeFn    func(context.Context, string) error
}

func (p *applyTestProvider) Manifest() provider.Manifest { return p.manifest }

func (p *applyTestProvider) Bootstrap(ctx context.Context) error {
	if p.bootstrapFn != nil {
		return p.bootstrapFn(ctx)
	}
	return nil
}

func (p *applyTestProvider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	if p.probeFn != nil {
		return p.probeFn(ctx, sf)
	}
	return nil, nil
}

func (p *applyTestProvider) Plan(ctx context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	if p.planFn != nil {
		return p.planFn(ctx, desired, observed)
	}
	return nil, nil
}

func (p *applyTestProvider) Apply(ctx context.Context, action provider.Action) error {
	if p.applyFn != nil {
		return p.applyFn(ctx, action)
	}
	return nil
}

func (p *applyTestProvider) Remove(ctx context.Context, resourceID string) error {
	if p.removeFn != nil {
		return p.removeFn(ctx, resourceID)
	}
	return nil
}

func (p *applyTestProvider) List(_ context.Context, _ *hamsfile.File, _ *state.File) (string, error) {
	return "", nil
}

func TestRunApply_UsesFilePrefixStatePathAndProviderPlan(t *testing.T) {
	storeDir, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"brew"})
	hamsfilePath := filepath.Join(profileDir, "Homebrew.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "packages:\n  - app: git\n")

	var planCalls int
	var applied []provider.Action

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "brew",
			DisplayName: "Homebrew",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "Homebrew",
		},
		planFn: func(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
			planCalls++
			if desired.Path != hamsfilePath {
				t.Fatalf("Plan desired.Path = %q, want %q", desired.Path, hamsfilePath)
			}
			if observed.Provider != "brew" {
				t.Fatalf("observed.Provider = %q, want brew", observed.Provider)
			}
			return []provider.Action{{
				ID:       "git",
				Type:     provider.ActionInstall,
				Resource: "brew install git --formula",
			}}, nil
		},
		applyFn: func(_ context.Context, action provider.Action) error {
			applied = append(applied, action)
			if action.Resource != "brew install git --formula" {
				return fmt.Errorf("missing provider plan resource payload: %#v", action.Resource)
			}
			return nil
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register provider: %v", err)
	}

	spy := &sudo.SpyAcquirer{}
	if err := runApply(context.Background(), flags, registry, spy, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply error: %v", err)
	}

	if spy.AcquireCalls != 1 {
		t.Fatalf("Acquire calls = %d, want 1", spy.AcquireCalls)
	}
	if spy.StopCalls != 1 {
		t.Fatalf("Stop calls = %d, want 1 (via defer)", spy.StopCalls)
	}

	if planCalls != 1 {
		t.Fatalf("Plan calls = %d, want 1", planCalls)
	}
	if len(applied) != 1 || applied[0].ID != "git" {
		t.Fatalf("applied = %#v, want one git action", applied)
	}

	statePath := filepath.Join(stateDir, "Homebrew.state.yaml")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file %q not found: %v", statePath, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "brew.state.yaml")); err == nil {
		t.Fatalf("unexpected name-based state file was written")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat unexpected name-based state file: %v", err)
	}

	sf, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	wantHash := expectedConfigHashFromFile(hamsfilePath)
	if sf.ConfigHash != wantHash {
		t.Fatalf("ConfigHash = %q, want %q", sf.ConfigHash, wantHash)
	}
	if got := sf.Resources["git"]; got == nil || got.State != state.StateOK {
		t.Fatalf("resource git state = %#v, want ok", got)
	}

	if gotStore := flags.Store; gotStore != storeDir {
		t.Fatalf("flags.Store = %q, want %q", gotStore, storeDir)
	}
}

func TestRunApply_PersistsConfigHashAndRemovesOnNextRun(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"brew"})
	hamsfilePath := filepath.Join(profileDir, "Homebrew.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "packages:\n  - app: git\n")

	var installed, removed []string

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "brew",
			DisplayName: "Homebrew",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "Homebrew",
		},
		planFn: func(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
			return provider.ComputePlan(desired.ListApps(), observed, observed.ConfigHash), nil
		},
		applyFn: func(_ context.Context, action provider.Action) error {
			installed = append(installed, action.ID)
			return nil
		},
		removeFn: func(_ context.Context, resourceID string) error {
			removed = append(removed, resourceID)
			return nil
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register provider: %v", err)
	}

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("first runApply error: %v", err)
	}
	writeApplyTestFile(t, hamsfilePath, "packages: []\n")
	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("second runApply error: %v", err)
	}

	if !slices.Equal(installed, []string{"git"}) {
		t.Fatalf("installed = %v, want [git]", installed)
	}
	if !slices.Equal(removed, []string{"git"}) {
		t.Fatalf("removed = %v, want [git]", removed)
	}

	sf, err := state.Load(filepath.Join(stateDir, "Homebrew.state.yaml"))
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	wantEmptyHash := expectedConfigHashFromFile(hamsfilePath)
	if sf.ConfigHash != wantEmptyHash {
		t.Fatalf("ConfigHash = %q, want empty-config hash %q", sf.ConfigHash, wantEmptyHash)
	}
	if got := sf.Resources["git"]; got == nil || got.State != state.StateRemoved {
		t.Fatalf("resource git state = %#v, want removed", got)
	}
}

func TestRunApply_BootstrapsProvidersInDAGOrderBeforePlanning(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"bash", "brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")
	// bash must have its own hamsfile for the stage-1 artifact-presence
	// filter to include it; otherwise brew's DAG dependency points at a
	// provider that has nothing to do and is correctly pruned.
	writeApplyTestFile(t, filepath.Join(profileDir, "bash.hams.yaml"), "packages: []\n")

	var sequence []string

	base := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "bash",
			DisplayName: "bash",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "bash",
		},
		bootstrapFn: func(context.Context) error {
			sequence = append(sequence, "bootstrap:bash")
			return nil
		},
	}

	main := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "brew",
			DisplayName: "Homebrew",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "Homebrew",
			DependsOn: []provider.DependOn{
				{Provider: "bash"},
			},
		},
		bootstrapFn: func(context.Context) error {
			sequence = append(sequence, "bootstrap:brew")
			return nil
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			sequence = append(sequence, "plan:brew")
			return nil, nil
		},
	}

	registry := provider.NewRegistry()
	for _, p := range []provider.Provider{main, base} {
		if err := registry.Register(p); err != nil {
			t.Fatalf("Register provider: %v", err)
		}
	}

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply error: %v", err)
	}

	want := []string{"bootstrap:bash", "bootstrap:brew", "plan:brew"}
	if !slices.Equal(sequence, want) {
		t.Fatalf("sequence = %v, want %v", sequence, want)
	}
}

func setupApplyTestEnv(t *testing.T, providerPriority []string) (storeDir, profileDir, stateDir string, flags *provider.GlobalFlags) {
	t.Helper()

	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	storeDir = filepath.Join(root, "store")
	profileDir = filepath.Join(storeDir, "macOS")
	stateDir = filepath.Join(storeDir, ".state", "test-machine")

	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("MkdirAll profile: %v", err)
	}

	// Machine-scoped fields (profile_tag, machine_id) live in the global config.
	globalBody := strings.Join([]string{
		"profile_tag: macOS",
		"machine_id: test-machine",
		"",
	}, "\n")
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"), globalBody)

	// Store-level config only carries store-scoped fields.
	storeBody := strings.Join([]string{
		formatProviderPriority(providerPriority),
		"",
	}, "\n")
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"), storeBody)

	return storeDir, profileDir, stateDir, &provider.GlobalFlags{Store: storeDir}
}

func formatProviderPriority(priority []string) string {
	if len(priority) == 0 {
		return "provider_priority: []"
	}

	var b strings.Builder
	b.WriteString("provider_priority:\n")
	for _, name := range priority {
		b.WriteString("  - ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeApplyTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile %q: %v", path, err)
	}
}

func expectedConfigHashFromFile(path string) string {
	hf, err := hamsfile.Read(path)
	if err != nil {
		panic(fmt.Sprintf("expectedConfigHashFromFile: %v", err))
	}
	data, err := yaml.Marshal(hf.Root)
	if err != nil {
		panic(fmt.Sprintf("expectedConfigHashFromFile marshal: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// --- prune-orphans tests ---

// writePruneOrphanState writes a state file at <stateDir>/<filePrefix>.state.yaml
// declaring a single resource in state=ok. Used to simulate the state-only
// scenario (state present, hamsfile missing) for the four prune-orphans tests.
func writePruneOrphanState(t *testing.T, stateDir, filePrefix, providerName, resourceID string) string {
	t.Helper()
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	statePath := filepath.Join(stateDir, filePrefix+".state.yaml")
	body := strings.Join([]string{
		"schema_version: 2",
		"provider: " + providerName,
		"machine_id: test-machine",
		"resources:",
		"  " + resourceID + ":",
		"    state: ok",
		"    first_install_at: \"20260101T000000\"",
		"    updated_at: \"20260101T000000\"",
		"",
	}, "\n")
	writeApplyTestFile(t, statePath, body)
	return statePath
}

// pruneOrphanProvider returns a minimal apt-shaped provider that records every
// Remove call. Plan computes one remove-action per state resource that is NOT
// in the desired hamsfile (mirrors provider.ComputePlan semantics for package
// providers, but kept inline so the test has zero dependency surface).
func pruneOrphanProvider(removed *[]string) *applyTestProvider {
	return &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "apt",
			DisplayName: "apt",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "apt",
		},
		planFn: func(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
			declared := map[string]bool{}
			for _, app := range desired.ListApps() {
				declared[app] = true
			}
			var actions []provider.Action
			for id, r := range observed.Resources {
				if r.State == state.StateRemoved {
					continue
				}
				if !declared[id] {
					actions = append(actions, provider.Action{
						ID:   id,
						Type: provider.ActionRemove,
					})
				}
			}
			return actions, nil
		},
		removeFn: func(_ context.Context, id string) error {
			*removed = append(*removed, id)
			return nil
		},
	}
}

// 3.1 With --prune-orphans, a provider that has only a state file (no
// hamsfile) reconciles against an empty desired-state — every tracked
// resource is removed, and state.<resource>.state transitions to "removed".
func TestApply_PruneOrphans_RemovesOrphanedStateResources(t *testing.T) {
	_, _, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	statePath := writePruneOrphanState(t, stateDir, "apt", "apt", "htop")

	var removed []string
	registry := provider.NewRegistry()
	if err := registry.Register(pruneOrphanProvider(&removed)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", true, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if !slices.Contains(removed, "htop") {
		t.Errorf("removed = %v, want to contain htop", removed)
	}

	sf, loadErr := state.Load(statePath)
	if loadErr != nil {
		t.Fatalf("state.Load: %v", loadErr)
	}
	r, ok := sf.Resources["htop"]
	if !ok {
		t.Fatal("htop missing from state after prune")
	}
	if r.State != state.StateRemoved {
		t.Errorf("htop.State = %q, want %q", r.State, state.StateRemoved)
	}
	if r.RemovedAt == "" {
		t.Error("htop.RemovedAt is empty after prune")
	}
}

// 3.2 Default behavior (no flag): state-only providers are skipped, state
// remains untouched, no Remove call.
func TestApply_NoPruneOrphans_PreservesOrphanedStateResources(t *testing.T) {
	_, _, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	statePath := writePruneOrphanState(t, stateDir, "apt", "apt", "htop")
	originalState, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read original state: %v", readErr)
	}

	var removed []string
	registry := provider.NewRegistry()
	if err := registry.Register(pruneOrphanProvider(&removed)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty (state-only skip is the default)", removed)
	}
	currentState, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read current state: %v", readErr)
	}
	if string(currentState) != string(originalState) {
		t.Error("state file was modified despite state-only skip default")
	}
}

// 3.3 Defensive: if neither hamsfile nor state file exists, --prune-orphans
// must be a no-op (stage-1 should already have filtered the provider out,
// but apply must not panic if a registered provider somehow reaches this
// point in pruneOrphans=true mode).
func TestApply_PruneOrphans_NoStateFile_IsNoOp(t *testing.T) {
	_, _, _, flags := setupApplyTestEnv(t, []string{"apt"})

	var removed []string
	registry := provider.NewRegistry()
	if err := registry.Register(pruneOrphanProvider(&removed)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// stage-1 filter excludes the provider entirely (no artifacts at all),
	// so runApply prints "no providers match" and returns nil. Either way
	// no Remove must occur.
	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", true, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty (no state, nothing to prune)", removed)
	}
}

// 3.4 --prune-orphans does NOT affect a provider whose hamsfile is still
// present. The flag's semantics are scoped to state-only providers — if the
// hamsfile declares htop and state has htop=ok, htop stays installed.
func TestApply_PruneOrphans_HamsfilePresent_DoesNotPrune(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	hamsfilePath := filepath.Join(profileDir, "apt.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "packages:\n  - app: htop\n")
	statePath := writePruneOrphanState(t, stateDir, "apt", "apt", "htop")

	var removed []string
	registry := provider.NewRegistry()
	if err := registry.Register(pruneOrphanProvider(&removed)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", true, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty (htop is still declared in hamsfile)", removed)
	}
	sf, loadErr := state.Load(statePath)
	if loadErr != nil {
		t.Fatalf("state.Load: %v", loadErr)
	}
	r := sf.Resources["htop"]
	if r.State != state.StateOK {
		t.Errorf("htop.State = %q, want %q (still declared, must not transition)", r.State, state.StateOK)
	}
}

// TestApply_ProviderPanic_FlushesStateBeforeUnwinding asserts cycle-51:
// if a provider's Apply method panics mid-loop (buggy provider, OOM in
// runner, etc.), any in-memory state changes from actions that DID
// complete before the panic are flushed to disk before the process
// unwinds. Without the recover-and-save guard, a panic after installing
// N of M actions would lose the state updates, causing next apply to
// re-attempt already-installed resources.
func TestApply_ProviderPanic_FlushesStateBeforeUnwinding(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	hamsfilePath := filepath.Join(profileDir, "apt.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "packages:\n  - app: first\n  - app: second\n")

	// Provider returns two Install actions; Apply succeeds for "first"
	// (updates in-memory state) and panics on "second".
	applyCount := 0
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{
				{ID: "first", Type: provider.ActionInstall},
				{ID: "second", Type: provider.ActionInstall},
			}, nil
		},
		applyFn: func(_ context.Context, a provider.Action) error {
			applyCount++
			if a.ID == "second" {
				panic("synthesized provider panic")
			}
			return nil
		},
	}
	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The panic must propagate (we re-throw after saving), so catch it
	// in the test and assert both the recovery and state-on-disk.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic to propagate after state flush")
		}

		// State must have been flushed to disk with the successful
		// "first" entry — otherwise the next apply would re-install it.
		statePath := filepath.Join(stateDir, "apt.state.yaml")
		sf, loadErr := state.Load(statePath)
		if loadErr != nil {
			t.Fatalf("state file not written after panic: %v", loadErr)
		}
		if _, ok := sf.Resources["first"]; !ok {
			t.Errorf("state should contain successfully-installed 'first'; got %v", sf.Resources)
		}
		if applyCount != 2 {
			t.Errorf("Apply should have run twice (success + panic); got %d", applyCount)
		}
	}()

	//nolint:errcheck // we expect runApply to panic before returning; the deferred recover verifies
	runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	t.Fatal("runApply should have panicked; reached unreachable line")
}

// TestApply_CorruptedStateFile_SkipsProviderNotSilentReset locks in
// the cycle-43 data-integrity fix: when the state file exists but
// is unparseable (corruption, merge conflict, editor crash), apply
// MUST NOT silently replace it with an empty state. Doing so would
// lose drift detection for every tracked resource and potentially
// re-trigger installs. The provider should be skipped and reported
// via ExitPartialFailure, exactly like cycle 39's broken-hamsfile
// path.
func TestApply_CorruptedStateFile_SkipsProviderNotSilentReset(t *testing.T) {
	_, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"apt"})
	hamsfilePath := filepath.Join(profileDir, "apt.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "packages:\n  - app: htop\n")

	// Create a state file with corrupt YAML — parse will fail but the
	// file exists (so we're NOT in the ErrNotExist fallback branch).
	statePath := filepath.Join(stateDir, "apt.state.yaml")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	writeApplyTestFile(t, statePath, "this is : totally : broken : yaml :")

	var applied []provider.Action
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "htop", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, a provider.Action) error {
			applied = append(applied, a)
			return nil
		},
	}
	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected ExitPartialFailure when state is corrupted; got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitPartialFailure {
		t.Fatalf("expected *UserFacingError{ExitPartialFailure}; got %v (type %T)", err, err)
	}

	// The critical assertion: NO apply actions ran. Silent-reset
	// behavior would have produced an action for "htop" against the
	// synthesized empty state.
	if len(applied) != 0 {
		t.Errorf("corrupted state must skip provider, not apply %d actions: %+v", len(applied), applied)
	}

	// And the state file on disk must still contain the corrupt
	// content — hams hasn't overwritten it with an empty state.
	contents, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("state file missing after run (should be preserved): %v", readErr)
	}
	if !strings.Contains(string(contents), "totally : broken") {
		t.Errorf("state file was rewritten; expected corrupt contents preserved, got %q", contents)
	}
}

// TestApply_DryRun_SkippedProvider_ReturnsPartialFailure locks in the
// cycle 39 fix: when dry-run planning encounters a broken hamsfile
// (here, the provider's Plan returns an error), the skipped-providers
// branch must return ExitPartialFailure — NOT silently exit 0.
// CI preview scripts depend on this semantic.
func TestApply_DryRun_SkippedProvider_ReturnsPartialFailure(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	hamsfilePath := filepath.Join(profileDir, "apt.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "packages:\n  - app: htop\n")

	registry := provider.NewRegistry()
	planErr := errors.New("synthesized: plan failed")
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return nil, planErr
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	flags.DryRun = true
	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("dry-run should return an error when a provider is skipped; got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitPartialFailure {
		t.Errorf("Code = %d, want ExitPartialFailure (%d)", ufe.Code, hamserr.ExitPartialFailure)
	}
	if !strings.Contains(ufe.Message, "dry-run") {
		t.Errorf("message should mention dry-run; got %q", ufe.Message)
	}
}

// TestRunApply_NonTTYWithoutProfileEmitsUserError asserts cycle 75:
// when stdin is not a terminal AND profile_tag/machine_id are
// unconfigured, runApply MUST NOT invoke the interactive prompt
// (which would read EOF and surface as
// "profile init: reading profile tag: EOF"). Instead it returns a
// *UserFacingError with ExitUsageError whose message names the
// missing keys and whose suggestions show how to set them.
//
// This is the CI / cloud-init path: users pipe the command from a
// script with /dev/null on stdin. Go's test runner also has a
// non-TTY stdin by default, so this test exercises exactly that
// environment without additional plumbing.
func TestRunApply_NonTTYWithoutProfileEmitsUserError(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	storeDir := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	// Empty global config — no profile_tag, no machine_id.
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"), "")
	// Store exists and points somewhere; keeps runApply past the
	// earlier "no store" check so the profile-missing branch is
	// actually reached.
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"),
		"store_path: "+storeDir+"\n")

	flags := &provider.GlobalFlags{Store: storeDir}
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected error when profile_tag/machine_id missing on non-TTY; got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want ExitUsageError (%d)", ufe.Code, hamserr.ExitUsageError)
	}
	// Message must name the missing keys so users know what to fix.
	if !strings.Contains(ufe.Message, "profile_tag") {
		t.Errorf("message should name profile_tag; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "machine_id") {
		t.Errorf("message should name machine_id; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "stdin is not a terminal") {
		t.Errorf("message should explain non-TTY cause; got %q", ufe.Message)
	}
	// Suggestions must teach the fix.
	joined := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(joined, "config set profile_tag") {
		t.Errorf("suggestions should recommend `hams config set profile_tag`; got %v", ufe.Suggestions)
	}
	if !strings.Contains(joined, "config set machine_id") {
		t.Errorf("suggestions should recommend `hams config set machine_id`; got %v", ufe.Suggestions)
	}
	// And the error MUST NOT contain the confusing EOF surface.
	if strings.Contains(err.Error(), "EOF") {
		t.Errorf("error should not leak `EOF`; got %q", err.Error())
	}
}

// TestRunApply_NonTTYWithProfileFlagButNoMachineID asserts that
// --profile alone doesn't bypass the machine_id requirement — the
// error still surfaces with ExitUsageError and names machine_id.
// Without this, users would set --profile=macOS, still hit the
// prompt on the machine_id field, and see the same cryptic EOF
// error.
func TestRunApply_NonTTYWithProfileFlagButNoMachineID(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	storeDir := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	// profile_tag set, machine_id missing.
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\n")
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"),
		"store_path: "+storeDir+"\n")

	flags := &provider.GlobalFlags{Store: storeDir, Profile: "linux"} // override profile_tag; still no machine_id
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected error when machine_id missing on non-TTY")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Message, "machine_id") {
		t.Errorf("message should name machine_id; got %q", ufe.Message)
	}
	if strings.Contains(ufe.Message, "profile_tag") {
		t.Errorf("message should NOT name profile_tag (user already set it via --profile); got %q", ufe.Message)
	}
}

// TestRunApply_InterruptedContextReturnsPartialFailure locks in cycle 84:
// when the root context is canceled mid-apply (Ctrl+C / SIGTERM from
// root.go's signal.NotifyContext), runApply MUST NOT fall through to
// the "hams apply complete" summary and return nil. Instead it emits
// a UserFacingError with ExitPartialFailure whose message names the
// interruption and whose suggestions point the user at `hams refresh`
// and the re-run path.
//
// Without this, Ctrl+C during a long-running apply produced a silent
// exit 0 + "0 installed" summary — the user's shell couldn't tell
// they had canceled.
func TestRunApply_InterruptedContextReturnsPartialFailure(t *testing.T) {
	storeDir, profileDir, _, flags := setupApplyTestEnv(t, []string{"brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"),
		"packages:\n  - app: git\n")

	// A provider whose Apply would hang indefinitely; but since the
	// context starts already canceled, provider.Execute's ctx.Done
	// check returns immediately with a ctx.Err() in the result Errors.
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "brew",
			DisplayName: "Homebrew",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "Homebrew",
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "git", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error {
			t.Fatalf("Apply should not be called when context is already canceled")
			return nil
		},
	}
	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before running — simulates early Ctrl+C

	err := runApply(ctx, flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("runApply should surface the cancellation; got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitPartialFailure {
		t.Errorf("Code = %d, want ExitPartialFailure (%d)", ufe.Code, hamserr.ExitPartialFailure)
	}
	if !strings.Contains(ufe.Message, "interrupted") {
		t.Errorf("message should say `interrupted`; got %q", ufe.Message)
	}
	// Suggestions must teach the user how to recover.
	joined := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(joined, "hams refresh") {
		t.Errorf("suggestions should mention `hams refresh`; got %v", ufe.Suggestions)
	}
	if !strings.Contains(joined, "Re-run") {
		t.Errorf("suggestions should point to re-run; got %v", ufe.Suggestions)
	}
	_ = storeDir
}

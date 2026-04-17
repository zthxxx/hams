package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
			// Cycle 227: this test asserts the sudo lifecycle (Acquire
			// + Stop) wires correctly. Pre-cycle-227 sudo was acquired
			// unconditionally; cycle 227 gates on Manifest.RequiresSudo.
			// Set it true here so the test continues to exercise the
			// Acquire → Stop path. The dedicated
			// TestRunApply_NoSudoPromptWhenNoProviderRequiresIt asserts
			// the inverse (RequiresSudo=false → no prompt).
			RequiresSudo: true,
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

// TestRunApply_NoSudoPromptWhenNoProviderRequiresIt — cycle 227.
// `hams apply` previously called sudoAcq.Acquire unconditionally,
// prompting for a password even when the active profile only had
// non-sudo providers (cargo / npm / pnpm / uv / brew / git-clone).
// Spec §"Sudo management": "Operations that do not require sudo
// SHALL NOT prompt for credentials." Cycle 227 gates Acquire on
// `Manifest.RequiresSudo` for at least one selected provider.
func TestRunApply_NoSudoPromptWhenNoProviderRequiresIt(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"cargo"})
	hamsfilePath := filepath.Join(profileDir, "cargo.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "cli:\n  - app: ripgrep\n")

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "cargo",
			DisplayName: "cargo",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "cargo",
			// RequiresSudo intentionally false — this is the no-prompt path.
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{
				ID: "ripgrep", Type: provider.ActionInstall, Resource: "ripgrep",
			}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error { return nil },
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	spy := &sudo.SpyAcquirer{}
	if err := runApply(context.Background(), flags, registry, spy, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if spy.AcquireCalls != 0 {
		t.Errorf("Acquire calls = %d, want 0 (no provider has RequiresSudo)", spy.AcquireCalls)
	}
	// Stop is wired via defer at runApply entry, so it always fires.
	if spy.StopCalls != 1 {
		t.Errorf("Stop calls = %d, want 1 (defer always fires regardless of Acquire)", spy.StopCalls)
	}
}

// TestRunApply_SudoPromptWhenAptIncluded — cycle 227 inverse guard.
// When the resolved provider set includes a RequiresSudo=true entry
// (apt is the canonical v1 case), runApply MUST prompt exactly once
// at startup — matching the spec's "Sudo acquired once for full
// apply" scenario. The seed manifest below mirrors apt's by setting
// RequiresSudo=true.
func TestRunApply_SudoPromptWhenAptIncluded(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	hamsfilePath := filepath.Join(profileDir, "apt.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "cli:\n  - app: htop\n")

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:         "apt",
			DisplayName:  "apt",
			Platforms:    []provider.Platform{provider.PlatformAll},
			FilePrefix:   "apt",
			RequiresSudo: true,
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{
				ID: "htop", Type: provider.ActionInstall, Resource: "htop",
			}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error { return nil },
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	spy := &sudo.SpyAcquirer{}
	if err := runApply(context.Background(), flags, registry, spy, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if spy.AcquireCalls != 1 {
		t.Errorf("Acquire calls = %d, want 1 (apt has RequiresSudo=true)", spy.AcquireCalls)
	}
	if spy.StopCalls != 1 {
		t.Errorf("Stop calls = %d, want 1", spy.StopCalls)
	}
}

// failingAcquirer is a sudo.Acquirer that always returns an error
// from Acquire — used to exercise the cycle-228 ExitSudoError path.
type failingAcquirer struct {
	stopped bool
}

func (f *failingAcquirer) Acquire(_ context.Context) error {
	return fmt.Errorf("user canceled sudo prompt")
}

func (f *failingAcquirer) Stop() { f.stopped = true }

// TestRunApply_SudoFailureExitsWithSudoError — cycle 228. Per
// cli-architecture/spec.md §"Sudo not granted": "WHEN the user
// cancels the sudo prompt or sudo times out during startup THEN the
// process SHALL exit with code 10". Pre-cycle-228 a failed Acquire
// was downgraded to a slog.Warn and apply continued — apt's later
// runner.Install would fail with a generic provider error, the user
// got the wrong exit code, and CI scripts couldn't tell apart "user
// canceled" from "apt-get errored". Now: hard-exit with
// ExitSudoError + recovery hints.
func TestRunApply_SudoFailureExitsWithSudoError(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"apt"})
	hamsfilePath := filepath.Join(profileDir, "apt.hams.yaml")
	writeApplyTestFile(t, hamsfilePath, "cli:\n  - app: htop\n")

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:         "apt",
			DisplayName:  "apt",
			Platforms:    []provider.Platform{provider.PlatformAll},
			FilePrefix:   "apt",
			RequiresSudo: true,
		},
		// planFn shouldn't be reached — the sudo failure short-circuits.
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			t.Fatal("Plan should not be called after sudo acquisition fails")
			return nil, nil
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	failer := &failingAcquirer{}
	err := runApply(context.Background(), flags, registry, failer, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected ExitSudoError, got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitSudoError {
		t.Fatalf("expected ExitSudoError (code %d), got %v (%T)", hamserr.ExitSudoError, err, err)
	}
	if !strings.Contains(ufe.Message, "sudo acquisition failed") {
		t.Errorf("error message should mention sudo failure; got %q", ufe.Message)
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
	// Create the `linux` profile dir so cycle-92's explicit-profile
	// validation passes — this test only cares about the machine_id
	// branch, not profile dir existence.
	if err := os.MkdirAll(filepath.Join(storeDir, "linux"), 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}

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

// TestRunApply_PreApplyStateSaveFailureListIsAlphabetical locks in
// cycle 153: when pre-apply refresh fails to save multiple
// providers' probed state, the resulting `stateSaveFailures` slice
// must be populated alphabetically. Previously runApply iterated
// the probeResults map directly (Go map iteration is non-
// deterministic), so each apply shuffled the order of the per-
// provider slog.Error lines AND the eventual final summary's
// "Warning: N provider(s) failed to persist state" list. Apply-side
// parallel of cycle 151's runRefresh fix; symmetric with cycles
// 148-152.
func TestRunApply_PreApplyStateSaveFailureListIsAlphabetical(t *testing.T) {
	storeDir, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"zeta", "alpha", "mu"})
	// NOT dry-run — cycle 241 made dry-run skip the pre-apply refresh
	// save loop entirely (per the global --dry-run "no side effects"
	// contract). With Save now skipped under dry-run, the failure list
	// the test exercises is unreachable that way. Use a non-dry-run
	// apply with planFn returning no actions, so the pre-apply refresh
	// runs (and its save fails on the read-only stateDir) but the
	// per-provider Execute / post-apply save loop has nothing to do.

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
			planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
				// Empty plan — Execute is a no-op. The post-apply
				// save loop sees no changes, so the only stateSaveFailures
				// entries come from the pre-apply refresh.
				return nil, nil
			},
		}
		if err := registry.Register(p); err != nil {
			t.Fatalf("Register %s: %v", nameCopy, err)
		}
	}

	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir stateDir: %v", err)
	}
	if err := os.Chmod(stateDir, 0o500); err != nil {
		t.Fatalf("chmod stateDir read-only: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(stateDir, 0o700); err != nil {
			t.Logf("restore stateDir perms: %v", err)
		}
	})

	// Run apply. Pre-apply refresh saves all 3 providers in the
	// (now-sorted) order alpha → mu → zeta. Each Save fails because
	// the state dir is read-only. Without dry-run, the surrounding
	// runApply may or may not return ExitPartialFailure depending on
	// whether post-apply save also runs into the readonly dir; we
	// only care here about the per-provider error ordering in stderr,
	// captured via the `collect` helper below.
	captureStderr(t, func() {
		// Capture stderr to consume slog output without polluting test
		// output. We don't assert on the returned error here — the
		// test's focus is the per-provider stderr ordering.
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", false, "", "", false, bootstrapMode{}); err != nil {
			// Expected: pre-apply refresh hits read-only stateDir,
			// returns ExitPartialFailure. The test's focus is
			// stderr ordering, not the returned error.
			t.Logf("runApply returned (expected): %v", err)
		}
	})

	// As of cycle 154 the dry-run early exit DOES surface
	// stateSaveFailures with ExitPartialFailure (instead of silently
	// printing "No changes made"). The separate dedicated test for
	// that error shape is TestRunApply_DryRunStateSaveFailureSurfacesAsError.
	// This test focuses on ordering: run apply N times, capture stderr
	// (which carries the per-provider slog.Error lines from the pre-
	// apply refresh save loop), and confirm the order of provider
	// names is stable + alphabetical across runs.
	//
	// collect returns the order in which the 3 provider names APPEAR in
	// the stderr stream. Stable + alphabetical = [alpha, mu, zeta].
	collect := func() []string {
		out := captureStderr(t, func() {
			if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", false, "", "", false, bootstrapMode{}); err != nil {
				_ = err // err expected; pre-apply save failures don't surface as runApply error in dry-run
			}
		})
		// Find each provider's "failed to save" log line and record its
		// byte offset; sort by offset to yield the order they appeared.
		type hit struct {
			name string
			idx  int
		}
		var hits []hit
		for _, name := range []string{"alpha", "mu", "zeta"} {
			if idx := strings.Index(out, `provider=`+name); idx >= 0 {
				hits = append(hits, hit{name, idx})
			}
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].idx < hits[j].idx })
		got := make([]string, 0, len(hits))
		for _, h := range hits {
			got = append(got, h.name)
		}
		return got
	}

	first := collect()
	if len(first) < 3 {
		t.Skipf("could not capture all 3 provider error markers from stderr; got %v", first)
	}
	for range 10 {
		got := collect()
		if !slices.Equal(got, first) {
			t.Errorf("provider error order differs across runs:\nfirst: %v\nlater: %v", first, got)
			break
		}
	}

	// Assert alphabetical ordering of provider names in stderr.
	want := []string{"alpha", "mu", "zeta"}
	if !slices.Equal(first, want) {
		t.Errorf("provider error order = %v, want %v (alphabetical)", first, want)
	}

	_ = storeDir
}

// TestRunApply_JSONOutput locks in cycle 183: `hams --json apply`
// previously printed the prose summary and ignored --json. CI
// scripts orchestrating multi-machine applies need a parseable
// shape to detect partial failures programmatically.
func TestRunApply_JSONOutput(t *testing.T) {
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
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "pkg-a", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error {
			return nil
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})

	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\nraw: %q", err, out)
	}
	for _, key := range []string{"installed", "updated", "removed", "skipped", "failed", "failed_providers", "skipped_providers", "state_save_errors", "success", "dry_run", "elapsed_ms"} {
		if _, ok := data[key]; !ok {
			t.Errorf("JSON missing required key %q; got: %v", key, data)
		}
	}
	// Cycle 238: elapsed_ms must be a non-negative number.
	if elapsed, ok := data["elapsed_ms"].(float64); !ok || elapsed < 0 {
		t.Errorf("elapsed_ms = %v (ok=%v), want non-negative", data["elapsed_ms"], ok)
	}
	if data["success"] != true {
		t.Errorf("success = %v, want true on happy-path apply", data["success"])
	}
	// Cycle 230: dry_run must always be present and false on a real apply.
	if dr, ok := data["dry_run"].(bool); !ok || dr {
		t.Errorf("dry_run = %v (ok=%v), want false on a real apply", data["dry_run"], ok)
	}
	// Cycle 231: failed_providers should be an empty array (NOT null)
	// on a happy-path apply so consumers can iterate without nil-checking.
	if fp, ok := data["failed_providers"].([]any); !ok || len(fp) != 0 {
		t.Errorf("failed_providers = %v, want []", data["failed_providers"])
	}
	// nil-safety: empty arrays should be [] not null.
	if sp, ok := data["skipped_providers"].([]any); !ok || len(sp) != 0 {
		t.Errorf("skipped_providers = %v, want []", data["skipped_providers"])
	}
	if sse, ok := data["state_save_errors"].([]any); !ok || len(sse) != 0 {
		t.Errorf("state_save_errors = %v, want []", data["state_save_errors"])
	}

	_ = storeDir
}

// TestRunApply_ProfileMismatchClearErrorMessage locks in cycle 194:
// when the configured profile_tag doesn't match any dir in the
// store, apply previously printed the generic "No providers match"
// message. Users who cloned a store without their profile_tag
// couldn't tell whether the store was empty or their config was
// wrong. Now: name the missing profile, name its path, AND list
// the profiles that DO exist in the store + suggest the fix.
func TestRunApply_ProfileMismatchClearErrorMessage(t *testing.T) {
	storeDir, _, _, flags := setupApplyTestEnv(t, []string{"apt"})

	// setupApplyTestEnv creates profileDir at storeDir/macOS and
	// writes profile_tag=macOS. Simulate profile-mismatch by renaming
	// the macOS dir away AND creating TWO unrelated profile dirs as
	// the "suggestions" we expect to see enumerated.
	flags.Profile = "" // clear explicit --profile; config wins
	if err := os.Rename(filepath.Join(storeDir, "macOS"), filepath.Join(storeDir, "_removed")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	for _, sibling := range []string{"linux", "openwrt"} {
		if err := os.MkdirAll(filepath.Join(storeDir, sibling), 0o750); err != nil {
			t.Fatalf("mkdir sibling: %v", err)
		}
	}

	// Register apt so the registry has a match.
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
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})

	if !strings.Contains(out, "profile directory") {
		t.Errorf("output should mention 'profile directory'; got:\n%s", out)
	}
	if !strings.Contains(out, "Available profiles in this store") {
		t.Errorf("output should list available profiles; got:\n%s", out)
	}
	if !strings.Contains(out, "linux") || !strings.Contains(out, "openwrt") {
		t.Errorf("output should name the sibling profiles; got:\n%s", out)
	}
	if !strings.Contains(out, "hams config set profile_tag") {
		t.Errorf("output should suggest config fix; got:\n%s", out)
	}
}

// TestRunApply_JSONOutput_FailedProvidersNamesFailures — cycle 231.
// Pre-cycle-231 the JSON summary said `"failed": N` but didn't name
// WHICH providers failed — a CI script that wanted to retry only the
// failed subset (`hams apply --only=$(jq -r '.failed_providers | join(",")')`)
// had to fall back to grepping slog output. Add `failed_providers`
// (list of provider names with at least one failed action). Empty
// array on happy path; populated on partial failure.
func TestRunApply_JSONOutput_FailedProvidersNamesFailures(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"alpha", "beta"})
	flags.JSON = true
	writeApplyTestFile(t, filepath.Join(profileDir, "alpha.hams.yaml"), "cli:\n  - app: pkg-good\n")
	writeApplyTestFile(t, filepath.Join(profileDir, "beta.hams.yaml"), "cli:\n  - app: pkg-bad\n")

	registry := provider.NewRegistry()
	good := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "alpha", DisplayName: "alpha", FilePrefix: "alpha",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "pkg-good", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error { return nil },
	}
	bad := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "beta", DisplayName: "beta", FilePrefix: "beta",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "pkg-bad", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error {
			return fmt.Errorf("simulated install failure")
		},
	}
	if err := registry.Register(good); err != nil {
		t.Fatalf("Register good: %v", err)
	}
	if err := registry.Register(bad); err != nil {
		t.Fatalf("Register bad: %v", err)
	}

	out := captureStdout(t, func() {
		// runApply returns ExitPartialFailure on any failed action; we
		// expect that here.
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err == nil {
			t.Fatal("expected ExitPartialFailure due to bad provider")
		}
	})

	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output not valid JSON: %v\nraw: %q", err, out)
	}
	failedCount, ok := data["failed"].(float64)
	if !ok || failedCount < 1 {
		t.Errorf("failed = %v, want >=1", data["failed"])
	}
	failedProviders, ok := data["failed_providers"].([]any)
	if !ok {
		t.Fatalf("failed_providers missing or wrong type; got %v", data["failed_providers"])
	}
	if len(failedProviders) != 1 {
		t.Errorf("failed_providers = %v, want exactly [beta]", failedProviders)
	}
	if len(failedProviders) >= 1 {
		if name, ok := failedProviders[0].(string); !ok || name != "beta" {
			t.Errorf("failed_providers[0] = %v, want \"beta\"", failedProviders[0])
		}
	}
}

// TestRunApply_TextOutput_NamesFailedProviders — cycle 235.
// Symmetric with cycle 231's JSON failed_providers and cycle 234's
// refresh text naming. Pre-cycle-235 the prose summary said
// `... %d failed` (count only) — interactive users had to grep slog
// to find WHICH providers failed. Asserts stdout contains the
// failing provider name with the new "had failed actions:" phrase.
func TestRunApply_TextOutput_NamesFailedProviders(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"alpha", "beta"})
	writeApplyTestFile(t, filepath.Join(profileDir, "alpha.hams.yaml"), "cli:\n  - app: pkg-good\n")
	writeApplyTestFile(t, filepath.Join(profileDir, "beta.hams.yaml"), "cli:\n  - app: pkg-bad\n")

	registry := provider.NewRegistry()
	good := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "alpha", DisplayName: "alpha", FilePrefix: "alpha",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "pkg-good", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error { return nil },
	}
	bad := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "beta", DisplayName: "beta", FilePrefix: "beta",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "pkg-bad", Type: provider.ActionInstall}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error {
			return fmt.Errorf("simulated install failure")
		},
	}
	if err := registry.Register(good); err != nil {
		t.Fatalf("Register good: %v", err)
	}
	if err := registry.Register(bad); err != nil {
		t.Fatalf("Register bad: %v", err)
	}

	out := captureStdout(t, func() {
		// runApply returns ExitPartialFailure on any failed action; expected.
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err == nil {
			t.Fatal("expected ExitPartialFailure due to bad provider")
		}
	})

	if !strings.Contains(out, "beta") {
		t.Errorf("text output should name 'beta' as failed; got %q", out)
	}
	if !strings.Contains(out, "had failed actions:") {
		t.Errorf("text output should use cycle-235 phrase 'had failed actions:'; got %q", out)
	}
}

// TestRunApply_DryRunJSONHasNoProse locks in cycle 187: `hams
// --json --dry-run apply` previously printed multiple prose lines
// ("[dry-run] Would apply configurations", "[dry-run] Provider
// execution order", per-provider previews, "[dry-run] No changes
// made") BEFORE/AFTER the JSON, making the output unparseable.
// Now: all dry-run prose is suppressed in JSON mode; only the
// JSON summary is emitted.
func TestRunApply_DryRunJSONHasNoProse(t *testing.T) {
	storeDir, profileDir, _, flags := setupApplyTestEnv(t, []string{"alpha"})
	flags.JSON = true
	flags.DryRun = true

	writeApplyTestFile(t, filepath.Join(profileDir, "alpha.hams.yaml"),
		"packages:\n  - app: pkg-a\n")

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "alpha", DisplayName: "alpha", FilePrefix: "alpha",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{ID: "pkg-a", Type: provider.ActionInstall}}, nil
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
			t.Fatalf("apply --json --dry-run: %v", err)
		}
	})

	// Output must NOT contain any of the prose dry-run markers.
	proseMarkers := []string{
		"[dry-run] Would apply",
		"[dry-run] Provider execution order",
		"[dry-run] No changes made",
		"no changes (",
		"+ install",
	}
	for _, marker := range proseMarkers {
		if strings.Contains(out, marker) {
			t.Errorf("JSON mode output contains prose marker %q; got:\n%s", marker, out)
		}
	}

	// Must be parseable as JSON.
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &data); err != nil {
		t.Fatalf("JSON mode output not parseable: %v\nraw: %q", err, out)
	}
	if data["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", data["dry_run"])
	}
	if data["success"] != true {
		t.Errorf("success = %v, want true on happy dry-run", data["success"])
	}
	// Cycle 240: dry-run JSON must include elapsed_ms (schema parity
	// with real-run JSON from cycle 238).
	if elapsed, ok := data["elapsed_ms"].(float64); !ok || elapsed < 0 {
		t.Errorf("elapsed_ms = %v (ok=%v), want non-negative", data["elapsed_ms"], ok)
	}
	// Cycle 244: planned_actions must be present and list the one
	// install action the test's planFn returned. Per cli-architecture
	// spec §"Dry-run apply shows planned actions" — dry-run output
	// SHALL display each action with its type and resource identity.
	planned, ok := data["planned_actions"].([]any)
	if !ok {
		t.Fatalf("planned_actions missing or wrong type: %v", data["planned_actions"])
	}
	if len(planned) != 1 {
		t.Fatalf("planned_actions len = %d, want 1 (alpha): %v", len(planned), planned)
	}
	entry, ok := planned[0].(map[string]any)
	if !ok {
		t.Fatalf("planned_actions[0] not a map: %T", planned[0])
	}
	if entry["provider"] != "alpha" {
		t.Errorf("provider = %v, want alpha", entry["provider"])
	}
	actions, ok := entry["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("actions = %v, want 1 install", actions)
	}
	act, ok := actions[0].(map[string]any)
	if !ok {
		t.Fatalf("actions[0] not a map: %T", actions[0])
	}
	if act["type"] != "install" {
		t.Errorf("action type = %v, want install", act["type"])
	}
	if act["id"] != "pkg-a" {
		t.Errorf("action id = %v, want pkg-a", act["id"])
	}

	_ = storeDir
}

// TestRunApply_JSONOutput_FailureListsAreSorted locks in cycle 254:
// when multiple providers land in `failed_providers` /
// `skipped_providers` / `state_save_errors`, the JSON output MUST
// sort the names alphabetically. Pre-cycle-254 these slices held
// DAG-iteration order — which can differ from alphabetical when
// providers have `DependsOn` relationships that force a
// non-alphabetical topological sort. Inconsistent with
// `probe_failed_providers` (cycle 232 sorts that one) and the
// text-mode `failedProviders` warning (cycle 235 sorts that one).
// With cycle 254 every JSON failure list is alphabetical, matching
// probe_failed_providers and text-mode text.
//
// Construct a dependency chain that forces a non-alphabetical DAG
// order: zeta (root) → alpha (depends on zeta) → beta (depends on
// alpha). DAG topo-sort: zeta, alpha, beta. If cycle 254's sort is
// removed, failed_providers would land in that same order. With the
// sort, it becomes alpha, beta, zeta.
func TestRunApply_JSONOutput_FailureListsAreSorted(t *testing.T) {
	// Priority slice doesn't affect DAG resolution once DependsOn
	// edges are set; we just need them registered.
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"zeta", "alpha", "beta"})
	flags.JSON = true

	for _, name := range []string{"zeta", "alpha", "beta"} {
		writeApplyTestFile(t, filepath.Join(profileDir, name+".hams.yaml"),
			"packages:\n  - app: always-fails\n")
	}

	// Dependency chain zeta → alpha → beta forces DAG order
	// zeta, alpha, beta (non-alphabetical).
	depsByName := map[string][]provider.DependOn{
		"zeta":  nil,
		"alpha": {{Provider: "zeta"}},
		"beta":  {{Provider: "alpha"}},
	}

	registry := provider.NewRegistry()
	for _, name := range []string{"zeta", "alpha", "beta"} {
		n := name
		p := &applyTestProvider{
			manifest: provider.Manifest{
				Name: n, DisplayName: n, FilePrefix: n,
				Platforms: []provider.Platform{provider.PlatformAll},
				DependsOn: depsByName[n],
			},
			planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
				return []provider.Action{{ID: "always-fails", Type: provider.ActionInstall}}, nil
			},
			applyFn: func(_ context.Context, _ provider.Action) error {
				return fmt.Errorf("synthetic failure from %s", n)
			},
		}
		if err := registry.Register(p); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}

	var data map[string]any
	out := captureStdout(t, func() {
		err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
		// Expect ExitPartialFailure UFE — apply had failing actions.
		if err == nil {
			t.Fatalf("runApply should error when every provider fails")
		}
	})
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &data); err != nil {
		t.Fatalf("output not parseable as JSON: %v\nraw: %q", err, out)
	}

	failedProviders, ok := data["failed_providers"].([]any)
	if !ok || len(failedProviders) != 3 {
		t.Fatalf("failed_providers should have 3 entries; got %v", data["failed_providers"])
	}
	want := []string{"alpha", "beta", "zeta"}
	for i, expected := range want {
		got, ok := failedProviders[i].(string)
		if !ok {
			t.Fatalf("failed_providers[%d] not a string: %T", i, failedProviders[i])
		}
		if got != expected {
			t.Errorf("failed_providers[%d] = %q, want %q (list should be alphabetical, not DAG order zeta/alpha/beta)",
				i, got, expected)
		}
	}
}

// TestRunApply_DryRunFromRepoNotCached_JSONEmits locks in cycle 251:
// when `hams --json --dry-run apply --from-repo=<X>` is invoked and
// <X> is not a local path AND not already cached under
// ${HAMS_DATA_HOME}/repo/, runApply takes the "Would clone" exit
// path. Pre-cycle-251 this path was `return nil` with zero bytes on
// stdout — CI scripts running `... | jq .` failed on empty input.
// Now: JSON mode emits the empty dry-run summary shape so consumers
// get a parseable object.
func TestRunApply_DryRunFromRepoNotCached_JSONEmits(t *testing.T) {
	// Isolate HOME paths so the clone-cache lookup definitely misses.
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	flags := &provider.GlobalFlags{
		JSON:   true,
		DryRun: true,
	}

	out := captureStdout(t, func() {
		err := runApply(context.Background(), flags, provider.NewRegistry(), sudo.NoopAcquirer{},
			"owner/nonexistent-dry-run-repo-"+t.Name(),
			true, "", "", false, bootstrapMode{})
		if err != nil {
			t.Fatalf("runApply: %v", err)
		}
	})

	// Stdout must parse as JSON (not be empty, not contain prose).
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Fatalf("stdout is empty; expected a dry-run JSON summary")
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		t.Fatalf("output not parseable as JSON: %v\nraw: %q", err, out)
	}
	if data["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", data["dry_run"])
	}
	if data["success"] != true {
		t.Errorf("success = %v, want true (no providers planned, trivially successful)", data["success"])
	}
}

// TestRunApply_NoProvidersMatch_JSONMode locks in cycle 247: when
// the stage-1 artifact filter produces zero providers (no hamsfile,
// no state), `hams --json apply` must emit a parseable JSON object,
// not the prose "no providers match" message.
//
// Pre-cycle-247, runApply called reportNoProvidersMatch
// unconditionally, so `hams --json apply --only=alpha` (on a profile
// with no alpha artifacts) dumped text to stdout. CI scripts piping
// through `jq` got a parse error instead of an empty-apply summary.
//
// Now: flags.JSON routes through emitEmptyApplyJSON which emits the
// same shape as a healthy zero-work real apply (installed=0,
// failed=0, skipped=0, success=true, dry_run=false, elapsed_ms≥0,
// plus the normalized empty slice fields). Consumers distinguish
// "no providers selected" from "all succeeded with zero resources"
// via the aggregate counts — but both yield valid JSON.
func TestRunApply_NoProvidersMatch_JSONMode(t *testing.T) {
	_, _, _, flags := setupApplyTestEnv(t, []string{"alpha"})
	flags.JSON = true
	// Note: NO hamsfile written — alpha has no artifacts at all.

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "alpha", DisplayName: "alpha", FilePrefix: "alpha",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
			t.Fatalf("runApply: %v", err)
		}
	})

	// Output MUST parse as JSON.
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &data); err != nil {
		t.Fatalf("JSON mode output not parseable: %v\nraw: %q", err, out)
	}

	// Must NOT contain the text-mode prose.
	proseMarkers := []string{
		"No providers match:",
		"no hamsfile or state file",
	}
	for _, marker := range proseMarkers {
		if strings.Contains(out, marker) {
			t.Errorf("JSON mode contains prose marker %q; got:\n%s", marker, out)
		}
	}

	// success=true (trivially — nothing to do), dry_run=false, and
	// every aggregate is zero.
	if data["success"] != true {
		t.Errorf("success = %v, want true (empty apply succeeds trivially)", data["success"])
	}
	if data["dry_run"] != false {
		t.Errorf("dry_run = %v, want false", data["dry_run"])
	}
	for _, field := range []string{"installed", "updated", "removed", "skipped", "failed"} {
		if v, ok := data[field].(float64); !ok || v != 0 {
			t.Errorf("%s = %v (ok=%v), want 0", field, data[field], ok)
		}
	}
	if _, ok := data["elapsed_ms"].(float64); !ok {
		t.Errorf("elapsed_ms missing or wrong type: %v", data["elapsed_ms"])
	}
}

// TestRunApply_DryRunSkipsStateSaveEntirely locks in cycle 241.
// cli-architecture/spec.md §--dry-run mandates: "No Hamsfile writes,
// state file writes, lock acquisitions, or wrapped CLI invocations
// SHALL occur during dry-run." A stateDir that was made read-only
// MUST NOT trigger a "state save failure" error path under dry-run
// — because dry-run MUST NOT attempt the save in the first place.
//
// Pre-cycle-241 the pre-apply refresh called sf.Save unconditionally,
// which on a read-only stateDir produced an ExitPartialFailure (and
// on a fresh/missing stateDir, silently CREATED the directory + file
// as a side effect — violating spec). Cycle 241 gates sf.Save on
// `!flags.DryRun`; the probe data is discarded; next non-dry-run
// apply re-probes and persists.
//
// This test supersedes the cycle-154 TestRunApply_DryRunStateSaveFailureSurfacesAsError
// which locked in the spec-violating behavior. The "detect perm issues
// early" feature it preserved is acceptable to lose — users discover
// perm issues at first real apply, which is the natural mutation point.
func TestRunApply_DryRunSkipsStateSaveEntirely(t *testing.T) {
	storeDir, profileDir, stateDir, flags := setupApplyTestEnv(t, []string{"alpha"})
	flags.DryRun = true

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

	// DO NOT pre-create stateDir — the spec-critical assertion is that
	// dry-run leaves the fs untouched. A fresh store with no `.state/`
	// subdirectory must STAY WITHOUT `.state/` after dry-run.
	if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
		t.Fatalf("stateDir unexpectedly exists before dry-run: %v", statErr)
	}

	out := captureStdout(t, func() {
		err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", false, "", "", false, bootstrapMode{})
		if err != nil {
			t.Fatalf("dry-run should succeed, got error: %v", err)
		}
	})

	// stdout should show the standard dry-run preview lines.
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("dry-run output should contain '[dry-run]' marker; got:\n%s", out)
	}
	// Spec invariant: no state file and no state dir created.
	if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
		t.Errorf("dry-run MUST NOT create stateDir %q; stat err: %v", stateDir, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(stateDir, "alpha.state.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("dry-run MUST NOT create state file; stat err: %v", statErr)
	}

	_ = storeDir
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

// TestRunApply_NonexistentStorePathEmitsUserError locks in cycle 87:
// when cfg.StorePath (or --store) names a directory that doesn't
// exist, runApply MUST surface that directly instead of propagating
// a confusing "creating lock directory: mkdir X: permission denied"
// error from state.NewLock.Acquire. The previous error pointed at
// the `.state/<machine-id>/.lock` subpath, which had nothing to do
// with the actual misconfiguration (the store_path itself).
func TestRunApply_NonexistentStorePathEmitsUserError(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\nmachine_id: mid1\n")

	flags := &provider.GlobalFlags{Store: "/definitely/does/not/exist/ever"}
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
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
	// Message must name the specific bad path so users can copy-paste.
	if !strings.Contains(ufe.Message, "/definitely/does/not/exist/ever") {
		t.Errorf("message should name the bad path; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "does not exist or is not a directory") {
		t.Errorf("message should explain what's wrong; got %q", ufe.Message)
	}
	// Error must NOT mention the downstream .lock symptom.
	if strings.Contains(ufe.Message, ".lock") || strings.Contains(ufe.Message, "lock directory") {
		t.Errorf("error should not point at the .lock subpath (that's a symptom); got %q", ufe.Message)
	}
	joined := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(joined, "hams store init") && !strings.Contains(joined, "--from-repo") {
		t.Errorf("suggestions should teach recovery; got %v", ufe.Suggestions)
	}
}

// TestRunApply_StorePathIsFileNotDir asserts the same error fires
// when store_path points at a regular file rather than a directory —
// catches typos like `store_path: ~/.config/hams/hams.config.yaml`
// where the user accidentally pointed at the config file.
func TestRunApply_StorePathIsFileNotDir(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\nmachine_id: mid1\n")

	// Create a file (not a directory) and point --store at it.
	fileAsStore := filepath.Join(t.TempDir(), "oops.yaml")
	if err := os.WriteFile(fileAsStore, []byte("not a store"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	flags := &provider.GlobalFlags{Store: fileAsStore}
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected error when store_path points at a file")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Message, "is not a directory") {
		t.Errorf("message should distinguish file vs missing; got %q", ufe.Message)
	}
}

// TestRunApply_ExplicitProfileNotFoundEmitsUserError locks in cycle 92:
// when the user types `hams --profile=Linux apply` (typo) and
// `<store>/Linux` doesn't exist, runApply MUST surface that with
// ExitUsageError instead of silently printing "No providers match"
// + exit 0. Symmetric with cycle 87's store_path validation.
//
// The check fires ONLY when flags.Profile is explicitly set —
// profile_tag coming from config.yaml with no matching directory
// is treated as "empty profile, nothing to do" (user may not have
// any hamsfiles yet), which is different from an explicit CLI
// typo signaling intent.
func TestRunApply_ExplicitProfileNotFoundEmitsUserError(t *testing.T) {
	storeDir, _, _, _ := setupApplyTestEnv(t, nil)

	flags := &provider.GlobalFlags{Store: storeDir, Profile: "Linux"}
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected error when --profile dir doesn't exist; got nil (should not silently skip)")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want ExitUsageError (%d)", ufe.Code, hamserr.ExitUsageError)
	}
	if !strings.Contains(ufe.Message, "Linux") {
		t.Errorf("message should name the typo'd profile; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "not found") {
		t.Errorf("message should say the profile isn't found; got %q", ufe.Message)
	}
	// Suggestions must teach the user how to enumerate / create profiles.
	joined := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(joined, "ls ") {
		t.Errorf("suggestions should point at `ls <store>`; got %v", ufe.Suggestions)
	}
	if !strings.Contains(joined, "mkdir") {
		t.Errorf("suggestions should offer the mkdir path; got %v", ufe.Suggestions)
	}
}

// TestRunApply_ConfigProfileSilentlyEmptyIsNotAnError asserts the
// converse of the cycle-92 check: when profile_tag comes from
// `hams.config.yaml` (not from an explicit `--profile` flag) AND
// the profile dir is empty/missing, runApply MUST still succeed
// with "No providers match" rather than erroring. Users shouldn't
// be forced to create empty profile dirs just to run apply.
func TestRunApply_ConfigProfileSilentlyEmptyIsNotAnError(t *testing.T) {
	// setupApplyTestEnv creates the profile dir at "macOS". We
	// override cfg's profile_tag via the global config to point at
	// a profile without a dir, but DON'T pass --profile so the
	// cycle-92 check doesn't fire.
	_, _, _, flags := setupApplyTestEnv(t, nil)
	configHome := os.Getenv("HAMS_CONFIG_HOME")

	// Rewrite global config with profile_tag pointing at "ghost"
	// (no directory will ever exist at <store>/ghost).
	globalCfg := filepath.Join(configHome, "hams.config.yaml")
	writeApplyTestFile(t, globalCfg, "profile_tag: ghost\nmachine_id: test-machine\n")

	// Leave flags.Profile unset so the check is skipped.
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{})
	if err != nil {
		t.Fatalf("config-derived empty profile should NOT error; got: %v", err)
	}
}

// TestRunApply_FromRepoAndStoreAreMutuallyExclusive locks in cycle 100:
// `hams apply --from-repo=X --store=Y` used to silently honor only
// --from-repo (cloning to ${HAMS_DATA_HOME}/repo/X/), confusing users
// who thought --store would redirect the clone. Now we emit a clear
// UserFacingError naming both flags + the ${HAMS_DATA_HOME}/repo/...
// clone location so the user can pick the right one.
func TestRunApply_FromRepoAndStoreAreMutuallyExclusive(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	storeDir := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: macOS\nmachine_id: mid1\n")

	flags := &provider.GlobalFlags{Store: storeDir}
	registry := provider.NewRegistry()

	// Both --from-repo and --store passed; expect mutual-exclusion error.
	err := runApply(context.Background(), flags, registry,
		sudo.NoopAcquirer{}, "never-cloned/somerepo", true, "", "", false, bootstrapMode{})
	if err == nil {
		t.Fatal("expected error when both --from-repo and --store are set")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want ExitUsageError (%d)", ufe.Code, hamserr.ExitUsageError)
	}
	if !strings.Contains(ufe.Message, "--from-repo") || !strings.Contains(ufe.Message, "--store") {
		t.Errorf("message should name BOTH flags; got %q", ufe.Message)
	}
	if !strings.Contains(ufe.Message, "mutually exclusive") {
		t.Errorf("message should say mutually exclusive; got %q", ufe.Message)
	}
	joined := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(joined, "HAMS_DATA_HOME") {
		t.Errorf("suggestions should explain where --from-repo clones; got %v", ufe.Suggestions)
	}
}

// TestOnlyMissingArtifacts pins cycle 226's helper contract: given a
// --only CSV and two provider lists, return the subset of --only names
// that are VALID registered providers but lack artifacts for the
// active profile. Empty inputs → nil. Unknown names → silently
// skipped (validation happens upstream in filterProviders). Order
// matches --only input order so the user-facing message is
// predictable.
func TestOnlyMissingArtifacts(t *testing.T) {
	t.Parallel()

	brew := &applyTestProvider{manifest: provider.Manifest{Name: "brew", FilePrefix: "brew"}}
	apt := &applyTestProvider{manifest: provider.Manifest{Name: "apt", FilePrefix: "apt"}}
	cargo := &applyTestProvider{manifest: provider.Manifest{Name: "cargo", FilePrefix: "cargo"}}
	all := []provider.Provider{brew, apt, cargo}
	stage1 := []provider.Provider{apt} // only apt has artifacts

	cases := []struct {
		name string
		only string
		want []string
	}{
		{"empty-only", "", nil},
		{"whitespace-only", "   ", nil},
		{"single-missing", "brew", []string{"brew"}},
		{"single-present", "apt", nil},
		{"mixed-csv", "brew,apt,cargo", []string{"brew", "cargo"}},
		{"unknown-skipped", "bogus", nil},
		{"mixed-unknown-and-missing", "bogus,brew", []string{"brew"}},
		{"csv-spaces", " brew , cargo ", []string{"brew", "cargo"}},
		{"case-insensitive", "BREW", []string{"brew"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := onlyMissingArtifacts(tc.only, all, stage1)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("[%d] got %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

// TestRunApply_SudoPromptWhenBashHamsfileHasSudoEntry locks in
// cycle 232: a bash-only profile whose hamsfile has at least one
// `sudo: true` entry MUST trigger the upfront sudoAcq.Acquire.
// Pre-cycle-232 the cycle-227 gate only checked Manifest.RequiresSudo
// (bash's Manifest can't declare sudo statically — it depends on
// per-entry `sudo: true` fields), so bash's runtime sudo scripts
// prompted independently during Execute instead of reusing the
// cached credential. Now: runApply peeks into bash.hams.yaml via
// bash.HamsfileHasSudoEntries and surfaces it as a sudo-needing
// provider.
func TestRunApply_SudoPromptWhenBashHamsfileHasSudoEntry(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"bash"})
	// Seed a bash hamsfile with ONE sudo:true entry. cycle 229's
	// URN-warning path will also fire, but doesn't affect the sudo
	// decision.
	bashHamsfile := filepath.Join(profileDir, "bash.hams.yaml")
	writeApplyTestFile(t, bashHamsfile, `install:
  - urn: "urn:hams:bash:needs-sudo"
    run: "apt-get install -y nix"
    sudo: true
`)

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "bash",
			DisplayName: "Bash",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "bash",
			// RequiresSudo intentionally false — the dynamic check
			// reading the hamsfile is what we're exercising.
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return []provider.Action{{
				ID:       "urn:hams:bash:needs-sudo",
				Type:     provider.ActionInstall,
				Resource: "dummy",
			}}, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error { return nil },
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	spy := &sudo.SpyAcquirer{}
	if err := runApply(context.Background(), flags, registry, spy, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if spy.AcquireCalls != 1 {
		t.Errorf("Acquire calls = %d, want 1 (bash hamsfile has sudo:true entry)", spy.AcquireCalls)
	}
}

// TestRunApply_NoSudoPromptWhenBashHamsfileHasNoSudoEntry is the
// negative case for cycle 232: a bash profile whose hamsfile lacks
// any `sudo: true` entries MUST NOT prompt. The cycle-227 spirit
// (don't prompt when not needed) is preserved when bash scripts
// run unprivileged.
func TestRunApply_NoSudoPromptWhenBashHamsfileHasNoSudoEntry(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"bash"})
	bashHamsfile := filepath.Join(profileDir, "bash.hams.yaml")
	writeApplyTestFile(t, bashHamsfile, `install:
  - urn: "urn:hams:bash:no-sudo"
    run: "echo hello"
`)

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name:        "bash",
			DisplayName: "Bash",
			Platforms:   []provider.Platform{provider.PlatformAll},
			FilePrefix:  "bash",
		},
		planFn: func(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
			return nil, nil
		},
		applyFn: func(_ context.Context, _ provider.Action) error { return nil },
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	spy := &sudo.SpyAcquirer{}
	if err := runApply(context.Background(), flags, registry, spy, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	if spy.AcquireCalls != 0 {
		t.Errorf("Acquire calls = %d, want 0 (bash hamsfile has no sudo:true entry)", spy.AcquireCalls)
	}
}

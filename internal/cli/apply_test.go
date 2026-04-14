package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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
	if err := runApply(context.Background(), flags, registry, spy, "", true, "", ""); err != nil {
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

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", ""); err != nil {
		t.Fatalf("first runApply error: %v", err)
	}
	writeApplyTestFile(t, hamsfilePath, "packages: []\n")
	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", ""); err != nil {
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

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", ""); err != nil {
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

	configBody := strings.Join([]string{
		"profile_tag: macOS",
		"machine_id: test-machine",
		formatProviderPriority(providerPriority),
		"",
	}, "\n")
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"), configBody)

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

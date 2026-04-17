package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
)

// TestEnsureStoreScaffolded_CreatesDirAndTemplates asserts the happy
// path: pristine host, user invokes a provider, scaffold materializes
// the store directory + `.gitignore` + `hams.config.yaml` + `git
// init` side effect. Regression gate for CLAUDE.md's Current Tasks
// bullet about "auto-create one at the default location" for
// provider wraps.
func TestEnsureStoreScaffolded_CreatesDirAndTemplates(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "") // force defaultStoreLocation under dataHome

	// Swap git init to a fake so the test doesn't shell out to the
	// real git binary.
	origGitInit := gitInitExec
	defer func() { gitInitExec = origGitInit }()
	var gitInitCalledFor string
	gitInitExec = func(_ context.Context, dir string) error {
		gitInitCalledFor = dir
		// Simulate git by making the `.git` dir the real git init would
		// have created.
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	path, err := EnsureStoreScaffolded(context.Background(), paths, flags)
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	wantStore := filepath.Join(dataHome, "store")
	if path != wantStore {
		t.Errorf("scaffold returned %q, want %q", path, wantStore)
	}
	if gitInitCalledFor != wantStore {
		t.Errorf("git init not called on store dir; got %q, want %q",
			gitInitCalledFor, wantStore)
	}
	for _, name := range []string{".gitignore", "hams.config.yaml", ".git"} {
		p := filepath.Join(wantStore, name)
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected %s after scaffold; stat err = %v", name, statErr)
		}
	}

	// Assert the persisted global config points back at the
	// scaffolded store so the next `hams apply` finds it.
	globalConfig, err := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("global config not persisted: %v", err)
	}
	if !strings.Contains(string(globalConfig), "store_path:") {
		t.Errorf("global config missing store_path after scaffold; got:\n%s",
			string(globalConfig))
	}
	if !strings.Contains(string(globalConfig), wantStore) {
		t.Errorf("global config store_path doesn't point at scaffolded path %q; got:\n%s",
			wantStore, string(globalConfig))
	}
}

// TestEnsureStoreScaffolded_Idempotent asserts the helper does not
// touch an already-populated store. Running scaffold twice on the
// same directory must leave the user's existing `.gitignore` / config
// untouched (the user may have hand-edited them).
func TestEnsureStoreScaffolded_Idempotent(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origGitInit := gitInitExec
	defer func() { gitInitExec = origGitInit }()
	gitInitCalls := 0
	gitInitExec = func(_ context.Context, dir string) error {
		gitInitCalls++
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	wantStore := filepath.Join(dataHome, "store")

	// First call — scaffolds from scratch.
	if _, err := EnsureStoreScaffolded(context.Background(), paths, flags); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}

	// Hand-edit the .gitignore to prove we don't clobber user changes.
	handEdited := "# user added line\n.env\n"
	if err := os.WriteFile(filepath.Join(wantStore, ".gitignore"),
		[]byte(handEdited), 0o600); err != nil {
		t.Fatalf("write user .gitignore: %v", err)
	}

	// Second call — must be a no-op for the .gitignore content.
	if _, err := EnsureStoreScaffolded(context.Background(), paths, flags); err != nil {
		t.Fatalf("second scaffold: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(wantStore, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(got) != handEdited {
		t.Errorf(".gitignore was rewritten on idempotent call.\nwant:\n%s\ngot:\n%s",
			handEdited, string(got))
	}
	// git init runs exactly once (on first call; second has `.git`
	// already).
	if gitInitCalls != 1 {
		t.Errorf("git init called %d times, want 1", gitInitCalls)
	}
}

// TestEnsureStoreScaffolded_SeedsIdentityKeys asserts that first-time
// scaffold populates profile_tag + machine_id in the global config,
// not just store_path. Without this, every subsequent `hams
// <provider> …` invocation fired "profile_tag is empty / machine_id
// is empty" warnings — making the post-onboarding experience look
// broken even though the tool was working correctly. The seeded
// values are conservative defaults (profile_tag="default",
// machine_id=DeriveMachineID()) that the user can override later
// with `hams config set`.
func TestEnsureStoreScaffolded_SeedsIdentityKeys(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")
	// Pin DeriveMachineID to a deterministic value so the assertion
	// doesn't depend on the test host's real hostname.
	origLookup := config.HostnameLookup
	t.Cleanup(func() { config.HostnameLookup = origLookup })
	config.HostnameLookup = func() (string, error) { return "testbox", nil }

	origGitInit := gitInitExec
	t.Cleanup(func() { gitInitExec = origGitInit })
	gitInitExec = func(_ context.Context, dir string) error {
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	if _, err := EnsureStoreScaffolded(context.Background(), paths, flags); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.ProfileTag != "default" {
		t.Errorf("profile_tag = %q, want %q after scaffold", cfg.ProfileTag, "default")
	}
	if cfg.MachineID != "testbox" {
		t.Errorf("machine_id = %q, want %q after scaffold", cfg.MachineID, "testbox")
	}
}

// TestEnsureStoreScaffolded_DoesNotClobberUserIdentity asserts that
// scaffold respects an already-set profile_tag or machine_id. A user
// who ran `hams config set profile_tag macOS` before their first
// provider install must keep "macOS", not be silently overwritten
// with "default".
func TestEnsureStoreScaffolded_DoesNotClobberUserIdentity(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	// Pre-seed global config with a user-chosen profile_tag + machine_id.
	if err := os.MkdirAll(configHome, 0o750); err != nil {
		t.Fatalf("mkdir configHome: %v", err)
	}
	userConfig := "profile_tag: macOS\nmachine_id: laptop-m5x\n"
	if err := os.WriteFile(filepath.Join(configHome, "hams.config.yaml"),
		[]byte(userConfig), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	origGitInit := gitInitExec
	t.Cleanup(func() { gitInitExec = origGitInit })
	gitInitExec = func(_ context.Context, dir string) error {
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	if _, err := EnsureStoreScaffolded(context.Background(), paths, flags); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.ProfileTag != "macOS" {
		t.Errorf("scaffold clobbered user's profile_tag: got %q, want %q",
			cfg.ProfileTag, "macOS")
	}
	if cfg.MachineID != "laptop-m5x" {
		t.Errorf("scaffold clobbered user's machine_id: got %q, want %q",
			cfg.MachineID, "laptop-m5x")
	}
}

// TestEnsureStoreScaffolded_RespectsHamsStoreEnv verifies that
// `HAMS_STORE=<path>` overrides the default under `HAMS_DATA_HOME`.
// Users who want the store somewhere specific (e.g. `~/Projects/
// hams-store`) must not be silently redirected to data_home.
func TestEnsureStoreScaffolded_RespectsHamsStoreEnv(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	explicitStore := filepath.Join(root, "my-own-store")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", explicitStore)

	origGitInit := gitInitExec
	defer func() { gitInitExec = origGitInit }()
	gitInitExec = func(_ context.Context, dir string) error {
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	got, err := EnsureStoreScaffolded(context.Background(), paths, flags)
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if got != explicitStore {
		t.Errorf("scaffold used %q, want HAMS_STORE override %q", got, explicitStore)
	}
}

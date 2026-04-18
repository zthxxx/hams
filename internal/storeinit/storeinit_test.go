package storeinit_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/storeinit"
)

// fakeGitInitOK writes an empty `.git` directory, matching what a
// successful `git init` would materialize for the purposes of the
// scaffold's stat probe.
func fakeGitInitOK(dir string) func(context.Context, string) error {
	return func(_ context.Context, target string) error {
		if target != dir {
			return errors.New("fakeGitInitOK: unexpected dir")
		}
		return os.MkdirAll(filepath.Join(target, ".git"), 0o750)
	}
}

// captureSlog swaps the default slog handler to a bytes.Buffer for
// the duration of the test so the caller can assert log breadcrumbs.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestBootstrap_CreatesDirAndTemplates(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origExec := storeinit.ExecGitInit
	t.Cleanup(func() { storeinit.ExecGitInit = origExec })
	wantStore := filepath.Join(dataHome, "store")
	var execCalledFor string
	storeinit.ExecGitInit = func(ctx context.Context, dir string) error {
		execCalledFor = dir
		return fakeGitInitOK(dir)(ctx, dir)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	path, err := storeinit.Bootstrap(context.Background(), paths, flags)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if path != wantStore {
		t.Errorf("Bootstrap returned %q, want %q", path, wantStore)
	}
	if execCalledFor != wantStore {
		t.Errorf("ExecGitInit called with %q, want %q", execCalledFor, wantStore)
	}
	for _, name := range []string{".gitignore", "hams.config.yaml", ".git"} {
		p := filepath.Join(wantStore, name)
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected %s after Bootstrap; stat err = %v", name, statErr)
		}
	}

	globalConfig, readErr := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if readErr != nil {
		t.Fatalf("global config not persisted: %v", readErr)
	}
	if !strings.Contains(string(globalConfig), "store_path:") {
		t.Errorf("global config missing store_path; got:\n%s", string(globalConfig))
	}
	if !strings.Contains(string(globalConfig), wantStore) {
		t.Errorf("global config store_path doesn't point at scaffolded path %q; got:\n%s",
			wantStore, string(globalConfig))
	}
}

func TestBootstrap_Idempotent(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origExec := storeinit.ExecGitInit
	t.Cleanup(func() { storeinit.ExecGitInit = origExec })
	var execCalls int
	storeinit.ExecGitInit = func(ctx context.Context, dir string) error {
		execCalls++
		return fakeGitInitOK(dir)(ctx, dir)
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	wantStore := filepath.Join(dataHome, "store")

	if _, err := storeinit.Bootstrap(context.Background(), paths, flags); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}

	// Hand-edit .gitignore; second call must not clobber it.
	handEdited := "# user added line\n.env\n"
	if err := os.WriteFile(filepath.Join(wantStore, ".gitignore"),
		[]byte(handEdited), 0o600); err != nil {
		t.Fatalf("write user .gitignore: %v", err)
	}

	if _, err := storeinit.Bootstrap(context.Background(), paths, flags); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(wantStore, ".gitignore"))
	if readErr != nil {
		t.Fatalf("read .gitignore: %v", readErr)
	}
	if string(got) != handEdited {
		t.Errorf(".gitignore rewritten on idempotent call.\nwant:\n%s\ngot:\n%s", handEdited, string(got))
	}
	if execCalls != 1 {
		t.Errorf("ExecGitInit called %d times, want 1", execCalls)
	}
}

func TestBootstrap_FallsBackToGoGit_WhenGitMissing(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origExec := storeinit.ExecGitInit
	origGoGit := storeinit.GoGitInit
	t.Cleanup(func() {
		storeinit.ExecGitInit = origExec
		storeinit.GoGitInit = origGoGit
	})
	// Force the "git not found" leg: ExecGitInit returns an error that
	// wraps exec.ErrNotFound, so ensureGitRepo falls through to GoGitInit.
	storeinit.ExecGitInit = func(_ context.Context, _ string) error {
		return errors.Join(errors.New("fake: no git"), exec.ErrNotFound)
	}
	var goGitCalledFor string
	storeinit.GoGitInit = func(_ context.Context, dir string) error {
		goGitCalledFor = dir
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o750)
	}
	logs := captureSlog(t)

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	wantStore := filepath.Join(dataHome, "store")

	path, err := storeinit.Bootstrap(context.Background(), paths, flags)
	if err != nil {
		t.Fatalf("Bootstrap with missing git: %v", err)
	}
	if path != wantStore {
		t.Errorf("Bootstrap returned %q, want %q", path, wantStore)
	}
	if goGitCalledFor != wantStore {
		t.Errorf("GoGitInit called with %q, want %q", goGitCalledFor, wantStore)
	}
	if !strings.Contains(logs.String(), "bundled go-git") {
		t.Errorf("expected log to mention 'bundled go-git'; got:\n%s", logs.String())
	}
	// Templates must land regardless of which init path was used.
	for _, name := range []string{".gitignore", "hams.config.yaml", ".git"} {
		p := filepath.Join(wantStore, name)
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected %s after Bootstrap; stat err = %v", name, statErr)
		}
	}
}

func TestBootstrap_PropagatesNonNotFoundExecErrors(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origExec := storeinit.ExecGitInit
	origGoGit := storeinit.GoGitInit
	t.Cleanup(func() {
		storeinit.ExecGitInit = origExec
		storeinit.GoGitInit = origGoGit
	})
	// Generic exec error — simulates a hung hook or permission failure.
	// ensureGitRepo MUST propagate this unchanged and MUST NOT retry
	// via GoGitInit.
	execErr := errors.New("git hook crashed: permission denied")
	storeinit.ExecGitInit = func(_ context.Context, _ string) error { return execErr }
	var goGitCalls int
	storeinit.GoGitInit = func(_ context.Context, _ string) error {
		goGitCalls++
		return nil
	}

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	_, err := storeinit.Bootstrap(context.Background(), paths, flags)
	if err == nil {
		t.Fatalf("Bootstrap returned nil; want wrapped exec error")
	}
	if !strings.Contains(err.Error(), "git hook crashed") {
		t.Errorf("error did not preserve the exec failure: %v", err)
	}
	if goGitCalls != 0 {
		t.Errorf("GoGitInit was called %d times; should never be called for non-NotFound errors", goGitCalls)
	}
}

func TestBootstrap_SeedsIdentityDefaults(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origLookup := config.HostnameLookup
	t.Cleanup(func() { config.HostnameLookup = origLookup })
	config.HostnameLookup = func() (string, error) { return "testbox", nil }

	origExec := storeinit.ExecGitInit
	t.Cleanup(func() { storeinit.ExecGitInit = origExec })
	storeinit.ExecGitInit = fakeGitInitOK(filepath.Join(dataHome, "store"))

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	if _, err := storeinit.Bootstrap(context.Background(), paths, flags); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.ProfileTag != "default" {
		t.Errorf("profile_tag = %q, want %q", cfg.ProfileTag, "default")
	}
	if cfg.MachineID != "testbox" {
		t.Errorf("machine_id = %q, want %q", cfg.MachineID, "testbox")
	}
}

func TestBootstrap_DoesNotClobberUserIdentity(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	if err := os.MkdirAll(configHome, 0o750); err != nil {
		t.Fatalf("mkdir configHome: %v", err)
	}
	userConfig := "profile_tag: macOS\nmachine_id: laptop-m5x\n"
	if err := os.WriteFile(filepath.Join(configHome, "hams.config.yaml"),
		[]byte(userConfig), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	origExec := storeinit.ExecGitInit
	t.Cleanup(func() { storeinit.ExecGitInit = origExec })
	storeinit.ExecGitInit = fakeGitInitOK(filepath.Join(dataHome, "store"))

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	if _, err := storeinit.Bootstrap(context.Background(), paths, flags); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	cfg, err := config.Load(paths, "", "")
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.ProfileTag != "macOS" {
		t.Errorf("scaffold clobbered profile_tag: got %q, want %q", cfg.ProfileTag, "macOS")
	}
	if cfg.MachineID != "laptop-m5x" {
		t.Errorf("scaffold clobbered machine_id: got %q, want %q", cfg.MachineID, "laptop-m5x")
	}
}

func TestBootstrap_RespectsHamsStoreEnv(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	explicitStore := filepath.Join(root, "my-own-store")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", explicitStore)

	origExec := storeinit.ExecGitInit
	t.Cleanup(func() { storeinit.ExecGitInit = origExec })
	storeinit.ExecGitInit = fakeGitInitOK(explicitStore)

	paths := config.ResolvePaths()
	flags := &provider.GlobalFlags{}
	got, err := storeinit.Bootstrap(context.Background(), paths, flags)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if got != explicitStore {
		t.Errorf("Bootstrap used %q, want HAMS_STORE override %q", got, explicitStore)
	}
}

func TestBootstrap_DryRunIsSideEffectFree(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("HAMS_STORE", "")

	origExec := storeinit.ExecGitInit
	origGoGit := storeinit.GoGitInit
	t.Cleanup(func() {
		storeinit.ExecGitInit = origExec
		storeinit.GoGitInit = origGoGit
	})
	var execCalls, goGitCalls int
	storeinit.ExecGitInit = func(_ context.Context, _ string) error { execCalls++; return nil }
	storeinit.GoGitInit = func(_ context.Context, _ string) error { goGitCalls++; return nil }

	paths := config.ResolvePaths()
	stderr := &bytes.Buffer{}
	flags := &provider.GlobalFlags{DryRun: true, Err: stderr}
	path, err := storeinit.Bootstrap(context.Background(), paths, flags)
	if err != nil {
		t.Fatalf("Bootstrap dry-run: %v", err)
	}
	wantStore := filepath.Join(dataHome, "store")
	if path != wantStore {
		t.Errorf("dry-run path = %q, want %q", path, wantStore)
	}
	// Dry-run must not create the store on disk.
	if _, statErr := os.Stat(wantStore); statErr == nil {
		t.Errorf("dry-run created %s (should not)", wantStore)
	}
	if execCalls != 0 || goGitCalls != 0 {
		t.Errorf("dry-run invoked git init (exec=%d, gogit=%d); want 0/0", execCalls, goGitCalls)
	}
	if !strings.Contains(stderr.String(), "[dry-run]") {
		t.Errorf("dry-run stderr missing preview line; got: %q", stderr.String())
	}
	if _, statErr := os.Stat(filepath.Join(configHome, "hams.config.yaml")); statErr == nil {
		t.Errorf("dry-run wrote global config (should not)")
	}
}

func TestBootstrapped(t *testing.T) {
	// GIVEN: non-existent directory → false.
	if storeinit.Bootstrapped("") {
		t.Errorf("Bootstrapped(\"\") = true, want false")
	}
	tmp := t.TempDir()
	if storeinit.Bootstrapped(tmp) {
		t.Errorf("empty dir should not look bootstrapped")
	}
	// Directory with only .git → false (missing hams.config.yaml).
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if storeinit.Bootstrapped(tmp) {
		t.Errorf(".git-only dir should not look bootstrapped")
	}
	// Both → true.
	if err := os.WriteFile(filepath.Join(tmp, "hams.config.yaml"), []byte("tag: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !storeinit.Bootstrapped(tmp) {
		t.Errorf("store with .git + hams.config.yaml should look bootstrapped")
	}
}

// TestBootstrap_PropertyIdempotent asserts that calling Bootstrap an
// arbitrary number of times on the same fresh HAMS_DATA_HOME produces
// the same on-disk template content as a single call. This is the
// property the scaffolder's idempotency docstring promises; pgregory's
// rapid exercises it with randomized call counts.
func TestBootstrap_PropertyIdempotent(t *testing.T) {
	// Rapid reuses rt across shrinks; use the outer *testing.T for
	// ephemeral dirs + env overrides, which is exactly what rapid's
	// docs recommend for property tests that need real filesystem
	// setup.
	rapid.Check(t, func(rt *rapid.T) {
		root := t.TempDir()
		configHome := filepath.Join(root, "config")
		dataHome := filepath.Join(root, "data")
		storePath := filepath.Join(dataHome, "store")
		t.Setenv("HAMS_CONFIG_HOME", configHome)
		t.Setenv("HAMS_DATA_HOME", dataHome)
		t.Setenv("HAMS_STORE", "")

		origExec := storeinit.ExecGitInit
		t.Cleanup(func() { storeinit.ExecGitInit = origExec })
		storeinit.ExecGitInit = fakeGitInitOK(storePath)

		paths := config.ResolvePaths()
		flags := &provider.GlobalFlags{}

		// First call — baseline.
		if _, err := storeinit.Bootstrap(context.Background(), paths, flags); err != nil {
			rt.Fatalf("first Bootstrap: %v", err)
		}
		baselineGitignore, readErr := os.ReadFile(filepath.Join(storePath, ".gitignore"))
		if readErr != nil {
			rt.Fatalf("read baseline .gitignore: %v", readErr)
		}
		baselineConfig, readErr := os.ReadFile(filepath.Join(storePath, "hams.config.yaml"))
		if readErr != nil {
			rt.Fatalf("read baseline hams.config.yaml: %v", readErr)
		}

		// N further calls — content must not drift.
		n := rapid.IntRange(0, 5).Draw(rt, "extraCalls")
		for i := range n {
			if _, err := storeinit.Bootstrap(context.Background(), paths, flags); err != nil {
				rt.Fatalf("Bootstrap #%d: %v", i+2, err)
			}
		}
		gotGitignore, readErr := os.ReadFile(filepath.Join(storePath, ".gitignore"))
		if readErr != nil {
			rt.Fatalf("read post .gitignore: %v", readErr)
		}
		if !bytes.Equal(baselineGitignore, gotGitignore) {
			rt.Fatalf(".gitignore drifted after %d extra calls", n)
		}
		gotConfig, readErr := os.ReadFile(filepath.Join(storePath, "hams.config.yaml"))
		if readErr != nil {
			rt.Fatalf("read post hams.config.yaml: %v", readErr)
		}
		if !bytes.Equal(baselineConfig, gotConfig) {
			rt.Fatalf("hams.config.yaml drifted after %d extra calls", n)
		}
	})
}

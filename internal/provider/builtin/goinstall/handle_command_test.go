package goinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
)

type goInstallHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newGoInstallHarness(t *testing.T) *goInstallHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: profileTag, MachineID: "test-machine"}
	runner := NewFakeCmdRunner()
	p := New(cfg, runner)
	return &goInstallHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "goinstall.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *goInstallHarness) hamsfileApps() []string {
	h.t.Helper()
	if _, err := os.Stat(h.hamsfilePath); err != nil {
		return nil
	}
	f, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		h.t.Fatalf("read hamsfile: %v", err)
	}
	return f.ListApps()
}

// U1 — bare module path gets `@latest` injected and that pinned form
// is what lands in the hamsfile, so a later `hams apply` reproduces
// the install deterministically.
func TestHandleCommand_U1_InstallRecordsPinnedPath(t *testing.T) {
	h := newGoInstallHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.org/x/tools/cmd/goimports"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	want := "golang.org/x/tools/cmd/goimports@latest"
	if h.runner.CallCount(fakeOpInstall, want) != 1 {
		t.Errorf("runner.Install(%q) calls = %d, want 1", want, h.runner.CallCount(fakeOpInstall, want))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != want {
		t.Errorf("hamsfile apps = %v, want [%q]", apps, want)
	}
}

// U2 — explicit version pin is preserved verbatim.
func TestHandleCommand_U2_InstallPreservesExplicitVersion(t *testing.T) {
	h := newGoInstallHarness(t)
	pkg := "github.com/example/tool@v1.2.3"
	if err := h.provider.HandleCommand(context.Background(), []string{"install", pkg}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, pkg) != 1 {
		t.Errorf("runner.Install(%q) calls = %d, want 1", pkg, h.runner.CallCount(fakeOpInstall, pkg))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != pkg {
		t.Errorf("hamsfile apps = %v, want [%q]", apps, pkg)
	}
}

// U3 — idempotent re-install.
func TestHandleCommand_U3_InstallIsIdempotent(t *testing.T) {
	h := newGoInstallHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(),
			[]string{"install", "golang.org/x/tools/cmd/goimports"}, nil, h.flags); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("want single entry, got %v", apps)
	}
}

// U4 — install failure leaves hamsfile untouched.
func TestHandleCommand_U4_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newGoInstallHarness(t)
	// FakeCmdRunner.Install succeeds by default; use an injected error.
	pkg := "example.com/nope@latest"
	h.runner.WithInstallError(pkg, errors.New("go install: module lookup disabled"))
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "example.com/nope"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created")
	}
}

// U5 — dry-run skips runner and hamsfile.
func TestHandleCommand_U5_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newGoInstallHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.org/x/tools/cmd/goimports"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on dry-run")
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
}

// U6 — multi-path install records every package on full success.
func TestHandleCommand_U6_MultiPathInstallRecordsAll(t *testing.T) {
	h := newGoInstallHarness(t)
	args := []string{"install", "golang.org/x/tools/cmd/goimports", "honnef.co/go/tools/cmd/staticcheck"}
	if err := h.provider.HandleCommand(context.Background(), args, nil, h.flags); err != nil {
		t.Fatalf("multi install: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{
		"golang.org/x/tools/cmd/goimports@latest":   true,
		"honnef.co/go/tools/cmd/staticcheck@latest": true,
	}
	for _, a := range apps {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing apps: %v", want)
	}
}

// U7 — multi-path install is atomic on partial failure.
func TestHandleCommand_U7_MultiPathInstallAtomicOnFailure(t *testing.T) {
	h := newGoInstallHarness(t)
	h.runner.WithInstallError("example.com/broken@latest", errors.New("fail"))
	err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "example.com/ok", "example.com/broken"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

// U8 — go-install flags must not leak into the recorded paths.
func TestHandleCommand_U8_FlagsNotRecordedAsPaths(t *testing.T) {
	h := newGoInstallHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "-v", "golang.org/x/tools/cmd/goimports"}, nil, h.flags); err != nil {
		t.Fatalf("flagged install: %v", err)
	}
	apps := h.hamsfileApps()
	want := "golang.org/x/tools/cmd/goimports@latest"
	if len(apps) != 1 || apps[0] != want {
		t.Errorf("hamsfile apps = %v, want [%q]", apps, want)
	}
}

// U9 — flags-only returns usage error.
func TestHandleCommand_U9_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newGoInstallHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "-v"}, nil, h.flags); err == nil {
		t.Fatal("expected usage error")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called")
	}
}

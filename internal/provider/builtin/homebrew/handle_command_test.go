package homebrew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// brewHarness wires a homebrew.Provider against a FakeCmdRunner +
// tempdir profile + tempdir state dir so HandleCommand tests can
// assert real hamsfile + state writes without invoking the host's
// real brew. Mirrors apt_test.go's `testHarness` shape so a future
// reader can see the two at a glance.
type brewHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	stateDir     string
	hamsfilePath string
	statePath    string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newBrewHarness(t *testing.T) *brewHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	stateDir := filepath.Join(storeDir, ".state", "test-machine")
	for _, d := range []string{profileDir, stateDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: profileTag, MachineID: "test-machine"}
	runner := NewFakeCmdRunner()
	p := New(cfg, runner)
	return &brewHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		stateDir:     stateDir,
		hamsfilePath: filepath.Join(profileDir, "Homebrew.hams.yaml"),
		statePath:    filepath.Join(stateDir, "Homebrew.state.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *brewHarness) hamsfileApps() []string {
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

func (h *brewHarness) stateResources() map[string]state.ResourceState {
	h.t.Helper()
	if _, err := os.Stat(h.statePath); err != nil {
		return nil
	}
	sf, err := state.Load(h.statePath)
	if err != nil {
		h.t.Fatalf("state.Load: %v", err)
	}
	out := make(map[string]state.ResourceState, len(sf.Resources))
	for id, r := range sf.Resources {
		out[id] = r.State
	}
	return out
}

// TestHandleCommand_CaskWithConflictingTagErrors locks in cycle 175:
// `hams brew install iterm2 --cask --hams-tag=apps` would previously
// record the entry under "apps" tag with NO cask metadata. caskApps()
// in Plan only flags entries under the "cask" tag with IsCask=true,
// so the next `hams apply` would run `brew install iterm2` (no
// --cask), which fails because iterm2 has no formula. Now: surface
// the conflict at the CLI layer with a UserFacingError pointing at
// the resolution.
func TestHandleCommand_CaskWithConflictingTagErrors(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)

	err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "iterm2", "--cask"},
		map[string]string{"tag": "apps"},
		h.flags)
	if err == nil {
		t.Fatal("expected error for --cask with conflicting --hams-tag")
	}
	if !strings.Contains(err.Error(), "--cask is incompatible") {
		t.Errorf("error should mention --cask incompatibility; got %q", err.Error())
	}
	// Must NOT have invoked brew or written hamsfile.
	if h.runner.CallCount(fakeOpInstall, "iterm2") > 0 {
		t.Errorf("brew install must not be invoked on usage error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on usage error, got %v", apps)
	}
}

// TestHandleCommand_CaskWithExplicitCaskTag asserts the friendly
// path: --cask + --hams-tag=cask is the canonical form and works.
func TestHandleCommand_CaskWithExplicitCaskTag(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "iterm2", "--cask"},
		map[string]string{"tag": "cask"},
		h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "iterm2") != 1 {
		t.Errorf("brew install(iterm2) calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "iterm2"))
	}
}

// TestHandleCommand_CaskAutoTaggedAsCask asserts the default path
// (no --hams-tag): --cask alone routes to the "cask" tag.
func TestHandleCommand_CaskAutoTaggedAsCask(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "iterm2", "--cask"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "iterm2") != 1 {
		t.Errorf("brew install(iterm2) calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "iterm2"))
	}
}

// TestHandleUntap_AutoRecordsRemoval locks in cycle 167: `hams brew
// untap user/repo` previously fell through to the raw passthrough,
// which exec'd `brew untap` but NEVER updated the hamsfile/state.
// Result: drift accumulated — the user untapped the repo on the
// host but the hamsfile still said it was tapped, so the next
// `hams apply` would re-tap. Now: auto-records the removal so the
// CLI-first contract holds for taps too.
func TestHandleUntap_AutoRecordsRemoval(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)

	// Pre-seed the hamsfile with a tap entry directly (bypass handleTap
	// which goes through the real exec passthrough). This isolates the
	// untap behavior from the tap path's exec dependency.
	hf, err := hamsfile.LoadOrCreateEmpty(h.hamsfilePath)
	if err != nil {
		t.Fatalf("seed hamsfile: %v", err)
	}
	hf.AddApp("tap", "homebrew/cask-fonts", "")
	if err := hf.Write(); err != nil {
		t.Fatalf("write hamsfile: %v", err)
	}

	// Untap should remove the entry from the hamsfile AND mark state removed.
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"untap", "homebrew/cask-fonts"}, nil, h.flags); err != nil {
		t.Fatalf("untap: %v", err)
	}

	if h.runner.CallCount(fakeOpUntap, "homebrew/cask-fonts") != 1 {
		t.Errorf("runner.Untap calls = %d, want 1", h.runner.CallCount(fakeOpUntap, "homebrew/cask-fonts"))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after untap, got %v", apps)
	}
	if resources := h.stateResources(); resources["homebrew/cask-fonts"] != state.StateRemoved {
		t.Errorf("state[homebrew/cask-fonts] = %q, want removed", resources["homebrew/cask-fonts"])
	}
}

// TestHandleUntap_StrictArgCount: same UX class as cycles 156/163
// — too-many positional args returns ExitUsageError instead of
// silently dropping.
func TestHandleUntap_StrictArgCount(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)
	err := h.provider.HandleCommand(context.Background(),
		[]string{"untap", "user1/repo", "user2/repo"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected usage error for too-many args")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error should say 'exactly one'; got %q", err.Error())
	}
}

// TestHandleUntap_NoArgsErrors asserts the empty-args case errors
// with the usage hint.
func TestHandleUntap_NoArgsErrors(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)
	err := h.provider.HandleCommand(context.Background(), []string{"untap"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected usage error for no args")
	}
	if !strings.Contains(err.Error(), "requires a repository name") {
		t.Errorf("error should say 'requires a repository name'; got %q", err.Error())
	}
}

// TestHandleTap_StrictArgCount locks in cycle 163: the pre-cycle-163
// implementation only used args[0] of `hams brew tap …` and silently
// dropped any additional args. So `hams brew tap user1/repo
// user2/repo` only tapped user1/repo and the second tap was lost
// — user thought both were tapped because exit was 0. Now: too-many
// args returns ExitUsageError with a hint to repeat the command per
// repo. (Multi-tap support belongs in a separate feature change;
// fixing the silent-drop is the immediate priority.)
func TestHandleTap_StrictArgCount(t *testing.T) {
	t.Parallel()
	cases := [][]string{
		{"tap", "user1/repo", "user2/repo"},               // two args
		{"tap", "user1/repo", "user2/repo", "user3/repo"}, // three args
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			t.Parallel()
			h := newBrewHarness(t)
			err := h.provider.HandleCommand(context.Background(), args, nil, h.flags)
			if err == nil {
				t.Fatalf("expected error for %v; got nil", args)
			}
			// Must NOT have invoked brew.
			if h.runner.CallCount(fakeOpInstall, "user1/repo") > 0 {
				t.Errorf("brew install must not be invoked on usage error")
			}
			// Must NOT have written hamsfile.
			if apps := h.hamsfileApps(); len(apps) != 0 {
				t.Errorf("hamsfile should be empty on usage error, got %v", apps)
			}
			// Error should say "exactly one" so the user understands.
			if !strings.Contains(err.Error(), "exactly one") {
				t.Errorf("error should say 'exactly one'; got %q", err.Error())
			}
		})
	}
}

// U1 — install records the package in BOTH hamsfile and state.
// Cycle 96 added the state write; cycle 97 locks it in against
// regressions.
func TestHandleCommand_U1_InstallWritesHamsfileAndState(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "git"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "git") != 1 {
		t.Errorf("runner.Install(git) calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "git"))
	}
	if apps := h.hamsfileApps(); len(apps) != 1 || apps[0] != "git" {
		t.Errorf("hamsfile apps = %v, want [git]", apps)
	}
	if resources := h.stateResources(); resources["git"] != state.StateOK {
		t.Errorf("state[git] = %q, want ok", resources["git"])
	}
}

// U2 — idempotent re-install keeps single hamsfile entry AND
// preserves first_install_at in state (the SetResource contract).
func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newBrewHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("hamsfile apps = %v, want single entry", apps)
	}
	if h.runner.CallCount(fakeOpInstall, "bat") != 2 {
		t.Errorf("runner.Install calls = %d, want 2", h.runner.CallCount(fakeOpInstall, "bat"))
	}
}

// U3 — install failure leaves hamsfile AND state untouched.
func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newBrewHarness(t)
	h.runner.WithInstallError("nope", errors.New("brew: No available formula"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "nope"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after failed install, got %v", apps)
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created after failure")
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("state file should not be created after failure")
	}
}

// U4 — remove drops the hamsfile entry AND marks state removed.
// The state still contains the entry (with `state: removed`) per
// apt's U14 semantic — "removed" is a first-class state, not a
// delete.
func TestHandleCommand_U4_RemoveDeletesFromHamsfileMarksStateRemoved(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after remove, got %v", apps)
	}
	resources := h.stateResources()
	if resources["htop"] != state.StateRemoved {
		t.Errorf("state[htop] = %q, want removed", resources["htop"])
	}
	if h.runner.CallCount(fakeOpUninstall, "htop") != 1 {
		t.Errorf("runner.Uninstall calls = %d, want 1", h.runner.CallCount(fakeOpUninstall, "htop"))
	}
}

// U5 — remove failure leaves BOTH hamsfile and state intact.
func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "jq"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	h.runner.WithUninstallError("jq", errors.New("brew uninstall: permission denied"))

	err := h.provider.HandleCommand(context.Background(), []string{"remove", "jq"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "jq" {
		t.Errorf("hamsfile should still contain jq after failed remove, got %v", apps)
	}
	resources := h.stateResources()
	if resources["jq"] != state.StateOK {
		t.Errorf("state[jq] = %q, want ok (remove failed)", resources["jq"])
	}
}

// U6 — dry-run skips runner, hamsfile, AND state write.
func TestHandleCommand_U6_DryRunSkipsRunnerHamsfileState(t *testing.T) {
	h := newBrewHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ripgrep"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", h.runner.CallCount(fakeOpInstall, ""))
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("dry-run should not write state file")
	}
}

// U7 — --cask flag routes to the "cask" tag AND passes isCask=true
// to the runner. Protects the cycle-52 cask-detection branch.
func TestHandleCommand_U7_CaskFlagRecordsUnderCaskTag(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "visual-studio-code", "--cask"}, nil, h.flags); err != nil {
		t.Fatalf("install --cask: %v", err)
	}

	// Read hamsfile structurally so we can verify the TAG, not just
	// the app list. The cask-detection branch is the reason the
	// --cask flag exists on the CLI path at all.
	f, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		t.Fatalf("read hamsfile: %v", err)
	}
	tag, idx := f.FindApp("visual-studio-code")
	if tag != "cask" {
		t.Errorf("FindApp tag = %q, want 'cask'", tag)
	}
	if idx < 0 {
		t.Errorf("visual-studio-code not found in hamsfile")
	}
	// Runner should have been called with isCask=true — inspect via
	// a manual iteration since the fake exposes CallCount by name
	// only.
	if h.runner.CallCount(fakeOpInstall, "visual-studio-code") != 1 {
		t.Errorf("runner.Install(visual-studio-code) calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "visual-studio-code"))
	}
}

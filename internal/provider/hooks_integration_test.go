package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// hookStubProvider satisfies the Provider interface for hooks-integration
// tests. Apply just records which actions it saw; the executor's
// pre/post hook dispatch runs via the production code path.
type hookStubProvider struct {
	applied []string
}

func (s *hookStubProvider) Manifest() Manifest {
	return Manifest{Name: "stub", DisplayName: "Stub", FilePrefix: "stub"}
}
func (s *hookStubProvider) Bootstrap(_ context.Context) error { return nil }
func (s *hookStubProvider) Probe(_ context.Context, _ *state.File) ([]ProbeResult, error) {
	return nil, nil
}
func (s *hookStubProvider) Plan(_ context.Context, _ *hamsfile.File, _ *state.File) ([]Action, error) {
	return nil, nil
}
func (s *hookStubProvider) Apply(_ context.Context, action Action) error {
	s.applied = append(s.applied, action.ID)
	return nil
}
func (s *hookStubProvider) Remove(_ context.Context, _ string) error { return nil }
func (s *hookStubProvider) List(_ context.Context, _ *hamsfile.File, _ *state.File) (string, error) {
	return "", nil
}

// TestHooks_EndToEnd_ProducesSideEffect verifies the full hamsfile →
// Plan → Execute → runHook pipeline: a declared pre_install hook
// that writes to a tempfile must actually leave that file on disk
// after Apply runs.
//
// This is the first test that exercises the complete integration
// from YAML parsing through the executor's hook dispatch — unit
// tests cover each step in isolation (hooks_parse_test.go for
// parsing, executor_test.go for dispatch), but the end-to-end wiring
// lived only behind Docker-based integration tests until now.
func TestHooks_EndToEnd_ProducesSideEffect(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	marker := filepath.Join(dir, "hook-fired")

	// Synthesize a hamsfile with a pre_install hook that writes a
	// marker file so the test can verify the hook actually ran.
	yamlBody := "cli:\n" +
		"  - app: htop\n" +
		"    hooks:\n" +
		"      pre_install:\n" +
		"        - run: touch " + marker + "\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlBody), &doc); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	hf := &hamsfile.File{Path: "test", Root: &doc}

	// Build an install action for htop and populate its Hooks via
	// the production code path.
	actions := []Action{{ID: "htop", Type: ActionInstall}}
	actions = PopulateActionHooks(actions, hf)
	if actions[0].Hooks == nil || len(actions[0].Hooks.PreInstall) != 1 {
		t.Fatalf("pre_install hook did not populate; actions[0].Hooks = %+v", actions[0].Hooks)
	}

	// Execute via the real Execute path so runPhasePreHooks →
	// RunPreInstallHooks → runHook chain runs exactly as in
	// production. hookStubProvider.Apply records the install; the
	// executor's pre-hook dispatch fires first.
	stub := &hookStubProvider{}
	sf := state.New("stub", "test-machine")
	result := Execute(context.Background(), stub, actions, sf)

	if result.Failed != 0 {
		t.Fatalf("execute reported %d failed actions; errors=%v", result.Failed, result.Errors)
	}

	// Verify the marker file exists — the hook actually ran on the
	// host shell.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("pre_install hook did not produce expected marker %q: %v", marker, err)
	}

	// And the apply path still completed.
	if len(stub.applied) != 1 || stub.applied[0] != "htop" {
		t.Errorf("expected Apply to see [htop]; got %v", stub.applied)
	}
}

// TestHooks_EndToEnd_DeferredPostInstall_RunsAfterProvider asserts
// that defer:true post_install hooks are collected and executed via
// RunDeferredHooks rather than inline. Proves the full deferred-hook
// wiring works end-to-end.
func TestHooks_EndToEnd_DeferredPostInstall_RunsAfterProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inline := filepath.Join(dir, "inline-fired")
	deferred := filepath.Join(dir, "deferred-fired")

	yamlBody := "cli:\n" +
		"  - app: htop\n" +
		"    hooks:\n" +
		"      post_install:\n" +
		"        - run: touch " + inline + "\n" +
		"        - run: touch " + deferred + "\n" +
		"          defer: true\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlBody), &doc); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	hf := &hamsfile.File{Path: "test", Root: &doc}

	actions := []Action{{ID: "htop", Type: ActionInstall}}
	actions = PopulateActionHooks(actions, hf)

	// Execute runs the non-deferred hooks inline.
	stub := &hookStubProvider{}
	sf := state.New("stub", "test-machine")
	result := Execute(context.Background(), stub, actions, sf)
	if result.Failed != 0 {
		t.Fatalf("execute reported %d failed; errors=%v", result.Failed, result.Errors)
	}

	// Inline hook fired during Execute.
	if _, err := os.Stat(inline); err != nil {
		t.Fatalf("inline post_install hook did not produce marker: %v", err)
	}
	// Deferred hook should NOT have fired yet.
	if _, err := os.Stat(deferred); err == nil {
		t.Fatalf("deferred hook should NOT fire inline; marker present")
	}

	// Collect deferred hooks from the executed actions and run them.
	var deferredHooks []DeferredHook
	for _, a := range actions {
		if a.Hooks != nil {
			deferredHooks = append(deferredHooks, CollectDeferredHooks(a.ID, a.Hooks.PostInstall)...)
		}
	}
	errs := RunDeferredHooks(context.Background(), deferredHooks, sf)
	if len(errs) != 0 {
		t.Fatalf("RunDeferredHooks errors: %v", errs)
	}
	if _, err := os.Stat(deferred); err != nil {
		t.Fatalf("deferred hook did not fire: %v", err)
	}
}

package provider

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// parseYAMLHookNode helps tests build yaml.Node trees of the shape
// AppHookNode would return.
func parseYAMLHookNode(t *testing.T, raw string) *yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return &n
}

// TestParseHookSet_ReturnsNilForNonMapping asserts a nil/non-mapping
// node returns nil (no panic).
func TestParseHookSet_ReturnsNilForNonMapping(t *testing.T) {
	t.Parallel()
	if hs := ParseHookSet(nil); hs != nil {
		t.Errorf("nil node should return nil, got %+v", hs)
	}
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: "not-a-map"}
	if hs := ParseHookSet(scalar); hs != nil {
		t.Errorf("scalar should return nil, got %+v", hs)
	}
}

// TestParseHookSet_ReturnsNilForEmptyMapping asserts that an empty
// `hooks: {}` mapping yields nil — no Hooks means no work for the
// executor and lets the caller skip the field entirely.
func TestParseHookSet_ReturnsNilForEmptyMapping(t *testing.T) {
	t.Parallel()
	node := parseYAMLHookNode(t, "{}")
	if hs := ParseHookSet(node); hs != nil {
		t.Errorf("empty mapping should return nil, got %+v", hs)
	}
}

// TestParseHookSet_ParsesAllFourPhases asserts that pre_install,
// post_install, pre_update, and post_update keys all populate their
// respective HookSet slices.
func TestParseHookSet_ParsesAllFourPhases(t *testing.T) {
	t.Parallel()
	node := parseYAMLHookNode(t, `pre_install:
  - run: echo pre-install
post_install:
  - run: echo post-install
pre_update:
  - run: echo pre-update
post_update:
  - run: echo post-update
`)
	hs := ParseHookSet(node)
	if hs == nil {
		t.Fatal("ParseHookSet returned nil for non-empty input")
	}
	if len(hs.PreInstall) != 1 || hs.PreInstall[0].Command != "echo pre-install" {
		t.Errorf("PreInstall = %+v", hs.PreInstall)
	}
	if len(hs.PostInstall) != 1 || hs.PostInstall[0].Command != "echo post-install" {
		t.Errorf("PostInstall = %+v", hs.PostInstall)
	}
	if len(hs.PreUpdate) != 1 || hs.PreUpdate[0].Command != "echo pre-update" {
		t.Errorf("PreUpdate = %+v", hs.PreUpdate)
	}
	if len(hs.PostUpdate) != 1 || hs.PostUpdate[0].Command != "echo post-update" {
		t.Errorf("PostUpdate = %+v", hs.PostUpdate)
	}
}

// TestParseHookSet_PreservesHookType asserts each parsed Hook has
// the right Type — the executor uses Type to select the correct
// dispatch path.
func TestParseHookSet_PreservesHookType(t *testing.T) {
	t.Parallel()
	node := parseYAMLHookNode(t, `pre_install:
  - run: x
post_install:
  - run: y
`)
	hs := ParseHookSet(node)
	if hs.PreInstall[0].Type != HookPreInstall {
		t.Errorf("PreInstall hook Type = %v, want HookPreInstall", hs.PreInstall[0].Type)
	}
	if hs.PostInstall[0].Type != HookPostInstall {
		t.Errorf("PostInstall hook Type = %v, want HookPostInstall", hs.PostInstall[0].Type)
	}
}

// TestParseHookSet_ParsesDeferTrue asserts that `defer: true` (and
// the YAML 1.1 variants yes/on/1) are accepted; the default is false.
func TestParseHookSet_ParsesDeferTrue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		raw       string
		wantDefer bool
	}{
		{"defer-true", `post_install:
  - run: x
    defer: true`, true},
		{"defer-yes", `post_install:
  - run: x
    defer: yes`, true},
		{"defer-on", `post_install:
  - run: x
    defer: on`, true},
		{"defer-1", `post_install:
  - run: x
    defer: 1`, true},
		{"defer-false", `post_install:
  - run: x
    defer: false`, false},
		{"defer-absent", `post_install:
  - run: x`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node := parseYAMLHookNode(t, tc.raw)
			hs := ParseHookSet(node)
			if got := hs.PostInstall[0].Defer; got != tc.wantDefer {
				t.Errorf("Defer = %v, want %v", got, tc.wantDefer)
			}
		})
	}
}

// TestParseHookSet_SkipsEntriesWithoutRun asserts that hook items
// missing `run:` are dropped — an empty-command Hook would silently
// pass through to the executor and be a no-op shell call.
func TestParseHookSet_SkipsEntriesWithoutRun(t *testing.T) {
	t.Parallel()
	node := parseYAMLHookNode(t, `pre_install:
  - defer: true
  - run: actual command
  - run: ""
`)
	hs := ParseHookSet(node)
	if len(hs.PreInstall) != 1 {
		t.Fatalf("expected 1 well-formed Hook (others dropped), got %d: %+v",
			len(hs.PreInstall), hs.PreInstall)
	}
	if hs.PreInstall[0].Command != "actual command" {
		t.Errorf("kept hook = %+v, want command='actual command'", hs.PreInstall[0])
	}
}

// TestParseHookSet_UnknownKeysSilentlySkipped asserts forward-compat
// with v1.1 hook keys (e.g., a hypothetical pre_remove): unknown keys
// are silently dropped, not panicked over.
func TestParseHookSet_UnknownKeysSilentlySkipped(t *testing.T) {
	t.Parallel()
	node := parseYAMLHookNode(t, `pre_remove:
  - run: cleanup
post_install:
  - run: ok
`)
	hs := ParseHookSet(node)
	if hs == nil {
		t.Fatal("nil result")
	}
	if len(hs.PostInstall) != 1 {
		t.Errorf("PostInstall = %v, want 1", hs.PostInstall)
	}
}

// TestPopulateActionHooks_AttachesHookSetForMatchingApp tests the
// integration through hamsfile + ParseHookSet. The hamsfile in the
// test has hooks for "vim" and not for "nano"; only the vim action
// gets Hooks populated.
func TestPopulateActionHooks_AttachesHookSetForMatchingApp(t *testing.T) {
	t.Parallel()
	// Build a hamsfile in-memory with a vim entry having hooks and
	// a nano entry without.
	body := `cli:
  - app: vim
    hooks:
      post_install:
        - run: echo "vim ready"
  - app: nano
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	hf := &hamsfile.File{Path: "test", Root: &doc}

	actions := []Action{
		{ID: "vim", Type: ActionInstall},
		{ID: "nano", Type: ActionInstall},
	}
	got := PopulateActionHooks(actions, hf)

	if got[0].Hooks == nil {
		t.Errorf("vim action should have Hooks, got nil")
	} else if len(got[0].Hooks.PostInstall) != 1 || got[0].Hooks.PostInstall[0].Command != `echo "vim ready"` {
		t.Errorf("vim Hooks = %+v", got[0].Hooks)
	}

	if got[1].Hooks != nil {
		t.Errorf("nano action should NOT have Hooks (none declared), got %+v", got[1].Hooks)
	}
}

// TestPopulateActionHooks_NilDesiredIsSafe asserts that passing nil
// (e.g., a synthesized empty hamsfile in prune-orphan mode) does not
// panic and leaves actions unchanged.
func TestPopulateActionHooks_NilDesiredIsSafe(t *testing.T) {
	t.Parallel()
	actions := []Action{{ID: "x", Type: ActionInstall}}
	got := PopulateActionHooks(actions, nil)
	if got[0].Hooks != nil {
		t.Errorf("expected nil Hooks; got %+v", got[0].Hooks)
	}
}

// TestPopulateActionHooks_EndToEndIntegration tests that hooks
// populated through this flow execute correctly via the existing
// runPhasePreHooks logic. We don't call the real executor here —
// this just verifies the wiring produces a HookSet shape the
// executor accepts.
func TestPopulateActionHooks_EndToEndIntegration(t *testing.T) {
	t.Parallel()
	body := `cli:
  - app: vim
    hooks:
      pre_install:
        - run: "true"
      post_install:
        - run: "true"
        - run: "true"
          defer: true
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	hf := &hamsfile.File{Path: "test", Root: &doc}

	actions := []Action{{ID: "vim", Type: ActionInstall}}
	got := PopulateActionHooks(actions, hf)

	hs := got[0].Hooks
	if hs == nil {
		t.Fatal("expected Hooks populated")
	}
	if len(hs.PreInstall) != 1 || len(hs.PostInstall) != 2 {
		t.Fatalf("hook counts: pre=%d post=%d", len(hs.PreInstall), len(hs.PostInstall))
	}
	if !hs.PostInstall[1].Defer {
		t.Errorf("second post-install hook should have defer=true")
	}

	// Verify CollectDeferredHooks (existing engine surface) sees the
	// deferred entry — proves the parsed Hook is shape-compatible
	// with the executor's downstream consumers.
	deferred := CollectDeferredHooks("vim", hs.PostInstall)
	if len(deferred) != 1 {
		t.Errorf("CollectDeferredHooks = %d, want 1", len(deferred))
	}

	// And the executor's hook-failed state path uses state.File —
	// ensure we can construct one without errors. (Not invoking the
	// runner; just confirming type compatibility.)
	_ = state.New("test", "test-machine")
}

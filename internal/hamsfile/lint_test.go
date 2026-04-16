package hamsfile

import (
	"path/filepath"
	"testing"
)

// TestLintDeferredFeatures_NoHooks asserts an empty hamsfile yields
// an empty DeferredFeatures result (the warning path is the
// exception, not the rule).
func TestLintDeferredFeatures_NoHooks(t *testing.T) {
	t.Parallel()
	f := NewEmpty(filepath.Join(t.TempDir(), "Homebrew.hams.yaml"))
	got := LintDeferredFeatures(f)
	if got.HasAny() {
		t.Errorf("empty hamsfile should report no deferred features; got %+v", got)
	}
}

// TestLintDeferredFeatures_DetectsHooksUnderApp asserts that a
// `hooks:` block with at least one of the four recognized hook keys
// trips the warning. The reported ID is the `app` value so the user
// can navigate directly to the offending entry.
func TestLintDeferredFeatures_DetectsHooksUnderApp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	yamlBody := []byte(`cli:
  - app: htop
    intro: "Process viewer"
  - app: visual-studio-code
    hooks:
      post_install:
        - run: hams code-ext install ms-python.python
`)
	if err := writeTestFile(t, path, yamlBody); err != nil {
		t.Fatalf("setup error: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	got := LintDeferredFeatures(f)
	if !got.HasAny() {
		t.Fatalf("expected hooks warning to trigger; got empty")
	}
	if len(got.HookEntries) != 1 || got.HookEntries[0] != "visual-studio-code" {
		t.Errorf("HookEntries = %v, want [visual-studio-code]", got.HookEntries)
	}
}

// TestLintDeferredFeatures_DetectsHooksUnderURN asserts URN-based
// items (script-type providers) are also covered.
func TestLintDeferredFeatures_DetectsHooksUnderURN(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bash.hams.yaml")
	yamlBody := []byte(`scripts:
  - urn: urn:hams:bash:install-deps
    run: brew install something
    hooks:
      pre_install:
        - run: echo "preparing"
`)
	if err := writeTestFile(t, path, yamlBody); err != nil {
		t.Fatalf("setup error: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	got := LintDeferredFeatures(f)
	if len(got.HookEntries) != 1 || got.HookEntries[0] != "urn:hams:bash:install-deps" {
		t.Errorf("HookEntries = %v, want [urn:hams:bash:install-deps]", got.HookEntries)
	}
}

// TestLintDeferredFeatures_EmptyHooksBlockIsNotFlagged asserts that a
// `hooks: {}` literal does NOT trigger the warning. Some providers'
// auto-record paths emit empty mappings; users who intentionally keep
// the structural placeholder shouldn't be spammed.
func TestLintDeferredFeatures_EmptyHooksBlockIsNotFlagged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	yamlBody := []byte(`cli:
  - app: htop
    hooks: {}
`)
	if err := writeTestFile(t, path, yamlBody); err != nil {
		t.Fatalf("setup error: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	got := LintDeferredFeatures(f)
	if got.HasAny() {
		t.Errorf("empty hooks: {} should NOT trigger warning; got %+v", got)
	}
}

// TestLintDeferredFeatures_HooksWithEmptyArraysIsNotFlagged asserts
// that `hooks: { pre_install: [] }` (declared key, no entries) is
// also not flagged. Users who declared the structure but cleared the
// list intentionally shouldn't get noise.
func TestLintDeferredFeatures_HooksWithEmptyArraysIsNotFlagged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	yamlBody := []byte(`cli:
  - app: htop
    hooks:
      pre_install: []
      post_install: []
`)
	if err := writeTestFile(t, path, yamlBody); err != nil {
		t.Fatalf("setup error: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	got := LintDeferredFeatures(f)
	if got.HasAny() {
		t.Errorf("hooks with empty arrays should NOT trigger warning; got %+v", got)
	}
}

// TestLintDeferredFeatures_MultipleHookedApps verifies the result
// preserves all matching IDs in stable order (so a multi-app warning
// names every offender).
func TestLintDeferredFeatures_MultipleHookedApps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	yamlBody := []byte(`cli:
  - app: vim
    hooks:
      post_install:
        - run: echo a
  - app: emacs
  - app: nano
    hooks:
      pre_install:
        - run: echo b
`)
	if err := writeTestFile(t, path, yamlBody); err != nil {
		t.Fatalf("setup error: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	got := LintDeferredFeatures(f)
	if len(got.HookEntries) != 2 {
		t.Fatalf("HookEntries = %v, want 2", got.HookEntries)
	}
	want := map[string]bool{"vim": true, "nano": true}
	for _, id := range got.HookEntries {
		if !want[id] {
			t.Errorf("unexpected ID in HookEntries: %q", id)
		}
	}
}

// writeTestFile is a small helper to write yaml content to a tempdir
// path inside tests. Returns nil on success, error otherwise.
func writeTestFile(t *testing.T, path string, body []byte) error {
	t.Helper()
	if err := AtomicWrite(path, body); err != nil {
		return err
	}
	return nil
}

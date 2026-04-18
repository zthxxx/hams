package hamsfile

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAppHookNode_ReturnsNilForEmpty asserts an empty hamsfile
// returns nil for any app ID.
func TestAppHookNode_ReturnsNilForEmpty(t *testing.T) {
	t.Parallel()
	f := NewEmpty(filepath.Join(t.TempDir(), "empty.hams.yaml"))
	if node := f.AppHookNode("anything"); node != nil {
		t.Errorf("empty hamsfile should return nil; got %+v", node)
	}
}

// TestAppHookNode_ReturnsNilWhenAppHasNoHooks asserts an item without
// a `hooks:` key yields nil (vs an empty mapping or panic).
func TestAppHookNode_ReturnsNilWhenAppHasNoHooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	body := []byte(`cli:
  - app: htop
    intro: "Process viewer"
`)
	if err := AtomicWrite(path, body); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if node := f.AppHookNode("htop"); node != nil {
		t.Errorf("htop has no hooks; should return nil, got %+v", node)
	}
}

// TestAppHookNode_ReturnsMatchingNode asserts the returned node IS
// the mapping value of `hooks:` and that re-parsing it preserves the
// hook keys.
func TestAppHookNode_ReturnsMatchingNode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	body := []byte(`cli:
  - app: visual-studio-code
    hooks:
      pre_install:
        - run: echo "before"
      post_install:
        - run: defaults write com.example value -bool true
        - run: hams code install ms-python.python
          defer: true
`)
	if err := AtomicWrite(path, body); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	node := f.AppHookNode("visual-studio-code")
	if node == nil {
		t.Fatal("expected non-nil hook node")
	}
	if node.Kind != yaml.MappingNode {
		t.Errorf("hook node kind = %v, want MappingNode", node.Kind)
	}

	// Sanity-check that pre_install + post_install keys are present.
	keys := map[string]bool{}
	for k := 0; k < len(node.Content)-1; k += 2 {
		keys[node.Content[k].Value] = true
	}
	if !keys["pre_install"] || !keys["post_install"] {
		t.Errorf("hook node missing expected keys; got %v", keys)
	}
}

// TestAppHookNode_FindsURNItem asserts script-type items (urn:
// instead of app:) are also matched.
func TestAppHookNode_FindsURNItem(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bash.hams.yaml")
	body := []byte(`scripts:
  - urn: urn:hams:bash:install-deps
    run: brew install something
    hooks:
      pre_install:
        - run: echo "preparing"
`)
	if err := AtomicWrite(path, body); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	node := f.AppHookNode("urn:hams:bash:install-deps")
	if node == nil {
		t.Fatal("expected non-nil hook node for URN item")
	}
}

// TestAppHookNode_NonMatchingAppReturnsNil asserts a stranger app ID
// returns nil even when the hamsfile has hooks for OTHER apps.
func TestAppHookNode_NonMatchingAppReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "Homebrew.hams.yaml")
	body := []byte(`cli:
  - app: vim
    hooks:
      post_install:
        - run: echo a
`)
	if err := AtomicWrite(path, body); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if node := f.AppHookNode("emacs"); node != nil {
		t.Errorf("emacs not in hamsfile; should return nil")
	}
}

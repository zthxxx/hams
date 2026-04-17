package storeinit_test

import (
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/storeinit"
)

func TestBootstrap_FreshDirIsValidStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "store")

	if err := storeinit.Bootstrap(target); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if !storeinit.Bootstrapped(target) {
		t.Fatalf("Bootstrapped(%s) = false, want true after Bootstrap", target)
	}

	for _, want := range []string{".gitignore", "hams.config.yaml", "default", ".git"} {
		path := filepath.Join(target, want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist after Bootstrap, got: %v", path, err)
		}
	}
}

func TestBootstrap_EmptyPathRejected(t *testing.T) {
	t.Parallel()
	if err := storeinit.Bootstrap(""); err == nil {
		t.Fatal("Bootstrap(\"\") returned nil, want error")
	}
}

func TestBootstrap_PreservesExistingFiles(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "store")
	if err := storeinit.Bootstrap(target); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}

	custom := []byte("# user-edited config\nstore_path: /elsewhere\n")
	configPath := filepath.Join(target, "hams.config.yaml")
	if err := os.WriteFile(configPath, custom, 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	if err := storeinit.Bootstrap(target); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading after second Bootstrap: %v", err)
	}
	if string(got) != string(custom) {
		t.Errorf("Bootstrap clobbered hand-edited config\nwant: %q\ngot:  %q", custom, got)
	}
}

// TestBootstrap_PropertyIdempotent encodes the invariant: re-running Bootstrap
// repeatedly on a fresh temp dir always produces the same valid store and
// never errors. Property-based per CLAUDE.md testing convention.
func TestBootstrap_PropertyIdempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		repeats := rapid.IntRange(1, 6).Draw(t, "repeats")
		dir := filepath.Join(testTempDir(t), "store")
		for i := range repeats {
			if err := storeinit.Bootstrap(dir); err != nil {
				t.Fatalf("Bootstrap iteration %d: %v", i, err)
			}
			if !storeinit.Bootstrapped(dir) {
				t.Fatalf("Bootstrapped(%s) = false after iteration %d", dir, i)
			}
		}
	})
}

func TestBootstrap_NestedNonExistentParents(t *testing.T) {
	t.Parallel()
	deep := filepath.Join(t.TempDir(), "a", "b", "c", "store")
	if err := storeinit.Bootstrap(deep); err != nil {
		t.Fatalf("Bootstrap on deeply nested path: %v", err)
	}
	if !storeinit.Bootstrapped(deep) {
		t.Fatalf("Bootstrapped(%s) = false", deep)
	}
}

func testTempDir(t *rapid.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "storeinit-rapid-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			t.Errorf("temp dir cleanup: %v", rmErr)
		}
	})
	return dir
}

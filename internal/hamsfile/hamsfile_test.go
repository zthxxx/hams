package hamsfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

const sampleYAML = `# Homebrew packages
development-tool:
  - app: htop
    intro: Improved top (interactive process viewer)
  - app: jq
    intro: Lightweight JSON processor

# Terminal utilities
terminal-tool:
  - app: lazygit
    intro: Simple terminal UI for git commands
`

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.hams.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestRead_EmptyFile(t *testing.T) {
	path := writeTempFile(t, "empty: []\n")
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	tags := f.Tags()
	if len(tags) != 1 || tags[0] != "empty" {
		t.Errorf("Tags() = %v, want [empty]", tags)
	}
}

func TestRead_ParsesTags(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	tags := f.Tags()
	if len(tags) != 2 {
		t.Fatalf("Tags() = %v, want 2 tags", tags)
	}
	if tags[0] != "development-tool" || tags[1] != "terminal-tool" {
		t.Errorf("Tags() = %v, want [development-tool, terminal-tool]", tags)
	}
}

func TestFindApp_Found(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	tag, idx := f.FindApp("lazygit")
	if tag != "terminal-tool" || idx != 0 {
		t.Errorf("FindApp('lazygit') = (%q, %d), want ('terminal-tool', 0)", tag, idx)
	}
}

func TestFindApp_NotFound(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	_, idx := f.FindApp("nonexistent")
	if idx != -1 {
		t.Errorf("FindApp('nonexistent') index = %d, want -1", idx)
	}
}

func TestAddApp_ExistingTag(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	f.AddApp("development-tool", "ripgrep", "Fast recursive grep")

	tag, idx := f.FindApp("ripgrep")
	if tag != "development-tool" || idx < 0 {
		t.Errorf("after AddApp, FindApp('ripgrep') = (%q, %d), want ('development-tool', >=0)", tag, idx)
	}
}

func TestAddApp_NewTag(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	f.AddApp("network-tool", "curl", "Transfer data with URLs")

	tags := f.Tags()
	found := false
	for _, tag := range tags {
		if tag == "network-tool" {
			found = true
		}
	}
	if !found {
		t.Errorf("after AddApp new tag, Tags() = %v, want to contain 'network-tool'", tags)
	}

	tag, idx := f.FindApp("curl")
	if tag != "network-tool" || idx < 0 {
		t.Errorf("FindApp('curl') = (%q, %d), want ('network-tool', >=0)", tag, idx)
	}
}

func TestRemoveApp(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	removed := f.RemoveApp("htop")
	if !removed {
		t.Error("RemoveApp('htop') = false, want true")
	}

	_, idx := f.FindApp("htop")
	if idx != -1 {
		t.Errorf("after RemoveApp, FindApp('htop') index = %d, want -1", idx)
	}
}

func TestRemoveApp_NotFound(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	removed := f.RemoveApp("nonexistent")
	if removed {
		t.Error("RemoveApp('nonexistent') = true, want false")
	}
}

func TestWrite_PreservesComments(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	// Write back without modifications.
	if err := f.Write(); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test file path from TempDir
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# Homebrew packages") {
		t.Error("written file lost '# Homebrew packages' comment")
	}
	if !strings.Contains(content, "# Terminal utilities") {
		t.Error("written file lost '# Terminal utilities' comment")
	}
}

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.yaml")

	err := atomicWrite(path, []byte("hello"))
	if err != nil {
		t.Fatalf("atomicWrite error: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test file path from TempDir
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want 'hello'", string(data))
	}
}

func TestAtomicWrite_NoTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	if err := atomicWrite(path, []byte("data")); err != nil {
		t.Fatalf("atomicWrite error: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".hams-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file %q still exists after successful write", e.Name())
		}
	}
}

// Property-based: round-trip preserves all app names.
func TestProperty_RoundtripPreservesApps(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		appName := rapid.StringMatching(`[a-z][a-z0-9\-]{1,20}`).Draw(t, "app")
		tag := rapid.StringMatching(`[a-z][a-z\-]{2,15}`).Draw(t, "tag")

		dir, err := os.MkdirTemp("", "hamsfile-property-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(dir) //nolint:errcheck // cleanup in property test
		path := filepath.Join(dir, "test.hams.yaml")
		if err := os.WriteFile(path, []byte("placeholder:\n  - app: keep\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		f, err := Read(path)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		f.AddApp(tag, appName, "test")
		if err := f.Write(); err != nil {
			t.Fatalf("Write: %v", err)
		}

		f2, err := Read(path)
		if err != nil {
			t.Fatalf("re-Read: %v", err)
		}

		foundTag, idx := f2.FindApp(appName)
		if idx < 0 {
			t.Fatalf("app %q not found after round-trip", appName)
		}
		if foundTag != tag {
			t.Errorf("tag = %q, want %q", foundTag, tag)
		}
	})
}

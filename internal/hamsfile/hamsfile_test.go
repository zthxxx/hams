package hamsfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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

// TestRemoveAppField_RemovesExistingField locks in cycle 173:
// RemoveAppField clears a single key/value pair from an app's
// mapping node. Used by CLI handlers (e.g. apt's bare-install
// auto-record) to UNPIN a previously-pinned resource — without
// it, AddAppWithFields' merge-skip-empty semantic would leave
// the stale pin in the hamsfile.
func TestRemoveAppField_RemovesExistingField(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hams.yaml")
	if err := os.WriteFile(path,
		[]byte("cli:\n  - app: nginx\n    version: 1.24.0\n    source: stable\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if removed := f.RemoveAppField("nginx", "version"); !removed {
		t.Error("RemoveAppField('nginx', 'version') = false, want true")
	}

	// version field cleared, source preserved.
	fields := f.AppFields("nginx")
	if v, present := fields["version"]; present {
		t.Errorf("version field still present: %q", v)
	}
	if fields["source"] != "stable" {
		t.Errorf("source field clobbered: got %q, want 'stable'", fields["source"])
	}
}

// TestRemoveAppField_IdempotentOnAbsent: calling RemoveAppField on a
// missing key (or a missing app) is a safe no-op so callers don't
// need to pre-check.
func TestRemoveAppField_IdempotentOnAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hams.yaml")
	if err := os.WriteFile(path,
		[]byte("cli:\n  - app: nginx\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if removed := f.RemoveAppField("nginx", "version"); removed {
		t.Error("RemoveAppField on missing field should return false")
	}
	if removed := f.RemoveAppField("nonexistent-app", "version"); removed {
		t.Error("RemoveAppField on missing app should return false")
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
	writeErr := f.Write()
	if writeErr != nil {
		t.Fatalf("Write error: %v", writeErr)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile error: %v", readErr)
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

	err := AtomicWrite(path, []byte("hello"))
	if err != nil {
		t.Fatalf("AtomicWrite error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want 'hello'", string(data))
	}
}

// TestAtomicWrite_ParentIsFileErrors asserts AtomicWrite surfaces
// an error (not silent overwrite) when the target's parent
// directory path is actually a file. This is a real scenario: a
// user typos the profile path as a filename, or an earlier step
// wrote a file where a directory was expected.
func TestAtomicWrite_ParentIsFileErrors(t *testing.T) {
	dir := t.TempDir()
	// Create "parent" as a file (not a directory) to force MkdirAll to fail.
	parentFile := filepath.Join(dir, "parent")
	if err := os.WriteFile(parentFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("seed parent file: %v", err)
	}
	// AtomicWrite target is <parent>/test.yaml — MkdirAll on
	// <parent> fails because <parent> is already a file.
	target := filepath.Join(parentFile, "test.yaml")
	err := AtomicWrite(target, []byte("hello"))
	if err == nil {
		t.Fatalf("AtomicWrite into a file-parent should error")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("error should mention 'creating directory', got: %v", err)
	}
}

// TestAtomicWrite_EmptyDataWritesEmptyFile asserts writing zero
// bytes produces an empty file rather than an error or skip. This
// is the expected behavior when a hamsfile has no entries after
// RemoveApp drains everything — the file should still persist as
// an explicit "yes, this is empty" marker.
func TestAtomicWrite_EmptyDataWritesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")

	if err := AtomicWrite(path, []byte{}); err != nil {
		t.Fatalf("AtomicWrite empty: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("empty write produced size=%d, want 0", info.Size())
	}
}

// TestAtomicWrite_OverwriteExisting asserts AtomicWrite is truly
// atomic and leaves no intermediate state: overwriting a file with
// new content replaces it completely with the new payload.
func TestAtomicWrite_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	if err := AtomicWrite(path, []byte("first")); err != nil {
		t.Fatalf("first AtomicWrite: %v", err)
	}
	if err := AtomicWrite(path, []byte("second")); err != nil {
		t.Fatalf("second AtomicWrite: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "second" {
		t.Errorf("after overwrite, content = %q, want 'second'", string(data))
	}
}

func TestAtomicWrite_NoTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	if err := AtomicWrite(path, []byte("data")); err != nil {
		t.Fatalf("AtomicWrite error: %v", err)
	}

	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatalf("ReadDir error: %v", readDirErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".hams-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file %q still exists after successful write", e.Name())
		}
	}
}

// Property-based: round-trip preserves all app names.
func TestSetPreviewCmd(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.hams.yaml")
	content := "config:\n  - urn: urn:hams:defaults:com.apple.dock.autohide\n    intro: dock autohide\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}

	f.SetPreviewCmd("urn:hams:defaults:com.apple.dock.autohide", "defaults write com.apple.dock autohide -bool true")
	if writeErr := f.Write(); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Re-read and verify preview-cmd persisted.
	f2, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path) //nolint:errcheck // test
	if !strings.Contains(string(data), "preview-cmd") {
		t.Error("preview-cmd field not found in written file")
	}
	_ = f2 // f2 loaded successfully, field persists.
}

func TestProperty_PreviewCmdSurvivesRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	var counter atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		urnName := rapid.StringMatching(`urn:hams:defaults:[a-z][a-z0-9\.]{3,20}`).Draw(t, "urn")
		previewCmd := rapid.StringMatching(`defaults write [a-z\.\- ]{5,30}`).Draw(t, "cmd")

		dir := filepath.Join(baseDir, fmt.Sprintf("pcmd-%d", counter.Add(1)))
		if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
			t.Fatalf("MkdirAll: %v", mkErr)
		}
		path := filepath.Join(dir, "test.hams.yaml")
		content := fmt.Sprintf("config:\n  - urn: %s\n    intro: test entry\n", urnName)
		if writeErr := os.WriteFile(path, []byte(content), 0o600); writeErr != nil {
			t.Fatalf("write: %v", writeErr)
		}

		f, readErr := Read(path)
		if readErr != nil {
			t.Fatalf("Read: %v", readErr)
		}

		f.SetPreviewCmd(urnName, previewCmd)
		if saveErr := f.Write(); saveErr != nil {
			t.Fatalf("Write: %v", saveErr)
		}

		// Re-read and verify.
		data, dataErr := os.ReadFile(path)
		if dataErr != nil {
			t.Fatalf("ReadFile: %v", dataErr)
		}
		if !strings.Contains(string(data), "preview-cmd") {
			t.Fatal("preview-cmd field not found after round-trip")
		}
		if !strings.Contains(string(data), previewCmd) {
			t.Fatalf("preview-cmd value %q not found in file", previewCmd)
		}
	})
}

func TestProperty_RoundtripPreservesApps(t *testing.T) {
	baseDir := t.TempDir()
	var counter atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		appName := rapid.StringMatching(`[a-z][a-z0-9\-]{1,20}`).Draw(t, "app")
		tag := rapid.StringMatching(`[a-z][a-z\-]{2,15}`).Draw(t, "tag")

		dir := filepath.Join(baseDir, fmt.Sprintf("run-%d", counter.Add(1)))
		if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
			t.Fatalf("MkdirAll: %v", mkErr)
		}
		path := filepath.Join(dir, "test.hams.yaml")
		writeErr := os.WriteFile(path, []byte("placeholder:\n  - app: keep\n"), 0o600)
		if writeErr != nil {
			t.Fatalf("write: %v", writeErr)
		}

		f, readErr := Read(path)
		if readErr != nil {
			t.Fatalf("Read: %v", readErr)
		}

		f.AddApp(tag, appName, "test")
		saveErr := f.Write()
		if saveErr != nil {
			t.Fatalf("Write: %v", saveErr)
		}

		f2, reReadErr := Read(path)
		if reReadErr != nil {
			t.Fatalf("re-Read: %v", reReadErr)
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

// TestAddAppWithFields_RoundTripsExtraFields verifies that structured
// `version` and `source` fields survive a write+read cycle.
func TestAddAppWithFields_RoundTripsExtraFields(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.hams.yaml"
	if err := os.WriteFile(path, []byte("cli: []\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	f.AddAppWithFields("cli", "nginx", "", map[string]string{
		"version": "1.24.0",
		"source":  "bookworm-backports",
	})
	if writeErr := f.Write(); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "app: nginx") {
		t.Errorf("app line missing: %q", body)
	}
	if !strings.Contains(body, "version: 1.24.0") {
		t.Errorf("version line missing: %q", body)
	}
	if !strings.Contains(body, "source: bookworm-backports") {
		t.Errorf("source line missing: %q", body)
	}
}

// TestAddAppWithFields_BareNameDoesNotEmitEmptyFields verifies that
// passing empty-string extras does NOT pollute the YAML — bare-name
// entries continue to round-trip identically.
func TestAddAppWithFields_BareNameDoesNotEmitEmptyFields(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.hams.yaml"
	if err := os.WriteFile(path, []byte("cli: []\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	f.AddAppWithFields("cli", "htop", "", map[string]string{
		"version": "",
		"source":  "",
	})
	if writeErr := f.Write(); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "version:") || strings.Contains(body, "source:") {
		t.Errorf("empty extras leaked into YAML: %q", body)
	}
}

func TestAppFields_ReturnsStructuredFields(t *testing.T) {
	tmp := t.TempDir() + "/test.hams.yaml"
	if err := os.WriteFile(tmp, []byte("cli:\n  - app: nginx\n    version: \"1.24.0\"\n    source: bp\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := f.AppFields("nginx")
	wantVer, wantSrc := "1.24.0", "bp"
	if got["version"] != wantVer || got["source"] != wantSrc {
		t.Errorf("AppFields(nginx) = %v, want version=%q source=%q", got, wantVer, wantSrc)
	}
	if _, ok := got["app"]; ok {
		t.Errorf("AppFields leaked the app key: %v", got)
	}
}

func TestAppFields_ReturnsNilForUnknown(t *testing.T) {
	tmp := t.TempDir() + "/test.hams.yaml"
	if err := os.WriteFile(tmp, []byte("cli: []\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := f.AppFields("nginx"); got != nil {
		t.Errorf("AppFields(nginx) = %v, want nil for unknown entry", got)
	}
}

func TestAppFields_ReturnsNilForBareEntry(t *testing.T) {
	tmp := t.TempDir() + "/test.hams.yaml"
	if err := os.WriteFile(tmp, []byte("cli:\n  - app: htop\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := f.AppFields("htop")
	if len(got) != 0 {
		t.Errorf("AppFields(htop) = %v, want nil/empty for bare entry", got)
	}
}

// Locks in the in-place upgrade: a bare entry gains a pin without
// duplicating the entry or moving it across tags.
func TestAddAppWithFields_UpgradesBareEntryToPinned(t *testing.T) {
	tmp := t.TempDir() + "/test.hams.yaml"
	if err := os.WriteFile(tmp, []byte("cli:\n  - app: nginx\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	f.AddAppWithFields("cli", "nginx", "", map[string]string{"version": "1.24.0"})
	if writeErr := f.Write(); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "version: 1.24.0") {
		t.Errorf("upgrade did not write version: %q", body)
	}
	if strings.Count(body, "app: nginx") != 1 {
		t.Errorf("entry was duplicated, want exactly one app: nginx — got %q", body)
	}
}

// Empty extras on an existing entry must be a no-op (round-trip
// invariant; never re-emit empty strings as fields).
func TestAddAppWithFields_EmptyExtrasOnExistingIsNoop(t *testing.T) {
	tmp := t.TempDir() + "/test.hams.yaml"
	original := "cli:\n  - app: nginx\n    version: \"1.24.0\"\n"
	if err := os.WriteFile(tmp, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	f.AddAppWithFields("cli", "nginx", "", map[string]string{"version": "", "source": ""})
	if writeErr := f.Write(); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "app: nginx") {
		t.Errorf("noop call lost the app entry: %q", body)
	}
	// Tolerate either quoted or unquoted YAML scalar form for the version.
	if !strings.Contains(body, "1.24.0") {
		t.Errorf("noop call lost the version pin: %q", body)
	}
	if strings.Contains(body, "source:") {
		t.Errorf("empty source extra leaked into YAML: %q", body)
	}
}

// TestLoadOrCreateEmpty_MissingFile asserts the create-empty branch:
// a non-existent path returns a fresh in-memory File rooted at the
// requested path. The caller persists via Write(). This is the path
// every provider's `loadOrCreateHamsfile` hits on first install.
func TestLoadOrCreateEmpty_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never-existed.hams.yaml")

	f, err := LoadOrCreateEmpty(path)
	if err != nil {
		t.Fatalf("LoadOrCreateEmpty on missing file: %v", err)
	}
	if f == nil {
		t.Fatal("returned nil File")
	}
	if f.Path != path {
		t.Errorf("Path = %q, want %q", f.Path, path)
	}
	if f.Root == nil {
		t.Error("Root should be a fresh document node, not nil")
	}
	// The parent directory must be auto-created so Write() later doesn't
	// fail with ENOENT.
	if _, statErr := os.Stat(filepath.Dir(path)); statErr != nil {
		t.Errorf("parent dir should exist after LoadOrCreateEmpty: %v", statErr)
	}
}

// TestLoadOrCreateEmpty_ExistingFile asserts the read path: an
// existing hamsfile is loaded verbatim, not replaced with empty.
func TestLoadOrCreateEmpty_ExistingFile(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := LoadOrCreateEmpty(path)
	if err != nil {
		t.Fatalf("LoadOrCreateEmpty: %v", err)
	}
	apps := f.ListApps()
	if len(apps) != 3 {
		t.Errorf("expected 3 apps from sample YAML, got %d: %v", len(apps), apps)
	}
}

// TestLoadOrCreateEmpty_NonExistFileSurfacesOtherErrors asserts
// that non-ErrNotExist errors propagate (e.g., a path whose PARENT
// is a regular file → mkdir fails with ENOTDIR).
func TestLoadOrCreateEmpty_NonMissingErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file at <dir>/block so <dir>/block/x.yaml
	// can't be created (parent is a file, not a dir).
	blocker := filepath.Join(dir, "block")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	_, err := LoadOrCreateEmpty(filepath.Join(blocker, "child.hams.yaml"))
	if err == nil {
		t.Error("expected error when parent path is a file")
	}
}

// TestListApps_MultipleTagsAndFields asserts that ListApps returns
// every `app:` and `urn:` value across all top-level tags, regardless
// of extra fields in each entry.
func TestListApps_MultipleTagsAndFields(t *testing.T) {
	const yamlDoc = `dev:
  - app: htop
    intro: process viewer
  - app: jq
cli:
  - urn: urn:hams:apt:curl
    version: "8.0"
`
	path := writeTempFile(t, yamlDoc)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	apps := f.ListApps()
	want := map[string]bool{"htop": true, "jq": true, "urn:hams:apt:curl": true}
	if len(apps) != len(want) {
		t.Fatalf("got %d apps, want %d: %v", len(apps), len(want), apps)
	}
	for _, a := range apps {
		if !want[a] {
			t.Errorf("unexpected app %q", a)
		}
	}
}

// TestListApps_EmptyAndMalformed asserts that ListApps returns nil
// on an empty/malformed root without panicking.
func TestListApps_EmptyAndMalformed(t *testing.T) {
	empty := NewEmpty("x.yaml")
	if got := empty.ListApps(); len(got) != 0 {
		t.Errorf("empty hamsfile should have no apps, got %v", got)
	}

	nilRoot := &File{Path: "x.yaml"}
	if got := nilRoot.ListApps(); got != nil {
		t.Errorf("nil-root File should return nil apps, got %v", got)
	}
}

// TestListApps_SkipsEmptyAndWhitespaceEntries locks in cycle 98:
// an entry with `app: ""` or `app: "   "` (e.g., from a git merge
// conflict or a manual YAML edit) MUST NOT flow into the provider
// install path. Previously ListApps returned the empty string, and
// downstream `apt install ""` / `brew install ""` failed with
// cryptic shell errors. Now empty/whitespace values are filtered.
func TestListApps_SkipsEmptyAndWhitespaceEntries(t *testing.T) {
	const yamlDoc = `cli:
  - app: ""
  - app: "  "
  - app: valid-pkg
  - app: another
`
	path := writeTempFile(t, yamlDoc)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := f.ListApps()
	want := []string{"valid-pkg", "another"}
	if len(got) != len(want) {
		t.Fatalf("ListApps = %v, want %v", got, want)
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("ListApps missing %q", w)
		}
	}
	for _, g := range got {
		if g == "" || strings.TrimSpace(g) == "" {
			t.Errorf("ListApps returned empty/whitespace value %q", g)
		}
	}
}

// TestValidateNoDuplicateApps_NoDupsReturnsNil locks in cycle 255's
// happy path: a hamsfile with unique apps across all tags passes
// validation.
func TestValidateNoDuplicateApps_NoDupsReturnsNil(t *testing.T) {
	path := writeTempFile(t, sampleYAML)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if dupErr := f.ValidateNoDuplicateApps(); dupErr != nil {
		t.Errorf("unique apps should pass; got %v", dupErr)
	}
}

// TestValidateNoDuplicateApps_CrossTagDupRejected locks in cycle 255
// per schema-design spec §"Duplicate app identity across groups is
// rejected": the same app under two tags returns a DuplicateAppError
// that names both tags.
func TestValidateNoDuplicateApps_CrossTagDupRejected(t *testing.T) {
	yamlBody := `# tags
development-tool:
  - app: git
    intro: version control
terminal-tool:
  - app: git
    intro: duplicated here by accident
`
	path := writeTempFile(t, yamlBody)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	dupErr := f.ValidateNoDuplicateApps()
	if dupErr == nil {
		t.Fatal("expected DuplicateAppError for cross-tag duplicate; got nil")
	}
	var de *DuplicateAppError
	if !errors.As(dupErr, &de) {
		t.Fatalf("expected *DuplicateAppError; got %T: %v", dupErr, dupErr)
	}
	if de.App != "git" {
		t.Errorf("App = %q, want %q", de.App, "git")
	}
	if len(de.Tags) != 2 || de.Tags[0] != "development-tool" || de.Tags[1] != "terminal-tool" {
		t.Errorf("Tags = %v, want [development-tool terminal-tool] (document order)", de.Tags)
	}
	if !strings.Contains(dupErr.Error(), `duplicate app "git"`) {
		t.Errorf("Error() should name the duplicate app; got %q", dupErr.Error())
	}
}

// TestValidateNoDuplicateApps_SameTagRepeatAccepted locks in cycle
// 255's documented boundary: same-tag repeats are NOT rejected here
// (they fold into a single action via ComputePlan's dedup, and
// rejecting mid-edit would break hand-fixing workflows). Only
// cross-tag duplicates fail validation.
func TestValidateNoDuplicateApps_SameTagRepeatAccepted(t *testing.T) {
	yamlBody := `packages:
  - app: git
  - app: git
`
	path := writeTempFile(t, yamlBody)
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if dupErr := f.ValidateNoDuplicateApps(); dupErr != nil {
		t.Errorf("same-tag repeat should not trigger validation; got %v", dupErr)
	}
}

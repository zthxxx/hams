package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pgregory.net/rapid"
)

func TestNew_Defaults(t *testing.T) {
	f := New("homebrew", "MacbookM5X")
	if f.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", f.SchemaVersion, SchemaVersion)
	}
	if f.Provider != "homebrew" {
		t.Errorf("Provider = %q, want 'homebrew'", f.Provider)
	}
	if f.MachineID != "MacbookM5X" {
		t.Errorf("MachineID = %q, want 'MacbookM5X'", f.MachineID)
	}
	if len(f.Resources) != 0 {
		t.Errorf("Resources should be empty, got %d", len(f.Resources))
	}
}

func TestSetResource_OK(t *testing.T) {
	f := New("homebrew", "test")
	f.SetResource("htop", StateOK, WithVersion("3.3.0"))

	r := f.Resources["htop"]
	if r == nil {
		t.Fatal("resource 'htop' not found")
	}
	if r.State != StateOK {
		t.Errorf("State = %q, want 'ok'", r.State)
	}
	if r.Version != "3.3.0" {
		t.Errorf("Version = %q, want '3.3.0'", r.Version)
	}
	if r.FirstInstallAt == "" {
		t.Error("FirstInstallAt should be set")
	}
}

func TestSetResource_Failed(t *testing.T) {
	f := New("homebrew", "test")
	f.SetResource("broken", StateFailed, WithError("network timeout"))

	r := f.Resources["broken"]
	if r.State != StateFailed {
		t.Errorf("State = %q, want 'failed'", r.State)
	}
	if r.LastError != "network timeout" {
		t.Errorf("LastError = %q, want 'network timeout'", r.LastError)
	}
}

func TestSetResource_OKClearsError(t *testing.T) {
	f := New("homebrew", "test")
	f.SetResource("htop", StateFailed, WithError("oops"))
	f.SetResource("htop", StateOK, WithVersion("3.3.0"))

	r := f.Resources["htop"]
	if r.LastError != "" {
		t.Errorf("LastError should be cleared on OK, got %q", r.LastError)
	}
}

func TestPendingResources(t *testing.T) {
	f := New("homebrew", "test")
	f.SetResource("htop", StateOK)
	f.SetResource("jq", StateFailed)
	f.SetResource("curl", StatePending)
	f.SetResource("git", StateRemoved)

	pending := f.PendingResources()
	if len(pending) != 2 {
		t.Fatalf("PendingResources() = %v, want 2 items", pending)
	}

	has := make(map[string]bool)
	for _, id := range pending {
		has[id] = true
	}
	if !has["jq"] || !has["curl"] {
		t.Errorf("PendingResources() = %v, want jq and curl", pending)
	}
}

func TestSaveAndLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "Homebrew.state.yaml")

	f := New("homebrew", "MacbookM5X")
	f.SetResource("htop", StateOK, WithVersion("3.3.0"))
	f.SetResource("jq", StateFailed, WithError("not found"))

	if err := f.Save(path); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loaded.Provider != "homebrew" {
		t.Errorf("Provider = %q, want 'homebrew'", loaded.Provider)
	}
	if loaded.Resources["htop"].Version != "3.3.0" {
		t.Errorf("htop version = %q, want '3.3.0'", loaded.Resources["htop"].Version)
	}
	if loaded.Resources["jq"].LastError != "not found" {
		t.Errorf("jq error = %q, want 'not found'", loaded.Resources["jq"].LastError)
	}
}

func TestProperty_SaveLoadRoundtrip(t *testing.T) {
	baseDir := t.TempDir()
	var counter atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "provider")
		machineID := rapid.StringMatching(`[A-Za-z0-9]{3,15}`).Draw(t, "machineID")
		appName := rapid.StringMatching(`[a-z][a-z0-9\-]{1,20}`).Draw(t, "app")
		version := rapid.StringMatching(`[0-9]+\.[0-9]+\.[0-9]+`).Draw(t, "version")

		dir := filepath.Join(baseDir, fmt.Sprintf("run-%d", counter.Add(1)))
		if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
			t.Fatalf("MkdirAll: %v", mkErr)
		}
		path := filepath.Join(dir, "test.state.yaml")

		f := New(provider, machineID)
		f.SetResource(appName, StateOK, WithVersion(version))

		saveErr := f.Save(path)
		if saveErr != nil {
			t.Fatalf("Save: %v", saveErr)
		}

		loaded, loadErr := Load(path)
		if loadErr != nil {
			t.Fatalf("Load: %v", loadErr)
		}

		if loaded.Provider != provider {
			t.Errorf("Provider = %q, want %q", loaded.Provider, provider)
		}
		r, ok := loaded.Resources[appName]
		if !ok || r == nil {
			t.Fatalf("resource %q not found", appName)
		}
		if r.Version != version {
			t.Errorf("Version = %q, want %q", r.Version, version)
		}
	})
}

// S1: New resource with StateOK sets FirstInstallAt and UpdatedAt; no RemovedAt.
func TestSetResource_S1_FirstInstall(t *testing.T) {
	f := New("apt", "test")
	f.SetResource("bat", StateOK)
	r := f.Resources["bat"]
	if r.FirstInstallAt == "" {
		t.Error("FirstInstallAt should be set on first install")
	}
	if r.UpdatedAt == "" {
		t.Error("UpdatedAt should be set on first install")
	}
	if r.UpdatedAt != r.FirstInstallAt {
		t.Errorf("UpdatedAt (%q) should equal FirstInstallAt (%q) on first install", r.UpdatedAt, r.FirstInstallAt)
	}
	if r.RemovedAt != "" {
		t.Errorf("RemovedAt should be empty on first install, got %q", r.RemovedAt)
	}
}

// S2: Re-install preserves FirstInstallAt, bumps UpdatedAt.
func TestSetResource_S2_ReInstallPreservesFirstInstallAt(t *testing.T) {
	f := New("apt", "test")
	f.SetResource("bat", StateOK)
	firstInstall := f.Resources["bat"].FirstInstallAt

	// Force a distinct timestamp via sub-second sleep avoidance: overwrite now.
	// We rely on property: re-call SetResource with StateOK and check FirstInstallAt unchanged.
	// To guarantee a distinct UpdatedAt, manually set it backward before re-calling.
	f.Resources["bat"].UpdatedAt = "19700101T000000"
	f.SetResource("bat", StateOK)
	r := f.Resources["bat"]
	if r.FirstInstallAt != firstInstall {
		t.Errorf("FirstInstallAt should not change on re-install: got %q, want %q", r.FirstInstallAt, firstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped on re-install")
	}
}

// S3: StateRemoved sets RemovedAt, bumps UpdatedAt, leaves FirstInstallAt.
func TestSetResource_S3_RemoveSetsRemovedAt(t *testing.T) {
	f := New("apt", "test")
	f.SetResource("bat", StateOK)
	firstInstall := f.Resources["bat"].FirstInstallAt
	f.Resources["bat"].UpdatedAt = "19700101T000000"

	f.SetResource("bat", StateRemoved)
	r := f.Resources["bat"]
	if r.State != StateRemoved {
		t.Errorf("State = %q, want removed", r.State)
	}
	if r.RemovedAt == "" {
		t.Error("RemovedAt should be set on remove")
	}
	if r.FirstInstallAt != firstInstall {
		t.Errorf("FirstInstallAt should not change on remove: got %q, want %q", r.FirstInstallAt, firstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped on remove")
	}
	if r.RemovedAt != r.UpdatedAt {
		t.Errorf("RemovedAt (%q) should equal UpdatedAt (%q) on remove", r.RemovedAt, r.UpdatedAt)
	}
}

// S4: StateOK after StateRemoved clears RemovedAt, preserves FirstInstallAt.
func TestSetResource_S4_ReInstallAfterRemoveClearsRemovedAt(t *testing.T) {
	f := New("apt", "test")
	f.SetResource("bat", StateOK)
	firstInstall := f.Resources["bat"].FirstInstallAt
	f.SetResource("bat", StateRemoved)
	if f.Resources["bat"].RemovedAt == "" {
		t.Fatal("precondition: RemovedAt should be set after remove")
	}
	f.Resources["bat"].UpdatedAt = "19700101T000000"

	f.SetResource("bat", StateOK)
	r := f.Resources["bat"]
	if r.RemovedAt != "" {
		t.Errorf("RemovedAt should be cleared on re-install: got %q", r.RemovedAt)
	}
	if r.FirstInstallAt != firstInstall {
		t.Errorf("FirstInstallAt should not change on re-install-after-remove: got %q, want %q", r.FirstInstallAt, firstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped on re-install-after-remove")
	}
	if r.State != StateOK {
		t.Errorf("State = %q, want ok", r.State)
	}
}

// S5: Legacy v1 state file with install_at migrates to schema_version 2 with first_install_at.
func TestLoad_S5_V1MigrationRenamesInstallAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apt.state.yaml")
	legacy := `schema_version: 1
provider: apt
machine_id: sandbox
resources:
  bat:
    state: ok
    install_at: "20260410T091500"
    updated_at: "20260410T091500"
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion after load = %d, want %d", loaded.SchemaVersion, SchemaVersion)
	}
	r := loaded.Resources["bat"]
	if r == nil {
		t.Fatal("resource bat missing after migration")
	}
	if r.FirstInstallAt != "20260410T091500" {
		t.Errorf("FirstInstallAt after migration = %q, want %q", r.FirstInstallAt, "20260410T091500")
	}

	if err := loaded.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // path comes from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "schema_version: 2") {
		t.Errorf("rewritten file missing schema_version: 2\n%s", out)
	}
	if !strings.Contains(out, "first_install_at: ") {
		t.Errorf("rewritten file missing first_install_at\n%s", out)
	}
	if strings.Contains(out, "install_at:") && !strings.Contains(out, "first_install_at:") {
		t.Errorf("rewritten file still contains legacy install_at\n%s", out)
	}
	// Stronger check: no bare "install_at:" line (only first_install_at should appear).
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "install_at:") {
			t.Errorf("rewritten file contains legacy install_at line: %q\n%s", trimmed, out)
		}
	}
}

// S6: V2 round-trip preserves every field.
func TestSaveAndLoad_S6_V2RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apt.state.yaml")

	f := New("apt", "sandbox")
	f.SetResource("bat", StateOK, WithVersion("0.24.0"))
	f.SetResource("htop", StateOK)
	f.SetResource("htop", StateRemoved)

	batFirstInstall := f.Resources["bat"].FirstInstallAt
	htopFirstInstall := f.Resources["htop"].FirstInstallAt
	htopRemovedAt := f.Resources["htop"].RemovedAt

	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", loaded.SchemaVersion, SchemaVersion)
	}
	if loaded.Resources["bat"].FirstInstallAt != batFirstInstall {
		t.Errorf("bat FirstInstallAt changed through round-trip: got %q, want %q", loaded.Resources["bat"].FirstInstallAt, batFirstInstall)
	}
	if loaded.Resources["htop"].FirstInstallAt != htopFirstInstall {
		t.Errorf("htop FirstInstallAt changed: got %q, want %q", loaded.Resources["htop"].FirstInstallAt, htopFirstInstall)
	}
	if loaded.Resources["htop"].RemovedAt != htopRemovedAt {
		t.Errorf("htop RemovedAt changed: got %q, want %q", loaded.Resources["htop"].RemovedAt, htopRemovedAt)
	}
	if loaded.Resources["bat"].RemovedAt != "" {
		t.Errorf("bat RemovedAt should be empty, got %q", loaded.Resources["bat"].RemovedAt)
	}
}

// Reject state files with schema_version newer than the binary supports.
func TestLoad_FutureSchemaVersionRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.state.yaml")
	data := fmt.Sprintf("schema_version: %d\nprovider: apt\nmachine_id: sandbox\n", SchemaVersion+1)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should reject future schema version")
	}
	if !strings.Contains(err.Error(), "self-upgrade") {
		t.Errorf("error should recommend self-upgrade, got %q", err.Error())
	}
}

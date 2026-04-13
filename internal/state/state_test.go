package state

import (
	"fmt"
	"os"
	"path/filepath"
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
	if r.InstallAt == "" {
		t.Error("InstallAt should be set")
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

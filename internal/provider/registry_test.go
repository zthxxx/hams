package provider

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// stubProvider is a minimal provider for testing registry operations.
type stubProvider struct {
	manifest Manifest
}

func (s *stubProvider) Manifest() Manifest { return s.manifest }

func (s *stubProvider) Bootstrap(_ context.Context) error { return nil }

func (s *stubProvider) Probe(_ context.Context, _ *state.File) ([]ProbeResult, error) {
	return nil, nil
}

func (s *stubProvider) Plan(_ context.Context, _ *hamsfile.File, _ *state.File) ([]Action, error) {
	return nil, nil
}

func (s *stubProvider) Apply(_ context.Context, _ Action) error { return nil }

func (s *stubProvider) Remove(_ context.Context, _ string) error { return nil }

func (s *stubProvider) List(_ context.Context, _ *hamsfile.File, _ *state.File) (string, error) {
	return "", nil
}

func newStub(name, display string) *stubProvider {
	return &stubProvider{
		manifest: Manifest{
			Name:        name,
			DisplayName: display,
			Platforms:   []Platform{PlatformAll},
		},
	}
}

func mustRegister(t *testing.T, r *Registry, p Provider) {
	t.Helper()
	if err := r.Register(p); err != nil {
		t.Fatalf("Register(%s) error: %v", p.Manifest().Name, err)
	}
}

func TestRegistry_Register_And_Get(t *testing.T) {
	r := NewRegistry()
	stub := newStub("brew", "Homebrew")

	if err := r.Register(stub); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	got := r.Get("brew")
	if got == nil {
		t.Fatal("Get('brew') returned nil")
	}
	if got.Manifest().DisplayName != "Homebrew" {
		t.Errorf("DisplayName = %q, want 'Homebrew'", got.Manifest().DisplayName)
	}
}

func TestRegistry_Register_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newStub("Brew", "Homebrew")); err != nil {
		t.Fatal(err)
	}
	if r.Get("brew") == nil {
		t.Error("Get('brew') should find 'Brew' (case-insensitive)")
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newStub("brew", "Homebrew")); err != nil {
		t.Fatal(err)
	}
	err := r.Register(newStub("brew", "Homebrew2"))
	if err == nil {
		t.Error("duplicate registration should error")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	if r.Get("nonexistent") != nil {
		t.Error("Get for nonexistent should return nil")
	}
}

func TestRegistry_Names_Sorted(t *testing.T) {
	r := NewRegistry()
	mustRegister(t, r, newStub("pnpm", "pnpm"))
	mustRegister(t, r, newStub("brew", "Homebrew"))
	mustRegister(t, r, newStub("apt", "apt"))

	names := r.Names()
	if len(names) != 3 {
		t.Fatalf("Names() = %v, want 3", names)
	}
	if names[0] != "apt" || names[1] != "brew" || names[2] != "pnpm" {
		t.Errorf("Names() = %v, want [apt brew pnpm]", names)
	}
}

func TestRegistry_Ordered_Priority(t *testing.T) {
	r := NewRegistry()
	mustRegister(t, r, newStub("apt", "apt"))
	mustRegister(t, r, newStub("brew", "Homebrew"))
	mustRegister(t, r, newStub("pnpm", "pnpm"))
	mustRegister(t, r, newStub("custom", "Custom"))

	ordered := r.Ordered([]string{"brew", "apt", "pnpm"})
	if len(ordered) != 4 {
		t.Fatalf("Ordered returned %d, want 4", len(ordered))
	}
	names := make([]string, len(ordered))
	for i, p := range ordered {
		names[i] = p.Manifest().Name
	}
	// Priority order first, then alphabetical for unlisted.
	if names[0] != "brew" || names[1] != "apt" || names[2] != "pnpm" || names[3] != "custom" {
		t.Errorf("Ordered = %v, want [brew apt pnpm custom]", names)
	}
}

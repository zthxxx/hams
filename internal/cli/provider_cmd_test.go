package cli

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestStripGlobalFlags_DryRun(t *testing.T) {
	flags := &provider.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "htop", "--dry-run"}, flags)
	if !flags.DryRun {
		t.Error("DryRun should be set")
	}
	if len(cleaned) != 2 || cleaned[0] != "install" || cleaned[1] != "htop" {
		t.Errorf("cleaned = %v, want [install htop]", cleaned)
	}
}

func TestStripGlobalFlags_ConfigWithValue(t *testing.T) {
	flags := &provider.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "--config", "/tmp/cfg.yaml", "htop"}, flags)
	if flags.Config != "/tmp/cfg.yaml" {
		t.Errorf("Config = %q, want /tmp/cfg.yaml", flags.Config)
	}
	if len(cleaned) != 2 || cleaned[0] != "install" || cleaned[1] != "htop" {
		t.Errorf("cleaned = %v, want [install htop]", cleaned)
	}
}

func TestStripGlobalFlags_ConfigEquals(t *testing.T) {
	flags := &provider.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "--config=/tmp/cfg.yaml", "htop"}, flags)
	if flags.Config != "/tmp/cfg.yaml" {
		t.Errorf("Config = %q, want /tmp/cfg.yaml", flags.Config)
	}
	if len(cleaned) != 2 {
		t.Errorf("cleaned = %v, want 2 items", cleaned)
	}
}

// Mock provider for testing.
type mockProvider struct {
	name          string
	displayName   string
	lastArgs      []string
	lastHamsFlags map[string]string
	lastFlags     *provider.GlobalFlags
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) DisplayName() string { return m.displayName }
func (m *mockProvider) HandleCommand(_ context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	m.lastArgs = args
	m.lastHamsFlags = hamsFlags
	m.lastFlags = flags
	return nil
}

func TestParseProviderArgs_PreservesSeparator(t *testing.T) {
	flags := &provider.GlobalFlags{}
	_, passthrough := parseProviderArgs([]string{"run", "--", "-v", "--flag"}, flags)

	// The "--" must be preserved so the underlying CLI receives it.
	expected := []string{"run", "--", "-v", "--flag"}
	if len(passthrough) != len(expected) {
		t.Fatalf("passthrough = %v, want %v", passthrough, expected)
	}
	for i, want := range expected {
		if passthrough[i] != want {
			t.Errorf("passthrough[%d] = %q, want %q", i, passthrough[i], want)
		}
	}
}

func TestParseProviderArgs_HamsFlagsBeforeSeparator(t *testing.T) {
	flags := &provider.GlobalFlags{}
	hamsFlags, passthrough := parseProviderArgs([]string{"run", "--hams-local", "--", "--hams-tag=foo"}, flags)

	// --hams-local before -- should be captured.
	if _, ok := hamsFlags["local"]; !ok {
		t.Error("hams[local] should exist")
	}
	// --hams-tag=foo after -- should be in passthrough, not captured.
	if _, ok := hamsFlags["tag"]; ok {
		t.Error("hams[tag] should not exist (after --)")
	}
	// passthrough should be: run, --, --hams-tag=foo
	if len(passthrough) != 3 || passthrough[0] != "run" || passthrough[1] != "--" || passthrough[2] != "--hams-tag=foo" {
		t.Errorf("passthrough = %v, want [run -- --hams-tag=foo]", passthrough)
	}
}

func TestRouteToProvider_HelpIntercept(t *testing.T) {
	mock := &mockProvider{name: "brew", displayName: "Homebrew"}

	err := routeToProvider(mock, []string{"install", "--help"}, &provider.GlobalFlags{})
	if err != nil {
		t.Fatalf("routeToProvider --help error: %v", err)
	}

	if mock.lastArgs != nil {
		t.Error("provider should not receive args when --help is present")
	}
}

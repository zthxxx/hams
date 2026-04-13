package cli

import (
	"testing"

	"github.com/zthxxx/hams/internal/cliutil"
)

func TestSplitHamsFlags_Basic(t *testing.T) {
	hams, pass := cliutil.SplitHamsFlags([]string{"install", "htop", "--hams:tag=devtools", "--cask"})
	if hams["tag"] != "devtools" {
		t.Errorf("hams[tag] = %q, want 'devtools'", hams["tag"])
	}
	if len(pass) != 3 || pass[0] != "install" || pass[1] != "htop" || pass[2] != "--cask" {
		t.Errorf("passthrough = %v, want [install htop --cask]", pass)
	}
}

func TestSplitHamsFlags_ForceForward(t *testing.T) {
	hams, pass := cliutil.SplitHamsFlags([]string{"install", "--", "--hams:tag=foo", "--cask"})
	if len(hams) != 0 {
		t.Errorf("hams flags should be empty after --, got %v", hams)
	}
	if len(pass) != 3 || pass[0] != "install" || pass[1] != "--hams:tag=foo" || pass[2] != "--cask" {
		t.Errorf("passthrough = %v, want [install --hams:tag=foo --cask]", pass)
	}
}

func TestSplitHamsFlags_BooleanFlag(t *testing.T) {
	hams, _ := cliutil.SplitHamsFlags([]string{"install", "htop", "--hams:lucky", "--hams:local"})
	if _, ok := hams["lucky"]; !ok {
		t.Error("hams[lucky] should exist")
	}
	if _, ok := hams["local"]; !ok {
		t.Error("hams[local] should exist")
	}
}

func TestStripGlobalFlags_DryRun(t *testing.T) {
	flags := &cliutil.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "htop", "--dry-run"}, flags)
	if !flags.DryRun {
		t.Error("DryRun should be set")
	}
	if len(cleaned) != 2 || cleaned[0] != "install" || cleaned[1] != "htop" {
		t.Errorf("cleaned = %v, want [install htop]", cleaned)
	}
}

func TestStripGlobalFlags_ConfigWithValue(t *testing.T) {
	flags := &cliutil.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "--config", "/tmp/cfg.yaml", "htop"}, flags)
	if flags.Config != "/tmp/cfg.yaml" {
		t.Errorf("Config = %q, want /tmp/cfg.yaml", flags.Config)
	}
	if len(cleaned) != 2 || cleaned[0] != "install" || cleaned[1] != "htop" {
		t.Errorf("cleaned = %v, want [install htop]", cleaned)
	}
}

func TestStripGlobalFlags_ConfigEquals(t *testing.T) {
	flags := &cliutil.GlobalFlags{}
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
	name        string
	displayName string
	lastArgs    []string
	lastFlags   *cliutil.GlobalFlags
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) DisplayName() string { return m.displayName }
func (m *mockProvider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	m.lastArgs = args
	m.lastFlags = flags
	return nil
}

func TestRouteToProvider_HelpIntercept(t *testing.T) {
	mock := &mockProvider{name: "brew", displayName: "Homebrew"}

	err := routeToProvider(mock, []string{"install", "--help"}, &cliutil.GlobalFlags{})
	if err != nil {
		t.Fatalf("routeToProvider --help error: %v", err)
	}

	if mock.lastArgs != nil {
		t.Error("provider should not receive args when --help is present")
	}
}

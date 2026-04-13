package cli

import (
	"testing"
)

func TestSplitHamsFlags_Basic(t *testing.T) {
	hams, pass := SplitHamsFlags([]string{"install", "htop", "--hams:tag=devtools", "--cask"})
	if hams["tag"] != "devtools" {
		t.Errorf("hams[tag] = %q, want 'devtools'", hams["tag"])
	}
	if len(pass) != 3 || pass[0] != "install" || pass[1] != "htop" || pass[2] != "--cask" {
		t.Errorf("passthrough = %v, want [install htop --cask]", pass)
	}
}

func TestSplitHamsFlags_ForceForward(t *testing.T) {
	hams, pass := SplitHamsFlags([]string{"install", "--", "--hams:tag=foo", "--cask"})
	if len(hams) != 0 {
		t.Errorf("hams flags should be empty after --, got %v", hams)
	}
	if len(pass) != 3 || pass[0] != "install" || pass[1] != "--hams:tag=foo" || pass[2] != "--cask" {
		t.Errorf("passthrough = %v, want [install --hams:tag=foo --cask]", pass)
	}
}

func TestSplitHamsFlags_BooleanFlag(t *testing.T) {
	hams, _ := SplitHamsFlags([]string{"install", "htop", "--hams:lucky", "--hams:local"})
	if _, ok := hams["lucky"]; !ok {
		t.Error("hams[lucky] should exist")
	}
	if _, ok := hams["local"]; !ok {
		t.Error("hams[local] should exist")
	}
}

func TestSplitHamsFlags_MultipleValues(t *testing.T) {
	hams, _ := SplitHamsFlags([]string{"install", "--hams:tag=dev,network"})
	if hams["tag"] != "dev,network" {
		t.Errorf("hams[tag] = %q, want 'dev,network'", hams["tag"])
	}
}

func TestSplitHamsFlags_NoHamsFlags(t *testing.T) {
	hams, pass := SplitHamsFlags([]string{"install", "htop", "--cask"})
	if len(hams) != 0 {
		t.Errorf("hams flags should be empty, got %v", hams)
	}
	if len(pass) != 3 {
		t.Errorf("passthrough = %v, want 3 items", pass)
	}
}

// Mock provider for testing.
type mockProvider struct {
	name        string
	displayName string
	lastArgs    []string
	lastFlags   *GlobalFlags
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) DisplayName() string { return m.displayName }
func (m *mockProvider) HandleCommand(args []string, flags *GlobalFlags) error {
	m.lastArgs = args
	m.lastFlags = flags
	return nil
}

func TestRegisterProvider_And_Route(t *testing.T) {
	// Save and restore registry.
	old := providerRegistry
	providerRegistry = make(map[string]ProviderHandler)
	defer func() { providerRegistry = old }()

	mock := &mockProvider{name: "brew", displayName: "Homebrew"}
	RegisterProvider(mock)

	flags := &GlobalFlags{Debug: true}
	root, _ := NewRootCmd()
	AddProviderCommands(root, flags)

	root.SetArgs([]string{"brew", "install", "htop"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if len(mock.lastArgs) != 2 || mock.lastArgs[0] != "install" || mock.lastArgs[1] != "htop" {
		t.Errorf("lastArgs = %v, want [install htop]", mock.lastArgs)
	}
	if !mock.lastFlags.Debug {
		t.Error("Debug flag should be propagated")
	}
}

func TestRouteToProvider_HelpIntercept(t *testing.T) {
	mock := &mockProvider{name: "brew", displayName: "Homebrew"}

	// --help should be intercepted, not forwarded to provider.
	err := routeToProvider(mock, []string{"install", "--help"}, &GlobalFlags{})
	if err != nil {
		t.Fatalf("routeToProvider --help error: %v", err)
	}

	// Provider should NOT have received the command.
	if mock.lastArgs != nil {
		t.Error("provider should not receive args when --help is present")
	}
}

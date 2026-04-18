package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

// TestApply_AutoInit_WritesConfigOnFirstRun exercises the "fresh
// machine, one-shot apply" workflow:
//
//   - Global config file does NOT exist (pristine machine).
//   - User passes `--tag=macOS` (plus a valid --store pointing at a
//     local fixture store so the apply has somewhere to read from).
//   - Hams auto-initializes ~/.config/hams/hams.config.yaml with
//     profile_tag=macOS + machine_id=<DeriveMachineID result>.
//
// Regression gate for CLAUDE.md's Current Tasks bullet: "If no hams
// config file exists when `hams apply` runs, auto-create one at the
// default location".
func TestApply_AutoInit_WritesConfigOnFirstRun(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	storeDir := filepath.Join(root, "store")
	profileDir := filepath.Join(storeDir, "macOS")

	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	// Store needs a hams.config.yaml to load — ResolvePaths requires
	// it as the StorePath anchor, but the body can be minimal.
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"),
		"provider_priority: []\n")

	// Pre-flight invariant: no global config exists yet.
	if _, err := os.Stat(filepath.Join(configHome, "hams.config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("pre-flight: global config should not exist; stat err = %v", err)
	}

	// Pin DeriveMachineID to a deterministic value for this test so
	// the asserted machine_id string is stable across CI hosts.
	origHostname := config.HostnameLookup
	defer func() { config.HostnameLookup = origHostname }()
	config.HostnameLookup = func() (string, error) { return "ci-runner", nil }
	t.Setenv("HAMS_MACHINE_ID", "")

	flags := &provider.GlobalFlags{Store: storeDir, Tag: "macOS"}
	registry := provider.NewRegistry()

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{},
		"", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply first run: %v", err)
	}

	// Post-apply invariant: global config materialized with the
	// values we supplied + derived.
	data, err := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("global config not created: %v", err)
	}
	body := string(data)
	for _, want := range []string{"profile_tag: macOS", "machine_id: ci-runner"} {
		if !strings.Contains(body, want) {
			t.Errorf("global config missing %q; got:\n%s", want, body)
		}
	}
}

// TestApply_AutoInit_DoesNotOverwriteExistingConfig locks in the
// "first-run scope" invariant: if the user ALREADY has a global
// config (even partial), auto-init MUST NOT clobber it. Matches the
// openspec proposal section "Auto-init is lossless".
func TestApply_AutoInit_DoesNotOverwriteExistingConfig(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	storeDir := filepath.Join(root, "store")
	profileDir := filepath.Join(storeDir, "linux")

	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}

	// Pre-existing global config: user carefully set linux + their_host.
	preExisting := "profile_tag: linux\nmachine_id: their_host\n"
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"), preExisting)
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"),
		"provider_priority: []\n")

	// Caller passes --tag=macOS, a DIFFERENT value. The expectation
	// is that the CLI flag wins for this invocation but the
	// persisted config remains linux/their_host.
	flags := &provider.GlobalFlags{Store: storeDir, Tag: "macOS"}
	registry := provider.NewRegistry()

	// Expect failure because the `macOS` profile dir doesn't exist —
	// but the failure happens AFTER config is loaded, and the
	// auto-init path should not have fired. Either error or success,
	// the assertion afterward is on the config file contents.
	applyErr := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{},
		"", true, "", "", false, bootstrapMode{})
	_ = applyErr // intentionally ignored — the invariant under test
	//             is the config file contents, not the apply outcome.

	data, err := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("global config missing post-run: %v", err)
	}
	if string(data) != preExisting {
		t.Errorf("pre-existing global config was mutated.\nwant:\n%s\ngot:\n%s",
			preExisting, string(data))
	}
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

func TestResolveBootstrapConsent_DenyFlagShortCircuits(t *testing.T) {
	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{Deny: true}, brerr); got != bootDecisionDeny {
		t.Fatalf("expected Deny, got %d", got)
	}
}

func TestResolveBootstrapConsent_AllowFlagRuns(t *testing.T) {
	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{Allow: true}, brerr); got != bootDecisionRun {
		t.Fatalf("expected Run, got %d", got)
	}
}

func TestResolveBootstrapConsent_NonTTYDefaultsToDeny(t *testing.T) {
	origTTY := bootstrapPromptIsTTY
	defer func() { bootstrapPromptIsTTY = origTTY }()
	bootstrapPromptIsTTY = func() bool { return false }

	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{}, brerr); got != bootDecisionDeny {
		t.Fatalf("expected Deny on non-TTY, got %d", got)
	}
}

func TestResolveBootstrapConsent_TTYYesRuns(t *testing.T) {
	origTTY := bootstrapPromptIsTTY
	origIn, origOut := bootstrapPromptIn, bootstrapPromptOut
	defer func() {
		bootstrapPromptIsTTY = origTTY
		bootstrapPromptIn = origIn
		bootstrapPromptOut = origOut
	}()
	bootstrapPromptIsTTY = func() bool { return true }
	bootstrapPromptIn = strings.NewReader("y\n")
	var out bytes.Buffer
	bootstrapPromptOut = &out

	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "echo yes"}
	if got := resolveBootstrapConsent(bootstrapMode{}, brerr); got != bootDecisionRun {
		t.Fatalf("expected Run after 'y', got %d", got)
	}
	if !strings.Contains(out.String(), "echo yes") {
		t.Errorf("prompt output should include the script text, got %q", out.String())
	}
	if !strings.Contains(out.String(), "sudo password") {
		t.Errorf("prompt output should warn about sudo, got %q", out.String())
	}
}

func TestResolveBootstrapConsent_TTYNoDenies(t *testing.T) {
	origTTY := bootstrapPromptIsTTY
	origIn := bootstrapPromptIn
	defer func() {
		bootstrapPromptIsTTY = origTTY
		bootstrapPromptIn = origIn
	}()
	bootstrapPromptIsTTY = func() bool { return true }
	bootstrapPromptIn = strings.NewReader("n\n")
	bootstrapPromptOut = &bytes.Buffer{}

	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{}, brerr); got != bootDecisionDeny {
		t.Fatalf("expected Deny after 'n', got %d", got)
	}
}

func TestResolveBootstrapConsent_TTYEmptyAnswerDenies(t *testing.T) {
	origTTY := bootstrapPromptIsTTY
	origIn := bootstrapPromptIn
	defer func() {
		bootstrapPromptIsTTY = origTTY
		bootstrapPromptIn = origIn
	}()
	bootstrapPromptIsTTY = func() bool { return true }
	bootstrapPromptIn = strings.NewReader("\n")
	bootstrapPromptOut = &bytes.Buffer{}

	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{}, brerr); got != bootDecisionDeny {
		t.Fatalf("expected Deny on empty (default=N), got %d", got)
	}
}

func TestResolveBootstrapConsent_TTYSkipReturnsSkipProvider(t *testing.T) {
	origTTY := bootstrapPromptIsTTY
	origIn := bootstrapPromptIn
	defer func() {
		bootstrapPromptIsTTY = origTTY
		bootstrapPromptIn = origIn
	}()
	bootstrapPromptIsTTY = func() bool { return true }
	bootstrapPromptIn = strings.NewReader("s\n")
	bootstrapPromptOut = &bytes.Buffer{}

	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{}, brerr); got != bootDecisionSkipProvider {
		t.Fatalf("expected SkipProvider after 's', got %d", got)
	}
}

func TestResolveBootstrapConsent_EOFTreatsAsDeny(t *testing.T) {
	origTTY := bootstrapPromptIsTTY
	origIn := bootstrapPromptIn
	defer func() {
		bootstrapPromptIsTTY = origTTY
		bootstrapPromptIn = origIn
	}()
	bootstrapPromptIsTTY = func() bool { return true }
	bootstrapPromptIn = strings.NewReader("") // immediate EOF
	bootstrapPromptOut = &bytes.Buffer{}

	brerr := &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
	if got := resolveBootstrapConsent(bootstrapMode{}, brerr); got != bootDecisionDeny {
		t.Fatalf("expected Deny on EOF, got %d", got)
	}
}

// End-to-end test: runApply with a provider that signals
// BootstrapRequiredError, --no-bootstrap, verifies fail-fast error.
func TestRunApply_NoBootstrapFailsFastWithActionableError(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")

	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		bootstrapFn: func(context.Context) error {
			return &provider.BootstrapRequiredError{
				Provider: "brew", Binary: "brew", Script: "install.sh",
			}
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}

	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{Deny: true})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Errorf("expected 'bootstrap failed' in error message, got %q", err.Error())
	}
}

// End-to-end test: runApply with --bootstrap and a provider whose
// Bootstrap returns ErrBootstrapRequired, verifies that RunBootstrap is
// called via the registered bash provider and Bootstrap is retried.
func TestRunApply_BootstrapFlagDelegatesThroughBashProvider(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"bash", "brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")
	writeApplyTestFile(t, filepath.Join(profileDir, "bash.hams.yaml"), "packages: []\n")

	var (
		scriptInvocations []string
		brewLookups       int
	)

	// Bash provider must implement BashScriptRunner.
	bash := &bashRunnerFake{
		applyTestProvider: applyTestProvider{
			manifest: provider.Manifest{
				Name: "bash", DisplayName: "bash", FilePrefix: "bash",
				Platforms: []provider.Platform{provider.PlatformAll},
			},
		},
		runScript: func(_ context.Context, script string) error {
			scriptInvocations = append(scriptInvocations, script)
			return nil
		},
	}

	// Brew provider: first Bootstrap says "need bootstrap"; second returns nil.
	brew := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
			Platforms: []provider.Platform{provider.PlatformAll},
			DependsOn: []provider.DependOn{{Provider: "bash", Script: "my-install-script"}},
		},
		bootstrapFn: func(context.Context) error {
			brewLookups++
			if brewLookups == 1 {
				return &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "my-install-script"}
			}
			return nil
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(bash); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	if err := registry.Register(brew); err != nil {
		t.Fatalf("register brew: %v", err)
	}

	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{Allow: true})
	if err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if brewLookups != 2 {
		t.Errorf("expected Bootstrap to be called twice (pre/post script), got %d", brewLookups)
	}
	if len(scriptInvocations) != 1 || scriptInvocations[0] != "my-install-script" {
		t.Errorf("bash RunScript invocations = %v, want ['my-install-script']", scriptInvocations)
	}
}

// End-to-end test: runApply with --bootstrap but the script fails. The
// bootstrap-failed path must capture the error as fatal (hamsfile
// exists) and return a UserFacingError naming the provider.
func TestRunApply_BootstrapScriptFailureIsFatal(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"bash", "brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")
	writeApplyTestFile(t, filepath.Join(profileDir, "bash.hams.yaml"), "packages: []\n")

	bash := &bashRunnerFake{
		applyTestProvider: applyTestProvider{
			manifest: provider.Manifest{
				Name: "bash", DisplayName: "bash", FilePrefix: "bash",
				Platforms: []provider.Platform{provider.PlatformAll},
			},
		},
		runScript: func(context.Context, string) error {
			return errors.New("install.sh exited 127")
		},
	}

	brew := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
			Platforms: []provider.Platform{provider.PlatformAll},
			DependsOn: []provider.DependOn{{Provider: "bash", Script: "install.sh"}},
		},
		bootstrapFn: func(context.Context) error {
			return &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "install.sh"}
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(bash); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	if err := registry.Register(brew); err != nil {
		t.Fatalf("register brew: %v", err)
	}

	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{Allow: true})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") || !strings.Contains(err.Error(), "brew") {
		t.Errorf("expected 'bootstrap failed' + brew in error, got %q", err.Error())
	}
}

// End-to-end test: --bootstrap AND --no-bootstrap are mutually exclusive.
func TestRunApply_BootstrapFlagsMutuallyExclusive(t *testing.T) {
	_, _, _, flags := setupApplyTestEnv(t, nil)
	registry := provider.NewRegistry()

	err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{Allow: true, Deny: true})
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got %q", err.Error())
	}
}

// bashRunnerFake embeds applyTestProvider and also implements
// provider.BashScriptRunner for end-to-end bootstrap tests.
type bashRunnerFake struct {
	applyTestProvider
	runScript func(context.Context, string) error
}

func (b *bashRunnerFake) RunScript(ctx context.Context, script string) error {
	return b.runScript(ctx, script)
}

var _ provider.BashScriptRunner = (*bashRunnerFake)(nil)

// hamsfilePresent is deliberately not tested at unit level here — it's a
// thin wrapper over os.Stat exercised by the end-to-end tests above.
// The no-hamsfile branch is exercised by TestRunApply_SkipsStateOnlyProvidersByDefault.
var _ = os.Stat

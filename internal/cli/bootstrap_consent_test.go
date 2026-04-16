package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
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
// Per builtin-providers spec scenario "Bootstrap emits actionable
// error when --bootstrap is not set", the UserFacingError body SHALL
// name the missing binary, the exact script text from the manifest,
// and the `hams apply --bootstrap` remedy.
func TestRunApply_NoBootstrapFailsFastWithActionableError(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")

	const installScript = `/bin/bash -c "$(curl -fsSL https://example.com/install.sh)"`
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		bootstrapFn: func(context.Context) error {
			return &provider.BootstrapRequiredError{
				Provider: "brew", Binary: "brew", Script: installScript,
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

	// Spec contract: the structured error body carries binary + script
	// + remedy. UserFacingError.Suggestions is where those land.
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T (%v)", err, err)
	}
	allSuggestions := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(allSuggestions, "brew") {
		t.Errorf("suggestions should name the missing binary 'brew'; got %q", allSuggestions)
	}
	if !strings.Contains(allSuggestions, installScript) {
		t.Errorf("suggestions should include the install script verbatim; got %q", allSuggestions)
	}
	if !strings.Contains(allSuggestions, "--bootstrap") {
		t.Errorf("suggestions should include the --bootstrap remedy; got %q", allSuggestions)
	}
}

// Dry-run MUST preserve --bootstrap's INTENT (user consented) without
// the side effect: hams prints what WOULD run and leaves the host
// untouched. A dry-run that actually forked /bin/bash to run install.sh
// would violate the core dry-run contract in ways the user would not
// recover from cleanly (partial brew install, PATH mutations, etc).
func TestRunApply_DryRunBootstrapDoesNotExecuteScript(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"bash", "brew"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")
	writeApplyTestFile(t, filepath.Join(profileDir, "bash.hams.yaml"), "packages: []\n")
	flags.DryRun = true // THE test lever

	var scriptInvocations []string
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

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{Allow: true}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	if len(scriptInvocations) != 0 {
		t.Errorf("dry-run must NOT execute the bootstrap script; got invocations %v", scriptInvocations)
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

	// When consent was `Run` but the install script failed, the
	// UserFacingError suggestions MUST still surface the script that
	// was attempted — otherwise users see a generic error with no
	// breadcrumb back to the command that just broke.
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T (%v)", err, err)
	}
	allSuggestions := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(allSuggestions, "install.sh") {
		t.Errorf("suggestions should surface the attempted script 'install.sh' even when --bootstrap failed; got %q", allSuggestions)
	}
	if !strings.Contains(allSuggestions, "brew") {
		t.Errorf("suggestions should name the missing binary 'brew'; got %q", allSuggestions)
	}
}

// When --bootstrap script succeeds but the binary is still missing on
// retry (PATH hydration edge case), the UserFacingError should still
// surface which script was attempted so users can investigate where
// the binary landed.
func TestRunApply_BootstrapRetryStillMissingSurfacesScript(t *testing.T) {
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
		runScript: func(context.Context, string) error { return nil }, // script succeeds
	}

	// Brew provider: both Bootstrap calls signal missing, simulating the
	// PATH-hydration miss case (install.sh exited 0 but brew still
	// unreachable on the process PATH).
	const script = "my-install.sh"
	brew := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
			Platforms: []provider.Platform{provider.PlatformAll},
			DependsOn: []provider.DependOn{{Provider: "bash", Script: script}},
		},
		bootstrapFn: func(context.Context) error {
			return &provider.BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: script}
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
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T (%v)", err, err)
	}
	allSuggestions := strings.Join(ufe.Suggestions, "\n")
	if !strings.Contains(allSuggestions, script) {
		t.Errorf("suggestions should surface the attempted script %q even when retry-still-missing; got %q", script, allSuggestions)
	}
}

// TTY skip ('s' answer) must cascade to DAG-dependent providers so
// hams doesn't silently run vscodeext / mas against a brew that was
// just opted out of. Per the cascading-skip guardrail in apply.go.
func TestRunApply_SkipBootstrapCascadesToDependents(t *testing.T) {
	_, profileDir, _, flags := setupApplyTestEnv(t, []string{"brew", "code-ext"})
	writeApplyTestFile(t, filepath.Join(profileDir, "Homebrew.hams.yaml"), "packages: []\n")
	writeApplyTestFile(t, filepath.Join(profileDir, "vscodeext.hams.yaml"), "packages: []\n")

	// Interactive TTY prompt with 's' (skip-this-provider) answer.
	origTTY := bootstrapPromptIsTTY
	origIn := bootstrapPromptIn
	defer func() {
		bootstrapPromptIsTTY = origTTY
		bootstrapPromptIn = origIn
	}()
	bootstrapPromptIsTTY = func() bool { return true }
	bootstrapPromptIn = strings.NewReader("s\n")
	bootstrapPromptOut = &bytes.Buffer{}

	var bootstrapCalls, probeCalls []string
	brew := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
		bootstrapFn: func(context.Context) error {
			bootstrapCalls = append(bootstrapCalls, "brew")
			return &provider.BootstrapRequiredError{
				Provider: "brew", Binary: "brew", Script: "install.sh",
			}
		},
	}
	codeExt := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "code-ext", DisplayName: "VS Code Extensions", FilePrefix: "vscodeext",
			Platforms: []provider.Platform{provider.PlatformAll},
			DependsOn: []provider.DependOn{{Provider: "brew"}},
		},
		bootstrapFn: func(context.Context) error {
			bootstrapCalls = append(bootstrapCalls, "code-ext")
			return nil
		},
		probeFn: func(context.Context, *state.File) ([]provider.ProbeResult, error) {
			probeCalls = append(probeCalls, "code-ext")
			return nil, nil
		},
	}

	registry := provider.NewRegistry()
	if err := registry.Register(brew); err != nil {
		t.Fatalf("register brew: %v", err)
	}
	if err := registry.Register(codeExt); err != nil {
		t.Fatalf("register code-ext: %v", err)
	}

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply: %v", err)
	}
	// code-ext's Bootstrap is reached before the cascade runs (the
	// loop calls every provider's Bootstrap once). The cascade ensures
	// code-ext is skipped from the REMAINDER of the pipeline.
	// ProbeAll only runs against providers that survived the skip
	// filter — so probeCalls must NOT include code-ext.
	for _, c := range probeCalls {
		if c == "code-ext" {
			t.Errorf("code-ext should have been cascade-skipped; probeCalls = %v", probeCalls)
		}
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

package provider

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// bashFake is a Provider + BashScriptRunner used for bootstrap tests.
type bashFake struct {
	stubProvider
	invocations []string
	err         error
}

func (b *bashFake) RunScript(_ context.Context, script string) error {
	b.invocations = append(b.invocations, script)
	return b.err
}

// bashlessFake is a Provider that does NOT implement BashScriptRunner.
type bashlessFake struct{ stubProvider }

func newBashFake() *bashFake {
	return &bashFake{stubProvider: stubProvider{manifest: Manifest{
		Name: "bash", DisplayName: "Bash", FilePrefix: "bash",
		Platforms: []Platform{PlatformAll},
	}}}
}

func newDependent(deps []DependOn) *stubProvider {
	return &stubProvider{manifest: Manifest{
		Name: "brew", DisplayName: "Homebrew", FilePrefix: "Homebrew",
		Platforms: []Platform{PlatformAll},
		DependsOn: deps,
	}}
}

// Compile-time assertions that our fakes satisfy the interfaces.
var (
	_ Provider         = (*bashFake)(nil)
	_ BashScriptRunner = (*bashFake)(nil)
	_ Provider         = (*bashlessFake)(nil)
	_ = hamsfile.File{} // keep hamsfile import referenced for stubProvider's signature
	_ = state.File{}    // keep state import referenced for stubProvider's signature
)

func TestRunBootstrap_DelegatesRegisteredScript(t *testing.T) {
	t.Parallel()
	runner := newBashFake()
	reg := NewRegistry()
	if err := reg.Register(runner); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	p := newDependent([]DependOn{{Provider: "bash", Script: "echo hi"}})

	if err := RunBootstrap(context.Background(), p, reg); err != nil {
		t.Fatalf("RunBootstrap: %v", err)
	}
	if len(runner.invocations) != 1 || runner.invocations[0] != "echo hi" {
		t.Fatalf("expected one invocation of 'echo hi', got %v", runner.invocations)
	}
}

func TestRunBootstrap_SkipsPlatformGated(t *testing.T) {
	t.Parallel()
	runner := newBashFake()
	reg := NewRegistry()
	if err := reg.Register(runner); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	otherOS := Platform("darwin")
	if runtime.GOOS == "darwin" {
		otherOS = Platform("linux")
	}
	p := newDependent([]DependOn{{Provider: "bash", Script: "echo skipped", Platform: otherOS}})

	if err := RunBootstrap(context.Background(), p, reg); err != nil {
		t.Fatalf("RunBootstrap: %v", err)
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("expected zero invocations, got %v", runner.invocations)
	}
}

func TestRunBootstrap_ErrorsOnMissingHostProvider(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	p := newDependent([]DependOn{{Provider: "bash", Script: "echo nope"}})

	err := RunBootstrap(context.Background(), p, reg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bash") || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("error %q should mention missing provider name and 'not registered'", err)
	}
}

func TestRunBootstrap_ErrorsOnNonBashHost(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	bl := &bashlessFake{stubProvider: stubProvider{manifest: Manifest{
		Name: "bash", DisplayName: "Bash", FilePrefix: "bash",
		Platforms: []Platform{PlatformAll},
	}}}
	if err := reg.Register(bl); err != nil {
		t.Fatalf("register bashless: %v", err)
	}
	p := newDependent([]DependOn{{Provider: "bash", Script: "echo nope"}})

	err := RunBootstrap(context.Background(), p, reg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "BashScriptRunner") {
		t.Fatalf("error %q should mention BashScriptRunner", err)
	}
}

func TestRunBootstrap_PropagatesScriptError(t *testing.T) {
	t.Parallel()
	bang := errors.New("script exploded")
	runner := newBashFake()
	runner.err = bang
	reg := NewRegistry()
	if err := reg.Register(runner); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	p := newDependent([]DependOn{{Provider: "bash", Script: "false"}})

	err := RunBootstrap(context.Background(), p, reg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, bang) {
		t.Fatalf("error should wrap the underlying script error, got %v", err)
	}
}

func TestRunBootstrap_SkipsEmptyScriptEntries(t *testing.T) {
	t.Parallel()
	runner := newBashFake()
	reg := NewRegistry()
	if err := reg.Register(runner); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	p := newDependent([]DependOn{{Provider: "bash"}})

	if err := RunBootstrap(context.Background(), p, reg); err != nil {
		t.Fatalf("RunBootstrap: %v", err)
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("expected zero invocations for empty-script entry, got %v", runner.invocations)
	}
}

func TestRunBootstrap_AllEntriesInOrder(t *testing.T) {
	t.Parallel()
	runner := newBashFake()
	reg := NewRegistry()
	if err := reg.Register(runner); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	p := newDependent([]DependOn{
		{Provider: "bash", Script: "first"},
		{Provider: "bash", Script: "second"},
	})

	if err := RunBootstrap(context.Background(), p, reg); err != nil {
		t.Fatalf("RunBootstrap: %v", err)
	}
	if len(runner.invocations) != 2 || runner.invocations[0] != "first" || runner.invocations[1] != "second" {
		t.Fatalf("invocations not in declaration order: %v", runner.invocations)
	}
}

func TestBootstrapAllowed_ContextDefaults(t *testing.T) {
	t.Parallel()
	if BootstrapAllowed(context.Background()) {
		t.Fatalf("expected false by default")
	}
	ctx := WithBootstrapAllowed(context.Background(), true)
	if !BootstrapAllowed(ctx) {
		t.Fatalf("expected true after WithBootstrapAllowed(true)")
	}
	ctx = WithBootstrapAllowed(ctx, false)
	if BootstrapAllowed(ctx) {
		t.Fatalf("expected false after WithBootstrapAllowed(false)")
	}
}

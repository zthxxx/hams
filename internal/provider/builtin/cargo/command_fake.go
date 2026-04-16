package cargo

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It records
// every call it receives, maintains a virtual "installed crates" set
// keyed by crate name (with version), and supports configured failures
// for install/uninstall to simulate cargo errors without ever shelling
// out.
type FakeCmdRunner struct {
	mu              sync.Mutex
	installed       map[string]string // crate name → version
	calls           []fakeCall
	installErrors   map[string]error
	uninstallErrors map[string]error
	lookPathError   error
}

type fakeCall struct {
	op   string
	args string // for Install/Uninstall: the crate; for List: ""
}

const (
	fakeOpList      = "list"
	fakeOpInstall   = "install"
	fakeOpUninstall = "uninstall"
	fakeOpLookPath  = "lookpath"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner with no installed
// crates and no configured errors.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		installed:       make(map[string]string),
		installErrors:   make(map[string]error),
		uninstallErrors: make(map[string]error),
	}
}

// Seed marks crate as installed at the given version before the test
// starts. Returns the receiver for fluent chaining.
func (f *FakeCmdRunner) Seed(crate, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[crate] = version
	return f
}

// WithInstallError makes subsequent Install(ctx, crate) calls return err.
// The crate is NOT added to the installed set when an error is configured.
func (f *FakeCmdRunner) WithInstallError(crate string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[crate] = err
	return f
}

// WithUninstallError makes subsequent Uninstall(ctx, crate) calls return err.
// The crate stays in the installed set when an error is configured.
func (f *FakeCmdRunner) WithUninstallError(crate string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallErrors[crate] = err
	return f
}

// WithLookPathError configures the fake to return err from LookPath,
// simulating "cargo not on $PATH". Use to test Bootstrap failure paths.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// List implements CmdRunner. Returns the synthesized `cargo install
// --list` output for the current installed set, in cargo's documented
// format ("name vX.Y.Z:\n    binary\n").
func (f *FakeCmdRunner) List(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpList})

	var b strings.Builder
	for crate, ver := range f.installed {
		fmt.Fprintf(&b, "%s v%s:\n    %s\n", crate, ver, crate)
	}
	return b.String(), nil
}

// Install implements CmdRunner.
func (f *FakeCmdRunner) Install(_ context.Context, crate string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, args: crate})
	if err, ok := f.installErrors[crate]; ok {
		return err
	}
	f.installed[crate] = "fake-1.0.0"
	return nil
}

// Uninstall implements CmdRunner.
func (f *FakeCmdRunner) Uninstall(_ context.Context, crate string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpUninstall, args: crate})
	if err, ok := f.uninstallErrors[crate]; ok {
		return err
	}
	delete(f.installed, crate)
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. crate filters by
// the installed-crate name (pass "" to count any call to op).
func (f *FakeCmdRunner) CallCount(op, crate string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if crate == "" || c.args == crate {
			n++
		}
	}
	return n
}

// IsInstalled reports whether the fake currently models crate as
// installed. Tests use this to verify the post-condition of an
// Install/Uninstall call.
func (f *FakeCmdRunner) IsInstalled(crate string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.installed[crate]
	return ok
}

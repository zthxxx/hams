package apt

import (
	"context"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It records every
// call it receives, maintains a virtual "installed" set, and supports
// configured failures for install/remove to simulate apt-get errors without
// ever shelling out.
type FakeCmdRunner struct {
	mu            sync.Mutex
	installed     map[string]string
	calls         []fakeCall
	installErrors map[string]error
	removeErrors  map[string]error
}

type fakeCall struct {
	op  string
	pkg string
}

const (
	fakeOpInstall     = "install"
	fakeOpRemove      = "remove"
	fakeOpIsInstalled = "is_installed"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner with no installed packages
// and no configured errors.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		installed:     make(map[string]string),
		installErrors: make(map[string]error),
		removeErrors:  make(map[string]error),
	}
}

// Seed marks pkg as installed at the given version before the test starts.
func (f *FakeCmdRunner) Seed(pkg, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[pkg] = version
	return f
}

// WithInstallError makes subsequent Install(ctx, pkg) calls return err.
// The package is NOT added to the installed set when an error is configured.
func (f *FakeCmdRunner) WithInstallError(pkg string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[pkg] = err
	return f
}

// WithRemoveError makes subsequent Remove(ctx, pkg) calls return err.
// The package stays in the installed set when an error is configured.
func (f *FakeCmdRunner) WithRemoveError(pkg string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeErrors[pkg] = err
	return f
}

// Install implements CmdRunner.
func (f *FakeCmdRunner) Install(_ context.Context, pkg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, pkg: pkg})
	if err, ok := f.installErrors[pkg]; ok {
		return err
	}
	f.installed[pkg] = "fake-1.0.0"
	return nil
}

// Remove implements CmdRunner.
func (f *FakeCmdRunner) Remove(_ context.Context, pkg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpRemove, pkg: pkg})
	if err, ok := f.removeErrors[pkg]; ok {
		return err
	}
	delete(f.installed, pkg)
	return nil
}

// IsInstalled implements CmdRunner.
func (f *FakeCmdRunner) IsInstalled(_ context.Context, pkg string) (installed bool, version string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpIsInstalled, pkg: pkg})
	v, ok := f.installed[pkg]
	return ok, v, nil
}

// CallCount returns how many times op was invoked for pkg (pkg == "" to count
// any pkg). op is one of the fakeOp* constants.
func (f *FakeCmdRunner) CallCount(op, pkg string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if pkg == "" || c.pkg == pkg {
			n++
		}
	}
	return n
}

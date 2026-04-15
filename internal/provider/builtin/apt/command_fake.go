package apt

import (
	"context"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It records every
// call it receives, maintains a virtual "installed" set, and supports
// configured failures for install/remove to simulate apt-get errors without
// ever shelling out.
//
// FakeCmdRunner is safe for concurrent use but test parallelism is rarely
// useful here — tests usually exercise a single flow at a time.
type FakeCmdRunner struct {
	mu sync.Mutex

	// Installed maps package name → version for packages currently installed
	// from the fake's perspective. Seed it before a test if IsInstalled
	// should return true for given packages.
	Installed map[string]string

	// Calls records every invocation in order.
	Calls []FakeCall

	installErrors map[string]error
	removeErrors  map[string]error
}

// FakeCall captures one CmdRunner method call.
type FakeCall struct {
	Op  string // "install" | "remove" | "is_installed"
	Pkg string
}

// NewFakeCmdRunner returns a fresh FakeCmdRunner with no installed packages
// and no configured errors.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		Installed:     make(map[string]string),
		installErrors: make(map[string]error),
		removeErrors:  make(map[string]error),
	}
}

// WithInstallError makes subsequent Install(ctx, pkg) calls return err.
// The package is NOT added to Installed when an error is configured.
func (f *FakeCmdRunner) WithInstallError(pkg string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[pkg] = err
	return f
}

// WithRemoveError makes subsequent Remove(ctx, pkg) calls return err.
// The package stays in Installed when an error is configured.
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
	f.Calls = append(f.Calls, FakeCall{Op: "install", Pkg: pkg})
	if err, ok := f.installErrors[pkg]; ok {
		return err
	}
	f.Installed[pkg] = "fake-1.0.0"
	return nil
}

// Remove implements CmdRunner.
func (f *FakeCmdRunner) Remove(_ context.Context, pkg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Op: "remove", Pkg: pkg})
	if err, ok := f.removeErrors[pkg]; ok {
		return err
	}
	delete(f.Installed, pkg)
	return nil
}

// IsInstalled implements CmdRunner.
func (f *FakeCmdRunner) IsInstalled(_ context.Context, pkg string) (installed bool, version string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Op: "is_installed", Pkg: pkg})
	v, ok := f.Installed[pkg]
	return ok, v, nil
}

// CallCount returns how many times op was invoked for pkg (pkg == "" to count
// any pkg).
func (f *FakeCmdRunner) CallCount(op, pkg string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if c.Op != op {
			continue
		}
		if pkg == "" || c.Pkg == pkg {
			n++
		}
	}
	return n
}

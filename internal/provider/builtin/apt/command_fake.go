package apt

import (
	"context"
	"slices"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It records every
// call it receives, maintains a virtual "installed" set, and supports
// configured failures for install/remove to simulate apt-get errors without
// ever shelling out. Install/Remove model apt-get's transactional semantics:
// if any package in the args triggers a configured error, NO package in the
// batch transitions (matching apt-get's "all-or-nothing on dep resolution").
type FakeCmdRunner struct {
	mu            sync.Mutex
	installed     map[string]string
	calls         []fakeCall
	installErrors map[string]error
	removeErrors  map[string]error
}

type fakeCall struct {
	op   string
	args []string
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

// Install implements CmdRunner. Models apt-get's transactional install:
// if any package name in args has a configured installError, the batch
// fails atomically — no package transitions to installed.
func (f *FakeCmdRunner) Install(_ context.Context, args []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, args: append([]string(nil), args...)})
	pkgs := pkgArgsOnly(args)
	for _, pkg := range pkgs {
		if err, ok := f.installErrors[pkg]; ok {
			return err
		}
	}
	for _, pkg := range pkgs {
		f.installed[pkg] = "fake-1.0.0"
	}
	return nil
}

// Remove implements CmdRunner. Same transactional semantics as Install.
func (f *FakeCmdRunner) Remove(_ context.Context, args []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpRemove, args: append([]string(nil), args...)})
	pkgs := pkgArgsOnly(args)
	for _, pkg := range pkgs {
		if err, ok := f.removeErrors[pkg]; ok {
			return err
		}
	}
	for _, pkg := range pkgs {
		delete(f.installed, pkg)
	}
	return nil
}

// IsInstalled implements CmdRunner.
func (f *FakeCmdRunner) IsInstalled(_ context.Context, pkg string) (installed bool, version string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpIsInstalled, args: []string{pkg}})
	v, ok := f.installed[pkg]
	return ok, v, nil
}

// CallCount returns how many times op was invoked containing pkg (pkg == ""
// to count any call). op is one of the fakeOp* constants.
func (f *FakeCmdRunner) CallCount(op, pkg string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if pkg == "" {
			n++
			continue
		}
		if slices.Contains(c.args, pkg) {
			n++
		}
	}
	return n
}

// LastCallArgs returns the args slice of the most recent call to op, or nil
// if no such call exists. Tests use this to assert flag passthrough.
func (f *FakeCmdRunner) LastCallArgs(op string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].op == op {
			return append([]string(nil), f.calls[i].args...)
		}
	}
	return nil
}

// pkgArgsOnly returns the non-flag entries from args. Mirrors packageArgs
// in apt.go but kept private to the fake.
func pkgArgsOnly(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a != "" && a[0] == '-' {
			continue
		}
		out = append(out, a)
	}
	return out
}

package goinstall

import (
	"context"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. The
// installed-set is keyed on the canonical pkg form (post-@latest
// injection), so tests assert against the same form the production
// provider stores in state.
type FakeCmdRunner struct {
	mu            sync.Mutex
	installed     map[string]bool
	calls         []fakeCall
	installErrors map[string]error
	lookPathError error
}

type fakeCall struct {
	op  string
	pkg string
}

const (
	fakeOpInstall  = "install"
	fakeOpProbe    = "is_installed"
	fakeOpLookPath = "lookpath"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		installed:     make(map[string]bool),
		installErrors: make(map[string]error),
	}
}

// Seed marks pkg as installed (any version-suffix form maps to the
// same binary; tests pass the bare module path).
func (f *FakeCmdRunner) Seed(pkg string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[pkg] = true
	return f
}

// WithInstallError configures Install(pkg) to return err.
func (f *FakeCmdRunner) WithInstallError(pkg string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[pkg] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
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
	// Strip @version when storing — IsBinaryInstalled is called later
	// with the bare module path.
	bare := stripVersion(pkg)
	f.installed[bare] = true
	return nil
}

// IsBinaryInstalled implements CmdRunner.
func (f *FakeCmdRunner) IsBinaryInstalled(_ context.Context, pkg string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpProbe, pkg: pkg})
	return f.installed[stripVersion(pkg)]
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. pkg filters by the
// install-target string (pass "" to count any call to op).
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

// stripVersion returns pkg with any @version suffix removed.
func stripVersion(pkg string) string {
	for i, r := range pkg {
		if r == '@' {
			return pkg[:i]
		}
	}
	return pkg
}

package npm

import (
	"context"
	"encoding/json"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. List
// synthesizes the npm-list JSON shape from the virtual installed set
// so the production parser path is exercised end-to-end.
type FakeCmdRunner struct {
	mu              sync.Mutex
	installed       map[string]string
	calls           []fakeCall
	installErrors   map[string]error
	uninstallErrors map[string]error
	lookPathError   error
}

type fakeCall struct {
	op  string
	pkg string
}

const (
	fakeOpList      = "list"
	fakeOpInstall   = "install"
	fakeOpUninstall = "uninstall"
	fakeOpLookPath  = "lookpath"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		installed:       make(map[string]string),
		installErrors:   make(map[string]error),
		uninstallErrors: make(map[string]error),
	}
}

// Seed marks pkg as installed at the given version.
func (f *FakeCmdRunner) Seed(pkg, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[pkg] = version
	return f
}

// WithInstallError configures Install(pkg) to return err.
func (f *FakeCmdRunner) WithInstallError(pkg string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[pkg] = err
	return f
}

// WithUninstallError configures Uninstall(pkg) to return err.
func (f *FakeCmdRunner) WithUninstallError(pkg string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallErrors[pkg] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// List implements CmdRunner. Synthesizes the {"dependencies": {...}}
// JSON shape from the virtual installed set so parseNpmList exercises
// its full parsing path.
func (f *FakeCmdRunner) List(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpList})

	deps := map[string]map[string]string{}
	for pkg, ver := range f.installed {
		deps[pkg] = map[string]string{"version": ver}
	}
	raw, err := json.Marshal(map[string]any{"dependencies": deps})
	if err != nil {
		return "", err
	}
	return string(raw), nil
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

// Uninstall implements CmdRunner.
func (f *FakeCmdRunner) Uninstall(_ context.Context, pkg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpUninstall, pkg: pkg})
	if err, ok := f.uninstallErrors[pkg]; ok {
		return err
	}
	delete(f.installed, pkg)
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. pkg filters by the
// installed-package name (pass "" to count any call to op).
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

// IsInstalled reports whether the fake currently models pkg as installed.
func (f *FakeCmdRunner) IsInstalled(pkg string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.installed[pkg]
	return ok
}

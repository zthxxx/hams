package uv

import (
	"context"
	"strings"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests.
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

// Seed marks tool as installed at the given version.
func (f *FakeCmdRunner) Seed(tool, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[tool] = version
	return f
}

// WithInstallError configures Install(tool) to return err.
func (f *FakeCmdRunner) WithInstallError(tool string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[tool] = err
	return f
}

// WithUninstallError configures Uninstall(tool) to return err.
func (f *FakeCmdRunner) WithUninstallError(tool string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallErrors[tool] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// List implements CmdRunner. Synthesizes "tool vX.Y.Z\n" lines from
// the virtual installed-set in cargo's documented format.
func (f *FakeCmdRunner) List(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpList})

	var b strings.Builder
	for tool, ver := range f.installed {
		b.WriteString(tool)
		b.WriteString(" v")
		b.WriteString(ver)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// Install implements CmdRunner.
func (f *FakeCmdRunner) Install(_ context.Context, tool string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, pkg: tool})
	if err, ok := f.installErrors[tool]; ok {
		return err
	}
	f.installed[tool] = "fake-1.0.0"
	return nil
}

// Uninstall implements CmdRunner.
func (f *FakeCmdRunner) Uninstall(_ context.Context, tool string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpUninstall, pkg: tool})
	if err, ok := f.uninstallErrors[tool]; ok {
		return err
	}
	delete(f.installed, tool)
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. tool filters by
// package name (pass "" to count any call to op).
func (f *FakeCmdRunner) CallCount(op, tool string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if tool == "" || c.pkg == tool {
			n++
		}
	}
	return n
}

// IsInstalled reports whether the fake currently models tool as installed.
func (f *FakeCmdRunner) IsInstalled(tool string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.installed[tool]
	return ok
}

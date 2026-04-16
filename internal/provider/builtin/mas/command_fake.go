package mas

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. List
// synthesizes the mas-list line format from the virtual installed
// set so the production parseMasList path is exercised end-to-end.
type FakeCmdRunner struct {
	mu              sync.Mutex
	installed       map[string]string
	calls           []fakeCall
	installErrors   map[string]error
	uninstallErrors map[string]error
	lookPathError   error
}

type fakeCall struct {
	op    string
	appID string
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

// Seed marks appID as installed at the given version.
func (f *FakeCmdRunner) Seed(appID, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[appID] = version
	return f
}

// WithInstallError configures Install(appID) to return err.
func (f *FakeCmdRunner) WithInstallError(appID string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[appID] = err
	return f
}

// WithUninstallError configures Uninstall(appID) to return err.
func (f *FakeCmdRunner) WithUninstallError(appID string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallErrors[appID] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// List implements CmdRunner. Synthesizes "<appID>  AppName (version)"
// lines per parseMasList's documented format.
func (f *FakeCmdRunner) List(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpList})

	var b strings.Builder
	for appID, ver := range f.installed {
		fmt.Fprintf(&b, "%s  Name (%s)\n", appID, ver)
	}
	return b.String(), nil
}

// Install implements CmdRunner.
func (f *FakeCmdRunner) Install(_ context.Context, appID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, appID: appID})
	if err, ok := f.installErrors[appID]; ok {
		return err
	}
	f.installed[appID] = "fake-1.0.0"
	return nil
}

// Uninstall implements CmdRunner.
func (f *FakeCmdRunner) Uninstall(_ context.Context, appID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpUninstall, appID: appID})
	if err, ok := f.uninstallErrors[appID]; ok {
		return err
	}
	delete(f.installed, appID)
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. appID filters by
// app ID (pass "" to count any call to op).
func (f *FakeCmdRunner) CallCount(op, appID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if appID == "" || c.appID == appID {
			n++
		}
	}
	return n
}

// IsInstalled reports whether the fake currently models appID as installed.
func (f *FakeCmdRunner) IsInstalled(appID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.installed[appID]
	return ok
}

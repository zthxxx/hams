package vscodeext

import (
	"context"
	"strings"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. List
// synthesizes "publisher.extension@version\n" lines per
// parseExtensionList's documented format. Extension IDs are stored
// lowercased so reads are case-insensitive (matches production behavior).
type FakeCmdRunner struct {
	mu              sync.Mutex
	installed       map[string]string // ext id (lowercased) → version
	calls           []fakeCall
	installErrors   map[string]error
	uninstallErrors map[string]error
	lookPathError   error
}

type fakeCall struct {
	op string
	id string
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

// Seed marks id installed at version (id is normalized to lowercase
// to mirror the production parser's behavior).
func (f *FakeCmdRunner) Seed(id, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed[strings.ToLower(id)] = version
	return f
}

// WithInstallError configures Install(id) to return err.
func (f *FakeCmdRunner) WithInstallError(id string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[id] = err
	return f
}

// WithUninstallError configures Uninstall(id) to return err.
func (f *FakeCmdRunner) WithUninstallError(id string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallErrors[id] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// List implements CmdRunner. Synthesizes "id@version\n" lines per the
// real `code --list-extensions --show-versions` output format.
func (f *FakeCmdRunner) List(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpList})

	var b strings.Builder
	for id, ver := range f.installed {
		b.WriteString(id)
		b.WriteString("@")
		b.WriteString(ver)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// Install implements CmdRunner.
func (f *FakeCmdRunner) Install(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, id: id})
	if err, ok := f.installErrors[id]; ok {
		return err
	}
	f.installed[strings.ToLower(id)] = "fake-1.0.0"
	return nil
}

// Uninstall implements CmdRunner.
func (f *FakeCmdRunner) Uninstall(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpUninstall, id: id})
	if err, ok := f.uninstallErrors[id]; ok {
		return err
	}
	delete(f.installed, strings.ToLower(id))
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. id filters by
// extension ID (pass "" for any call to op).
func (f *FakeCmdRunner) CallCount(op, id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if id == "" || c.id == id {
			n++
		}
	}
	return n
}

// IsInstalled reports whether the fake currently models id as
// installed (case-insensitive lookup).
func (f *FakeCmdRunner) IsInstalled(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.installed[strings.ToLower(id)]
	return ok
}

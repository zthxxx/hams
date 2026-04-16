package homebrew

import (
	"context"
	"maps"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It models
// Homebrew's three kinds of installed objects (formulae, casks, taps)
// as three maps. Production's JSON parsing is bypassed — the fake
// returns parsed maps directly.
type FakeCmdRunner struct {
	mu              sync.Mutex
	formulae        map[string]string // formula name → version
	casks           map[string]string // cask token → version
	taps            map[string]bool
	calls           []fakeCall
	installErrors   map[string]error
	uninstallErrors map[string]error
	tapErrors       map[string]error
	formulaeErr     error
	casksErr        error
	tapsErr         error
}

type fakeCall struct {
	op     string
	name   string
	isCask bool
}

const (
	fakeOpListFormulae = "list_formulae"
	fakeOpListCasks    = "list_casks"
	fakeOpListTaps     = "list_taps"
	fakeOpInstall      = "install"
	fakeOpUninstall    = "uninstall"
	fakeOpTap          = "tap"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		formulae:        make(map[string]string),
		casks:           make(map[string]string),
		taps:            make(map[string]bool),
		installErrors:   make(map[string]error),
		uninstallErrors: make(map[string]error),
		tapErrors:       make(map[string]error),
	}
}

// SeedFormula marks name installed as a formula at version.
func (f *FakeCmdRunner) SeedFormula(name, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.formulae[name] = version
	return f
}

// SeedCask marks name installed as a cask at version.
func (f *FakeCmdRunner) SeedCask(name, version string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.casks[name] = version
	return f
}

// SeedTap marks repo as a registered tap.
func (f *FakeCmdRunner) SeedTap(repo string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.taps[repo] = true
	return f
}

// WithInstallError configures Install(name, _) to return err.
func (f *FakeCmdRunner) WithInstallError(name string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installErrors[name] = err
	return f
}

// WithUninstallError configures Uninstall(name) to return err.
func (f *FakeCmdRunner) WithUninstallError(name string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallErrors[name] = err
	return f
}

// WithTapError configures Tap(repo) to return err.
func (f *FakeCmdRunner) WithTapError(repo string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tapErrors[repo] = err
	return f
}

// WithFormulaeListError makes ListFormulae return err (simulates
// `brew info --json=v2 --installed --formula` failure, which is a
// hard error at the call site).
func (f *FakeCmdRunner) WithFormulaeListError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.formulaeErr = err
	return f
}

// WithCasksListError makes ListCasks return err (production logs and
// continues — the provider's listInstalled swallows cask-list errors).
func (f *FakeCmdRunner) WithCasksListError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.casksErr = err
	return f
}

// WithTapsListError makes ListTaps return err.
func (f *FakeCmdRunner) WithTapsListError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tapsErr = err
	return f
}

// ListFormulae implements CmdRunner.
func (f *FakeCmdRunner) ListFormulae(_ context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpListFormulae})
	if f.formulaeErr != nil {
		return nil, f.formulaeErr
	}
	out := make(map[string]string, len(f.formulae))
	maps.Copy(out, f.formulae)
	return out, nil
}

// ListCasks implements CmdRunner.
func (f *FakeCmdRunner) ListCasks(_ context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpListCasks})
	if f.casksErr != nil {
		return nil, f.casksErr
	}
	out := make(map[string]string, len(f.casks))
	maps.Copy(out, f.casks)
	return out, nil
}

// ListTaps implements CmdRunner.
func (f *FakeCmdRunner) ListTaps(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpListTaps})
	if f.tapsErr != nil {
		return nil, f.tapsErr
	}
	out := make([]string, 0, len(f.taps))
	for k := range f.taps {
		out = append(out, k)
	}
	return out, nil
}

// Install implements CmdRunner.
func (f *FakeCmdRunner) Install(_ context.Context, name string, isCask bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpInstall, name: name, isCask: isCask})
	if err, ok := f.installErrors[name]; ok {
		return err
	}
	if isCask {
		f.casks[name] = "fake-1.0.0"
	} else {
		f.formulae[name] = "fake-1.0.0"
	}
	return nil
}

// Uninstall implements CmdRunner. Removes from whichever set the name
// appears in (formulae or casks).
func (f *FakeCmdRunner) Uninstall(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpUninstall, name: name})
	if err, ok := f.uninstallErrors[name]; ok {
		return err
	}
	delete(f.formulae, name)
	delete(f.casks, name)
	return nil
}

// Tap implements CmdRunner.
func (f *FakeCmdRunner) Tap(_ context.Context, repo string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpTap, name: repo})
	if err, ok := f.tapErrors[repo]; ok {
		return err
	}
	f.taps[repo] = true
	return nil
}

// CallCount returns how many times op was invoked. name filters by
// the affected pkg/repo name (pass "" for any call to op).
func (f *FakeCmdRunner) CallCount(op, name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if name == "" || c.name == name {
			n++
		}
	}
	return n
}

// LastInstallIsCask reports whether the most recent Install call
// passed isCask=true. Returns (false, false) if no install occurred.
func (f *FakeCmdRunner) LastInstallIsCask() (isCask, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].op == fakeOpInstall {
			return f.calls[i].isCask, true
		}
	}
	return false, false
}

// IsFormulaeInstalled reports whether name is in the formulae set.
func (f *FakeCmdRunner) IsFormulaeInstalled(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.formulae[name]
	return ok
}

// IsCaskInstalled reports whether name is in the casks set.
func (f *FakeCmdRunner) IsCaskInstalled(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.casks[name]
	return ok
}

// IsTapRegistered reports whether repo is in the taps set.
func (f *FakeCmdRunner) IsTapRegistered(repo string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.taps[repo]
}

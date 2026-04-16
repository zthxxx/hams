package ansible

import (
	"context"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. Tracks
// every playbook invoked + supports configured per-playbook errors.
type FakeCmdRunner struct {
	mu              sync.Mutex
	calls           []fakeCall
	playbookErrors  map[string]error
	lookPathError   error
	playbooksRunSet map[string]bool
}

type fakeCall struct {
	op   string
	path string
}

const (
	fakeOpRunPlaybook = "run_playbook"
	fakeOpLookPath    = "lookpath"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		playbookErrors:  make(map[string]error),
		playbooksRunSet: make(map[string]bool),
	}
}

// WithPlaybookError configures RunPlaybook(path) to return err.
func (f *FakeCmdRunner) WithPlaybookError(path string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playbookErrors[path] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// RunPlaybook implements CmdRunner.
func (f *FakeCmdRunner) RunPlaybook(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpRunPlaybook, path: path})
	if err, ok := f.playbookErrors[path]; ok {
		return err
	}
	f.playbooksRunSet[path] = true
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. path filters by
// playbook path (pass "" for any call to op).
func (f *FakeCmdRunner) CallCount(op, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if path == "" || c.path == path {
			n++
		}
	}
	return n
}

// WasPlaybookRun reports whether the fake observed a successful
// RunPlaybook call for path (i.e., no error was configured).
func (f *FakeCmdRunner) WasPlaybookRun(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playbooksRunSet[path]
}

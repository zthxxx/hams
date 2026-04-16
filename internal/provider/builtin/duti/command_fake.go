package duti

import (
	"context"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. The
// associations map mirrors the real macOS state: ext → bundleID.
// QueryDefault returns a synthesized "BundleID\n" line so the
// production parseDutiOutput path is exercised.
type FakeCmdRunner struct {
	mu            sync.Mutex
	associations  map[string]string // ext → bundleID
	calls         []fakeCall
	queryErrors   map[string]error
	setErrors     map[string]error
	lookPathError error
}

type fakeCall struct {
	op  string
	ext string
}

const (
	fakeOpQuery    = "query"
	fakeOpSet      = "set"
	fakeOpLookPath = "lookpath"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		associations: make(map[string]string),
		queryErrors:  make(map[string]error),
		setErrors:    make(map[string]error),
	}
}

// Seed pre-binds ext to bundleID before the test starts.
func (f *FakeCmdRunner) Seed(ext, bundleID string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.associations[ext] = bundleID
	return f
}

// WithQueryError configures QueryDefault(ext) to return err.
func (f *FakeCmdRunner) WithQueryError(ext string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queryErrors[ext] = err
	return f
}

// WithSetError configures SetDefault(ext, _) to return err.
func (f *FakeCmdRunner) WithSetError(ext string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setErrors[ext] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// QueryDefault implements CmdRunner. Returns the bundleID followed by
// "\n" so production parseDutiOutput's first-non-blank-line scan
// recovers it.
func (f *FakeCmdRunner) QueryDefault(_ context.Context, ext string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpQuery, ext: ext})
	if err, ok := f.queryErrors[ext]; ok {
		return "", err
	}
	bid, ok := f.associations[ext]
	if !ok {
		return "", nil
	}
	return bid + "\n", nil
}

// SetDefault implements CmdRunner.
func (f *FakeCmdRunner) SetDefault(_ context.Context, ext, bundleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpSet, ext: ext})
	if err, ok := f.setErrors[ext]; ok {
		return err
	}
	f.associations[ext] = bundleID
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. ext filters by
// extension (pass "" to count any call to op).
func (f *FakeCmdRunner) CallCount(op, ext string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if ext == "" || c.ext == ext {
			n++
		}
	}
	return n
}

// AssociationOf returns the currently-bound bundleID for ext, or ""
// if unbound. Tests use this to verify Apply post-conditions.
func (f *FakeCmdRunner) AssociationOf(ext string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.associations[ext]
}

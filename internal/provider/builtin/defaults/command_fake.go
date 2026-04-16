package defaults

import (
	"context"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It models
// the macOS preferences DB as a map keyed by "<domain>:<key>" with the
// stored value (in its post-`defaults write` string form). Reads
// against missing keys return a configurable error so tests can
// simulate the real `defaults read <missing>` exit-1 behavior.
type FakeCmdRunner struct {
	mu            sync.Mutex
	prefs         map[string]string // "domain:key" → value
	calls         []fakeCall
	readErrors    map[string]error
	writeErrors   map[string]error
	deleteErrors  map[string]error
	lookPathError error
}

type fakeCall struct {
	op    string
	key   string // "domain:key"
	typeS string // for Write only
	value string
}

const (
	fakeOpRead     = "read"
	fakeOpWrite    = "write"
	fakeOpDelete   = "delete"
	fakeOpLookPath = "lookpath"
)

// NewFakeCmdRunner returns a fresh FakeCmdRunner.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{
		prefs:        make(map[string]string),
		readErrors:   make(map[string]error),
		writeErrors:  make(map[string]error),
		deleteErrors: make(map[string]error),
	}
}

// Seed pre-binds (domain, key) to value.
func (f *FakeCmdRunner) Seed(domain, key, value string) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prefs[domain+":"+key] = value
	return f
}

// WithReadError makes Read for (domain, key) return err.
func (f *FakeCmdRunner) WithReadError(domain, key string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readErrors[domain+":"+key] = err
	return f
}

// WithWriteError makes Write for (domain, key) return err.
func (f *FakeCmdRunner) WithWriteError(domain, key string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeErrors[domain+":"+key] = err
	return f
}

// WithDeleteError makes Delete for (domain, key) return err.
func (f *FakeCmdRunner) WithDeleteError(domain, key string, err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteErrors[domain+":"+key] = err
	return f
}

// WithLookPathError configures LookPath to return err.
func (f *FakeCmdRunner) WithLookPathError(err error) *FakeCmdRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookPathError = err
	return f
}

// Read implements CmdRunner.
func (f *FakeCmdRunner) Read(_ context.Context, domain, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := domain + ":" + key
	f.calls = append(f.calls, fakeCall{op: fakeOpRead, key: k})
	if err, ok := f.readErrors[k]; ok {
		return "", err
	}
	v, ok := f.prefs[k]
	if !ok {
		return "", errMissingKey
	}
	return v, nil
}

// Write implements CmdRunner.
func (f *FakeCmdRunner) Write(_ context.Context, domain, key, typeStr, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := domain + ":" + key
	f.calls = append(f.calls, fakeCall{op: fakeOpWrite, key: k, typeS: typeStr, value: value})
	if err, ok := f.writeErrors[k]; ok {
		return err
	}
	f.prefs[k] = value
	return nil
}

// Delete implements CmdRunner.
func (f *FakeCmdRunner) Delete(_ context.Context, domain, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := domain + ":" + key
	f.calls = append(f.calls, fakeCall{op: fakeOpDelete, key: k})
	if err, ok := f.deleteErrors[k]; ok {
		return err
	}
	delete(f.prefs, k)
	return nil
}

// LookPath implements CmdRunner.
func (f *FakeCmdRunner) LookPath() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{op: fakeOpLookPath})
	return f.lookPathError
}

// CallCount returns how many times op was invoked. key (in
// "domain:key" form) filters by the affected pref entry; pass "" for
// any.
func (f *FakeCmdRunner) CallCount(op, key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.op != op {
			continue
		}
		if key == "" || c.key == key {
			n++
		}
	}
	return n
}

// ValueOf returns the current value for (domain, key), or "" if absent.
// Tests use this to verify post-Apply state.
func (f *FakeCmdRunner) ValueOf(domain, key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prefs[domain+":"+key]
}

// errMissingKey is returned by Read when the key isn't seeded; mirrors
// `defaults read <missing>` exiting non-zero in production.
var errMissingKey = readMissingError{}

type readMissingError struct{}

func (readMissingError) Error() string { return "fake: defaults: key not present" }

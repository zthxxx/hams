package git

import (
	"context"
	"errors"
	"sync"
)

// FakeCmdRunner is an in-memory CmdRunner for unit tests. It records
// every call and tracks an in-memory KV store so tests can verify the
// provider invokes the runner with the right arguments AND in the
// right order — without ever exec-ing the host's real git binary.
type FakeCmdRunner struct {
	mu sync.Mutex

	// Store holds the simulated `git config --global` values keyed by
	// their config key (e.g., "user.name" -> "zthxxx"). Tests may
	// pre-populate it to simulate a host that already has values.
	Store map[string]string

	// SetCalls records every SetGlobal invocation in order.
	SetCalls []FakeGitCall
	// UnsetCalls records every UnsetGlobal invocation in order.
	UnsetCalls []FakeGitCall
	// GetCalls records every GetGlobal invocation in order.
	GetCalls []string

	// ForceSetError, when non-nil, is returned from every SetGlobal
	// call without mutating the Store. Used to assert that write
	// failures short-circuit the auto-record path.
	ForceSetError error
	// ForceUnsetError mirrors ForceSetError for UnsetGlobal.
	ForceUnsetError error
}

// FakeGitCall captures the arguments to a SetGlobal/UnsetGlobal call.
type FakeGitCall struct {
	Key   string
	Value string
}

// NewFakeCmdRunner returns a runner with an empty store.
func NewFakeCmdRunner() *FakeCmdRunner {
	return &FakeCmdRunner{Store: map[string]string{}}
}

// SetGlobal records the call and, on success, writes to the in-memory
// store. When ForceSetError is set, no write occurs.
func (r *FakeCmdRunner) SetGlobal(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.SetCalls = append(r.SetCalls, FakeGitCall{Key: key, Value: value})
	if r.ForceSetError != nil {
		return r.ForceSetError
	}
	r.Store[key] = value
	return nil
}

// UnsetGlobal records the call and, on success, removes the key from
// the in-memory store.
func (r *FakeCmdRunner) UnsetGlobal(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.UnsetCalls = append(r.UnsetCalls, FakeGitCall{Key: key})
	if r.ForceUnsetError != nil {
		return r.ForceUnsetError
	}
	delete(r.Store, key)
	return nil
}

// GetGlobal records the call and returns the stored value, or an
// error when the key is absent (mirroring real git's non-zero exit).
func (r *FakeCmdRunner) GetGlobal(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.GetCalls = append(r.GetCalls, key)
	v, ok := r.Store[key]
	if !ok {
		return "", errors.New("git config --get: key not found")
	}
	return v, nil
}

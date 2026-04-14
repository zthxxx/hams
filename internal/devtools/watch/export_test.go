package main

// Stats exposes internal counters to tests in this package.
//
// It lives in a _test.go file so the accessor is not part of the
// package's public surface while still being reachable from tests.
func (e *Engine) Stats() (active, pending bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active, e.pending
}

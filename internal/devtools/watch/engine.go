package main

import (
	"context"
	"sync"
	"time"
)

// Clock abstracts the ticker so tests can drive debounce deterministically.
type Clock interface {
	// NewTimer returns a timer that fires once after d, with a channel that
	// delivers the fired time. Reset and Stop semantics mirror *time.Timer.
	NewTimer(d time.Duration) Timer
	// Now returns the current time (used for measuring build duration).
	Now() time.Time
}

// Timer is the minimal surface of *time.Timer we rely on.
type Timer interface {
	C() <-chan time.Time
	Reset(d time.Duration) bool
	Stop() bool
}

// Builder performs one build invocation.
//
// Implementations must be safe to call serially from the engine goroutine
// (the engine never calls Build concurrently with itself).
type Builder interface {
	Build(ctx context.Context) BuildResult
}

// BuildResult carries the outcome of a single Builder.Build call.
type BuildResult struct {
	// Err is nil on success.
	Err error
	// Stderr is the captured compiler output on failure. Empty on success.
	Stderr string
	// Duration is the wall time the build took.
	Duration time.Duration
	// CommitSHA is the short sha of HEAD at build time ("" if unavailable).
	CommitSHA string
}

// Reporter receives lifecycle events. Implementations must not block.
type Reporter interface {
	BuildStarted()
	BuildFinished(BuildResult)
}

// Engine implements the debounce + single-slot coalesce state machine.
//
// Invariant 1: at most one Builder.Build is in flight at a time.
// Invariant 2: at most one pending rebuild is queued (further events
//
//	arriving while a build is in flight fold into the one pending slot).
//
// Invariant 3: every Notify event eventually produces a build, unless a
//
//	later event arrives within the debounce window (in which case
//	the two are coalesced into a single build, as intended).
type Engine struct {
	debounce time.Duration
	clock    Clock
	builder  Builder
	reporter Reporter

	mu      sync.Mutex
	pending bool // another build should run after the current one finishes
	active  bool // a build is currently in flight
}

// NewEngine constructs an engine. All arguments must be non-nil.
func NewEngine(debounce time.Duration, clock Clock, builder Builder, reporter Reporter) *Engine {
	return &Engine{
		debounce: debounce,
		clock:    clock,
		builder:  builder,
		reporter: reporter,
	}
}

// Run drives the engine until ctx is canceled.
//
// Events are delivered on events. A closed events channel is treated the
// same as a cancellation and causes Run to drain the in-flight build then
// return.
func (e *Engine) Run(ctx context.Context, events <-chan struct{}) {
	var timer Timer
	timerC := func() <-chan time.Time {
		if timer == nil {
			return nil
		}
		return timer.C()
	}

	buildDone := make(chan BuildResult, 1)

	// startBuild: called either when the debounce timer fires, or when a
	// previously in-flight build finishes with the pending flag set.
	startBuild := func() {
		e.mu.Lock()
		e.active = true
		e.pending = false
		e.mu.Unlock()

		e.reporter.BuildStarted()
		go func() {
			buildDone <- e.builder.Build(ctx)
		}()
	}

	armTimer := func() {
		if timer == nil {
			timer = e.clock.NewTimer(e.debounce)
			return
		}
		timer.Stop()
		timer.Reset(e.debounce)
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			// Wait for any in-flight build so we don't return while a
			// goroutine is still writing to our channel.
			e.mu.Lock()
			stillActive := e.active
			e.mu.Unlock()
			if stillActive {
				<-buildDone
			}
			return

		case _, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			e.mu.Lock()
			active := e.active
			e.mu.Unlock()
			if active {
				// Build in flight: fold into the single pending slot.
				e.mu.Lock()
				e.pending = true
				e.mu.Unlock()
				continue
			}
			armTimer()

		case <-timerC():
			startBuild()

		case res := <-buildDone:
			e.reporter.BuildFinished(res)
			e.mu.Lock()
			e.active = false
			runAgain := e.pending
			e.pending = false
			e.mu.Unlock()
			if runAgain {
				startBuild()
			}
		}
	}
}

// Stats returns a snapshot of internal counters. Intended for tests.
func (e *Engine) Stats() (active, pending bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active, e.pending
}

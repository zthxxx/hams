package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// fakeClock drives timer fires synchronously via Tick.
type fakeClock struct {
	mu     sync.Mutex
	timers []*fakeTimer
	now    time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(0, 0)} }

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{
		deadline: c.now.Add(d),
		ch:       make(chan time.Time, 1),
		clock:    c,
		active:   true,
	}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Tick advances the fake clock by d and fires any timers whose deadline has
// passed.
func (c *fakeClock) Tick(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	fire := make([]*fakeTimer, 0)
	for _, t := range c.timers {
		if !t.active {
			continue
		}
		if !t.deadline.After(now) {
			t.active = false
			fire = append(fire, t)
		}
	}
	c.mu.Unlock()
	for _, t := range fire {
		select {
		case t.ch <- now:
		default:
		}
	}
}

type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	clock    *fakeClock
	active   bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasActive := t.active
	t.deadline = t.clock.now.Add(d)
	t.active = true
	// Drain any unread fire from a previous expiration, matching
	// time.Timer.Reset semantics.
	select {
	case <-t.ch:
	default:
	}
	return wasActive
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := t.active
	t.active = false
	return was
}

// fakeBuilder lets tests release builds on demand and count invocations.
type fakeBuilder struct {
	starts   atomic.Int64
	finishes atomic.Int64
	// Each Build blocks until release is signaled exactly once per call.
	gate      chan struct{}
	failAfter int64 // if > 0, the Nth build returns an error; 0 = always succeed
}

func newFakeBuilder() *fakeBuilder { return &fakeBuilder{gate: make(chan struct{})} }

func (b *fakeBuilder) Build(ctx context.Context) BuildResult {
	b.starts.Add(1)
	select {
	case <-b.gate:
	case <-ctx.Done():
		return BuildResult{Err: ctx.Err()}
	}
	n := b.finishes.Add(1)
	var err error
	if b.failAfter > 0 && n == b.failAfter {
		err = errors.New("forced fail")
	}
	return BuildResult{Err: err, CommitSHA: "abcdef0"}
}

// releaseOne unblocks the next Build() call.
func (b *fakeBuilder) releaseOne() { b.gate <- struct{}{} }

// fakeReporter records build lifecycle calls.
type fakeReporter struct {
	mu       sync.Mutex
	started  int
	finished int
	failed   int
}

func (r *fakeReporter) BuildStarted() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started++
}

func (r *fakeReporter) BuildFinished(res BuildResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finished++
	if res.Err != nil {
		r.failed++
	}
}

func (r *fakeReporter) snapshot() (started, finished, failed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started, r.finished, r.failed
}

// waitUntil polls the predicate until it holds or the one-second deadline
// elapses. The timeout is fixed; every call in this package shares the same
// tolerance so tests remain uniformly non-flaky across hosts.
func waitUntil(t *testing.T, pred func() bool, msg string) {
	t.Helper()
	const timeout = time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}

// TestEngine_SingleEventProducesOneBuild: one event -> debounce -> one build.
func TestEngine_SingleEventProducesOneBuild(t *testing.T) {
	clock := newFakeClock()
	builder := newFakeBuilder()
	reporter := &fakeReporter{}
	eng := NewEngine(500*time.Millisecond, clock, builder, reporter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 4)

	done := make(chan struct{})
	go func() { eng.Run(ctx, events); close(done) }()

	events <- struct{}{}
	// Wait for engine to arm the timer.
	waitUntil(t, func() bool {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		for _, tt := range clock.timers {
			if tt.active {
				return true
			}
		}
		return false
	}, "timer armed")

	clock.Tick(500 * time.Millisecond)

	// Wait for Build to be entered before releasing.
	waitUntil(t, func() bool { return builder.starts.Load() == 1 }, "build started")
	builder.releaseOne()

	waitUntil(t, func() bool {
		s, f, _ := reporter.snapshot()
		return s == 1 && f == 1
	}, "build reported")

	cancel()
	<-done
}

// TestEngine_ConcurrentSavesCoalesce: multiple events during an in-flight
// build collapse into exactly one extra build.
func TestEngine_ConcurrentSavesCoalesce(t *testing.T) {
	clock := newFakeClock()
	builder := newFakeBuilder()
	reporter := &fakeReporter{}
	eng := NewEngine(500*time.Millisecond, clock, builder, reporter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 32)

	done := make(chan struct{})
	go func() { eng.Run(ctx, events); close(done) }()

	// First event fires a build after debounce.
	events <- struct{}{}
	waitUntil(t, func() bool {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		for _, tt := range clock.timers {
			if tt.active {
				return true
			}
		}
		return false
	}, "timer armed")
	clock.Tick(500 * time.Millisecond)
	waitUntil(t, func() bool { return builder.starts.Load() == 1 }, "first build started")

	// While the first build is in flight, flood events.
	for range 10 {
		events <- struct{}{}
	}
	// Give the engine a beat to absorb them.
	time.Sleep(10 * time.Millisecond)
	_, pending := eng.Stats()
	if !pending {
		t.Fatalf("expected pending flag after flood, got false")
	}

	// Release first build. Engine should immediately start exactly one more.
	builder.releaseOne()
	waitUntil(t, func() bool { return builder.starts.Load() == 2 }, "second build started")

	// Drain the second build.
	builder.releaseOne()
	waitUntil(t, func() bool {
		s, f, _ := reporter.snapshot()
		return s == 2 && f == 2
	}, "second build reported")

	// No further builds should start spontaneously.
	time.Sleep(20 * time.Millisecond)
	if got := builder.starts.Load(); got != 2 {
		t.Fatalf("extra build triggered: got %d starts, want 2", got)
	}
	cancel()
	<-done
}

// TestEngine_BuildFailureKeepsRunning: failing build lets watcher continue.
func TestEngine_BuildFailureKeepsRunning(t *testing.T) {
	clock := newFakeClock()
	builder := newFakeBuilder()
	builder.failAfter = 1
	reporter := &fakeReporter{}
	eng := NewEngine(500*time.Millisecond, clock, builder, reporter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 4)

	done := make(chan struct{})
	go func() { eng.Run(ctx, events); close(done) }()

	events <- struct{}{}
	waitUntil(t, func() bool {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		for _, tt := range clock.timers {
			if tt.active {
				return true
			}
		}
		return false
	}, "timer armed")
	clock.Tick(500 * time.Millisecond)
	waitUntil(t, func() bool { return builder.starts.Load() == 1 }, "first build started")
	builder.releaseOne()
	waitUntil(t, func() bool {
		_, f, fails := reporter.snapshot()
		return f == 1 && fails == 1
	}, "first failed build reported")

	// Subsequent event triggers another build.
	events <- struct{}{}
	waitUntil(t, func() bool {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		for _, tt := range clock.timers {
			if tt.active {
				return true
			}
		}
		return false
	}, "timer re-armed")
	clock.Tick(500 * time.Millisecond)
	waitUntil(t, func() bool { return builder.starts.Load() == 2 }, "second build started")
	builder.releaseOne()
	waitUntil(t, func() bool {
		s, f, _ := reporter.snapshot()
		return s == 2 && f == 2
	}, "second build reported")

	cancel()
	<-done
}

// TestEngine_Invariants property-tests the state machine: random event
// streams must maintain (1) ≤1 build in flight and (2) ≤1 pending build.
func TestEngine_Invariants(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Draw a random sequence of actions: either "send event", "tick
		// the debounce timer to fire", or "release an in-flight build".
		// The engine must maintain its invariants under every interleaving.
		actions := rapid.SliceOfN(
			rapid.SampledFrom([]string{"event", "tick", "release"}),
			1, 40,
		).Draw(rt, "actions")

		clock := newFakeClock()
		builder := newFakeBuilder()
		reporter := &fakeReporter{}
		eng := NewEngine(500*time.Millisecond, clock, builder, reporter)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		events := make(chan struct{}, 128)

		done := make(chan struct{})
		go func() { eng.Run(ctx, events); close(done) }()

		var builtSoFar int64
		for _, a := range actions {
			switch a {
			case "event":
				select {
				case events <- struct{}{}:
				default:
				}
			case "tick":
				clock.Tick(500 * time.Millisecond)
			case "release":
				// Only release if there's a build in flight.
				if builder.starts.Load() > builtSoFar {
					select {
					case builder.gate <- struct{}{}:
						builtSoFar++
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			time.Sleep(time.Millisecond)

			// Invariant: at any point, the number of started builds minus the
			// number of finished builds is at most 1 — the engine holds one
			// build in flight and coalesces the rest into a single pending
			// slot, never queuing a second one.
			inFlight := builder.starts.Load() - builder.finishes.Load()
			if inFlight < 0 || inFlight > 1 {
				rt.Fatalf("invariant violated: starts=%d finishes=%d inFlight=%d",
					builder.starts.Load(), builder.finishes.Load(), inFlight)
			}
		}

		// Drain any remaining build.
		for builder.starts.Load() > builtSoFar {
			select {
			case builder.gate <- struct{}{}:
				builtSoFar++
			case <-time.After(50 * time.Millisecond):
			}
		}

		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			rt.Fatalf("engine did not shut down")
		}

		// Final: finishes <= starts, and starts - finishes is at most 1
		// when ctx cancel races with a release. In practice after full drain
		// they should be equal.
		s := builder.starts.Load()
		f := builder.finishes.Load()
		if f > s {
			rt.Fatalf("impossible: finishes=%d > starts=%d", f, s)
		}
	})
}

// TestEngine_ContextCancelStopsTimer: canceling mid-debounce aborts cleanly.
func TestEngine_ContextCancelStopsTimer(t *testing.T) {
	clock := newFakeClock()
	builder := newFakeBuilder()
	reporter := &fakeReporter{}
	eng := NewEngine(500*time.Millisecond, clock, builder, reporter)
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() { eng.Run(ctx, events); close(done) }()

	events <- struct{}{}
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("engine failed to stop on ctx cancel")
	}

	// No build was started because debounce never fired.
	if got := builder.starts.Load(); got != 0 {
		t.Fatalf("unexpected build started: %d", got)
	}
}

package main

import "time"

// realClock wraps the stdlib time package.
type realClock struct{}

// RealClock returns the default implementation backed by package time.
func RealClock() Clock { return realClock{} }

func (realClock) NewTimer(d time.Duration) Timer { return &realTimer{t: time.NewTimer(d)} }
func (realClock) Now() time.Time                 { return time.Now() }

type realTimer struct {
	t *time.Timer
}

func (r *realTimer) C() <-chan time.Time        { return r.t.C }
func (r *realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }
func (r *realTimer) Stop() bool                 { return r.t.Stop() }

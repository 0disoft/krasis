package wininput

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// Constructed only after all native owner threads returned clean cancellation.
// The service can distinguish a liveness failure from unconfirmed cleanup.
type retiredHookProgress struct{ cause error }

func (e *retiredHookProgress) Error() string { return "source hook progress owner retired" }
func (e *retiredHookProgress) Unwrap() error { return e.cause }

func SourceHookProgressRetiredOnly(err error) bool {
	if retired, ok := err.(*retiredHookProgress); ok {
		return SourceHookProgressFailureOnly(retired.cause)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		found := false
		for _, child := range joined.Unwrap() {
			if SourceHookProgressRetiredOnly(child) {
				found = true
			} else if child != context.Canceled {
				return false
			}
		}
		return found
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return SourceHookProgressRetiredOnly(wrapped.Unwrap())
	}
	return false
}

func SourceHookProgressFailureOnly(err error) bool {
	if err == errHookCallbackStalled || err == errHookMouseProgress || err == context.Canceled {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		if len(joined.Unwrap()) == 0 {
			return false
		}
		for _, child := range joined.Unwrap() {
			if !SourceHookProgressFailureOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return SourceHookProgressFailureOnly(wrapped.Unwrap())
	}
	return false
}

var errHookCallbackStalled = errors.New("source hook callback progress unconfirmed")
var errHookMouseProgress = errors.New("physical raw mouse progress without hook progress")

// Native callbacks update atomics only. The source owner evaluates these on
// its own ticker; no timer, I/O or additional goroutine runs on the hook thread.
// Durations use the same monotonic origin, never the system wall clock.
type sourceHookProgress struct {
	origin       time.Time
	next         HookTimingObserver
	active       atomic.Int64
	mouse        atomic.Uint64
	raw          atomic.Int64
	missingSince int64 // source owner only
	lastMouse    uint64
}

func (p *sourceHookProgress) stamp(at time.Time) int64 { return int64(at.Sub(p.origin)) + 1 }
func (p *sourceHookProgress) ObserveHookStarted(at time.Time) {
	p.active.Store(p.stamp(at))
	if p.next != nil {
		p.next.ObserveHookStarted(at)
	}
}
func (p *sourceHookProgress) ObserveHookCompleted(at time.Time) {
	defer p.active.Store(0)
	if p.next != nil {
		p.next.ObserveHookCompleted(at)
	}
}
func (p *sourceHookProgress) ObserveRawInput(at time.Time) {
	if p.next != nil {
		p.next.ObserveRawInput(at)
	}
}
func (p *sourceHookProgress) observeMouse()                   { p.mouse.Add(1) }
func (p *sourceHookProgress) observePhysicalRaw(at time.Time) { p.raw.Store(p.stamp(at)) }

func (p *sourceHookProgress) failure(at time.Time) error {
	now := p.stamp(at)
	if started := p.active.Load(); started != 0 && now-started >= int64(time.Second) && p.active.Load() == started {
		return sourceStage("hook-callback-stalled", errHookCallbackStalled)
	}
	raw, mouse := p.raw.Load(), p.mouse.Load()
	if raw == 0 || now-raw > int64(250*time.Millisecond) {
		p.missingSince, p.lastMouse = 0, mouse
		return nil // No recent physical input is not a hook failure.
	}
	if mouse != p.lastMouse || p.missingSince == 0 {
		p.lastMouse, p.missingSince = mouse, now
		return nil
	}
	if now-p.missingSince >= int64(2*time.Second) {
		return sourceStage("hook-mouse-progress", errHookMouseProgress)
	}
	return nil
}

// A watchdog cannot wait on a mutex held by the callback it is retiring.
// The atomic fence rejects grants/suppression immediately; the source worker
// still owns ordinary deactivation, sender joins and remote held-input cleanup.
func (session *SourceSession) retireHookFailure(cause error) {
	session.terminal.Store(true)
	select {
	case session.faults <- cause:
	default:
	}
}

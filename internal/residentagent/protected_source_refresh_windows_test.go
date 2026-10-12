//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestProtectedSourceRefreshJoinsOldOwnerAndReadiesOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, cleaning, release, done := make(chan int, 2), make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var calls, ready atomic.Int32
	session := &wininput.SourceSession{}
	h := &protectedSource{blocked: func() bool { return false }}
	h.native = protectedTestHook(func(attempt context.Context, received *wininput.SourceSession, _ wininput.BoundaryObserver, signal func()) error {
		if received != session {
			t.Error("refresh replaced sender")
		}
		call := int(calls.Add(1))
		signal()
		started <- call
		<-attempt.Done()
		if call == 1 {
			if !errors.Is(context.Cause(attempt), wininput.ErrSourceCaptureRefresh) {
				t.Error("refresh cause missing")
			}
			close(cleaning)
			<-release
			signal() // A late old readiness callback cannot reopen capture.
		}
		return attempt.Err()
	})
	go func() { done <- h.RunWithBoundary(ctx, session, nil, func() { ready.Add(1) }) }()
	if awaitTransitionSignal(t, started) != 1 || !h.captureReady() {
		t.Fatal("first owner missing")
	}
	if !h.refreshDesktop(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("idle refresh rejected")
	}
	awaitTransitionSignal(t, cleaning)
	if !h.refreshDesktop(agentlifecycle.EventSessionLock) || h.captureReady() || calls.Load() != 1 {
		t.Fatal("old owner not joined or duplicate transition was not coalesced")
	}
	if _, err := h.AdmitBoundary(ctx, control.BoundaryIntent{}); !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) {
		t.Fatal("refresh admitted boundary", err)
	}
	close(release)
	if awaitTransitionSignal(t, started) != 2 || !h.captureReady() || ready.Load() != 1 {
		t.Fatal("replacement did not restore capture or signalled session ready twice")
	}
	cancel()
	if err := awaitTransitionSignal(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestProtectedSourceRefreshPreservesCleanupFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan struct{}), make(chan error, 1)
	failure := errors.New("native owner release failed")
	var calls atomic.Int32
	h := &protectedSource{blocked: func() bool { return false }}
	h.native = protectedTestHook(func(attempt context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, signal func()) error {
		calls.Add(1)
		signal()
		close(started)
		<-attempt.Done()
		return errors.Join(attempt.Err(), failure)
	})
	go func() { done <- h.RunWithBoundary(ctx, &wininput.SourceSession{}, nil, func() {}) }()
	awaitTransitionSignal(t, started)
	if !h.refreshDesktop(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("idle refresh rejected")
	}
	if err := awaitTransitionSignal(t, done); !errors.Is(err, failure) || calls.Load() != 1 {
		t.Fatal("failed owner replacement was retried or hidden", err)
	}
}

func TestProtectedReceiveOnlyReacquiresCaptureWhenDesktopReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var blocked atomic.Bool
	blocked.Store(true)
	initial, captured, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var ready atomic.Int32
	h := &protectedSource{blocked: blocked.Load, native: protectedTestHook(func(attempt context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, signal func()) error {
		signal()
		close(captured)
		<-attempt.Done()
		return attempt.Err()
	})}
	go func() {
		done <- h.RunWithBoundary(ctx, &wininput.SourceSession{}, nil, func() { ready.Add(1); close(initial) })
	}()
	awaitTransitionSignal(t, initial)
	if h.captureReady() {
		t.Fatal("secure desktop pretended to capture")
	}
	blocked.Store(false)
	awaitTransitionSignal(t, captured)
	if !h.captureReady() || ready.Load() != 1 {
		t.Fatal("desktop return did not restore same session capture")
	}
	cancel()
	awaitTransitionSignal(t, done)
}

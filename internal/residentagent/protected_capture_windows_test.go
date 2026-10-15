//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

type secureCaptureTest struct {
	active  atomic.Bool
	stopped chan struct{}
	cursor  atomic.Bool
}

func (c *secureCaptureTest) PrepareReturnCursor() func() { c.cursor.Store(true); return func() {} }
func (c *secureCaptureTest) RestoreReturnCursor()        { c.cursor.Store(false) }

func (c *secureCaptureTest) SourceCapture() wininput.SourceActivation { return c }
func (c *secureCaptureTest) Active() bool                             { return c.active.Load() }
func (c *secureCaptureTest) Activate() error                          { c.active.Store(true); return nil }
func (c *secureCaptureTest) Deactivate()                              { c.active.Store(false) }
func (c *secureCaptureTest) AdmitBoundary(context.Context, control.BoundaryIntent) (peer.BoundaryAdmissionDecision, error) {
	return peer.BoundaryAdmissionAllowed, nil
}
func (c *secureCaptureTest) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
	ready()
	<-ctx.Done()
	close(c.stopped)
	return ctx.Err()
}

func TestLockedSourceUsesProtectedCaptureInSameSessionAndFencesActiveRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secure := &secureCaptureTest{stopped: make(chan struct{})}
	h := &protectedSource{blocked: func() bool { return true }, native: protectedTestHook(func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
		t.Error("medium capture attempted on secure desktop")
		return errors.New("wrong owner")
	})}
	h.SetProtectedSource(secure)
	ready, done := make(chan struct{}), make(chan error, 1)
	session := &wininput.SourceSession{}
	go func() { done <- h.RunWithBoundary(ctx, session, nil, func() { close(ready) }) }()
	<-ready
	if h.PrepareReturnCursor() == nil || !secure.cursor.Load() {
		t.Fatal("ready protected cursor started a second passive worker")
	}
	decision, err := h.AdmitBoundary(ctx, control.BoundaryIntent{})
	if err != nil || decision != peer.BoundaryAdmissionAllowed {
		t.Fatal("secure native admission unavailable", err)
	}
	if err := h.SourceCapture().Activate(); err != nil || !secure.Active() {
		t.Fatal("protected activation failed", err)
	}
	if accepted, reason := h.refreshDesktopObserved("desktop_switch"); accepted || reason != sourceRefreshActiveOwner {
		t.Fatal("active protected owner replaced")
	}
	h.SourceCapture().Deactivate()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-secure.stopped:
	default:
		t.Fatal("owner was not joined")
	}
}

func TestLockNotificationSelectsReusableCaptureBeforeForegroundUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secure := &secureCaptureTest{stopped: make(chan struct{})}
	h := &protectedSource{blocked: func() bool { return false }, cursorProtected: func() bool { return false }, sessionLocked: func() bool { return true }, native: protectedTestHook(func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
		t.Error("stale foreground selected medium source")
		return errors.New("wrong owner")
	})}
	h.SetProtectedSource(secure)
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- h.RunWithBoundary(ctx, &wininput.SourceSession{}, nil, func() { close(ready) }) }()
	awaitTransitionSignal(t, ready)
	if h.PrepareReturnCursor() == nil || !secure.cursor.Load() {
		t.Fatal("lock return launched a second cursor owner")
	}
	cancel()
	if err := awaitTransitionSignal(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCleanIdleProtectedDesktopRetirementDoesNotRestartConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls, ready atomic.Int32
	session := &wininput.SourceSession{}
	h := &protectedSource{blocked: func() bool { return true }}
	h.SetProtectedSource(&retiringSecureTest{run: protectedTestHook(func(attempt context.Context, received *wininput.SourceSession, _ wininput.BoundaryObserver, signal func()) error {
		if received != session {
			t.Error("desktop retirement replaced source session")
		}
		signal()
		if calls.Add(1) == 1 {
			return errors.Join(wininput.ErrProtectedInput, inputworker.ErrSourceCaptureRetired)
		}
		cancel()
		return attempt.Err()
	})})
	err := h.RunWithBoundary(ctx, session, nil, func() { ready.Add(1) })
	if !errors.Is(err, context.Canceled) || calls.Load() != 2 || ready.Load() != 1 {
		t.Fatal("idle desktop switch restarted peer runner", err, calls.Load(), ready.Load())
	}
}

type retiringSecureTest struct {
	secureCaptureTest
	run protectedTestHook
}

func (s *retiringSecureTest) RunWithBoundary(ctx context.Context, session *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) error {
	return s.run(ctx, session, boundary, ready)
}

func TestProtectedRefreshDuringStartupWaitsForOwnershipBeforeRetirement(t *testing.T) {
	for _, cleanupFailed := range []bool{false, true} {
		t.Run(map[bool]string{false: "joined", true: "cleanup failed"}[cleanupFailed], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			startup, release := make(chan context.Context, 1), make(chan struct{})
			replacement, done := make(chan struct{}), make(chan error, 1)
			var calls, ready atomic.Int32
			failure := errors.New("native cleanup unconfirmed")
			h := &protectedSource{blocked: func() bool { return true }}
			h.SetProtectedSource(&retiringSecureTest{run: protectedTestHook(func(attempt context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, signal func()) error {
				if calls.Add(1) == 1 {
					startup <- attempt
					<-release
					// The real source client has verified sourceStart before this
					// callback. Only then can cancellation obtain sourceStop ACK.
					signal()
					<-attempt.Done()
					if cleanupFailed {
						return errors.Join(attempt.Err(), failure)
					}
					return attempt.Err()
				}
				signal()
				close(replacement)
				<-attempt.Done()
				return attempt.Err()
			})})
			go func() { done <- h.RunWithBoundary(ctx, &wininput.SourceSession{}, nil, func() { ready.Add(1) }) }()
			attempt := awaitTransitionSignal(t, startup)
			for i := 0; i < 2; i++ {
				if !h.refreshDesktop("desktop_switch") {
					t.Fatal("startup transition rejected")
				}
			}
			if attempt.Err() != nil || h.captureReady() || ready.Load() != 0 {
				t.Fatal("startup IPC interrupted or stale readiness published")
			}
			close(release)
			if cleanupFailed {
				if err := awaitTransitionSignal(t, done); !errors.Is(err, failure) || calls.Load() != 1 || ready.Load() != 0 {
					t.Fatal("cleanup failure hidden by startup cancellation", err)
				}
				return
			}
			awaitTransitionSignal(t, replacement)
			if calls.Load() != 2 || ready.Load() != 1 || !h.captureReady() {
				t.Fatal("coalesced transition did not replace exactly one owner")
			}
			cancel()
			if err := awaitTransitionSignal(t, done); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

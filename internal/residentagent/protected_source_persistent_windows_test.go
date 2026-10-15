//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

type persistentCaptureTest struct {
	retiringSecureTest
	held atomic.Bool
}

func (c *persistentCaptureTest) AdmitBoundary(context.Context, control.BoundaryIntent) (peer.BoundaryAdmissionDecision, error) {
	if c.held.Load() {
		return peer.BoundaryAdmissionBlockedPressedInput, nil
	}
	return peer.BoundaryAdmissionAllowed, nil
}

func TestInstalledDefaultCaptureSurvivesIdleLockWithWorkerLocalPress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan int, 2), make(chan error, 1)
	var calls, ready atomic.Int32
	session := &wininput.SourceSession{}
	secure := &persistentCaptureTest{}
	secure.run = func(attempt context.Context, received *wininput.SourceSession, _ wininput.BoundaryObserver, signal func()) error {
		if received != session {
			t.Error("lock replaced sender rather than capture owner")
		}
		call := int(calls.Add(1))
		signal()
		started <- call
		<-attempt.Done()
		return attempt.Err()
	}
	h := &protectedSource{blocked: func() bool { return false }, cursorProtected: func() bool { return false }, native: protectedTestHook(func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
		return errors.New("medium Default owner selected despite verified capture backend")
	})}
	h.SetProtectedSource(secure)
	go func() { done <- h.RunWithBoundary(ctx, session, nil, func() { ready.Add(1) }) }()
	select {
	case <-started:
	case err := <-done:
		t.Fatal("Default did not use protected capture", err)
	}
	// The local lock key belongs only to the native worker. The medium sender
	// must not retain a second ledger whose release occurs on another desktop.
	secure.held.Store(true)
	if !h.refreshDesktop("session_lock") {
		t.Fatal("idle installed source fell back to full connection recovery")
	}
	if awaitTransitionSignal(t, started) != 2 || ready.Load() != 1 || session.PhysicalPressCount() != 0 {
		t.Fatal("lock changed sender readiness or medium physical ownership")
	}
	if decision, err := h.AdmitBoundary(ctx, control.BoundaryIntent{}); err != nil || decision != peer.BoundaryAdmissionBlockedPressedInput {
		t.Fatal("capture replacement erased worker pressed-state admission", decision, err)
	}
	secure.held.Store(false)
	if decision, err := h.AdmitBoundary(ctx, control.BoundaryIntent{}); err != nil || decision != peer.BoundaryAdmissionAllowed {
		t.Fatal("released worker state did not permit a fresh query", decision, err)
	}
	cancel()
	if err := awaitTransitionSignal(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

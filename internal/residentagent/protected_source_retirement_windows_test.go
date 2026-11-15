//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/wininput"
)

type retiredSourceSink struct{}

func (retiredSourceSink) SendAction(context.Context, control.InputAction, func() bool) error {
	return nil
}
func (retiredSourceSink) Interrupt(error) error             { return nil }
func (retiredSourceSink) Fail(context.Context, error) error { return nil }

func runningRetirementSession(t *testing.T) *wininput.SourceSession {
	t.Helper()
	session, err := wininput.NewSourceSession(4, retiredSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- session.RunWithReady(ctx, func() { close(ready) }) }()
	awaitTransitionSignal(t, ready)
	t.Cleanup(func() { cancel(); awaitTransitionSignal(t, done) })
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
	return session
}

func TestRetiredSourceWaitsForRevokeBeforeReplacement(t *testing.T) {
	session := runningRetirementSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- settleRetiredSource(ctx, session) }()
	generation := awaitTransitionSignal(t, session.LocalReturns())
	if session.Active() || !session.LocalReturnPending() || session.Activate() == nil {
		t.Fatal("retired capture forwarded or accepted another lease")
	}
	select {
	case err := <-done:
		t.Fatal("replaced before revoke finished", err)
	default:
	}
	session.CompleteLocalReturn(generation)
	if err := awaitTransitionSignal(t, done); err != nil {
		t.Fatal(err)
	}
	if session.Activate() == nil {
		t.Fatal("completed revoke reopened capture before owner readiness")
	}
	session.ResumeCapture()
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredSourceMissingRevokeAndCancellationStayFatal(t *testing.T) {
	session := runningRetirementSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := settleRetiredSource(ctx, session)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, wininput.ErrProtectedInput) || !session.LocalReturnPending() {
		t.Fatal("missing revoke was accepted", err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if !errors.Is(settleRetiredSource(cancelled, session), context.Canceled) {
		t.Fatal("cancellation was lost")
	}
}

func TestActiveProtectedRetirementKeepsSameSessionUntilRevokeCompletes(t *testing.T) {
	session := runningRetirementSession(t)
	session.Deactivate()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls atomic.Int32
	replaced, done := make(chan struct{}), make(chan error, 1)
	h := &protectedSource{blocked: func() bool { return true }}
	h.SetProtectedSource(&retiringSecureTest{run: protectedTestHook(func(attempt context.Context, received *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
		if received != session {
			t.Error("replacement changed the source session")
		}
		ready()
		if calls.Add(1) == 1 {
			if err := session.Activate(); err != nil {
				return err
			}
			return errors.Join(wininput.ErrProtectedInput, inputworker.ErrSourceCaptureRetired)
		}
		close(replaced)
		<-attempt.Done()
		return attempt.Err()
	})})
	go func() { done <- h.RunWithBoundary(ctx, session, nil, func() {}) }()
	generation := awaitTransitionSignal(t, session.LocalReturns())
	if calls.Load() != 1 || session.Activate() == nil {
		t.Fatal("worker replaced before the outbound revoke completed")
	}
	session.CompleteLocalReturn(generation)
	awaitTransitionSignal(t, replaced)
	if calls.Load() != 2 {
		t.Fatal("clean active retirement did not replace the owner")
	}
	cancel()
	if err := awaitTransitionSignal(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

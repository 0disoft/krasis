//go:build windows

package wininput

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

// This opt-in check briefly activates a real native capture surface and restores
// foreground. It must run on an attended interactive desktop, never hosted CI.
func TestCaptureForegroundNativeExchange(t *testing.T) {
	if os.Getenv("KRASIS_NATIVE_FOREGROUND_SMOKE") != "1" {
		t.Skip("requires an interactive desktop and explicit native smoke opt-in")
	}
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	hook := NewSourceHook()
	capture := hook.capture
	ctx, cancel := context.WithCancel(context.Background())
	ready, finished := make(chan struct{}), make(chan error, 1)
	go func() { finished <- hook.Run(ctx, session, func() { close(ready) }) }()
	t.Cleanup(func() {
		capture.Deactivate()
		cancel()
		select {
		case err := <-finished:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("native capture owner did not retire")
		}
	})
	select {
	case <-ready:
	case err := <-finished:
		finished <- err // Cleanup still observes owner retirement.
		t.Fatalf("native surface failed before ready: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("native capture surface did not become ready")
	}
	for i := 0; i < 5; i++ {
		if os.Getenv("KRASIS_NATIVE_FOREGROUND_COMPARE") == "1" {
			profile, probe := handoff.WithCaptureProbe(context.Background())
			command := sourceWheelCaptureCommand{message: wmSourceWheelCaptureActivate, result: make(chan error, 1), profile: profile}
			current := capture.run.Load()
			current.commands <- command
			started := time.Now()
			procPostMessageWTouchpad.Call(current.window, wmSourceWheelCaptureActivate, 0, 0)
			select {
			case err := <-command.result:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("baseline activation did not complete")
			}
			_, exact := probe.FinishDetailed()
			t.Logf("baseline activation=%s foreground request=%dus", time.Since(started), exact.ForegroundRequestMicros)
			capture.Deactivate()
		}
		profile, probe := handoff.WithCaptureProbe(context.Background())
		started := time.Now()
		if err := capture.ActivateProfile(profile); err != nil {
			t.Fatal(err)
		}
		_, exact := probe.FinishDetailed()
		if !capture.Active() || !capture.OwnsWheel() {
			t.Fatal("activation acknowledged without native foreground ownership")
		}
		t.Logf("native activation=%s foreground request (including exchange)=%dus", time.Since(started), exact.ForegroundRequestMicros)
		capture.Deactivate()
		if capture.Active() {
			t.Fatal("deactivation retained native capture")
		}
	}
}

func TestCaptureForegroundExchangeRequestsOnAnotherThreadAndVerifiesOnOwner(t *testing.T) {
	for _, verified := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified", false: "unverified"}[verified], func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			t.Cleanup(restore)
			var foreground atomic.Uintptr
			foreground.Store(20)
			var requestThread, ownerThread atomic.Uint32
			var nativeCalls, dispatches atomic.Int32
			sourceWheelCaptureGetForeground = func() uintptr { return foreground.Load() }
			sourceWheelCaptureSetForeground = func(window uintptr) error {
				requestThread.Store(sourceWheelCaptureCurrentThread())
				nativeCalls.Add(1)
				return nil // Like cross-queue SetForegroundWindow, success is pending.
			}
			sourceForegroundTransitionDispatch = func(uintptr) {
				dispatches.Add(1)
				if verified {
					foreground.Store(10)
				}
			}
			sourceWheelCaptureProbeForeground = func(uintptr) error { return errors.New("no fallback attachment") }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 0, errors.New("fallback unavailable") }
			current := &sourceWheelCaptureRun{window: 10}
			exchange := newSourceForegroundExchange()
			finished := make(chan error, 1)
			go func() {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				ownerThread.Store(sourceWheelCaptureCurrentThread())
				finished <- current.activateWindowWithForeground(nil, func(uintptr) error { return exchange.request(current) })
			}()
			defer close(exchange.cancelled)
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case request := <-exchange.ready:
				request.complete(current, current.requestForegroundOnMonitorThread(context.Background(), time.Now().Add(time.Second)))
			case <-timer.C:
				t.Fatal("owner did not request foreground")
			}
			select {
			case err := <-finished:
				if verified && err != nil || !verified && !errors.Is(err, ErrSourceCaptureActivationVerificationLost) {
					t.Fatalf("activation error: %v", err)
				}
			case <-timer.C:
				t.Fatal("owner did not complete foreground verification")
			}
			if current.active.Load() != verified || nativeCalls.Load() != 1 || dispatches.Load() < 1 || ownerThread.Load() == requestThread.Load() {
				t.Fatal("request was synchronous on owner, duplicated, or committed without verification")
			}
		})
	}
}

func TestCaptureForegroundExchangeCancellationAndNativeCause(t *testing.T) {
	for _, stage := range []string{"before-ready", "after-ready", "native-error"} {
		t.Run(stage, func(t *testing.T) {
			current := &sourceWheelCaptureRun{}
			exchange := newSourceForegroundExchange()
			finished := make(chan error, 1)
			cause := errors.New("native foreground rejected")
			if stage == "before-ready" {
				close(exchange.cancelled)
			}
			go func() { finished <- exchange.request(current) }()
			if stage != "before-ready" {
				request := <-exchange.ready
				if stage == "after-ready" {
					close(exchange.cancelled)
					request.release() // Model an accepted caller retiring without a native call.
				} else {
					defer close(exchange.cancelled)
					request.complete(current, cause)
				}
			}
			want := error(ErrSourceCaptureCommandTimeout)
			if stage == "native-error" {
				want = cause
			}
			select {
			case err := <-finished:
				if !errors.Is(err, want) || current.active.Load() {
					t.Fatal("lost refusal/cause or activated after cancellation", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled exchange retained its owner")
			}
		})
	}
}

func TestCaptureForegroundCommandRejectsExpiredOrTerminalRequest(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
	sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("late native activation"); return nil }
	for _, deadline := range []time.Time{time.Now().Add(-time.Nanosecond), time.Now()} {
		current := &sourceWheelCaptureRun{}
		err := current.requestForegroundOnMonitorThread(context.Background(), deadline)
		if !errors.Is(err, ErrSourceCaptureCommandTimeout) || !errors.Is(current.err(), ErrSourceCaptureCommandTimeout) {
			t.Fatal("expired request was not fenced", err)
		}
	}
	current := &sourceWheelCaptureRun{terminalErr: context.Canceled}
	if err := current.requestForegroundOnMonitorThread(context.Background(), time.Now().Add(time.Second)); !errors.Is(err, context.Canceled) {
		t.Fatal("terminal cause was lost", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	current = &sourceWheelCaptureRun{}
	if err := current.requestForegroundOnMonitorThread(ctx, time.Now().Add(time.Second)); !errors.Is(err, context.Canceled) || !errors.Is(current.err(), context.Canceled) {
		t.Fatal("cancelled monitor context reached native activation", err)
	}
}

func TestCaptureForegroundExchangeKeepsEventUntilLateCallerRetires(t *testing.T) {
	current := &sourceWheelCaptureRun{}
	exchange := newSourceForegroundExchange()
	finished := make(chan error, 1)
	go func() { finished <- exchange.request(current) }()
	request := <-exchange.ready
	close(exchange.cancelled)
	if err := <-finished; !errors.Is(err, ErrSourceCaptureCommandTimeout) {
		t.Fatal("owner did not retire on cancellation", err)
	}
	if request.refs.Load() != 1 {
		t.Fatal("cancelled owner retained an event reference")
	}
	if err := windows.SetEvent(request.event); err != nil {
		t.Fatal("owner closed a handle still borrowed by native caller", err)
	}
	request.complete(current, nil)
	if request.refs.Load() != 0 {
		t.Fatal("late caller leaked the event")
	}
	if err := windows.SetEvent(request.event); !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		t.Fatal("completed native request retained an event handle", err)
	}
}

func TestCaptureForegroundExchangePumpsActivationBeforeCallerReturns(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
	pumped := make(chan struct{}, 1)
	sourceForegroundTransitionDispatch = func(window uintptr) {
		if window != 10 {
			t.Error("dispatch escaped the owned capture window")
		}
		select {
		case pumped <- struct{}{}:
		default:
		}
	}
	exchange := newSourceForegroundExchange()
	defer close(exchange.cancelled)
	current := &sourceWheelCaptureRun{window: 10}
	finished := make(chan error, 1)
	go func() { finished <- exchange.request(current) }()
	request := <-exchange.ready
	// Model Windows requiring the target queue to service activation before
	// SetForegroundWindow on the command thread is allowed to return.
	select {
	case <-pumped:
	case <-time.After(time.Second):
		request.release()
		t.Fatal("window owner parked on Go reply instead of servicing activation")
	}
	request.complete(current, nil)
	if err := <-finished; err != nil || request.refs.Load() != 0 {
		t.Fatal("native completion failed or retained its event", err)
	}
}

func TestCaptureForegroundExchangeWaitFailureRemainsTerminal(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
	previousWait := sourceForegroundExchangeWait
	t.Cleanup(func() { sourceForegroundExchangeWait = previousWait })
	cause := errors.New("native activation wait failed")
	sourceForegroundExchangeWait = func(windows.Handle) error { return cause }
	exchange := newSourceForegroundExchange()
	defer close(exchange.cancelled)
	current := &sourceWheelCaptureRun{}
	finished := make(chan error, 1)
	go func() { finished <- exchange.request(current) }()
	request := <-exchange.ready
	if err := <-finished; !errors.Is(err, cause) || !errors.Is(current.err(), cause) || current.active.Load() {
		t.Fatal("native wait failure did not fence capture", err)
	}
	request.complete(current, cause)
	if request.refs.Load() != 0 {
		t.Fatal("failed wait leaked its event")
	}
}

func TestCaptureForegroundCancellationDoesNotWaitForBlockedNativeRequest(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
	entered, releaseNative := make(chan struct{}), make(chan struct{})
	sourceWheelCaptureSetForeground = func(uintptr) error {
		close(entered)
		<-releaseNative
		return nil
	}
	current := &sourceWheelCaptureRun{}
	exchange := newSourceForegroundExchange()
	ownerDone, monitorDone := make(chan error, 1), make(chan struct{})
	go func() { ownerDone <- exchange.request(current) }()
	request := <-exchange.ready
	go func() {
		defer close(monitorDone)
		request.complete(current, current.requestForegroundOnMonitorThread(context.Background(), time.Now().Add(time.Second)))
	}()
	<-entered
	// This is the command caller's timeout path. Its timer and the native
	// requester run independently; native completion is not cancellation proof.
	current.fail(ErrSourceCaptureCommandTimeout)
	close(exchange.cancelled)
	select {
	case err := <-ownerDone:
		if !errors.Is(err, ErrSourceCaptureCommandTimeout) || current.active.Load() {
			t.Error("blocked request retained capture", err)
		}
	case <-time.After(time.Second):
		t.Error("owner waited for an uninterruptible native request")
	}
	if request.refs.Load() != 1 {
		t.Error("late native request lost its event ownership")
	}
	close(releaseNative)
	<-monitorDone
	if request.refs.Load() != 0 {
		t.Fatal("late native return leaked its event")
	}
}

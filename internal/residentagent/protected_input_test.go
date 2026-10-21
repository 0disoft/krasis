package residentagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

func TestProtectedObservationKeepsFirstCauseWithoutCachingDecision(t *testing.T) {
	for _, test := range []struct {
		reason wininput.ProtectedInputReason
		code   string
	}{
		{wininput.ProtectedInputDesktopUnavailable, "protected_input_desktop_unavailable"},
		{wininput.ProtectedInputNonDefaultDesktop, "protected_input_non_default_desktop"},
		{wininput.ProtectedInputCredentialBroker, "protected_input_credential_broker"},
		{wininput.ProtectedInputIdentityUnknown, "protected_input_identity_unknown"},
	} {
		t.Run(test.code, func(t *testing.T) {
			reason := wininput.ProtectedInputClear
			checks := 0
			var events []string
			active := observedProtectedInput(func() wininput.ProtectedInputReason {
				checks++
				return reason
			}, func(code string) { events = append(events, code) })
			if active() || len(events) != 0 {
				t.Fatal("clear observation stopped input or logged a cause")
			}
			reason = test.reason
			if !active() {
				t.Fatal("positive observation no longer guards input")
			}
			reason = wininput.ProtectedInputIdentityUnknown
			if !active() {
				t.Fatal("unknown identity must remain protected")
			}
			reason = wininput.ProtectedInputClear
			if active() {
				t.Fatal("observation cached a stale decision")
			}
			if checks != 4 || len(events) != 1 || events[0] != test.code {
				t.Fatalf("checks=%d events=%v", checks, events)
			}
		})
	}
}

func TestConcurrentProtectedMonitorAndInputRecordOneCause(t *testing.T) {
	var events []string
	active := observedProtectedInput(func() wininput.ProtectedInputReason {
		return wininput.ProtectedInputCredentialBroker
	}, func(code string) { events = append(events, code) })
	var workers sync.WaitGroup
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if !active() {
				t.Error("protected observation lost")
			}
		}()
	}
	workers.Wait()
	if len(events) != 1 || events[0] != "protected_input_credential_broker" {
		t.Fatalf("events=%v", events)
	}
}

func TestSelectedInputBackendIsFencedAndClearedWithSession(t *testing.T) {
	o := testOperations(t, nil)
	done := make(chan struct{})
	o.sessionDone = done
	o.sessionState = "CONNECTED"
	outcome := &sessionOutcome{connected: true}
	o.sessionOutcome = outcome
	observer := &sessionObserver{operations: o, done: done}
	observer.ObserveInputBackend(true)
	s, _ := o.Status(context.Background())
	if s.InputBackend != "PROTECTED" {
		t.Fatal(s.InputBackend)
	}
	observer.ObserveInputBackend(false)
	stale := &sessionObserver{operations: o, done: make(chan struct{})}
	stale.ObserveInputBackend(true)
	s, _ = o.Status(context.Background())
	if s.InputBackend != "PORTABLE" {
		t.Fatal("stale callback hid portable fallback", s.InputBackend)
	}
	o.finishSession(done, outcome, context.Canceled, true, false)
	observer.ObserveInputBackend(true)
	s, _ = o.Status(context.Background())
	if s.InputBackend != "UNKNOWN" {
		t.Fatal("completed session retained protected mode", s.InputBackend)
	}
}

func TestProtectedInputMonitorStopsOnceWhenPromptAppears(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time, 2)
	checks, stops := 0, 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorProtectedInput(ctx, ticks, func(context.Context) (bool, error) {
			checks++
			return checks == 2, nil
		}, func() { stops++; cancel() })
	}()
	ticks <- time.Now()
	ticks <- time.Now()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("protected input did not stop the session")
	}
	if checks != 2 || stops != 1 {
		t.Fatalf("checks=%d stops=%d", checks, stops)
	}
}

func TestProtectedInputCleanStopDoesNotHideCleanupFailure(t *testing.T) {
	if !protectedInputOnly(errors.Join(context.Canceled, wininput.ErrProtectedInput)) {
		t.Fatal("clean protected input stop was not classified")
	}
	if protectedInputOnly(errors.Join(context.Canceled, wininput.ErrProtectedInput, errors.New("release failed"))) {
		t.Fatal("cleanup failure was hidden")
	}
}

func TestProtectedInputReturnsLocalControlOnlyAfterVerifiedCleanup(t *testing.T) {
	for _, test := range []struct {
		name, want string
		err        error
	}{
		{"clean", "LOCAL", errors.Join(context.Canceled, wininput.ErrProtectedInput)},
		{"cleanup_failed", "FAULTED", errors.Join(context.Canceled, wininput.ErrProtectedInput, errors.New("release failed"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			operations := testOperations(t, nil)
			done := make(chan struct{})
			outcome := &sessionOutcome{connected: true}
			operations.sessionDone = done
			operations.sessionOutcome = outcome
			operations.sessionState = "CONNECTED"
			operations.finishSession(done, outcome, test.err, protectedInputOnly(test.err), false)
			status, err := operations.Status(context.Background())
			if err != nil || status.State != test.want {
				t.Fatalf("status=%+v error=%v, want %s", status, err, test.want)
			}
		})
	}
}

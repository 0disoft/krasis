package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

func TestProtectedInputMonitorStopsOnceWhenPromptAppears(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time, 2)
	checks, stops := 0, 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorProtectedInput(ctx, ticks, func() bool {
			checks++
			return checks == 2
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

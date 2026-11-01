//go:build windows

package wininput

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestCaptureOwnerProfilesSuccessAndRollbackWithoutChangingOutcome(t *testing.T) {
	for _, failRegister := range []bool{false, true} {
		name := "success"
		if failRegister {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			oldCursor := callSetSourceCaptureCursor
			callSetSourceCaptureCursor = func(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil }
			defer func() { callSetSourceCaptureCursor = oldCursor }()
			foreground := uintptr(20)
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
			if failRegister {
				sourceWheelCaptureRegisterWindow = func(_, _ uintptr, enabled bool) error {
					if enabled {
						return errors.New("synthetic register failure")
					}
					return nil
				}
			}
			var summary handoff.Summary
			c := handoff.New(func(s handoff.Summary) { summary = s })
			ctx := handoff.WithSession(context.Background(), c)
			p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
			result := make(chan error, 1)
			postedAt := make(chan time.Time, 1)
			postedAt <- time.Now().Add(-3 * time.Millisecond)
			current := &sourceWheelCaptureRun{window: 10, commands: make(chan sourceWheelCaptureCommand, 1)}
			current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureActivate, result: result, profile: p, queuedAt: time.Now().Add(-5 * time.Millisecond), postedAt: postedAt}
			current.handleCommand(wmSourceWheelCaptureActivate)
			err := <-result
			handoff.Finish(p, handoff.Outcome(err))
			if (err != nil) != failRegister || current.active.Load() == failRegister || !summary.Valid() || summary.Capture == nil {
				t.Fatal("profiling changed native outcome or omitted evidence", err, summary)
			}
			detail := summary.Capture
			if detail.Measured&(1<<handoff.CaptureHandlerTotal|1<<handoff.CaptureQueueToHandle|1<<handoff.CaptureTouchpadRegister) != 1<<handoff.CaptureHandlerTotal|1<<handoff.CaptureQueueToHandle|1<<handoff.CaptureTouchpadRegister || detail.Micros[handoff.CaptureQueueToHandle] < 5000 {
				t.Fatal("command timing detached from owner", detail)
			}
			if (detail.Measured&(1<<handoff.CaptureRollback) != 0) != failRegister || (detail.Measured&(1<<handoff.CaptureForeground) != 0) == failRegister {
				t.Fatal("partial rollback fabricated successful spans", detail)
			}
			wantExact := uint8(1 << handoff.CaptureExactQueueToHandle)
			if !failRegister {
				wantExact |= 1<<handoff.CaptureExactForegroundRequest | 1<<handoff.CaptureExactForegroundVerify
			}
			if detail.Exact.Measured != wantExact || detail.Exact.QueueToHandleMicros < 3000 || failRegister && detail.Exact.ForegroundRequestMicros != 0 {
				t.Fatal("exact owner timing detached from command or fabricated rollback spans", detail.Exact)
			}
			if summary.Focus == nil || summary.Focus.States[handoff.FocusBeforeShow] != 3 {
				t.Fatal("activation dropped original foreground observation", summary.Focus)
			}
			if failRegister && (summary.Focus.Observed != 15 || summary.Focus.Restore != 1 || summary.Focus.PostHide != 1 || summary.Focus.States != [4]uint8{3, 3, 3, 3}) {
				t.Fatal("native rollback lost original attempt focus evidence", summary.Focus)
			}
		})
	}
}

func TestCaptureProfilingStampMissingDoesNotBlockOwner(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	result := make(chan error, 1)
	current := &sourceWheelCaptureRun{window: 10, commands: make(chan sourceWheelCaptureCommand, 1)}
	current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureActivate, result: result, profile: p, queuedAt: time.Now(), postedAt: make(chan time.Time)}
	current.handleCommand(wmSourceWheelCaptureActivate)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	handoff.Finish(p, "ok")
	if summary.Capture == nil || summary.Capture.Exact.Measured&(1<<handoff.CaptureExactQueueToHandle) != 0 || summary.Capture.Exact.Measured&(1<<handoff.CaptureExactForegroundRequest) == 0 || !summary.Capture.Exact.Valid() {
		t.Fatal("missing optional profiling stamp blocked or changed owner activation", summary.Capture)
	}
}

func TestCaptureCommandOrderMismatchDoesNotConsumeProfilingMetadata(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	result := make(chan error, 1)
	postedAt := make(chan time.Time, 1)
	postedAt <- time.Now().Add(-time.Millisecond)
	current := &sourceWheelCaptureRun{window: 10, commands: make(chan sourceWheelCaptureCommand, 1)}
	current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureDeactivate, result: result, profile: p, queuedAt: time.Now().Add(-time.Millisecond), postedAt: postedAt}
	joined := make(chan struct{})
	go func() {
		// Rejection posts WM_QUIT on the owner thread. Retire that private queue
		// instead of leaving it on a reusable Go thread for subsequent tests.
		runtime.LockOSThread()
		defer close(joined)
		current.handleCommand(wmSourceWheelCaptureActivate)
	}()
	<-joined
	if err := <-result; err == nil {
		t.Fatal("command order mismatch accepted")
	}
	if summary.Capture != nil {
		t.Fatal("mismatched command consumed profiling metadata", summary.Capture)
	}
}

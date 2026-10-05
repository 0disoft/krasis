//go:build windows

package wininput

import (
	"context"
	"errors"
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
			current := &sourceWheelCaptureRun{window: 10, commands: make(chan sourceWheelCaptureCommand, 1)}
			current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureActivate, result: result, profile: p, queuedAt: time.Now().Add(-5 * time.Millisecond)}
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
			if summary.Focus == nil || summary.Focus.States[handoff.FocusBeforeShow] != 3 {
				t.Fatal("activation dropped original foreground observation", summary.Focus)
			}
			if failRegister && (summary.Focus.Observed != 15 || summary.Focus.Restore != 1 || summary.Focus.PostHide != 1 || summary.Focus.States != [4]uint8{3, 3, 3, 3}) {
				t.Fatal("native rollback lost original attempt focus evidence", summary.Focus)
			}
		})
	}
}

//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestFocusEarlyRollbackDoesNotInventPostHideObservation(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	sourceWheelCaptureGetForeground = func() uintptr { return 20 }
	sourceWheelCaptureSetPosition = func(uintptr, int32, int32) error { return errors.New("position failure before registration") }
	sourceWheelCaptureHideWithoutActivation = func(uintptr) error { t.Fatal("early rollback unexpectedly hid capture"); return nil }
	var summary handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summary = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	current := &sourceWheelCaptureRun{window: 10}
	err := current.activateWindowProfile(p)
	if _, rejected := err.(*sourceCaptureActivationRejected); !rejected || current.active.Load() || current.registered {
		t.Fatal("early failure changed rollback classification", err)
	}
	handoff.Finish(p, "error")
	if !summary.Valid() || summary.Focus == nil || summary.Focus.Observed != 1 || summary.Focus.States[0] != 3 || summary.Focus.ResultMeasured != 0 || summary.Focus.Measured != 0 {
		t.Fatal("early rollback invented hide or restore evidence", summary.Focus)
	}
}

func TestFocusOwnerRecordsReturnAndPostHideLoss(t *testing.T) {
	for _, lost := range []bool{false, true} {
		name := "restored"
		if lost {
			name = "NULL after hide"
		}
		t.Run(name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(10)
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
			sourceWheelCaptureHideWithoutActivation = func(uintptr) error {
				if lost {
					foreground = 0
				}
				return nil
			}
			var summary handoff.Summary
			c := handoff.New(func(s handoff.Summary) { summary = s })
			ctx := handoff.WithSession(context.Background(), c)
			p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, commands: make(chan sourceWheelCaptureCommand, 1)}
			current.active.Store(true)
			result := make(chan error, 1)
			current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureDeactivate, profile: p, result: result}
			current.handleCommand(wmSourceWheelCaptureDeactivate)
			if err := <-result; err != nil || current.active.Load() || current.savedFocus != 0 {
				t.Fatal("diagnostics changed cleanup", err)
			}
			handoff.Finish(p, "ok")
			post, state := uint8(1), uint8(3)
			if lost {
				post, state = 2, 1
			}
			if !summary.Valid() || summary.Focus == nil || summary.Focus.States != [4]uint8{0, 2, 3, state} || summary.Focus.Observed != 14 || summary.Focus.Restore != 2 || summary.Focus.PostHide != post || summary.Focus.ResultMeasured != 3 || summary.Focus.Measured != 3 {
				t.Fatal("owner return profile lost restore/hide distinction", summary.Focus)
			}
		})
	}
}

func TestFocusObserverDoesNoExtraNativeQueriesAfterRetirement(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	queries := 0
	sourceWheelCaptureGetForeground = func() uintptr { queries++; return 20 }
	c := handoff.New(nil)
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "source", "session", "epoch", time.Now())
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	current.observeFocus(p, handoff.FocusBeforeRestore)
	if queries != 1 {
		t.Fatal("live profile omitted observation")
	}
	handoff.Finish(p, "ok")
	current.observeFocus(p, handoff.FocusAfterRestore)
	current.observeFocus(nil, handoff.FocusAfterRestore)
	if queries != 1 {
		t.Fatal("late or absent profile queried native foreground")
	}
}

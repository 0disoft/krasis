//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestForegroundExchangeCompletionDuringPumpSkipsWaitAndDuplicatePump(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
	previousWait := sourceForegroundExchangeWait
	t.Cleanup(func() { sourceForegroundExchangeWait = previousWait })
	sourceForegroundExchangeWait = func(windows.Handle) error {
		t.Fatal("completed foreground request entered another native wait")
		return nil
	}
	exchange := newSourceForegroundExchange()
	exchange.ready = make(chan *sourceForegroundRequest, 1)
	current := &sourceWheelCaptureRun{window: 10}
	foreground, pumps := uintptr(20), 0
	var accepted *sourceForegroundRequest
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceForegroundTransitionDispatch = func(uintptr) {
		pumps++
		if pumps != 1 {
			t.Fatal("verified foreground serviced redundant sent callbacks")
		}
		accepted = <-exchange.ready
		foreground = current.window
		accepted.complete(current, nil)
	}
	if err := exchange.request(current); err != nil || pumps != 1 || accepted.refs.Load() != 0 || current.active.Load() {
		t.Fatal("completion lost error, event ownership or independent commit", err, pumps)
	}
}

func TestForegroundExchangeFinishVerifiesFreshOwnershipAndCancellation(t *testing.T) {
	for _, stage := range []string{"owned", "pending", "cancelled", "expired", "terminal", "read-cancelled", "native-error"} {
		t.Run(stage, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			t.Cleanup(restore)
			exchange := newSourceForegroundExchange()
			current := &sourceWheelCaptureRun{window: 10}
			reads, pumps := 0, 0
			sourceWheelCaptureGetForeground = func() uintptr {
				reads++
				if stage == "read-cancelled" {
					current.setError(context.Canceled)
				}
				if stage == "pending" {
					return 20
				}
				return 10
			}
			sourceForegroundTransitionDispatch = func(uintptr) { pumps++ }
			var reply, want error
			wantReads, wantPumps := 1, 0
			switch stage {
			case "pending":
				wantPumps = 1
			case "cancelled":
				close(exchange.cancelled)
				want, wantReads = ErrSourceCaptureCommandTimeout, 0
			case "expired":
				exchange.deadline = time.Now()
				want, wantReads = ErrSourceCaptureCommandTimeout, 0
			case "terminal":
				current.setError(context.Canceled)
				want, wantReads = context.Canceled, 0
			case "read-cancelled":
				want = context.Canceled
			case "native-error":
				reply = errors.New("foreground native failure")
				want, wantReads = reply, 0
			}
			if err := exchange.finish(current, reply); !errors.Is(err, want) || reads != wantReads || pumps != wantPumps || current.active.Load() {
				t.Fatal("completion changed refusal, pump count or activation authority", err, reads, pumps)
			}
		})
	}
}

func TestForegroundExchangeShortcutCannotCommitAfterOwnershipChanges(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
	current := &sourceWheelCaptureRun{window: 10}
	exchange := newSourceForegroundExchange()
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureProbeForeground = func(uintptr) error { return errors.New("foreign owner is unresponsive") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 1 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 2, nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error {
		t.Fatal("unresponsive foreground was attached")
		return nil
	}
	err := current.activateWindowWithForeground(nil, func(uintptr) error {
		foreground = 10
		if err := exchange.finish(current, nil); err != nil {
			return err
		}
		foreground = 30 // Another app wins after the shortcut's fresh sample.
		return nil
	})
	if !errors.Is(err, ErrSourceCaptureActivationVerificationLost) || !errors.Is(err, ErrSourceForegroundProbeFailed) || current.active.Load() {
		t.Fatal("completion sample was reused as activation authority", err)
	}
}

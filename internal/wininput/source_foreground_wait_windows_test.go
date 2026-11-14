//go:build windows

package wininput

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestForegroundMessageWaitPreservesQueuedInputAndEventWake(t *testing.T) {
	previous := callSourceForegroundWait
	t.Cleanup(func() { callSourceForegroundWait = previous })
	for _, event := range []windows.Handle{0, 123} {
		callSourceForegroundWait = func(args ...uintptr) (uintptr, uintptr, error) {
			count := uintptr(0)
			if event != 0 {
				count = 1
			}
			if len(args) != 5 || args[0] != count || (args[1] != 0) != (event != 0) || args[2] != 2 || args[3] != 0x04ff || args[4] != 0 {
				t.Fatalf("wait lost its event or can spin on old unremoved input: %v", args)
			}
			return count, 0, syscall.Errno(0)
		}
		if err := waitSourceForegroundMessages(event, time.Millisecond+time.Nanosecond); err != nil {
			t.Fatal(err)
		}
	}
	cause := syscall.ERROR_ACCESS_DENIED
	callSourceForegroundWait = func(...uintptr) (uintptr, uintptr, error) { return uintptr(^uint32(0)), 0, cause }
	if err := waitSourceForegroundMessages(0, time.Millisecond); !errors.Is(err, cause) {
		t.Fatalf("native wait error was lost: %v", err)
	}
}

func TestForegroundTransitionWakePumpsActivationBeforeRetrying(t *testing.T) {
	for _, stage := range []string{"activation", "unrelated", "timeout", "cancelled", "pump-cancelled", "failed"} {
		t.Run(stage, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			t.Cleanup(restore)
			previousWait, previousClock := callSourceForegroundWait, sourceForegroundTransitionClock
			t.Cleanup(func() { callSourceForegroundWait, sourceForegroundTransitionClock = previousWait, previousClock })
			now := time.Unix(1, 0)
			sourceForegroundTransitionClock = func() time.Time { return now }
			current := &sourceWheelCaptureRun{window: 10}
			foreground := uintptr(0)
			waits, pumps, reads := 0, 0, 0
			sourceWheelCaptureGetForeground = func() uintptr { reads++; return foreground }
			sourceForegroundTransitionDispatch = func(window uintptr) {
				if window != 10 {
					t.Fatal("pumped a foreign window")
				}
				pumps++
				if stage == "activation" || stage == "unrelated" && waits == 3 {
					foreground = window
				}
				if stage == "pump-cancelled" {
					current.setError(context.Canceled)
				}
			}
			cause := syscall.ERROR_ACCESS_DENIED
			callSourceForegroundWait = func(args ...uintptr) (uintptr, uintptr, error) {
				waits++
				if args[2] != uintptr(12-2*waits) {
					t.Fatalf("unrelated input renewed the 10ms budget: %v", args)
				}
				now = now.Add(2 * time.Millisecond)
				if stage == "cancelled" {
					current.setError(context.Canceled)
				}
				if stage == "failed" {
					return uintptr(^uint32(0)), 0, cause
				}
				return 0, 0, syscall.Errno(0)
			}
			err := waitSourceForegroundTransition(current, sourceForegroundTransitionInterval)
			wantErr, wantWaits, wantPumps, wantReads := error(nil), 1, 1, 1
			switch stage {
			case "unrelated":
				wantWaits, wantPumps, wantReads = 3, 3, 3
			case "timeout":
				wantWaits, wantPumps, wantReads = 5, 5, 5
			case "cancelled":
				wantErr, wantPumps, wantReads = context.Canceled, 0, 0
			case "pump-cancelled":
				wantErr, wantReads = context.Canceled, 0
			case "failed":
				wantErr, wantPumps, wantReads = cause, 0, 0
			}
			if !errors.Is(err, wantErr) || waits != wantWaits || pumps != wantPumps || reads != wantReads || current.active.Load() {
				t.Fatalf("unbounded wait or late native work: err=%v waits=%d pumps=%d reads=%d", err, waits, pumps, reads)
			}
		})
	}
}

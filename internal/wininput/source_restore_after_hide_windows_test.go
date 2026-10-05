//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPostHideRestoreRepeatedReturnsPreserveOriginalFailureAndSavedOwner(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	requests, hides := 0, 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureIsVisible = func(uintptr) bool { return true }
	sourceRestoreWindowEnabled = func(uintptr) bool { return true }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureProbeForeground = func(uintptr) error { return nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return nil }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		if window == 20 {
			requests++
			if requests%3 != 0 {
				return errors.New("reject before release")
			}
			if foreground != 0 {
				t.Fatal("post-hide request replaced an existing owner")
			}
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureShowWindow = func(_ uintptr, command uintptr) {
		if command == swHide {
			foreground = 0
			hides++
		}
	}
	current := &sourceWheelCaptureRun{window: 10}
	for i := 0; i < 10; i++ {
		var summary handoff.Summary
		c := handoff.New(func(s handoff.Summary) { summary = s })
		p := handoff.Begin(handoff.WithSession(context.Background(), c), "source", "session", "epoch", time.Now())
		if err := current.activateWindowProfile(p); err != nil {
			t.Fatal(err)
		}
		if err := current.deactivateWindowProfile(p); err != nil {
			t.Fatal(err)
		}
		handoff.Finish(p, "ok")
		f := summary.Focus
		if foreground != 20 || current.active.Load() || current.registered || current.savedFocus != 0 || current.attachedSource != 0 ||
			!summary.Valid() || f == nil || f.Restore != 3 || f.PostHide != 1 || !f.PostHideRestoreMeasured ||
			!f.PostHideRestoreAccepted || f.PostHideRestoreForeground != 3 || f.RestoreFallback != 4 || f.States != [4]uint8{3, 2, 2, 3} {
			t.Fatal("return lost saved ownership or original rejection evidence", i, f)
		}
	}
	if requests != 30 || hides != 10 {
		t.Fatal("restoration retried outside its one-shot budget", requests, hides)
	}
}

func TestPostHideRestoreObservesAcceptanceAndOwnerIndependently(t *testing.T) {
	for _, test := range []struct {
		name     string
		owner    uintptr
		accepted bool
		want     uint8
		pending  bool
	}{
		{"success", 20, true, 3, false}, {"accepted_but_null", 0, true, 1, false},
		{"rejected", 0, false, 1, false}, {"capture_retained", 10, true, 2, false},
		{"external_arrived", 30, true, 4, false}, {"pending_completes", 20, true, 3, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(0)
			requests, probes := 0, 0
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureIsVisible = func(uintptr) bool { return true }
			sourceRestoreWindowEnabled = func(uintptr) bool { return true }
			sourceWheelCaptureProbeForeground = func(window uintptr) error {
				if window != 20 {
					t.Fatal("probed an unrelated target")
				}
				probes++
				if requests != 0 && test.pending {
					foreground = test.owner
				}
				return nil
			}
			sourceWheelCaptureSetForeground = func(window uintptr) error {
				if window != 20 || foreground != 0 {
					t.Fatal("invalid one-shot restoration")
				}
				requests++
				if !test.pending {
					foreground = test.owner
				}
				if !test.accepted {
					return errors.New("denied")
				}
				return nil
			}
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("post-hide restoration attached a queue"); return nil }
			var summary handoff.Summary
			c := handoff.New(func(s handoff.Summary) { summary = s })
			p := handoff.Begin(handoff.WithSession(context.Background(), c), "source", "session", "epoch", time.Now())
			handoff.RecordFocusResult(p, 0, 3)
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			current.restoreAfterHideProfile(p, SourceFocusRestoreAPIRejected, ErrSourceFocusRestoreAPIRejected)
			handoff.Finish(p, "ok")
			f := summary.Focus
			wantProbes := 1
			if test.pending || test.accepted && test.owner == 0 {
				wantProbes = 2
			}
			if requests != 1 || probes != wantProbes || !summary.Valid() || f == nil || !f.PostHideRestoreMeasured ||
				f.PostHideRestoreForeground != test.want || f.PostHideRestoreAccepted != test.accepted || f.Restore != 3 || current.active.Load() {
				t.Fatal("post-hide outcome overwrote failure or trusted native acceptance", requests, probes, f)
			}
		})
	}
}

func TestPostHideRestoreGuardPreventsUnsafeOrStaleRequests(t *testing.T) {
	for _, mode := range []string{"external", "capture", "no_saved", "same_target", "hidden", "disabled", "destroyed", "unresponsive", "active", "registered", "attached", "terminal", "success", "attach_failed", "detach_failed", "owner_during_probe", "target_during_probe", "cancel_during_probe", "cancel_during_read"} {
		t.Run(mode, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			foreground := uintptr(0)
			valid := true
			result, cause := SourceFocusRestoreAPIRejected, error(ErrSourceFocusRestoreAPIRejected)
			switch mode {
			case "external":
				foreground = 30
			case "capture":
				foreground = 10
			case "no_saved":
				current.savedFocus = 0
			case "same_target":
				current.savedFocus = 10
			case "active":
				current.active.Store(true)
			case "registered":
				current.registered = true
			case "attached":
				current.attachedSource = 11
			case "terminal":
				current.setError(context.Canceled)
			case "success":
				result, cause = SourceFocusRestoreSucceeded, nil
			case "attach_failed":
				cause = errors.Join(cause, ErrSourceFocusRestoreThreadAttachFailed)
			case "detach_failed":
				cause = errors.Join(cause, ErrSourceFocusRestoreThreadDetachFailed)
			}
			sourceWheelCaptureGetForeground = func() uintptr {
				if mode == "cancel_during_read" {
					current.setError(context.Canceled)
				}
				return foreground
			}
			sourceWheelCaptureIsWindow = func(uintptr) bool { return valid && mode != "destroyed" }
			sourceWheelCaptureIsVisible = func(uintptr) bool { return mode != "hidden" }
			sourceRestoreWindowEnabled = func(uintptr) bool { return mode != "disabled" }
			sourceWheelCaptureProbeForeground = func(uintptr) error {
				switch mode {
				case "unresponsive":
					return errors.New("hung")
				case "owner_during_probe":
					foreground = 30
				case "target_during_probe":
					valid = false
				case "cancel_during_probe":
					current.setError(context.Canceled)
				}
				return nil
			}
			sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("ineligible restoration mutated focus"); return nil }
			current.restoreAfterHideProfile(nil, result, cause)
		})
	}
}

func TestPostHideRestoreCancellationAfterRequestCannotContinueNativeWork(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	requested := false
	sourceWheelCaptureIsVisible = func(uintptr) bool { return true }
	sourceRestoreWindowEnabled = func(uintptr) bool { return true }
	sourceWheelCaptureProbeForeground = func(uintptr) error {
		if requested {
			t.Fatal("cancelled request was probed")
		}
		return nil
	}
	sourceWheelCaptureGetForeground = func() uintptr {
		if requested {
			t.Fatal("cancelled request queried ownership")
		}
		return 0
	}
	sourceWheelCaptureSetForeground = func(uintptr) error { requested = true; current.setError(context.Canceled); return nil }
	current.restoreAfterHideProfile(nil, SourceFocusRestoreAPIRejected, ErrSourceFocusRestoreAPIRejected)
	if !requested || current.active.Load() {
		t.Fatal("request not exercised or capture reactivated")
	}
}

func TestPostHideRestoreCannotHideRetainedCaptureOrTurnRejectionIntoDisconnect(t *testing.T) {
	for _, retained := range []bool{false, true} {
		restore := stubSourceWheelCaptureActivation(t)
		foreground := uintptr(10)
		requests := 0
		sourceWheelCaptureGetForeground = func() uintptr { return foreground }
		sourceWheelCaptureIsVisible = func(uintptr) bool { return true }
		sourceRestoreWindowEnabled = func(uintptr) bool { return true }
		sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
		sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
		sourceWheelCaptureProbeForeground = func(uintptr) error { return nil }
		sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return nil }
		sourceWheelCaptureShowWindow = func(uintptr, uintptr) { foreground = 0 }
		sourceWheelCaptureSetForeground = func(uintptr) error {
			requests++
			if requests == 3 && retained {
				foreground = 10
			}
			return errors.New("rejected")
		}
		current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
		current.active.Store(true)
		err := current.deactivateWindow()
		restore()
		if requests != 3 || (err != nil) != retained || (retained && !errors.Is(err, ErrSourceFocusReleaseCaptureRetained)) || current.savedFocus != 0 || current.active.Load() {
			t.Fatal("post-hide rejection changed release safety policy", retained, requests, err)
		}
	}
}

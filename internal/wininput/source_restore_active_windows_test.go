//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestRestoreActiveRequiresFinalOwnershipAndAlwaysDetaches(t *testing.T) {
	for _, name := range []string{"restored", "unverified", "detach-failed", "lost-after-detach", "canceled-after-active"} {
		t.Run(name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(10)
			attached, calls, detaches := false, 0, 0
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			sourceWheelCaptureIsVisible = func(window uintptr) bool { return window == 20 }
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("foreground rejection") }
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			sourceWheelCaptureSetActive = func(window uintptr) {
				calls++
				if !attached || window != 20 || foreground != 10 {
					t.Fatal("activation did not address the owned saved queue")
				}
				if name != "unverified" {
					foreground = window
				}
				if name == "canceled-after-active" {
					current.setError(context.Canceled)
				}
			}
			sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
				if attach {
					attached = true
					return nil
				}
				detaches++
				if name == "detach-failed" {
					return errors.New("detach rejection")
				}
				attached = false
				if name == "lost-after-detach" {
					foreground = 30
				}
				return nil
			}
			var summary handoff.Summary
			c := handoff.New(func(s handoff.Summary) { summary = s })
			p := handoff.Begin(handoff.WithSession(context.Background(), c), "source", "s", "e", time.Now())
			result, err := current.restoreFocusFallbackProfile(p)
			handoff.Finish(p, handoff.Outcome(err))
			want := SourceFocusRestoreFallbackSucceeded
			switch name {
			case "unverified":
				want = SourceFocusRestoreFallbackAPIRejected
			case "detach-failed":
				want = SourceFocusRestoreFallbackDetachFailed
			case "lost-after-detach", "canceled-after-active":
				want = SourceFocusRestoreFallbackVerificationLost
			}
			activeResult := uint8(1)
			if name == "unverified" {
				activeResult = 2
			}
			if result != want || (err == nil) != (name == "restored") || calls != 1 || detaches != 1 || !summary.Valid() || summary.Focus == nil || !summary.Focus.RestoreActiveMeasured || summary.Focus.RestoreActive != activeResult || summary.Focus.RestoreRejectedForeground != 2 {
				t.Fatal("active restore lost cleanup, final proof or bounded evidence", result, err, calls, detaches, summary.Focus)
			}
			if name != "detach-failed" && current.attachedSource != 0 {
				t.Fatal("successful detach retained attachment")
			}
		})
	}
}

func TestRestoreActiveSkipsUnsafeOwnersAndCaptureActivation(t *testing.T) {
	for _, name := range []string{"null", "external", "target-already-observed", "destroyed", "hidden", "disabled", "owner-changed", "canceled-before-active", "activation"} {
		t.Run(name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			sourceWheelCaptureIsVisible = func(window uintptr) bool { return window == 20 && name != "hidden" }
			sourceWheelCaptureIsEnabled = func(uintptr) bool { return name != "disabled" }
			foreground := uintptr(10)
			switch name {
			case "null":
				foreground = 0
			case "external":
				foreground = 30
			case "target-already-observed":
				foreground = 20
			}
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("foreground rejection") }
			sourceWheelCaptureSetActive = func(uintptr) { t.Fatal("unsafe restore activation") }
			sourceWheelCaptureIsWindow = func(uintptr) bool {
				if name == "owner-changed" {
					foreground = 30
				}
				if name == "canceled-before-active" {
					current.setError(context.Canceled)
				}
				return name != "destroyed"
			}
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			detaches := 0
			sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
				if !attach {
					detaches++
				}
				return nil
			}
			if name == "activation" {
				current.setForegroundWithThreadAttach(10, 20)
			} else {
				current.restoreFocusFallback()
			}
			if detaches != 1 || current.attachedSource != 0 {
				t.Fatal("ineligible restore lost detach cleanup")
			}
		})
	}
}

func TestRestoreActiveAllowsNextCaptureWithoutPhysicalClick(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(10)
	sourceWheelCaptureIsVisible = func(window uintptr) bool { return window == 20 }
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return nil }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		if window == 20 {
			return errors.New("saved focus denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureSetActive = func(window uintptr) { foreground = window }
	sourceWheelCaptureHideWithoutActivation = func(uintptr) error {
		if foreground != 20 {
			t.Fatal("hide without a restored external owner")
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, registered: true}
	current.active.Store(true)
	if err := current.deactivateWindow(); err != nil || foreground != 20 || current.savedFocus != 0 {
		t.Fatal("return did not restore local owner", err)
	}
	if err := current.activateWindow(); err != nil || !current.active.Load() || current.savedFocus != 20 {
		t.Fatal("next capture still required a physical click", err)
	}
}

//go:build windows

package wininput

import (
	"errors"
	"testing"
	"time"
)

func TestCaptureShowPreservesForegroundForVerifiedFallback(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	stubForegroundTransitionPause(t, func(time.Duration) { t.Fatal("show discarded the foreground owner") })
	foreground := uintptr(20)
	sets, attaches := 0, 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureShowWindow = func(window, command uintptr) {
		if window != 10 {
			t.Fatal("wrong capture window")
		}
		if command == swShow {
			foreground = 0
		}
		if command != swShowWithoutActivation {
			t.Fatal("capture show independently activated the window")
		}
	}
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		sets++
		if sets == 1 {
			return errors.New("ordinary activation denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(window uintptr) (uint32, error) {
		if window != 20 {
			t.Fatal("fallback did not use the observed foreground owner")
		}
		return 22, nil
	}
	sourceWheelCaptureAttachThreads = func(_, _ uint32, _ bool) error { attaches++; return nil }
	current := &sourceWheelCaptureRun{window: 10}
	if err := current.activateWindow(); err != nil || !current.active.Load() || current.savedFocus != 20 || attaches != 2 {
		t.Fatal("capture show prevented the verified fallback", err, attaches)
	}
}

func TestCaptureNullFailureIdentifiesFirstObservedStage(t *testing.T) {
	for _, test := range []struct {
		name string
		want error
	}{
		{"before", ErrSourceForegroundMissingBeforeShow},
		{"show", ErrSourceForegroundMissingAfterShow},
		{"request", ErrSourceForegroundMissingAfterRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			stubForegroundTransitionPause(t, func(time.Duration) {})
			foreground := uintptr(20)
			if test.name == "before" {
				foreground = 0
			}
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureShowWindow = func(_ uintptr, command uintptr) {
				if command == swShowWithoutActivation && test.name == "show" {
					foreground = 0
				}
			}
			sourceWheelCaptureSetForeground = func(uintptr) error {
				if test.name == "request" {
					foreground = 0
				}
				return errors.New("denied")
			}
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("missing owner attached an unknown thread"); return nil }
			current := &sourceWheelCaptureRun{window: 10}
			err := current.activateWindow()
			if !errors.Is(err, test.want) || !errors.Is(err, ErrSourceForegroundUnavailable) || current.active.Load() || current.registered {
				t.Fatal("missing foreground stage or rollback proof was lost", err)
			}
			if _, ok := err.(*sourceCaptureActivationRejected); !ok {
				t.Fatal("clean failure lost safe retry proof", err)
			}
		})
	}
}

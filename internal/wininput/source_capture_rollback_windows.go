//go:build windows

package wininput

import (
	"context"
	"errors"
)

var (
	ErrSourceForegroundUnavailable         = errors.New("source foreground window unavailable")
	ErrSourceForegroundMissingBeforeShow   = errors.New("source foreground missing before capture show")
	ErrSourceForegroundMissingAfterShow    = errors.New("source foreground missing after capture show")
	ErrSourceForegroundMissingAfterRequest = errors.New("source foreground missing after activation request")
	ErrSourceForegroundThreadUnavailable   = errors.New("source foreground thread unavailable")
	ErrSourceForegroundProbeFailed         = errors.New("source foreground response probe failed")
	ErrSourceCaptureRollbackFailed         = errors.New("source capture rollback unverified")
	sourceWheelCaptureIsVisible            = func(window uintptr) bool {
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(window)
		return visible != 0
	}
)

// Only the native owner may construct this after checking every rollback step.
// A generic activation error (or an error joined with it) is never this proof.
type sourceCaptureActivationRejected struct{ cause error }

func (e *sourceCaptureActivationRejected) Error() string { return e.cause.Error() }
func (e *sourceCaptureActivationRejected) Unwrap() error { return e.cause }

func (current *sourceWheelCaptureRun) rollbackActivation(cause error) error {
	return current.rollbackActivationProfile(nil, cause)
}

func (current *sourceWheelCaptureRun) rollbackActivationProfile(profile context.Context, cause error) error {
	cleanupErr := current.deactivateWindowProfile(profile)
	// This second proof also runs when cleanup returned before any hide attempt.
	// Only deactivateWindowProfile records afterHide following a real hide call.
	_, focusErr := current.verifyPostHideFocusRelease()
	terminalErr := current.err()
	if sourceWheelCaptureIsVisible(current.window) || current.active.Load() || current.registered ||
		errors.Is(cause, ErrSourceCaptureThreadDetachFailed) || errors.Is(cause, ErrSourceFocusRestoreThreadDetachFailed) {
		cleanupErr = errors.Join(cleanupErr, ErrSourceCaptureRollbackFailed)
	}
	if cleanupErr != nil || focusErr != nil || terminalErr != nil {
		return errors.Join(cause, cleanupErr, focusErr, terminalErr, ErrSourceCaptureRollbackFailed)
	}
	current.forgetSavedFocus()
	return &sourceCaptureActivationRejected{cause: cause}
}

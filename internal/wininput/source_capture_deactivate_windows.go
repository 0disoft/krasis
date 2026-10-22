//go:build windows

package wininput

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/handoff"
)

func (current *sourceWheelCaptureRun) deactivateWindow() error {
	return current.deactivateWindowProfile(nil)
}

// retireWindow runs only when the native owner loop is leaving. The owned
// window is destroyed immediately afterwards; asking another application's input
// queue to restore focus must not hold up the owner's final join.
func (current *sourceWheelCaptureRun) retireWindow() error {
	return current.deactivateWindowWithFocus(nil, false)
}

func (current *sourceWheelCaptureRun) deactivateWindowProfile(profile context.Context) error {
	return current.deactivateWindowWithFocus(profile, true)
}

func (current *sourceWheelCaptureRun) deactivateWindowWithFocus(profile context.Context, restoreFocus bool) error {
	if scope := sourceLoginScope(current.loginContext); restoreFocus && scope != nil {
		if err := scope.Do(context.Background(), func() error { return nil }); err != nil {
			return current.retireLoginCapture()
		}
	}
	wasActive := current.active.Swap(false)
	wasRegistered := current.registered
	// Retry only the exact native pair we attached, never a newly discovered owner.
	detachErr := current.detachForegroundThreads()
	if !wasActive && !wasRegistered {
		current.releaseFullscreenPresentation()
		return detachErr
	}
	if current.observer != nil && wasActive {
		current.observer.ObserveWheelCaptureDeactivated()
	}
	cleanupErr := detachErr
	if wasRegistered {
		if unregisterErr := sourceWheelCaptureRegisterWindow(current.registerProc, current.window, false); unregisterErr != nil {
			cleanupErr = errors.Join(cleanupErr, ErrSourceCaptureUnregisterFailed, touchpadWin32Error("RegisterTouchpadCapableWindow(source FALSE)", unregisterErr))
		} else {
			current.registered = false
		}
	}
	if !restoreFocus {
		// SW_HIDE can synchronously activate another window. Use a nonactivating
		// hide and let owner-thread destruction release the final foreground.
		// Native detach/unregister failures above remain outstanding failures.
		current.forgetSavedFocus()
		hideErr := sourceWheelCaptureHideWithoutActivation(current.window)
		current.releaseFullscreenPresentation()
		return errors.Join(cleanupErr, hideErr)
	}
	// Restore while the capture window is still foreground. Hiding it first lets
	// Windows choose another foreground owner, which makes restoreFocus classify
	// every ordinary return as skipped and loses the saved application focus.
	current.observeFocus(profile, handoff.FocusBeforeRestore)
	stopRestore := handoff.FocusSpan(profile, 0)
	restoreResult, restoreErr := current.restoreFocusProfile(profile)
	stopRestore()
	handoff.RecordFocusResult(profile, 0, uint8(restoreResult))
	current.observeFocus(profile, handoff.FocusAfterRestore)
	stopHide := handoff.FocusSpan(profile, 1)
	hideErr := current.hideAfterFocusRestore()
	current.releaseFullscreenPresentation()
	stopHide()
	if hideErr != nil {
		cleanupErr = errors.Join(cleanupErr, hideErr, ErrSourceCaptureRollbackFailed)
	}
	if current.observer != nil && wasActive {
		current.observer.ObserveWheelFocusRestore(restoreResult)
	}
	if wasActive && cleanupErr == nil {
		// The first restore's failure remains recorded. Only a fresh NULL owner
		// after hide permits this separate, one-shot saved-window request.
		current.restoreAfterHideProfile(profile, restoreResult, restoreErr)
	}
	savedFocus := current.savedFocus
	current.forgetSavedFocus()
	// Partial activation can own registration and foreground before active=true.
	// It needs the same post-hide release proof as a fully active capture.
	postHideResult, postHideErr := current.verifyPostHideFocusReleaseProfile(profile, savedFocus)
	if restoreErr == nil {
		return errors.Join(cleanupErr, postHideErr)
	}
	if current.observer != nil {
		current.observer.ObserveWheelPostHideFocus(postHideResult)
	}
	if errors.Is(restoreErr, ErrSourceFocusRestoreThreadDetachFailed) || postHideErr != nil {
		return errors.Join(cleanupErr, restoreErr, postHideErr)
	}
	return cleanupErr
}

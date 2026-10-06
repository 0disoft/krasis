//go:build windows

package wininput

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

var callSourceRestoreEnabled = user32.NewProc("IsWindowEnabled").Call
var sourceRestoreWindowEnabled = func(window uintptr) bool {
	enabled, _, _ := callSourceRestoreEnabled(window)
	return enabled != 0
}

// Called only after hiding a fully active capture whose saved-window restore
// was rejected. Unlike restoreFocusProfile, this can use the saved target after
// capture has released foreground, but only while a fresh NULL owner is observed.
// It never joins another queue, retries in a loop or hides a window itself.
func (current *sourceWheelCaptureRun) restoreAfterHideProfile(profile context.Context, restoreResult SourceFocusRestoreResult, restoreErr error) {
	if restoreResult != SourceFocusRestoreAPIRejected || !errors.Is(restoreErr, ErrSourceFocusRestoreAPIRejected) ||
		errors.Is(restoreErr, ErrSourceFocusRestoreThreadAttachFailed) || errors.Is(restoreErr, ErrSourceFocusRestoreThreadDetachFailed) ||
		current.window == 0 || current.active.Load() || current.registered || current.attachedSource != 0 || current.err() != nil {
		return
	}
	target := current.savedFocus
	if target == 0 || target == current.window || !sourceFocusMatches(target, current.savedFocusIdentity) ||
		!sourceWheelCaptureIsVisible(target) || !sourceRestoreWindowEnabled(target) || sourceWheelCaptureGetForeground() != 0 {
		return
	}
	started := time.Now()
	if current.err() != nil {
		return
	}
	// Probe before attempting restoration, and recheck both target lifetime and
	// foreground after this wait. An intervening owner must never be replaced.
	if sourceWheelCaptureProbeForeground(target) != nil || current.err() != nil ||
		!sourceFocusMatches(target, current.savedFocusIdentity) || !sourceWheelCaptureIsVisible(target) || !sourceRestoreWindowEnabled(target) ||
		sourceWheelCaptureGetForeground() != 0 || current.err() != nil {
		return
	}
	requestErr := sourceWheelCaptureSetForeground(target)
	if current.err() != nil {
		return
	}
	foreground := sourceWheelCaptureGetForeground()
	if current.err() != nil {
		return
	}
	if requestErr == nil && foreground == 0 {
		// Complete a potentially asynchronous request with the existing 100ms
		// responsiveness probe. Do not probe or replace an observed external
		// owner, or issue a second foreground request.
		_ = sourceWheelCaptureProbeForeground(target)
		if current.err() != nil {
			return
		}
		foreground = sourceWheelCaptureGetForeground()
		if current.err() != nil {
			return
		}
	}
	handoff.RecordPostHideRestore(profile, sourceFocusState(foreground, current.window, target), requestErr == nil, time.Since(started))
}

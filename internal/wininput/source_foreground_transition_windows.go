//go:build windows

package wininput

import (
	"time"
	"unsafe"
)

const sourceForegroundTransitionAttempts = 5
const sourceForegroundTransitionInterval = 10 * time.Millisecond

var sourceForegroundTransitionPause = time.Sleep

const sourceForegroundSentMessages = 0x00400000 // PM_QS_SENDMESSAGE | PM_NOREMOVE

var callSourceForegroundMessages = procPeekMessageW.Call

var sourceForegroundTransitionDispatch = func(window uintptr) {
	if window == 0 {
		return
	}
	var pending message
	// PeekMessage services sent messages/internal activation events on this
	// owner thread. Never remove or dispatch queued input, commands or WM_QUIT
	// inside an activation command. Zero is normal when no posted item exists.
	callSourceForegroundMessages(uintptr(unsafe.Pointer(&pending)), window, 0, 0, sourceForegroundSentMessages)
}

// GetForegroundWindow may temporarily return NULL while activation changes.
// Resample before retrying activation of our own capture window. Windows allows
// SetForegroundWindow when no foreground process exists, though it may still
// deny the request. Passive polling alone can leave NULL until a physical click.
// Only a freshly observed owner proves success; never reuse savedFocus or
// attach to an unknown thread. Normal foreground paths have no added delay.
// The capture command retains its existing two-second outer deadline.
func (current *sourceWheelCaptureRun) foregroundAfterTransition() (uintptr, error) {
	if err := current.err(); err != nil {
		return 0, err
	}
	foreground := sourceWheelCaptureGetForeground()
	for attempt := 0; foreground == 0 && attempt < sourceForegroundTransitionAttempts; attempt++ {
		if err := current.err(); err != nil {
			return 0, err
		}
		sourceForegroundTransitionPause(sourceForegroundTransitionInterval)
		if err := current.err(); err != nil {
			return 0, err
		}
		// Sleeping inside our window procedure does not service an asynchronous
		// foreground activation. Give Windows a chance to finish it before the
		// next read; a physical click must not be needed to unblock the owner.
		sourceForegroundTransitionDispatch(current.window)
		if err := current.err(); err != nil {
			return 0, err
		}
		foreground = sourceWheelCaptureGetForeground()
		if foreground == 0 && current.window != 0 {
			// This runs on the capture owner thread after ordinary activation
			// failed. Request focus only for that capture, not a saved app, and
			// verify ownership even if the API's return value says success.
			if err := current.err(); err != nil {
				return 0, err
			}
			_ = sourceWheelCaptureSetForeground(current.window)
			if err := current.err(); err != nil {
				return 0, err
			}
			sourceForegroundTransitionDispatch(current.window)
			if err := current.err(); err != nil {
				return 0, err
			}
			foreground = sourceWheelCaptureGetForeground()
		}
	}
	if foreground == 0 {
		return 0, ErrSourceForegroundUnavailable
	}
	return foreground, nil
}

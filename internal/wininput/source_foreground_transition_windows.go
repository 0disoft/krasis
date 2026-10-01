//go:build windows

package wininput

import "time"

const sourceForegroundTransitionAttempts = 5
const sourceForegroundTransitionInterval = 10 * time.Millisecond

var sourceForegroundTransitionPause = time.Sleep

// GetForegroundWindow may temporarily return NULL while activation changes.
// Wait only for a newly observed foreground handle, never reuse savedFocus or
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
		foreground = sourceWheelCaptureGetForeground()
	}
	if foreground == 0 {
		return 0, ErrSourceForegroundUnavailable
	}
	return foreground, nil
}

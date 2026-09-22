//go:build windows

package wininput

import "errors"

// Called on the capture window's owning thread. Failed native cleanup remains
// an obligation; clearing a flag is never a substitute for a successful call.
func (current *sourceWheelCaptureRun) detachForegroundThreads() error {
	if current.attachedSource == 0 {
		return nil
	}
	if sourceWheelCaptureCurrentThread() != current.attachedSource {
		return errors.Join(ErrSourceCaptureThreadDetachFailed, ErrSourceForegroundThreadUnavailable)
	}
	if err := sourceWheelCaptureAttachThreads(current.attachedSource, current.attachedPeer, false); err != nil {
		return errors.Join(ErrSourceCaptureThreadDetachFailed, err)
	}
	current.attachedSource, current.attachedPeer = 0, 0
	return nil
}

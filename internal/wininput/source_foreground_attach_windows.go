//go:build windows

package wininput

import (
	"errors"

	"github.com/0disoft/krasis/internal/handoff"
)

// The extra result identifies existing exits only. Call order, cleanup and
// ownership proof are shared by activation and restoration without extra calls.
func (current *sourceWheelCaptureRun) setForegroundWithThreadAttachDetailed(targetWindow, peerWindow uintptr) (foregroundThreadFallbackResult, handoff.FocusFallbackStage, error) {
	if current.attachedSource != 0 {
		return foregroundThreadFallbackDetachFailed, handoff.FocusFallbackPendingDetach, ErrSourceCaptureThreadDetachFailed
	}
	if err := current.err(); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackTerminalBefore, err
	}
	if targetWindow == 0 || peerWindow == 0 {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackWindowUnavailable, ErrSourceForegroundUnavailable
	}
	sourceThread := sourceWheelCaptureCurrentThread()
	peerThread, err := sourceWheelCaptureWindowThread(peerWindow)
	if err != nil || sourceThread == 0 || peerThread == 0 || sourceThread == peerThread {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackThreadUnavailable, errors.Join(err, ErrSourceForegroundThreadUnavailable)
	}
	// This rejects an already-unresponsive window, preserving the existing probe.
	if err := sourceWheelCaptureProbeForeground(peerWindow); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackProbeFailed, errors.Join(ErrSourceForegroundProbeFailed, err)
	}
	if err := current.err(); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackTerminalBeforeAttach, err
	}
	if err := sourceWheelCaptureAttachThreads(sourceThread, peerThread, true); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackAttachFailed, err
	}
	current.attachedSource, current.attachedPeer = sourceThread, peerThread
	foregroundErr := current.err()
	foregroundStage := handoff.FocusFallbackTerminalBeforeSet
	if foregroundErr == nil {
		foregroundErr = sourceWheelCaptureSetForeground(targetWindow)
		foregroundStage = handoff.FocusFallbackSetFailed
	}
	verified := foregroundErr == nil && sourceWheelCaptureGetForeground() == targetWindow
	detachErr := current.detachForegroundThreads()
	if detachErr != nil {
		return foregroundThreadFallbackDetachFailed, handoff.FocusFallbackDetachFailed, errors.Join(detachErr, foregroundErr)
	}
	if foregroundErr != nil {
		return foregroundThreadFallbackAPIRejected, foregroundStage, foregroundErr
	}
	if err := current.err(); err != nil {
		return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackTerminalAfterDetach, err
	}
	// Observation while queues were joined is not final proof after detachment.
	if verified && sourceWheelCaptureGetForeground() != targetWindow {
		if err := current.err(); err != nil {
			return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackTerminalDuringCompletion, err
		}
		if targetWindow == current.window {
			sourceForegroundTransitionDispatch(targetWindow)
		} else if err := sourceWheelCaptureProbeForeground(targetWindow); err != nil {
			return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackCompletionProbeFailed, err
		}
		if err := current.err(); err != nil {
			return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackTerminalDuringCompletion, err
		}
		verified = sourceWheelCaptureGetForeground() == targetWindow
	}
	if !verified {
		return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackVerificationLost, errors.New("foreground fallback target was not verified")
	}
	return foregroundThreadFallbackSucceeded, handoff.FocusFallbackSucceeded, nil
}

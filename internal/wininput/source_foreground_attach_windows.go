//go:build windows

package wininput

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/handoff"
)

func (current *sourceWheelCaptureRun) setForegroundWithThreadAttachDetailed(targetWindow, peerWindow uintptr, profile context.Context) (foregroundThreadFallbackResult, handoff.FocusFallbackStage, uint8, error) {
	if current.attachedSource != 0 {
		return foregroundThreadFallbackDetachFailed, handoff.FocusFallbackPendingDetach, 0, ErrSourceCaptureThreadDetachFailed
	}
	if err := current.err(); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackTerminalBefore, 0, err
	}
	if targetWindow == 0 || peerWindow == 0 {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackWindowUnavailable, 0, ErrSourceForegroundUnavailable
	}
	sourceThread := sourceWheelCaptureCurrentThread()
	peerThread, err := sourceWheelCaptureWindowThread(peerWindow)
	if err != nil || sourceThread == 0 || peerThread == 0 || sourceThread == peerThread {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackThreadUnavailable, 0, errors.Join(err, ErrSourceForegroundThreadUnavailable)
	}
	// This rejects an already-unresponsive window, preserving the existing probe.
	if err := sourceWheelCaptureProbeForeground(peerWindow); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackProbeFailed, 0, errors.Join(ErrSourceForegroundProbeFailed, err)
	}
	if err := current.err(); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackTerminalBeforeAttach, 0, err
	}
	if err := sourceWheelCaptureAttachThreads(sourceThread, peerThread, true); err != nil {
		return foregroundThreadFallbackAttachFailed, handoff.FocusFallbackAttachFailed, 0, err
	}
	current.attachedSource, current.attachedPeer = sourceThread, peerThread
	foregroundErr := current.err()
	foregroundStage := handoff.FocusFallbackTerminalBeforeSet
	if foregroundErr == nil {
		foregroundErr = sourceWheelCaptureSetForeground(targetWindow)
		foregroundStage = handoff.FocusFallbackSetFailed
	}
	verified := foregroundErr == nil && sourceWheelCaptureGetForeground() == targetWindow
	var rejectedForeground uint8
	if foregroundErr != nil && foregroundStage == handoff.FocusFallbackSetFailed && handoff.FocusEnabled(profile) {
		foreground := sourceWheelCaptureGetForeground()
		rejectedForeground = sourceFocusState(foreground, current.window, current.savedFocus)
	}
	detachErr := current.detachForegroundThreads()
	if detachErr != nil {
		return foregroundThreadFallbackDetachFailed, handoff.FocusFallbackDetachFailed, rejectedForeground, errors.Join(detachErr, foregroundErr)
	}
	if foregroundErr != nil {
		return foregroundThreadFallbackAPIRejected, foregroundStage, rejectedForeground, foregroundErr
	}
	if err := current.err(); err != nil {
		return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackTerminalAfterDetach, rejectedForeground, err
	}
	// Observation while queues were joined is not final proof after detachment.
	if verified && sourceWheelCaptureGetForeground() != targetWindow {
		if err := current.err(); err != nil {
			return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackTerminalDuringCompletion, rejectedForeground, err
		}
		if targetWindow == current.window {
			sourceForegroundTransitionDispatch(targetWindow)
		} else if err := sourceWheelCaptureProbeForeground(targetWindow); err != nil {
			return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackCompletionProbeFailed, rejectedForeground, err
		}
		if err := current.err(); err != nil {
			return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackTerminalDuringCompletion, rejectedForeground, err
		}
		verified = sourceWheelCaptureGetForeground() == targetWindow
	}
	if !verified {
		return foregroundThreadFallbackVerificationLost, handoff.FocusFallbackVerificationLost, rejectedForeground, errors.New("foreground fallback target was not verified")
	}
	return foregroundThreadFallbackSucceeded, handoff.FocusFallbackSucceeded, rejectedForeground, nil
}

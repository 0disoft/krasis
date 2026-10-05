package handoff

import "context"

// Stages identify existing native exits. Zero is unobserved. Neither OS error
// text, native handles nor thread/process identities are retained.
type FocusFallbackStage uint8

const (
	FocusFallbackSucceeded FocusFallbackStage = iota + 1
	FocusFallbackPendingDetach
	FocusFallbackTerminalBefore
	FocusFallbackWindowUnavailable
	FocusFallbackThreadUnavailable
	FocusFallbackProbeFailed
	FocusFallbackTerminalBeforeAttach
	FocusFallbackAttachFailed
	FocusFallbackTerminalBeforeSet
	FocusFallbackSetFailed
	FocusFallbackDetachFailed
	FocusFallbackTerminalAfterDetach
	FocusFallbackTerminalDuringCompletion
	FocusFallbackCompletionProbeFailed
	FocusFallbackVerificationLost
)

// Result uses the existing restore fallback enum: 1 success, 2 attach failure,
// 3 detach failure, 4 API rejection, 5 verification loss. Stages refine that
// result without rewriting its meaning or authorizing a recovery operation.
func validFocusFallback(result uint8, stage FocusFallbackStage) bool {
	switch stage {
	case FocusFallbackSucceeded:
		return result == 1
	case FocusFallbackTerminalBefore, FocusFallbackWindowUnavailable, FocusFallbackThreadUnavailable,
		FocusFallbackProbeFailed, FocusFallbackTerminalBeforeAttach, FocusFallbackAttachFailed:
		return result == 2
	case FocusFallbackPendingDetach, FocusFallbackDetachFailed:
		return result == 3
	case FocusFallbackTerminalBeforeSet, FocusFallbackSetFailed:
		return result == 4
	case FocusFallbackTerminalAfterDetach, FocusFallbackTerminalDuringCompletion,
		FocusFallbackCompletionProbeFailed, FocusFallbackVerificationLost:
		return result == 5
	default:
		return false
	}
}

func RecordFocusFallback(ctx context.Context, result uint8, stage FocusFallbackStage) {
	RecordFocusFallbackObserved(ctx, result, stage, 0)
}

// Record the final outcome and optional pre-detach observation in one update.
// Zero means unobserved; observing a target does not override native rejection.
func RecordFocusFallbackObserved(ctx context.Context, result uint8, stage FocusFallbackStage, rejectedForeground uint8) {
	RecordFocusFallbackActive(ctx, result, stage, rejectedForeground, 0)
}

// Active 1 is foreground verified while joined; 2 is requested but unverified.
// Final fallback results still require ownership proof after detachment.
func validFocusActive(result uint8, stage FocusFallbackStage, active uint8) bool {
	if !validFocusFallback(result, stage) {
		return false
	}
	if active == 1 {
		return result == 1 || result == 5 || stage == FocusFallbackDetachFailed
	}
	return active == 2 && (stage == FocusFallbackSetFailed || stage == FocusFallbackDetachFailed)
}

func RecordFocusFallbackActive(ctx context.Context, result uint8, stage FocusFallbackStage, rejectedForeground, active uint8) {
	if !validFocusFallback(result, stage) || rejectedForeground > 4 ||
		(active != 0 && (rejectedForeground != 2 || !validFocusActive(result, stage, active))) ||
		(active == 0 && rejectedForeground != 0 && stage != FocusFallbackSetFailed && stage != FocusFallbackDetachFailed) {
		return
	}
	updateFocus(ctx, func(p *FocusProfile) {
		if !p.RestoreFallbackMeasured {
			p.RestoreFallback, p.RestoreFallbackStage = result, stage
			p.RestoreFallbackMeasured = true
			p.RestoreRejectedForeground = rejectedForeground
			p.RestoreRejectedForegroundMeasured = rejectedForeground != 0
			p.RestoreActive, p.RestoreActiveMeasured = active, active != 0
		}
	})
}

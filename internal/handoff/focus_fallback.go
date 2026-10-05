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
	if !validFocusFallback(result, stage) {
		return
	}
	updateFocus(ctx, func(p *FocusProfile) {
		if !p.RestoreFallbackMeasured {
			p.RestoreFallback, p.RestoreFallbackStage = result, stage
			p.RestoreFallbackMeasured = true
		}
	})
}

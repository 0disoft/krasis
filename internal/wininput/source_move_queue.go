package wininput

import "github.com/0disoft/krasis/internal/control"

// Normalized move axes are within +/-32767, so an int32-minimum X cannot be a
// valid payload. One atomic word owns both axes and the sender's claim marker.
const claimedQueuedMove uint64 = 1 << 63

func packQueuedMove(x, y int32) uint64            { return uint64(uint32(x))<<32 | uint64(uint32(y)) }
func unpackQueuedMove(move uint64) (int32, int32) { return int32(move >> 32), int32(move) }

// Producers serialize on queueMu. The sole concurrent writer is the sender,
// which can only replace a payload with the permanent claimed marker. One CAS
// suffices: a lost race must enqueue a new frame, never mutate the sent frame.
func (queued *queuedAction) mergeMove(x, y int32) bool {
	before := queued.move.Load()
	if before == claimedQueuedMove {
		return false
	}
	px, py := unpackQueuedMove(before)
	dx, dy := int64(px)+int64(x), int64(py)+int64(y)
	if dx == 0 && dy == 0 || dx < -control.MaxInputDelta || dx > control.MaxInputDelta || dy < -control.MaxInputDelta || dy > control.MaxInputDelta {
		return false
	}
	return queued.move.CompareAndSwap(before, packQueuedMove(int32(dx), int32(dy)))
}

func (queued *queuedAction) takeAction() (control.InputAction, bool) {
	action := queued.action
	if action.Kind == control.InputPointerMove {
		move := queued.move.Swap(claimedQueuedMove)
		if move == claimedQueuedMove {
			return control.InputAction{}, false
		}
		action.DeltaX, action.DeltaY = unpackQueuedMove(move)
	}
	return action, true
}

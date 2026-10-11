//go:build windows

package wininput

import "time"

const returnCursorSnapshotWait = 20 * time.Millisecond

type returnCursorSnapshotRequest struct {
	id, generation uint64
	result         chan returnCursorSnapshotResult
}

type returnCursorSnapshotResult struct {
	position point
	valid    bool
}

// PrepareReturnCursor runs on the peer worker, not the Winlogon-bound native
// owner. Snapshot through that owner's message queue on both desktop paths.
// No Go pointer crosses Win32 and at most one replaceable request is retained.
func (shield *returnCursorShield) requestPosition(generation uint64) (point, bool) {
	window := shield.window.Load()
	if window == 0 {
		return point{}, false
	}
	request := &returnCursorSnapshotRequest{id: shield.snapshotID.Add(1), generation: generation, result: make(chan returnCursorSnapshotResult, 1)}
	shield.snapshotRequest.Store(request)
	defer shield.snapshotRequest.CompareAndSwap(request, nil)
	if err := returnCursorPost(window, wmReturnCursorSnapshot, request.id); err != nil {
		shield.record("cursor-present", err)
		return point{}, false
	}
	timer := time.NewTimer(returnCursorSnapshotWait)
	defer timer.Stop()
	select {
	case result := <-request.result:
		return result.position, result.valid && shield.generation.Load() == generation
	case <-timer.C:
		return point{}, false
	}
}

// Called only by the shield window's native owner. A timed-out, superseded or
// locally invalidated snapshot cannot read the desktop or hide a later cursor.
func (shield *returnCursorShield) snapshotPosition(id uint64) {
	request := shield.snapshotRequest.Load()
	if request == nil || request.id != id {
		return
	}
	result := returnCursorSnapshotResult{}
	if shield.generation.Load() == request.generation && sourceLoginDesktopCheck(shield.loginContext) == nil {
		position, err := returnCursorPosition()
		if err == nil && shield.generation.Load() == request.generation {
			result = returnCursorSnapshotResult{position: position, valid: true}
		}
	}
	select {
	case request.result <- result:
	default:
	}
}

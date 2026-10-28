//go:build windows

package wininput

import "time"

type returnCursorSnapshotRequest struct {
	id, generation uint64
	position       point // written and read only on the native owner
	valid          bool
}

// Called only by the shield window's native owner. Superseded or locally
// invalidated requests cannot read the desktop or hide a later cursor.
func (shield *returnCursorShield) snapshotPosition(id uint64) {
	started, outcome := time.Now(), "superseded"
	defer func() { shield.observe("return_snapshot", outcome, started) }()
	request := shield.snapshotRequest.Load()
	if request == nil || request.id != id || shield.generation.Load() != request.generation {
		return
	}
	if sourceLoginDesktopCheck(shield.loginContext) != nil {
		outcome = "inactive"
		return
	}
	position, err := returnCursorPosition()
	if err != nil {
		outcome = "failed"
		return
	}
	if shield.generation.Load() == request.generation && shield.snapshotRequest.Load() == request {
		request.position, request.valid = position, true
		outcome = "accepted"
	}
}

func (shield *returnCursorShield) showPrepared(id uint64) {
	request := shield.snapshotRequest.Load()
	if request == nil || request.id != id || !shield.snapshotRequest.CompareAndSwap(request, nil) {
		return
	}
	if !request.valid || shield.generation.Load() != request.generation {
		return
	}
	shield.position = request.position
	shield.show(request.generation)
}

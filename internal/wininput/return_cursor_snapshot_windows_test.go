//go:build windows

package wininput

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestReturnCursorSnapshotsOnlyThroughNativeOwnerMessage(t *testing.T) {
	oldPost, oldPosition := returnCursorPost, returnCursorPosition
	t.Cleanup(func() { returnCursorPost, returnCursorPosition = oldPost, oldPosition })
	shield := &returnCursorShield{}
	shield.window.Store(1)
	ownerMessage := false
	reads := 0
	returnCursorPosition = func() (point, error) {
		if !ownerMessage {
			t.Fatal("peer worker read another desktop's cursor")
		}
		reads++
		return point{X: 0, Y: 400}, nil
	}
	returnCursorPost = func(window uintptr, message uint32, id uint64) error {
		if window != 1 || message != wmReturnCursorSnapshot {
			t.Fatal("wrong native owner request")
		}
		ownerMessage = true
		shield.snapshotPosition(id)
		ownerMessage = false
		return nil
	}
	position, ok := shield.requestPosition(shield.generation.Load())
	if !ok || position != (point{X: 0, Y: 400}) || reads != 1 || shield.snapshotRequest.Load() != nil {
		t.Fatal("owner snapshot missing or request retained")
	}
}

func TestReturnCursorDiscardsTimedOutAndSupersededSnapshots(t *testing.T) {
	oldPost, oldPosition := returnCursorPost, returnCursorPosition
	t.Cleanup(func() { returnCursorPost, returnCursorPosition = oldPost, oldPosition })
	synctest.Test(t, func(t *testing.T) {
		shield := &returnCursorShield{}
		shield.window.Store(1)
		var posted uint64
		returnCursorPost = func(_ uintptr, _ uint32, id uint64) error { posted = id; return nil }
		returnCursorPosition = func() (point, error) { t.Fatal("stale snapshot read desktop"); return point{}, nil }
		if _, ok := shield.requestPosition(0); ok {
			t.Fatal("timeout became success")
		}
		shield.snapshotPosition(posted)
		newer := &returnCursorSnapshotRequest{id: posted + 1, generation: 0, result: make(chan returnCursorSnapshotResult, 1)}
		shield.snapshotRequest.Store(newer)
		shield.snapshotPosition(posted)
		shield.generation.Add(1)
		shield.snapshotPosition(newer.id)
		if result := <-newer.result; result.valid {
			t.Fatal("local input failed to invalidate snapshot")
		}
	})
}

func TestReturnCursorInactiveLoginScopeNeverReadsOrdinaryCursor(t *testing.T) {
	oldPosition := returnCursorPosition
	t.Cleanup(func() { returnCursorPosition = oldPosition })
	returnCursorPosition = func() (point, error) { t.Fatal("inactive login scope read Default desktop"); return point{}, nil }
	shield := &returnCursorShield{loginContext: context.WithValue(context.Background(), sourceLoginDesktopKey{}, &inputdesktop.Scope{})}
	request := &returnCursorSnapshotRequest{id: 1, result: make(chan returnCursorSnapshotResult, 1)}
	shield.snapshotRequest.Store(request)
	shield.snapshotPosition(1)
	if result := <-request.result; result.valid {
		t.Fatal("inactive login scope accepted")
	}
}

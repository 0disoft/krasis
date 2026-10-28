//go:build windows

package wininput

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestReturnCursorDelayedOwnerDoesNotExpireOrBlockReturn(t *testing.T) {
	oldPost, oldPosition, oldShow := returnCursorPost, returnCursorPosition, returnCursorShowWindow
	t.Cleanup(func() { returnCursorPost, returnCursorPosition, returnCursorShowWindow = oldPost, oldPosition, oldShow })
	synctest.Test(t, func(t *testing.T) {
		shield := &returnCursorShield{}
		shield.window.Store(1)
		type queuedMessage struct {
			kind uint32
			id   uint64
		}
		var queued []queuedMessage
		reads, shown := 0, 0
		ownerMessage := false
		returnCursorPosition = func() (point, error) {
			if !ownerMessage {
				t.Fatal("peer worker read another desktop's cursor")
			}
			reads++
			return point{X: 0, Y: 400}, nil
		}
		returnCursorPost = func(window uintptr, message uint32, id uint64) error {
			if window != 1 {
				t.Fatal("wrong native owner")
			}
			queued = append(queued, queuedMessage{message, id})
			return nil
		}
		returnCursorShowWindow = func(uintptr, point) error { shown++; return nil }
		started := time.Now()
		complete := shield.prepare()
		if complete == nil || time.Since(started) != 0 || reads != 0 {
			t.Fatal("return waited for native snapshot")
		}
		complete() // Called only after the revoke write; neither operation waits for the owner.
		time.Sleep(100 * time.Millisecond)
		if len(queued) != 2 || queued[0].kind != wmReturnCursorSnapshot || queued[1].kind != wmReturnCursorShow {
			t.Fatal("native queue order changed")
		}
		ownerMessage = true
		shield.snapshotPosition(queued[0].id)
		shield.showPrepared(queued[1].id)
		if reads != 2 || shown != 1 || !shield.visible || shield.snapshotRequest.Load() != nil {
			t.Fatal("delayed native owner lost return presentation")
		}
	})
}

func TestReturnCursorDiscardsSupersededAndLocallyInvalidatedSnapshots(t *testing.T) {
	oldPost, oldPosition := returnCursorPost, returnCursorPosition
	t.Cleanup(func() { returnCursorPost, returnCursorPosition = oldPost, oldPosition })
	shield := &returnCursorShield{}
	shield.window.Store(1)
	var posted []uint64
	returnCursorPost = func(_ uintptr, _ uint32, id uint64) error { posted = append(posted, id); return nil }
	returnCursorPosition = func() (point, error) { t.Fatal("stale snapshot read desktop"); return point{}, nil }
	old := shield.prepare()
	shield.prepare()
	old()
	if len(posted) != 2 {
		t.Fatal("superseded return posted a show")
	}
	shield.snapshotPosition(posted[0])
	shield.showPrepared(posted[0])
	if shield.snapshotRequest.Load() == nil {
		t.Fatal("stale show discarded newer snapshot")
	}
	shield.beforeInput()
	shield.snapshotPosition(posted[1])
	shield.showPrepared(posted[1])
	if shield.visible || shield.snapshotRequest.Load() != nil {
		t.Fatal("local input failed to invalidate queued show")
	}
}

func TestReturnCursorInactiveLoginScopeNeverReadsOrdinaryCursor(t *testing.T) {
	oldPosition := returnCursorPosition
	t.Cleanup(func() { returnCursorPosition = oldPosition })
	returnCursorPosition = func() (point, error) { t.Fatal("inactive login scope read Default desktop"); return point{}, nil }
	shield := &returnCursorShield{loginContext: context.WithValue(context.Background(), sourceLoginDesktopKey{}, &inputdesktop.Scope{})}
	request := &returnCursorSnapshotRequest{id: 1}
	shield.snapshotRequest.Store(request)
	shield.snapshotPosition(1)
	shield.showPrepared(1)
	if request.valid || shield.visible {
		t.Fatal("inactive login scope accepted")
	}
}

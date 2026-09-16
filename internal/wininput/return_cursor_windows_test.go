//go:build windows

package wininput

import "testing"

func TestReturnCursorCancelsStalePresentationAndRestoresBeforeInput(t *testing.T) {
	oldPosition, oldShow, oldHide, oldPost := returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow, returnCursorPost
	t.Cleanup(func() {
		returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow, returnCursorPost = oldPosition, oldShow, oldHide, oldPost
	})
	position := point{X: 0, Y: 400}
	returnCursorPosition = func() (point, error) { return position, nil }
	shown, hidden := 0, 0
	returnCursorShowWindow = func(uintptr, point) { shown++ }
	returnCursorHideWindow = func(uintptr) { hidden++ }
	var queued []uint64
	returnCursorPost = func(_ uintptr, message uint32, generation uint64) {
		if message == wmReturnCursorShow {
			queued = append(queued, generation)
		}
	}
	shield := &returnCursorShield{}
	shield.window.Store(1)
	complete := shield.prepare()
	shield.beforeInput()
	complete()
	if len(queued) != 0 {
		t.Fatal("late completion ignored local input")
	}
	shield.prepare()()
	shield.beforeInput()
	shield.show(queued[0])
	if shown != 0 {
		t.Fatal("queued presentation ignored local input")
	}
	queued = nil
	shield.prepare()()
	shield.show(queued[0])
	if shown != 1 || !shield.visible {
		t.Fatal("return did not present")
	}
	shield.beforeInput()
	if hidden != 1 || shield.visible {
		t.Fatal("first input did not synchronously restore")
	}
	shield.beforeInput()
	if hidden != 1 {
		t.Fatal("idle input performed redundant native hide")
	}
	queued = nil
	shield.prepare()()
	shield.restore()
	shield.show(queued[0])
	if shown != 1 {
		t.Fatal("new entry failed to invalidate presentation")
	}
	queued = nil
	shield.prepare()()
	position.X++
	shield.show(queued[0])
	if shown != 1 {
		t.Fatal("cursor moved before presentation")
	}
}

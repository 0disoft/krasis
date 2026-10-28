//go:build windows

package wininput

import "testing"

func TestReturnCursorPendingShowSurvivesStationaryMouseReport(t *testing.T) {
	oldPost, oldPosition, oldShow := returnCursorPost, returnCursorPosition, returnCursorShowWindow
	t.Cleanup(func() { returnCursorPost, returnCursorPosition, returnCursorShowWindow = oldPost, oldPosition, oldShow })
	returnCursorPost = func(uintptr, uint32, uint64) error { return nil }
	position := point{X: 0, Y: 400}
	returnCursorPosition = func() (point, error) { return position, nil }
	shows := 0
	returnCursorShowWindow = func(uintptr, point) error { shows++; return nil }
	shield := &returnCursorShield{}
	shield.window.Store(1)
	complete := shield.prepare()
	request := shield.snapshotRequest.Load()
	shield.snapshotPosition(request.id)
	// The last injected/clipped coordinate can arrive between snapshot and
	// show even though the cursor has not actually moved from the boundary.
	if err := shield.beforeMouseInput(wmMouseMove, position); err != nil {
		t.Fatal(err)
	}
	complete()
	shield.showPrepared(request.id)
	if shows != 1 || !shield.visible {
		t.Fatal("stationary report invalidated the pending boundary shield")
	}
}

func TestReturnCursorPendingShowStillLosesToRealInput(t *testing.T) {
	oldPost, oldPosition, oldShow := returnCursorPost, returnCursorPosition, returnCursorShowWindow
	t.Cleanup(func() { returnCursorPost, returnCursorPosition, returnCursorShowWindow = oldPost, oldPosition, oldShow })
	returnCursorPost = func(uintptr, uint32, uint64) error { return nil }
	position := point{X: 0, Y: 400}
	returnCursorPosition = func() (point, error) { return position, nil }
	returnCursorShowWindow = func(uintptr, point) error { t.Error("real input was hidden by a pending show"); return nil }
	for _, input := range []string{"move", "button", "keyboard", "no-snapshot"} {
		shield := &returnCursorShield{}
		shield.window.Store(1)
		complete := shield.prepare()
		request := shield.snapshotRequest.Load()
		if input != "no-snapshot" {
			shield.snapshotPosition(request.id)
		}
		switch input {
		case "move":
			_ = shield.beforeMouseInput(wmMouseMove, point{X: 1, Y: 400})
		case "button":
			_ = shield.beforeMouseInput(0x0201, position)
		case "keyboard":
			_ = shield.beforeInput()
		case "no-snapshot":
			_ = shield.beforeMouseInput(wmMouseMove, position)
		}
		complete()
		shield.showPrepared(request.id)
		if shield.visible {
			t.Error("pending shield survived real or unobserved input", input)
		}
	}
}

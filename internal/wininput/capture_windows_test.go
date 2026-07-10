//go:build windows

package wininput

import "testing"

func TestObserveKeyboardSeparatesPhysicalAndInjected(t *testing.T) {
	var counters captureCounters
	counters.observeKeyboard(wmKeyDown, false)
	counters.observeKeyboard(wmKeyUp, false)
	counters.observeKeyboard(wmSysKeyDown, true)
	counters.observeKeyboard(wmSysKeyUp, true)
	counters.observeKeyboard(0, false)

	snapshot := counters.snapshot(0)
	if snapshot.Keyboard.PhysicalDown != 1 || snapshot.Keyboard.PhysicalUp != 1 {
		t.Fatalf("physical keyboard counts = %+v", snapshot.Keyboard)
	}
	if snapshot.Keyboard.InjectedDown != 1 || snapshot.Keyboard.InjectedUp != 1 {
		t.Fatalf("injected keyboard counts = %+v", snapshot.Keyboard)
	}
	if snapshot.Keyboard.Other != 1 {
		t.Fatalf("keyboard other = %d, want 1", snapshot.Keyboard.Other)
	}
}

func TestObserveMouseSeparatesEventClasses(t *testing.T) {
	var counters captureCounters
	counters.observeMouse(wmMouseMove, false, 0)
	counters.observeMouse(wmMouseMove, true, selfTestExtraInfo)
	counters.observeMouse(wmLButtonDown, false, 0)
	counters.observeMouse(wmLButtonUp, false, 0)
	counters.observeMouse(wmXButtonDown, true, 0)
	counters.observeMouse(wmXButtonUp, true, 0)
	counters.observeMouse(wmMouseWheel, false, 0)
	counters.observeMouse(wmMouseHWheel, true, 0)
	counters.observeMouse(0, false, 0)

	snapshot := counters.snapshot(0)
	if snapshot.Mouse.PhysicalMove != 1 || snapshot.Mouse.InjectedMove != 1 {
		t.Fatalf("mouse move counts = %+v", snapshot.Mouse)
	}
	if snapshot.Mouse.PhysicalButtonDown != 1 || snapshot.Mouse.PhysicalButtonUp != 1 {
		t.Fatalf("physical mouse button counts = %+v", snapshot.Mouse)
	}
	if snapshot.Mouse.InjectedButtonDown != 1 || snapshot.Mouse.InjectedButtonUp != 1 {
		t.Fatalf("injected mouse button counts = %+v", snapshot.Mouse)
	}
	if snapshot.Mouse.PhysicalWheel != 1 || snapshot.Mouse.InjectedWheel != 1 {
		t.Fatalf("mouse wheel counts = %+v", snapshot.Mouse)
	}
	if snapshot.Mouse.Other != 1 {
		t.Fatalf("mouse other = %d, want 1", snapshot.Mouse.Other)
	}
	if snapshot.Mouse.SelfTestInjectedMove != 1 {
		t.Fatalf("self-test injected move = %d, want 1", snapshot.Mouse.SelfTestInjectedMove)
	}
}

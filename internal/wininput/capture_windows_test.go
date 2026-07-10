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
	counters.observeMouse(wmMouseMove, false)
	counters.observeMouse(wmMouseMove, true)
	counters.observeMouse(wmLButtonDown, false)
	counters.observeMouse(wmLButtonUp, false)
	counters.observeMouse(wmXButtonDown, true)
	counters.observeMouse(wmXButtonUp, true)
	counters.observeMouse(wmMouseWheel, false)
	counters.observeMouse(wmMouseHWheel, true)
	counters.observeMouse(0, false)

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
}

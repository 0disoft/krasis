//go:build windows

package wininput

import (
	"errors"
	"testing"
)

func TestReturnCursorCancelsStalePresentationAndRestoresBeforeInput(t *testing.T) {
	oldPosition, oldShow, oldHide, oldPost := returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow, returnCursorPost
	t.Cleanup(func() {
		returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow, returnCursorPost = oldPosition, oldShow, oldHide, oldPost
	})
	position := point{X: 0, Y: 400}
	returnCursorPosition = func() (point, error) { return position, nil }
	shown, hidden := 0, 0
	returnCursorShowWindow = func(uintptr, point) error { shown++; return nil }
	returnCursorHideWindow = func(uintptr) error { hidden++; return nil }
	var queued []uint64
	returnCursorPost = func(_ uintptr, message uint32, generation uint64) error {
		if message == wmReturnCursorShow {
			queued = append(queued, generation)
		}
		return nil
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

func TestReturnCursorRetainsOwnerWhenNativeDestructionFails(t *testing.T) {
	oldDestroy := returnCursorDestroy
	t.Cleanup(func() { returnCursorDestroy = oldDestroy; returnCursorWindows.Delete(uintptr(123)) })
	shield := &returnCursorShield{}
	shield.window.Store(123)
	returnCursorWindows.Store(uintptr(123), shield)
	failure := errors.New("native destruction failed")
	returnCursorDestroy = func(uintptr) error { return failure }
	if err := shield.close(); !errors.Is(err, failure) || SourceFailureStage(err) != "cursor-release" {
		t.Fatalf("cleanup failure lost: %v", err)
	}
	if shield.window.Load() != 123 {
		t.Fatal("failed destruction discarded owner")
	}
	if _, ok := returnCursorWindows.Load(uintptr(123)); !ok {
		t.Fatal("failed destruction discarded callback owner")
	}
	returnCursorDestroy = func(uintptr) error { return nil }
	if err := shield.close(); !errors.Is(err, failure) {
		t.Fatal("late retry erased unverified recovery")
	}
	if shield.window.Load() != 0 {
		t.Fatal("successful destruction retained window")
	}
	if _, ok := returnCursorWindows.Load(uintptr(123)); ok {
		t.Fatal("successful destruction retained callback")
	}
}

func TestReturnCursorRestoreAndMessageFailuresAreNotReportedAsSuccess(t *testing.T) {
	oldHide, oldPost, oldDestroy := returnCursorHideWindow, returnCursorPost, returnCursorDestroy
	t.Cleanup(func() { returnCursorHideWindow, returnCursorPost, returnCursorDestroy = oldHide, oldPost, oldDestroy })
	failure := errors.New("native restore failed")
	returnCursorHideWindow = func(uintptr) error { return failure }
	returnCursorDestroy = func(uintptr) error { return nil }
	shield := &returnCursorShield{visible: true}
	shield.window.Store(456)
	if err := shield.beforeInput(); !errors.Is(err, failure) || !shield.visible || SourceFailureStage(err) != "cursor-restore" {
		t.Fatalf("hide failure lost: %v", err)
	}
	if err := shield.close(); !errors.Is(err, failure) || shield.window.Load() != 0 {
		t.Fatal("cleanup did not preserve restore failure")
	}
	returnCursorPost = func(uintptr, uint32, uint64) error { return failure }
	shield = &returnCursorShield{}
	shield.window.Store(456)
	shield.restore()
	if err := shield.close(); !errors.Is(err, failure) || SourceFailureStage(err) != "cursor-restore" {
		t.Fatalf("post failure lost: %v", err)
	}
}

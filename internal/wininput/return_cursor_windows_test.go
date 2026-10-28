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
	shield := &returnCursorShield{}
	shield.window.Store(1)
	returnCursorPost = func(_ uintptr, message uint32, generation uint64) error {
		if message == wmReturnCursorSnapshot {
			shield.snapshotPosition(generation)
		}
		if message == wmReturnCursorShow {
			queued = append(queued, generation)
		}
		return nil
	}
	complete := shield.prepare()
	shield.beforeInput()
	complete()
	if len(queued) != 0 {
		t.Fatal("late completion ignored local input")
	}
	shield.prepare()()
	shield.beforeInput()
	shield.showPrepared(queued[0])
	if shown != 0 {
		t.Fatal("queued presentation ignored local input")
	}
	queued = nil
	shield.prepare()()
	shield.showPrepared(queued[0])
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
	shield.showPrepared(queued[0])
	if shown != 1 {
		t.Fatal("new entry failed to invalidate presentation")
	}
	queued = nil
	shield.prepare()()
	position.X++
	shield.showPrepared(queued[0])
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

func TestReturnCursorNativeReentryCannotReshowAfterLocalInput(t *testing.T) {
	for _, stage := range []string{"position", "show"} {
		t.Run(stage, func(t *testing.T) {
			oldPosition, oldShow, oldHide := returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow
			t.Cleanup(func() {
				returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow = oldPosition, oldShow, oldHide
			})
			position := point{X: 10, Y: 20}
			shield := &returnCursorShield{position: position}
			shield.window.Store(123)
			nativeVisible, shows := false, 0
			input := func() {
				if err := shield.beforeInput(); err != nil {
					t.Fatal(err)
				}
			}
			returnCursorPosition = func() (point, error) {
				if stage == "position" {
					input()
				}
				return position, nil
			}
			returnCursorHideWindow = func(uintptr) error { nativeVisible = false; return nil }
			returnCursorShowWindow = func(uintptr, point) error {
				shows++
				if stage == "show" {
					input() // Native window calls may dispatch the input hook synchronously.
				}
				nativeVisible = true // The outer native operation completes after that hook.
				return nil
			}
			shield.show(shield.generation.Load())
			if nativeVisible || shield.visible || stage == "position" && shows != 0 {
				t.Fatalf("stale presentation survived %s reentry: native=%t ledger=%t shows=%d", stage, nativeVisible, shield.visible, shows)
			}
		})
	}
}

func TestReturnCursorHideReentryDoesNotRepeatNativeHide(t *testing.T) {
	oldHide := returnCursorHideWindow
	t.Cleanup(func() { returnCursorHideWindow = oldHide })
	shield := &returnCursorShield{visible: true}
	shield.window.Store(123)
	hides := 0
	returnCursorHideWindow = func(uintptr) error {
		hides++
		if hides == 1 {
			if err := shield.beforeInput(); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}
	if err := shield.beforeInput(); err != nil || hides != 1 || shield.visible {
		t.Fatalf("hide reentry repeated native work: calls=%d visible=%t err=%v", hides, shield.visible, err)
	}
}

func TestReturnCursorLateShowCleanupFailureRemainsVisibleAndFaulted(t *testing.T) {
	oldPosition, oldShow, oldHide := returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow
	t.Cleanup(func() {
		returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow = oldPosition, oldShow, oldHide
	})
	shield := &returnCursorShield{}
	shield.window.Store(123)
	failure, hides := errors.New("late native hide failed"), 0
	returnCursorPosition = func() (point, error) { return point{}, nil }
	returnCursorShowWindow = func(uintptr, point) error { return shield.beforeInput() }
	returnCursorHideWindow = func(uintptr) error {
		hides++
		if hides > 1 {
			return failure
		}
		return nil
	}
	shield.show(shield.generation.Load())
	if hides != 2 || !shield.visible || !errors.Is(shield.failureResult(), failure) || SourceFailureStage(shield.failureResult()) != "cursor-restore" {
		t.Fatalf("late hide failure lost: calls=%d visible=%t err=%v", hides, shield.visible, shield.failureResult())
	}
}

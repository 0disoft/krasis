//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
)

func TestProtectedReturnCursorDoesNotHideLocalMovementAndRestoresOnCancel(t *testing.T) {
	oldPosition, oldShow, oldHide, oldDestroy, oldPressed := returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow, returnCursorDestroy, sourceAnyInputPressed
	t.Cleanup(func() {
		returnCursorPosition, returnCursorShowWindow, returnCursorHideWindow, returnCursorDestroy, sourceAnyInputPressed = oldPosition, oldShow, oldHide, oldDestroy, oldPressed
	})
	position := point{X: 0, Y: 300}
	returnCursorPosition = func() (point, error) { return position, nil }
	sourceAnyInputPressed = func() bool { return false }
	shown, hidden := 0, 0
	returnCursorShowWindow = func(uintptr, point) error { shown++; return nil }
	returnCursorHideWindow = func(uintptr) error { hidden++; return nil }
	returnCursorDestroy = func(uintptr) error { return nil }
	for _, moved := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		shield := &returnCursorShield{position: position}
		shield.window.Store(1)
		if moved {
			shield.position.X++
		}
		ready := 0
		err := runProtectedReturnCursorOwner(ctx, func() error { return nil }, func() error { return nil }, shield, func() error { ready++; cancel(); return nil }, func() bool { return false })
		if err != nil || ready != 1 {
			t.Fatal("presentation did not retire cleanly", err, ready)
		}
		if err := shield.close(); err != nil {
			t.Fatal(err)
		}
	}
	if shown != 1 || hidden != 1 {
		t.Fatal("local cursor hidden or cancelled shield retained", shown, hidden)
	}
}

func TestProtectedReturnCursorChecksAuthorityBeforePresentation(t *testing.T) {
	failure := errors.New("revoked")
	shield := &returnCursorShield{}
	if err := runProtectedReturnCursorOwner(context.Background(), func() error { return failure }, func() error { t.Fatal("desktop accessed after revoke"); return nil }, shield, func() error { t.Fatal("false readiness"); return nil }, func() bool { return false }); err != failure {
		t.Fatal(err)
	}
}

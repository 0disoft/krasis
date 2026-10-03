//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func TestInjectionPortMoveDoesNotRepeatResidentCursorTracking(t *testing.T) {
	previousSend, previousCursor := callSendInput, callGetCursorPos
	t.Cleanup(func() { callSendInput, callGetCursorPos = previousSend, previousCursor })
	callGetCursorPos = func(*point) (uintptr, uintptr, error) {
		t.Fatal("worker queried cursor owned by resident return tracking")
		return 0, 0, errors.New("unavailable")
	}
	calls := 0
	callSendInput = func(count uintptr, event *input, size uintptr) (uintptr, uintptr, error) {
		calls++
		if count != 1 || event.Type != inputMouse || event.Mouse.Flags != mouseEventMove|mouseEventMoveNoCoalesce || event.Mouse.DX != 1 || event.Mouse.DY != -1 {
			t.Fatal("native move encoding changed")
		}
		return count, 0, nil
	}
	port := NewInjectionPort()
	if err := port.Move(context.Background(), 1, -1); err != nil || calls != 1 {
		t.Fatal("successful injection changed outcome or was replayed", err, calls)
	}
}

func TestInjectionPortRetainsValidationCancellationAndPartialOutcome(t *testing.T) {
	previous := callSendInput
	t.Cleanup(func() { callSendInput = previous })
	calls := 0
	callSendInput = func(count uintptr, event *input, size uintptr) (uintptr, uintptr, error) {
		calls++
		if count != 2 {
			t.Fatal("partial wheel request changed")
		}
		return 1, 0, errors.New("partial")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	port := NewInjectionPort()
	err := port.Move(ctx, 1, 1)
	certainty, known := application.InjectionCertaintyOf(err)
	if !errors.Is(err, context.Canceled) || !known || certainty != application.InjectionNotApplied || calls != 0 {
		t.Fatal("cancelled input reached native API", err)
	}
	for _, err := range []error{port.Move(context.Background(), 0, 0), port.Wheel(context.Background(), 0, 0), port.Press(context.Background(), control.PressedToken{})} {
		if err == nil || calls != 0 {
			t.Fatal("invalid input reached native API")
		}
	}
	err = port.Wheel(context.Background(), 120, 120)
	certainty, known = application.InjectionCertaintyOf(err)
	if !known || certainty != application.InjectionPartiallyApplied || calls != 1 {
		t.Fatal("partial outcome hidden or replayed", err, calls)
	}
}

func TestInjectionPortRecoveryRemainsOwnedByWorkerPressedState(t *testing.T) {
	previous := callSendInput
	t.Cleanup(func() { callSendInput = previous })
	var flags []uint32
	callSendInput = func(count uintptr, event *input, size uintptr) (uintptr, uintptr, error) {
		flags = append(flags, event.Mouse.Flags)
		return count, 0, nil
	}
	port := NewInjectionPort()
	var pressed control.PressedState
	ctx := context.Background()
	if err := application.InjectPress(ctx, &pressed, port, control.PressedToken{Kind: control.PressedMouseButton, Code: MouseButtonLeft}); err != nil {
		t.Fatal(err)
	}
	report, err := application.ReleaseAll(ctx, &pressed, port)
	if err != nil || report.Remaining != 0 || pressed.Count() != 0 || len(flags) != 2 || flags[0] != mouseEventLeftDown || flags[1] != mouseEventLeftUp {
		t.Fatal("native port lost recovery or inserted an extra event", report, err)
	}
}

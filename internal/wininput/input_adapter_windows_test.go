//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"unsafe"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

var _ application.InputPort = (*InputAdapter)(nil)

func TestKeyboardEventUsesScanCodeAndExtendedFlags(t *testing.T) {
	event := keyboardEvent(0x1D, true, true)
	keyboard := *(*keyboardInput)(unsafe.Pointer(&event.Mouse))
	if event.Type != inputKeyboard || keyboard.VirtualKey != 0 || keyboard.ScanCode != 0x1D {
		t.Fatalf("keyboard event = type:%d data:%+v", event.Type, keyboard)
	}
	wantFlags := uint32(keyEventScanCode | keyEventExtended | keyEventKeyUp)
	if keyboard.Flags != wantFlags || keyboard.ExtraInfo != krasisInjectedExtraInfo {
		t.Fatalf("keyboard flags=%x extra=%x", keyboard.Flags, keyboard.ExtraInfo)
	}
}

func TestMouseButtonEventMapsClosedButtonSet(t *testing.T) {
	tests := []struct {
		code     uint32
		released bool
		flags    uint32
		data     uint32
	}{
		{MouseButtonLeft, false, mouseEventLeftDown, 0},
		{MouseButtonLeft, true, mouseEventLeftUp, 0},
		{MouseButtonRight, false, mouseEventRightDown, 0},
		{MouseButtonMiddle, true, mouseEventMiddleUp, 0},
		{MouseButtonX1, false, mouseEventXDown, xButton1},
		{MouseButtonX2, true, mouseEventXUp, xButton2},
	}
	for _, test := range tests {
		flags, data, err := mouseButtonEvent(test.code, test.released)
		if err != nil {
			t.Fatalf("mouseButtonEvent(%d) error = %v", test.code, err)
		}
		if flags != test.flags || data != test.data {
			t.Fatalf("mouseButtonEvent(%d) = (%x,%d), want (%x,%d)", test.code, flags, data, test.flags, test.data)
		}
	}
}

func TestWheelEventsKeepVerticalThenHorizontalOrder(t *testing.T) {
	events, err := wheelEvents(-120, 120)
	if err != nil {
		t.Fatalf("wheelEvents() error = %v", err)
	}
	if len(events) != 2 || events[0].Mouse.Flags != mouseEventWheel || events[0].Mouse.MouseData != uint32(120) {
		t.Fatalf("vertical wheel event = %+v", events)
	}
	if events[1].Mouse.Flags != mouseEventHWheel || int32(events[1].Mouse.MouseData) != -120 {
		t.Fatalf("horizontal wheel event = %+v", events)
	}
}

func TestSendInputsRejectsCanceledContextWithoutCallingWindows(t *testing.T) {
	original := callSendInput
	called := false
	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
		called = true
		return 1, 0, nil
	}
	t.Cleanup(func() { callSendInput = original })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sendInputs(ctx, []input{mouseEvent(mouseEventMove, 0, 1, 0)})
	certainty, classified := application.InjectionCertaintyOf(err)
	if !errors.Is(err, context.Canceled) || !classified || certainty != application.InjectionNotApplied || called {
		t.Fatalf("sendInputs() error=%v called=%t", err, called)
	}
}

func TestSendInputsRejectsPartialInjection(t *testing.T) {
	original := callSendInput
	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
		return 1, 0, errors.New("partial")
	}
	t.Cleanup(func() { callSendInput = original })
	err := sendInputs(context.Background(), []input{
		mouseEvent(mouseEventWheel, 120, 0, 0),
		mouseEvent(mouseEventHWheel, 120, 0, 0),
	})
	certainty, classified := application.InjectionCertaintyOf(err)
	if !errors.Is(err, ErrInputInjection) || !classified || certainty != application.InjectionPartiallyApplied {
		t.Fatalf("sendInputs(partial) error = %v", err)
	}
}

func TestSendInputsClassifiesZeroAndImpossibleCounts(t *testing.T) {
	original := callSendInput
	t.Cleanup(func() { callSendInput = original })
	events := []input{mouseEvent(mouseEventMove, 0, 1, 0)}

	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
		return 0, 0, errors.New("blocked")
	}
	err := sendInputs(context.Background(), events)
	certainty, classified := application.InjectionCertaintyOf(err)
	if !classified || certainty != application.InjectionNotApplied {
		t.Fatalf("zero insertion certainty = %q, classified=%t, error=%v", certainty, classified, err)
	}

	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
		return 2, 0, errors.New("invalid count")
	}
	err = sendInputs(context.Background(), events)
	certainty, classified = application.InjectionCertaintyOf(err)
	if !classified || certainty != application.InjectionOutcomeUnknown {
		t.Fatalf("impossible insertion certainty = %q, classified=%t, error=%v", certainty, classified, err)
	}
}

func TestInputAdapterRejectsInvalidTokensBeforeCallingWindows(t *testing.T) {
	original := callSendInput
	called := false
	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
		called = true
		return 1, 0, nil
	}
	t.Cleanup(func() { callSendInput = original })
	adapter := NewInputAdapter()
	err := adapter.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 1 << 31})
	if !errors.Is(err, ErrUnsupportedInput) || called {
		t.Fatalf("Press(invalid) error=%v called=%t", err, called)
	}
}

func TestInputAdapterEntryCallsSetCursorPosInsideOppositeEdge(t *testing.T) {
	originalGet := callGetCursorPos
	originalSet := callSetCursorPos
	var gotX, gotY uintptr
	getCalls := 0
	callSetCursorPos = func(args ...uintptr) (uintptr, uintptr, error) {
		gotX, gotY = args[0], args[1]
		return 1, 0, nil
	}
	wantX, wantY, err := cursorEntryPoint(
		control.ScreenEdgeRight,
		systemMetric(smXVIRTUALSCREEN),
		systemMetric(smYVIRTUALSCREEN),
		systemMetric(smCXVIRTUALSCREEN),
		systemMetric(smCYVIRTUALSCREEN),
	)
	if err != nil {
		t.Fatalf("cursorEntryPoint() error = %v", err)
	}
	callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
		if getCalls == 0 {
			position.X, position.Y = 100, 200
		} else {
			position.X, position.Y = wantX, wantY
		}
		getCalls++
		return 1, 0, nil
	}
	t.Cleanup(func() {
		callGetCursorPos = originalGet
		callSetCursorPos = originalSet
	})
	adapter := NewInputAdapter()
	if err := adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight); err != nil {
		t.Fatalf("PrepareEntry() error = %v", err)
	}
	adapter.CommitEntry()
	if gotX != uintptr(uint32(wantX)) || gotY != uintptr(uint32(wantY)) {
		t.Fatalf("SetCursorPos = (%d,%d), want (%d,%d)", gotX, gotY, uint32(wantX), uint32(wantY))
	}
}

func TestInputAdapterEntryHonorsCancellationBeforeWindowsCall(t *testing.T) {
	original := callSetCursorPos
	called := false
	callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) {
		called = true
		return 1, 0, nil
	}
	t.Cleanup(func() { callSetCursorPos = original })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewInputAdapter().PrepareEntry(ctx, control.ScreenEdgeLeft); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("PrepareEntry(canceled) error=%v called=%t", err, called)
	}
}

func TestInputAdapterEntryRejectsClippedCursor(t *testing.T) {
	originalGet := callGetCursorPos
	originalSet := callSetCursorPos
	callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) { return 1, 0, nil }
	getCalls := 0
	callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
		if getCalls == 0 {
			position.X, position.Y = 100, 200
		} else {
			position.X, position.Y = 0, 0
		}
		getCalls++
		return 1, 0, nil
	}
	t.Cleanup(func() {
		callGetCursorPos = originalGet
		callSetCursorPos = originalSet
	})
	adapter := NewInputAdapter()
	if err := adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight); !errors.Is(err, ErrInputInjection) {
		t.Fatalf("PrepareEntry(clipped) error = %v", err)
	}
}

func TestInputAdapterRollbackRestoresOnlyOwnedEntryPosition(t *testing.T) {
	for _, test := range []struct {
		name         string
		currentX     int32
		currentY     int32
		wantSetCalls int
		wantOutcome  application.CursorRollbackOutcome
	}{
		{name: "unchanged entry", wantSetCalls: 2, wantOutcome: application.CursorRollbackRestored},
		{name: "cursor moved after entry", currentX: 777, currentY: 888, wantSetCalls: 1, wantOutcome: application.CursorRollbackNotRestored},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalGet := callGetCursorPos
			originalSet := callSetCursorPos
			before := point{X: 100, Y: 200}
			entryX, entryY, err := cursorEntryPoint(
				control.ScreenEdgeRight,
				systemMetric(smXVIRTUALSCREEN),
				systemMetric(smYVIRTUALSCREEN),
				systemMetric(smCXVIRTUALSCREEN),
				systemMetric(smCYVIRTUALSCREEN),
			)
			if err != nil {
				t.Fatal(err)
			}
			current := point{X: test.currentX, Y: test.currentY}
			if test.wantSetCalls == 2 {
				current = point{X: entryX, Y: entryY}
			}
			getCalls := 0
			callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
				switch getCalls {
				case 0:
					*position = before
				case 1:
					position.X, position.Y = entryX, entryY
				case 2:
					*position = current
				default:
					*position = before
				}
				getCalls++
				return 1, 0, nil
			}
			setCalls := 0
			callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) {
				setCalls++
				return 1, 0, nil
			}
			t.Cleanup(func() {
				callGetCursorPos = originalGet
				callSetCursorPos = originalSet
			})
			adapter := NewInputAdapter()
			if err := adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight); err != nil {
				t.Fatalf("PrepareEntry() error = %v", err)
			}
			outcome, err := adapter.RollbackEntry(context.Background())
			if err != nil {
				t.Fatalf("RollbackEntry() error = %v", err)
			}
			if outcome != test.wantOutcome {
				t.Fatalf("RollbackEntry() outcome = %q, want %q", outcome, test.wantOutcome)
			}
			if setCalls != test.wantSetCalls {
				t.Fatalf("SetCursorPos calls = %d, want %d", setCalls, test.wantSetCalls)
			}
		})
	}
}

func TestInputAdapterCommitPreventsRollback(t *testing.T) {
	originalGet := callGetCursorPos
	originalSet := callSetCursorPos
	getCalls := 0
	callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
		if getCalls == 0 {
			position.X, position.Y = 100, 200
		} else {
			position.X, position.Y, _ = cursorEntryPoint(
				control.ScreenEdgeRight,
				systemMetric(smXVIRTUALSCREEN),
				systemMetric(smYVIRTUALSCREEN),
				systemMetric(smCXVIRTUALSCREEN),
				systemMetric(smCYVIRTUALSCREEN),
			)
		}
		getCalls++
		return 1, 0, nil
	}
	setCalls := 0
	callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) {
		setCalls++
		return 1, 0, nil
	}
	t.Cleanup(func() {
		callGetCursorPos = originalGet
		callSetCursorPos = originalSet
	})
	adapter := NewInputAdapter()
	if err := adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight); err != nil {
		t.Fatal(err)
	}
	adapter.CommitEntry()
	if outcome, err := adapter.RollbackEntry(context.Background()); err != nil || outcome != application.CursorRollbackNotRestored {
		t.Fatal(err)
	}
	if setCalls != 1 {
		t.Fatalf("SetCursorPos calls = %d, want 1", setCalls)
	}
}

func TestInputAdapterRollbackRefusesUnknownEntryPosition(t *testing.T) {
	originalGet := callGetCursorPos
	originalSet := callSetCursorPos
	getCalls := 0
	callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
		if getCalls == 0 {
			position.X, position.Y = 100, 200
			getCalls++
			return 1, 0, nil
		}
		return 0, 0, errors.New("position unavailable")
	}
	setCalls := 0
	callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) {
		setCalls++
		return 1, 0, nil
	}
	t.Cleanup(func() {
		callGetCursorPos = originalGet
		callSetCursorPos = originalSet
	})
	adapter := NewInputAdapter()
	if err := adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight); !errors.Is(err, ErrInputInjection) {
		t.Fatalf("PrepareEntry() error = %v", err)
	}
	if _, err := adapter.RollbackEntry(context.Background()); !errors.Is(err, ErrCursorEntryState) {
		t.Fatalf("RollbackEntry() error = %v", err)
	}
	if setCalls != 1 {
		t.Fatalf("unknown entry rollback called SetCursorPos %d times", setCalls)
	}
}

func TestInputAdapterMoveEmitsArmedReturnIntent(t *testing.T) {
	originalGet := callGetCursorPos
	originalSend := callSendInput
	callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) { return 1, 0, nil }
	left := systemMetric(smXVIRTUALSCREEN)
	positions := []int32{left + defaultReturnRearmDistance, left}
	positionIndex := 0
	callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
		position.X = positions[positionIndex]
		positionIndex++
		return 1, 0, nil
	}
	t.Cleanup(func() {
		callGetCursorPos = originalGet
		callSendInput = originalSend
	})
	adapter := NewInputAdapter()
	intent := control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
	if err := adapter.ArmReturn(intent, control.ScreenEdgeRight); err != nil {
		t.Fatalf("ArmReturn() error = %v", err)
	}
	if err := adapter.Move(context.Background(), 1, 0); err != nil {
		t.Fatalf("Move(inward) error = %v", err)
	}
	if err := adapter.Move(context.Background(), -1, 0); err != nil {
		t.Fatalf("Move(return) error = %v", err)
	}
	select {
	case got := <-adapter.ReturnIntents():
		if got != intent {
			t.Fatalf("return intent = %+v", got)
		}
	default:
		t.Fatal("Move() did not emit return intent")
	}
}

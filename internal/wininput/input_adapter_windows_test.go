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
	callSendInput = func(uintptr, uintptr, uintptr) (uintptr, uintptr, error) {
		called = true
		return 1, 0, nil
	}
	t.Cleanup(func() { callSendInput = original })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sendInputs(ctx, []input{mouseEvent(mouseEventMove, 0, 1, 0)})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("sendInputs() error=%v called=%t", err, called)
	}
}

func TestSendInputsRejectsPartialInjection(t *testing.T) {
	original := callSendInput
	callSendInput = func(uintptr, uintptr, uintptr) (uintptr, uintptr, error) {
		return 1, 0, errors.New("partial")
	}
	t.Cleanup(func() { callSendInput = original })
	err := sendInputs(context.Background(), []input{
		mouseEvent(mouseEventWheel, 120, 0, 0),
		mouseEvent(mouseEventHWheel, 120, 0, 0),
	})
	if !errors.Is(err, ErrInputInjection) {
		t.Fatalf("sendInputs(partial) error = %v", err)
	}
}

func TestInputAdapterRejectsInvalidTokensBeforeCallingWindows(t *testing.T) {
	original := callSendInput
	called := false
	callSendInput = func(uintptr, uintptr, uintptr) (uintptr, uintptr, error) {
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

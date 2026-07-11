//go:build windows

package wininput

import (
	"context"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

var (
	callGetCursorPos = procGetCursorPos.Call
	callSetCursorPos = procSetCursorPos.Call
)

func NewInputAdapter() *InputAdapter {
	return newInputAdapter()
}

func (adapter *InputAdapter) Enter(ctx context.Context, sourceEdge control.ScreenEdge) error {
	if adapter == nil {
		return fmt.Errorf("%w: input adapter is not initialized", ErrInputInjection)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	x, y, err := cursorEntryPoint(
		sourceEdge,
		systemMetric(smXVIRTUALSCREEN),
		systemMetric(smYVIRTUALSCREEN),
		systemMetric(smCXVIRTUALSCREEN),
		systemMetric(smCYVIRTUALSCREEN),
	)
	if err != nil {
		return err
	}
	result, _, callErr := callSetCursorPos(uintptr(uint32(x)), uintptr(uint32(y)))
	if result == 0 {
		return fmt.Errorf("%w: SetCursorPos failed: %v", ErrInputInjection, callErr)
	}
	position, err := cursorPosition()
	if err != nil {
		return fmt.Errorf("%w: verify cursor entry: %v", ErrInputInjection, err)
	}
	if position.X != x || position.Y != y {
		return fmt.Errorf("%w: cursor entry was clipped to (%d,%d)", ErrInputInjection, position.X, position.Y)
	}
	return nil
}

func (adapter *InputAdapter) Press(ctx context.Context, token control.PressedToken) error {
	if err := adapter.writeDiscrete(ctx, token, false); err != nil {
		return err
	}
	return adapter.recordReturnInput(token, true)
}

func (adapter *InputAdapter) Release(ctx context.Context, token control.PressedToken) error {
	if err := adapter.writeDiscrete(ctx, token, true); err != nil {
		return err
	}
	return adapter.recordReturnInput(token, false)
}

func (adapter *InputAdapter) Move(ctx context.Context, deltaX, deltaY int32) error {
	if adapter == nil {
		return fmt.Errorf("%w: input adapter is not initialized", ErrInputInjection)
	}
	event, err := pointerMoveEvent(deltaX, deltaY)
	if err != nil {
		return err
	}
	if err := sendInputs(ctx, []input{event}); err != nil {
		return err
	}
	position, err := cursorPosition()
	if err != nil {
		return err
	}
	return adapter.observeReturnPosition(position.X, systemMetric(smXVIRTUALSCREEN), systemMetric(smCXVIRTUALSCREEN))
}

func (adapter *InputAdapter) Wheel(ctx context.Context, deltaX, deltaY int32) error {
	if adapter == nil {
		return fmt.Errorf("%w: input adapter is not initialized", ErrInputInjection)
	}
	events, err := wheelEvents(deltaX, deltaY)
	if err != nil {
		return err
	}
	return sendInputs(ctx, events)
}

func pointerMoveEvent(deltaX, deltaY int32) (input, error) {
	if deltaX == 0 && deltaY == 0 {
		return input{}, fmt.Errorf("%w: pointer move requires a non-zero delta", ErrUnsupportedInput)
	}
	return mouseEvent(mouseEventMove|mouseEventMoveNoCoalesce, 0, deltaX, deltaY), nil
}

func wheelEvents(deltaX, deltaY int32) ([]input, error) {
	if deltaX == 0 && deltaY == 0 {
		return nil, fmt.Errorf("%w: wheel requires a non-zero delta", ErrUnsupportedInput)
	}
	events := make([]input, 0, 2)
	if deltaY != 0 {
		events = append(events, mouseEvent(mouseEventWheel, uint32(deltaY), 0, 0))
	}
	if deltaX != 0 {
		events = append(events, mouseEvent(mouseEventHWheel, uint32(deltaX), 0, 0))
	}
	return events, nil
}

func (adapter *InputAdapter) writeDiscrete(ctx context.Context, token control.PressedToken, released bool) error {
	if adapter == nil {
		return fmt.Errorf("%w: input adapter is not initialized", ErrInputInjection)
	}
	if err := token.Validate(); err != nil {
		return errorsJoinUnsupported(err)
	}
	switch token.Kind {
	case control.PressedKeyboardKey:
		scanCode, extended, err := decodeScanCode(token.Code)
		if err != nil {
			return err
		}
		return sendInputs(ctx, []input{keyboardEvent(scanCode, extended, released)})
	case control.PressedMouseButton:
		flags, data, err := mouseButtonEvent(token.Code, released)
		if err != nil {
			return err
		}
		return sendInputs(ctx, []input{mouseEvent(flags, data, 0, 0)})
	default:
		return fmt.Errorf("%w: pressed token kind is not supported", ErrUnsupportedInput)
	}
}

func mouseButtonEvent(code uint32, released bool) (uint32, uint32, error) {
	if err := validateMouseButton(code); err != nil {
		return 0, 0, err
	}
	switch code {
	case MouseButtonLeft:
		if released {
			return mouseEventLeftUp, 0, nil
		}
		return mouseEventLeftDown, 0, nil
	case MouseButtonRight:
		if released {
			return mouseEventRightUp, 0, nil
		}
		return mouseEventRightDown, 0, nil
	case MouseButtonMiddle:
		if released {
			return mouseEventMiddleUp, 0, nil
		}
		return mouseEventMiddleDown, 0, nil
	case MouseButtonX1:
		if released {
			return mouseEventXUp, xButton1, nil
		}
		return mouseEventXDown, xButton1, nil
	case MouseButtonX2:
		if released {
			return mouseEventXUp, xButton2, nil
		}
		return mouseEventXDown, xButton2, nil
	default:
		return 0, 0, fmt.Errorf("%w: mouse button code is not supported", ErrUnsupportedInput)
	}
}

func errorsJoinUnsupported(err error) error {
	return fmt.Errorf("%w: %v", ErrUnsupportedInput, err)
}

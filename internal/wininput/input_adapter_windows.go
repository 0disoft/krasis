//go:build windows

package wininput

import (
	"context"
	"errors"
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

func (adapter *InputAdapter) PrepareEntry(ctx context.Context, sourceEdge control.ScreenEdge) error {
	if adapter == nil {
		return fmt.Errorf("%w: input adapter is not initialized", ErrInputInjection)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInputInjection)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, err := adapter.cursorEntryState()
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.pending {
		return fmt.Errorf("%w: another cursor entry is pending", ErrCursorEntryState)
	}
	before, err := cursorPosition()
	if err != nil {
		return fmt.Errorf("%w: snapshot cursor before entry: %v", ErrInputInjection, err)
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
	state.pending = true
	state.previousX = before.X
	state.previousY = before.Y
	position, err := cursorPosition()
	if err != nil {
		return fmt.Errorf("%w: verify cursor entry: %v", ErrInputInjection, err)
	}
	state.enteredKnown = true
	state.enteredX = position.X
	state.enteredY = position.Y
	if position.X != x || position.Y != y {
		return fmt.Errorf("%w: cursor entry was clipped to (%d,%d)", ErrInputInjection, position.X, position.Y)
	}
	return nil
}

func (adapter *InputAdapter) RollbackEntry(ctx context.Context) error {
	state, err := adapter.cursorEntryState()
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.pending {
		return nil
	}
	defer state.clear()
	if ctx == nil {
		return fmt.Errorf("%w: rollback context is required", ErrCursorEntryState)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrCursorEntryState, err)
	}
	if !state.enteredKnown {
		return fmt.Errorf("%w: entry position could not be verified", ErrCursorEntryState)
	}
	current, err := cursorPosition()
	if err != nil {
		return fmt.Errorf("%w: inspect cursor before rollback: %v", ErrCursorEntryState, err)
	}
	if current.X != state.enteredX || current.Y != state.enteredY {
		return nil
	}
	result, _, callErr := callSetCursorPos(
		uintptr(uint32(state.previousX)),
		uintptr(uint32(state.previousY)),
	)
	if result == 0 {
		return fmt.Errorf("%w: SetCursorPos rollback failed: %v", ErrCursorEntryState, callErr)
	}
	restored, err := cursorPosition()
	if err != nil {
		return fmt.Errorf("%w: verify cursor rollback: %v", ErrCursorEntryState, err)
	}
	if restored.X != state.previousX || restored.Y != state.previousY {
		return fmt.Errorf("%w: cursor rollback was clipped to (%d,%d)", ErrCursorEntryState, restored.X, restored.Y)
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

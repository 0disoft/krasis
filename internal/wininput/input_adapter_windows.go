//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

var (
	// Keep the Go/test boundary typed. Convert only at the real syscall call,
	// where the compiler can preserve the pointer for the duration of the call.
	callGetCursorPos = func(position *point) (uintptr, uintptr, error) {
		return procGetCursorPos.Call(uintptr(unsafe.Pointer(position)))
	}
	callSetCursorPos = procSetCursorPos.Call
)

func NewInputAdapter() *InputAdapter {
	return newInputAdapter()
}

func (adapter *InputAdapter) PrepareEntry(ctx context.Context, sourceEdge control.ScreenEdge) (err error) {
	stage := "state"
	var observation *application.CursorPlacementObservation
	defer func() {
		err = cursorTransactionFailure("entry", stage, err)
		if failure, ok := err.(*application.CursorTransactionFailure); ok && stage == "mismatch" {
			failure.Observation = observation
		}
	}()
	if adapter == nil {
		return fmt.Errorf("%w: input adapter is not initialized", ErrInputInjection)
	}
	stage = "context"
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInputInjection)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stage = "state"
	state, err := adapter.cursorEntryState()
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.pending {
		return fmt.Errorf("%w: another cursor entry is pending", ErrCursorEntryState)
	}
	stage = "snapshot"
	before, err := adapter.readCursor(ctx)
	if err != nil {
		return fmt.Errorf("%w: snapshot cursor before entry: %w", ErrInputInjection, err)
	}
	stage = "geometry"
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
	if adapter.injection != nil {
		// A lost worker ACK may follow an applied cursor write. Preserve a
		// pending, unverified entry instead of claiming rollback was unnecessary.
		state.pending = true
		state.previousX, state.previousY = before.X, before.Y
	}
	stage = "set"
	var position point
	if port, ok := adapter.injection.(application.CursorPlacementPort); ok {
		// Protected placement observes confinement and reads back on the same
		// authorized worker thread. Its expected target is not the readback.
		placement, placeErr := port.PlaceCursor(ctx, x, y)
		if placeErr != nil {
			if certainty, known := application.InjectionCertaintyOf(placeErr); known && certainty == application.InjectionNotApplied {
				state.clear()
			}
			return placeErr
		}
		x, y = placement.TargetX, placement.TargetY
		position = point{X: placement.X, Y: placement.Y}
		observation = placement.Observation
	} else {
		if err := adapter.writeCursor(ctx, x, y); err != nil {
			if certainty, known := application.InjectionCertaintyOf(err); known && certainty == application.InjectionNotApplied {
				state.clear()
			}
			return err
		}
		state.pending = true
		state.previousX, state.previousY = before.X, before.Y
		stage = "verify"
		position, err = adapter.readCursor(ctx)
		if err != nil {
			return fmt.Errorf("%w: verify cursor entry: %w", ErrInputInjection, err)
		}
	}
	state.pending = true
	state.previousX = before.X
	state.previousY = before.Y
	state.enteredKnown = true
	state.enteredX = position.X
	state.enteredY = position.Y
	stage = "mismatch"
	if position.X != x || position.Y != y {
		return fmt.Errorf("%w: cursor entry position does not match", ErrInputInjection)
	}
	return nil
}

func (adapter *InputAdapter) RollbackEntry(ctx context.Context) (outcome application.CursorRollbackOutcome, err error) {
	stage := "state"
	defer func() { err = cursorTransactionFailure("rollback", stage, err) }()
	state, err := adapter.cursorEntryState()
	if err != nil {
		return "", err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.pending {
		return application.CursorRollbackNotRestored, nil
	}
	defer state.clear()
	stage = "context"
	if ctx == nil {
		return "", fmt.Errorf("%w: rollback context is required", ErrCursorEntryState)
	}
	if err := ctx.Err(); err != nil {
		return "", errors.Join(ErrCursorEntryState, err)
	}
	stage = "unverified"
	if !state.enteredKnown {
		return "", fmt.Errorf("%w: entry position could not be verified", ErrCursorEntryState)
	}
	stage = "inspect"
	current, err := adapter.readCursor(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: inspect cursor before rollback: %w", ErrCursorEntryState, err)
	}
	if current.X != state.enteredX || current.Y != state.enteredY {
		return application.CursorRollbackNotRestored, nil
	}
	stage = "restore"
	if err := adapter.writeCursor(ctx, state.previousX, state.previousY); err != nil {
		return "", errors.Join(ErrCursorEntryState, err)
	}
	stage = "verify"
	restored, err := adapter.readCursor(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: verify cursor rollback: %w", ErrCursorEntryState, err)
	}
	stage = "mismatch"
	if restored.X != state.previousX || restored.Y != state.previousY {
		return "", fmt.Errorf("%w: cursor rollback position does not match", ErrCursorEntryState)
	}
	return application.CursorRollbackRestored, nil
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
	var injectErr error
	var position point
	positioned := false
	if adapter.injection != nil {
		if port, ok := adapter.injection.(application.PositionedMovePort); ok {
			position.X, position.Y, injectErr = port.MoveWithPosition(ctx, deltaX, deltaY)
			positioned = true
		} else {
			injectErr = adapter.injection.Move(ctx, deltaX, deltaY)
		}
	} else {
		injectErr = sendInputs(ctx, []input{event})
	}
	if injectErr != nil {
		return injectErr
	}
	started := time.Now()
	defer func() { handoff.Record(ctx, handoff.CursorPostprocess, time.Since(started)) }()
	if !positioned {
		position, err = adapter.readCursor(ctx)
		if err != nil {
			return err
		}
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
	if adapter.injection != nil {
		return adapter.injection.Wheel(ctx, deltaX, deltaY)
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
	if adapter.injection != nil {
		if err := token.Validate(); err != nil {
			return errorsJoinUnsupported(err)
		}
		if released {
			return adapter.injection.Release(ctx, token)
		}
		return adapter.injection.Press(ctx, token)
	}
	return writeNativeDiscrete(ctx, token, released)
}

func writeNativeDiscrete(ctx context.Context, token control.PressedToken, released bool) error {
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

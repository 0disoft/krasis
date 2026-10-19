//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/application"
)

var _ application.CursorPositionPort = nativeInjectionPort{}

func (adapter *InputAdapter) readCursor(ctx context.Context) (point, error) {
	if adapter.injection != nil {
		port, ok := adapter.injection.(application.CursorPositionPort)
		if !ok {
			return point{}, &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrUnsupportedInput}
		}
		x, y, err := port.CursorPosition(ctx)
		return point{X: x, Y: y}, err
	}
	return cursorPosition()
}

func (adapter *InputAdapter) writeCursor(ctx context.Context, x, y int32) error {
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf("%w: cursor operation context unavailable", ErrInputInjection)
	}
	if adapter.injection != nil {
		port, ok := adapter.injection.(application.CursorPositionPort)
		if !ok {
			return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrUnsupportedInput}
		}
		return port.SetCursorPosition(ctx, x, y)
	}
	result, _, err := callSetCursorPos(uintptr(uint32(x)), uintptr(uint32(y)))
	if result == 0 {
		return fmt.Errorf("%w: SetCursorPos failed: %v", ErrInputInjection, err)
	}
	return nil
}

func (nativeInjectionPort) CursorPosition(ctx context.Context) (int32, int32, error) {
	if ctx == nil || ctx.Err() != nil {
		return 0, 0, &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrInputInjection}
	}
	position, err := cursorPosition()
	if err != nil {
		return 0, 0, &application.InjectionError{Certainty: application.InjectionNotApplied,
			Cause: errors.Join(ErrInputInjection, application.ErrCursorObservationUnavailable, err)}
	}
	return position.X, position.Y, nil
}

func (nativeInjectionPort) SetCursorPosition(ctx context.Context, x, y int32) error {
	if ctx == nil || ctx.Err() != nil {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrInputInjection}
	}
	left, top := int64(systemMetric(smXVIRTUALSCREEN)), int64(systemMetric(smYVIRTUALSCREEN))
	width, height := int64(systemMetric(smCXVIRTUALSCREEN)), int64(systemMetric(smCYVIRTUALSCREEN))
	if width <= 0 || height <= 0 || int64(x) < left || int64(x) >= left+width || int64(y) < top || int64(y) >= top+height {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrUnsupportedInput}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	result, _, _ := callSetCursorPos(uintptr(uint32(x)), uintptr(uint32(y)))
	if result == 0 {
		return &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: ErrInputInjection}
	}
	return nil
}

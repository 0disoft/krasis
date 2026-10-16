package residentagent

import (
	"context"

	"github.com/0disoft/krasis/internal/application"
)

var _ application.CursorPositionPort = (*privilegedLeaseInput)(nil)
var _ application.PositionedMovePort = (*privilegedLeaseInput)(nil)
var _ application.CursorPlacementPort = (*privilegedLeaseInput)(nil)
var _ application.CursorEntryPort = (*privilegedLeaseInput)(nil)

func (input *privilegedLeaseInput) EnterCursor(ctx context.Context, x, y int32) (entry application.CursorEntry, err error) {
	err = input.perform(ctx, func(client privilegedClient) error {
		port, ok := client.(application.CursorEntryPort)
		if !ok {
			return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errPrivilegedInput}
		}
		var entryErr error
		entry, entryErr = port.EnterCursor(ctx, x, y)
		return entryErr
	})
	if err != nil && input.event != nil {
		input.event("installed_input_cursor_set_failed")
	}
	return
}

func (input *privilegedLeaseInput) PlaceCursor(ctx context.Context, x, y int32) (placement application.CursorPlacement, err error) {
	err = input.perform(ctx, func(client privilegedClient) error {
		port, ok := client.(application.CursorPlacementPort)
		if !ok {
			return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errPrivilegedInput}
		}
		var placeErr error
		placement, placeErr = port.PlaceCursor(ctx, x, y)
		return placeErr
	})
	if err != nil && input.event != nil {
		input.event("installed_input_cursor_set_failed")
	}
	return
}

func (input *privilegedLeaseInput) CursorPosition(ctx context.Context) (x, y int32, err error) {
	err = input.perform(ctx, func(client privilegedClient) error {
		port, ok := client.(application.CursorPositionPort)
		if !ok {
			return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errPrivilegedInput}
		}
		var queryErr error
		x, y, queryErr = port.CursorPosition(ctx)
		return queryErr
	})
	if err != nil && input.event != nil {
		input.event("installed_input_cursor_read_failed")
	}
	return
}

func (input *privilegedLeaseInput) SetCursorPosition(ctx context.Context, x, y int32) error {
	err := input.perform(ctx, func(client privilegedClient) error {
		port, ok := client.(application.CursorPositionPort)
		if !ok {
			return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errPrivilegedInput}
		}
		return port.SetCursorPosition(ctx, x, y)
	})
	if err != nil && input.event != nil {
		input.event("installed_input_cursor_set_failed")
	}
	return err
}

func (input *privilegedLeaseInput) MoveWithPosition(ctx context.Context, dx, dy int32) (x, y int32, err error) {
	err = input.perform(ctx, func(client privilegedClient) error {
		port, ok := client.(application.PositionedMovePort)
		if !ok {
			return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errPrivilegedInput}
		}
		var moveErr error
		x, y, moveErr = port.MoveWithPosition(ctx, dx, dy)
		return moveErr
	})
	return
}

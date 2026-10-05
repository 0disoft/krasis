package residentagent

import (
	"context"

	"github.com/0disoft/krasis/internal/application"
)

var _ application.CursorPositionPort = (*privilegedLeaseInput)(nil)
var _ application.PositionedMovePort = (*privilegedLeaseInput)(nil)

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

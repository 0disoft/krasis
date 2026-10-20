package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

var _ application.CursorPositionPort = (*Client)(nil)
var _ application.PositionedMovePort = (*Client)(nil)
var _ application.CursorPlacementPort = (*Client)(nil)
var _ application.CursorPlacementProbePort = (*Client)(nil)

func (client *Client) PlaceCursor(ctx context.Context, x, y int32) (placement application.CursorPlacement, err error) {
	op := opCursorPlace
	if handoff.WantCursorPlacement(ctx) {
		op = opCursorPlaceProbe
	}
	return client.placeCursor(ctx, x, y, op)
}

func (client *Client) ProbeCursorPlacement(ctx context.Context, x, y int32) (application.CursorPlacement, error) {
	return client.placeCursor(ctx, x, y, opCursorPlaceProbe)
}

func (client *Client) placeCursor(ctx context.Context, x, y int32, op operation) (placement application.CursorPlacement, err error) {
	err = client.performResponse(ctx, request{op: op, x: x, y: y}, func(ack response) {
		placement = application.CursorPlacement{TargetX: ack.targetX, TargetY: ack.targetY, X: ack.x, Y: ack.y}
		p := ack.observation
		if p != nil && (p.Flags&application.CursorTargetAdjusted != 0) == (x != ack.targetX || y != ack.targetY) && (p.Flags&application.CursorReadbackMatches != 0) == (ack.x == ack.targetX && ack.y == ack.targetY) {
			placement.Observation = p
		}
	})
	return
}

func placeCursorResponse(ctx context.Context, input application.InputPort, pressed *control.PressedState, command request, ack *response) error {
	port, ok := input.(application.CursorPlacementPort)
	if !ok || pressed.Count() != 0 {
		return notSent(ErrProtocol)
	}
	place := port.PlaceCursor
	if command.op == opCursorPlaceProbe {
		if probe, ok := input.(application.CursorPlacementProbePort); ok {
			place = probe.ProbeCursorPlacement
		}
	}
	placement, err := place(ctx, command.x, command.y)
	if err != nil {
		return err
	}
	ack.x, ack.y, ack.targetX, ack.targetY = placement.X, placement.Y, placement.TargetX, placement.TargetY
	ack.observation = placement.Observation
	return nil
}

func (client *Client) CursorPosition(ctx context.Context) (x, y int32, err error) {
	err = client.performResponse(ctx, request{op: opCursorRead}, func(ack response) { x, y = ack.x, ack.y })
	return
}

func (client *Client) SetCursorPosition(ctx context.Context, x, y int32) error {
	return client.perform(ctx, request{op: opCursorSet, x: x, y: y})
}

func (client *Client) MoveWithPosition(ctx context.Context, dx, dy int32) (x, y int32, err error) {
	if dx < -maxDelta || dx > maxDelta || dy < -maxDelta || dy > maxDelta || dx == 0 && dy == 0 {
		return 0, 0, notSent(ErrProtocol)
	}
	err = client.performResponse(ctx, request{op: opMoveRead, x: dx, y: dy}, func(ack response) { x, y = ack.x, ack.y })
	return
}

func readCursorResponse(ctx context.Context, input application.InputPort, ack *response) error {
	port, ok := input.(application.CursorPositionPort)
	if !ok {
		return notSent(ErrProtocol)
	}
	var err error
	ack.x, ack.y, err = port.CursorPosition(ctx)
	if err != nil && errors.Is(err, application.ErrCursorObservationUnavailable) && ctx.Err() == nil {
		if observe, ok := ctx.Value(cursorObservationScopeKey{}).(executor); ok {
			first := err
			err = observe(ctx, func() error {
				var retryErr error
				ack.x, ack.y, retryErr = port.CursorPosition(ctx)
				return retryErr
			})
			if err != nil {
				err = errors.Join(first, err)
			}
		}
	}
	return err
}

// Installed workers alone provide this local read-only reauthorization scope.
// It cannot arrive over IPC, dispatch input, renew a deadline or replace a lease.
type cursorObservationScopeKey struct{}

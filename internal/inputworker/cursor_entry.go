package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

func cursorEntryOperation(op operation) bool { return op == opCursorEntry || op == opCursorEntryProbe }
func cursorProbeOperation(op operation) bool {
	return op == opCursorPlaceProbe || op == opCursorEntryProbe
}
func cursorPlacementOperation(op operation) bool {
	return op == opCursorPlace || op == opCursorPlaceProbe || cursorEntryOperation(op)
}
func cursorResponseOperation(op operation) bool {
	return op == opCursorRead || op == opMoveRead || cursorPlacementOperation(op)
}

var _ application.CursorEntryPort = (*Client)(nil)

// The same lease, sequence, pressed guard and desktop scope cover the snapshot,
// the single placement and its readback. A missing ACK is never retried.
func (client *Client) EnterCursor(ctx context.Context, x, y int32) (application.CursorEntry, error) {
	op := opCursorEntry
	if handoff.WantCursorPlacement(ctx) {
		op = opCursorEntryProbe
	}
	return client.enterCursor(ctx, x, y, op)
}

func (client *Client) enterCursor(ctx context.Context, x, y int32, op operation) (entry application.CursorEntry, err error) {
	err = client.performResponse(ctx, request{op: op, x: x, y: y}, func(ack response) {
		entry = application.CursorEntry{
			CursorPlacement: application.CursorPlacement{TargetX: ack.targetX, TargetY: ack.targetY, X: ack.x, Y: ack.y},
			BeforeX:         ack.beforeX, BeforeY: ack.beforeY,
		}
		p := ack.observation
		if p != nil && (p.Flags&application.CursorTargetAdjusted != 0) == (x != ack.targetX || y != ack.targetY) && (p.Flags&application.CursorReadbackMatches != 0) == (ack.x == ack.targetX && ack.y == ack.targetY) {
			entry.Observation = p
		}
	})
	return
}

func enterCursorResponse(ctx context.Context, input application.InputPort, pressed *control.PressedState, command request, ack *response) error {
	position, ok := input.(application.CursorPositionPort)
	if !ok || pressed.Count() != 0 {
		return notSent(ErrProtocol)
	}
	if isolated, ok := input.(*isolatedInput); ok {
		// The native child already performs snapshot, placement and readback
		// in one authorized desktop transaction. Preserve the requested probe
		// and its single ACK rather than splitting it into two child calls.
		entry, err := isolated.Client.enterCursor(ctx, command.x, command.y, command.op)
		if err != nil {
			return err
		}
		ack.beforeX, ack.beforeY = entry.BeforeX, entry.BeforeY
		ack.x, ack.y, ack.targetX, ack.targetY = entry.X, entry.Y, entry.TargetX, entry.TargetY
		ack.observation = entry.Observation
		return nil
	}
	x, y, err := position.CursorPosition(ctx)
	if err != nil {
		return notSent(err) // No cursor write has been attempted.
	}
	if command.op == opCursorEntryProbe {
		command.op = opCursorPlaceProbe
	} else {
		command.op = opCursorPlace
	}
	if err := placeCursorResponse(ctx, input, pressed, command, ack); err != nil {
		return err
	}
	ack.beforeX, ack.beforeY = x, y
	return nil
}

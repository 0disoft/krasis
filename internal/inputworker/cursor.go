package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/application"
)

var _ application.CursorPositionPort = (*Client)(nil)
var _ application.PositionedMovePort = (*Client)(nil)

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
	return err
}

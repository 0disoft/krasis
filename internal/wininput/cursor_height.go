package wininput

import (
	"context"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func restoreCursorHeight(ctx context.Context, port application.CursorPositionPort, height control.EdgeHeight, top, size int32) error {
	if height == 0 {
		return nil
	}
	y, err := height.Coordinate(top, size)
	if err != nil {
		return err
	}
	x, _, err := port.CursorPosition(ctx)
	if err != nil {
		return err
	}
	return port.SetCursorPosition(ctx, x, y)
}

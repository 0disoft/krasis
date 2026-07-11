package application

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/control"
)

type InputPort interface {
	ReleasePort
	Press(context.Context, control.PressedToken) error
	Move(context.Context, int32, int32) error
	Wheel(context.Context, int32, int32) error
}

func InjectPress(ctx context.Context, state *control.PressedState, port InputPort, token control.PressedToken) error {
	if state == nil || port == nil {
		return errors.New("pressed state and input port are required")
	}
	if _, err := state.RecordPress(token); err != nil {
		return err
	}
	return port.Press(ctx, token)
}

func InjectRelease(
	ctx context.Context,
	state *control.PressedState,
	port InputPort,
	token control.PressedToken,
) (bool, error) {
	if state == nil || port == nil {
		return false, errors.New("pressed state and input port are required")
	}
	if err := token.Validate(); err != nil {
		return false, err
	}
	if !state.Contains(token) {
		return false, nil
	}
	if err := port.Release(ctx, token); err != nil {
		return false, err
	}
	return state.RecordRelease(token)
}

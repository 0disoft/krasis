//go:build !windows

package wininput

import (
	"context"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

type InputAdapter struct{}

func NewInputAdapter() *InputAdapter { return &InputAdapter{} }

func (adapter *InputAdapter) Enter(context.Context, control.ScreenEdge) error {
	return fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
}

func (adapter *InputAdapter) Press(context.Context, control.PressedToken) error {
	return fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
}

func (adapter *InputAdapter) Release(context.Context, control.PressedToken) error {
	return fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
}

func (adapter *InputAdapter) Move(context.Context, int32, int32) error {
	return fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
}

func (adapter *InputAdapter) Wheel(context.Context, int32, int32) error {
	return fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
}

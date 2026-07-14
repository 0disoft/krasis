//go:build !windows

package wininput

import (
	"context"
	"fmt"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func NewInputAdapter() *InputAdapter { return newInputAdapter() }

func (adapter *InputAdapter) PrepareEntry(context.Context, control.ScreenEdge) error {
	return fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
}

func (adapter *InputAdapter) RollbackEntry(context.Context) (application.CursorRollbackOutcome, error) {
	return "", fmt.Errorf("%w: input adapter requires Windows", ErrUnsupportedInput)
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

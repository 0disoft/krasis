package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"sync"
)

type productInputGuard interface {
	PreparePress(control.PressedToken) (bool, error)
	CancelPreparedPress(control.PressedToken) error
	Released(control.PressedToken) error
}

type guardedProductInput struct {
	agent.InputBoundaryPort
	guard productInputGuard
	mu    sync.Mutex
}

func (input *guardedProductInput) Press(ctx context.Context, token control.PressedToken) error {
	input.mu.Lock()
	defer input.mu.Unlock()
	prepared, err := input.guard.PreparePress(token)
	if err != nil {
		return err
	}
	if err := input.InputBoundaryPort.Press(ctx, token); err != nil {
		certainty, classified := application.InjectionCertaintyOf(err)
		if prepared && classified && certainty == application.InjectionNotApplied {
			return errors.Join(err, input.guard.CancelPreparedPress(token))
		}
		return err
	}
	return nil
}

func (input *guardedProductInput) Release(ctx context.Context, token control.PressedToken) error {
	input.mu.Lock()
	defer input.mu.Unlock()
	if err := input.InputBoundaryPort.Release(ctx, token); err != nil {
		return err
	}
	return input.guard.Released(token)
}

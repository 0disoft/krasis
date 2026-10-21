package residentagent

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/syncgate"
	"github.com/0disoft/krasis/internal/wininput"
)

type productInputGuard interface {
	PreparePress(control.PressedToken) (bool, error)
	CancelPreparedPress(control.PressedToken) error
	Released(control.PressedToken) error
}

type guardedProductInput struct {
	agent.InputBoundaryPort
	guard         productInputGuard
	log           *productlog.Logger
	mu            syncgate.Mutex
	geometry      func() error
	protected     func(context.Context) (bool, error)
	stopProtected func()
}

func (input *guardedProductInput) Press(ctx context.Context, token control.PressedToken) error {
	if err := input.mu.LockContext(ctx); err != nil {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1, Cause: err}
	}
	defer input.mu.Unlock()
	if err := input.checkProtectedInput(ctx); err != nil {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1, Cause: err}
	}
	prepared, err := input.guard.PreparePress(token)
	if err != nil {
		return err
	}
	if err := input.InputBoundaryPort.Press(ctx, token); err != nil {
		input.log.Injection("press", err)
		certainty, classified := application.InjectionCertaintyOf(err)
		if prepared && classified && certainty == application.InjectionNotApplied {
			return errors.Join(err, input.guard.CancelPreparedPress(token))
		}
		return err
	}
	return nil
}

func (input *guardedProductInput) Release(ctx context.Context, token control.PressedToken) error {
	if err := input.mu.LockContext(ctx); err != nil {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1, Cause: err}
	}
	defer input.mu.Unlock()
	if err := input.InputBoundaryPort.Release(ctx, token); err != nil {
		input.log.Injection("release", err)
		return err
	}
	return input.guard.Released(token)
}

func (input *guardedProductInput) Move(ctx context.Context, x, y int32) error {
	err := input.InputBoundaryPort.Move(ctx, x, y)
	input.log.Injection("move", err)
	return err
}
func (input *guardedProductInput) Wheel(ctx context.Context, x, y int32) error {
	err := input.InputBoundaryPort.Wheel(ctx, x, y)
	input.log.Injection("wheel", err)
	return err
}
func (input *guardedProductInput) PrepareEntry(ctx context.Context, edge control.ScreenEdge) error {
	if err := input.checkProtectedInput(ctx); err != nil {
		return err
	}
	if err := input.ValidateEntry(ctx); err != nil {
		return err
	}
	err := input.InputBoundaryPort.PrepareEntry(ctx, edge)
	input.log.Injection("entry", err)
	if err == nil {
		return input.ValidateEntry(ctx)
	}
	return err
}

func (input *guardedProductInput) checkProtectedInput(ctx context.Context) error {
	if input.protected == nil {
		return nil
	}
	active, err := input.protected(ctx)
	if err != nil {
		return err
	}
	if active {
		if input.stopProtected != nil {
			input.stopProtected()
		}
		return wininput.ErrProtectedInput
	}
	return nil
}

func (input *guardedProductInput) ValidateEntry(ctx context.Context) error {
	if ctx == nil {
		return errors.New("entry validation context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.geometry != nil {
		return input.geometry()
	}
	return nil
}
func (input *guardedProductInput) RollbackEntry(ctx context.Context) (application.CursorRollbackOutcome, error) {
	outcome, err := input.InputBoundaryPort.RollbackEntry(ctx)
	input.log.Injection("rollback", err)
	return outcome, err
}

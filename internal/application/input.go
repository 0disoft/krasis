package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

type InjectionCertainty string

const (
	InjectionNotApplied       InjectionCertainty = "NOT_APPLIED"
	InjectionPartiallyApplied InjectionCertainty = "PARTIALLY_APPLIED"
	InjectionOutcomeUnknown   InjectionCertainty = "OUTCOME_UNKNOWN"
)

type CursorRollbackOutcome string

const (
	CursorRollbackRestored    CursorRollbackOutcome = "RESTORED"
	CursorRollbackNotRestored CursorRollbackOutcome = "NOT_RESTORED"
)

func (outcome CursorRollbackOutcome) Validate() error {
	switch outcome {
	case CursorRollbackRestored, CursorRollbackNotRestored:
		return nil
	default:
		return fmt.Errorf("unsupported cursor rollback outcome %q", outcome)
	}
}

type InjectionError struct {
	Certainty InjectionCertainty
	Requested int
	Inserted  int
	Cause     error
}

func (failure *InjectionError) Error() string {
	if failure == nil {
		return "input injection failed"
	}
	return fmt.Sprintf(
		"input injection failed with certainty %s: inserted %d of %d events",
		failure.Certainty,
		failure.Inserted,
		failure.Requested,
	)
}

func (failure *InjectionError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func InjectionCertaintyOf(err error) (InjectionCertainty, bool) {
	var failure *InjectionError
	if !errors.As(err, &failure) || failure == nil {
		return "", false
	}
	return failure.Certainty, true
}

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
	recorded, err := state.RecordPress(token)
	if err != nil {
		return err
	}
	if err := port.Press(ctx, token); err != nil {
		certainty, classified := InjectionCertaintyOf(err)
		if recorded && classified && certainty == InjectionNotApplied {
			if _, rollbackErr := state.RecordRelease(token); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("rollback unapplied press: %w", rollbackErr))
			}
		}
		return err
	}
	return nil
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

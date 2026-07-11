package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

type ReleasePort interface {
	Release(context.Context, control.PressedToken) error
}

type ReleaseReport struct {
	Attempted int
	Released  int
	Remaining int
}

type TokenReleaseError struct {
	Token control.PressedToken
	Err   error
}

func (err TokenReleaseError) Error() string {
	return fmt.Sprintf("release %s input failed: %v", err.Token.Kind, err.Err)
}

func (err TokenReleaseError) Unwrap() error {
	return err.Err
}

func ReleaseAll(ctx context.Context, state *control.PressedState, port ReleasePort) (ReleaseReport, error) {
	if state == nil {
		return ReleaseReport{}, errors.New("pressed state is required")
	}
	if port == nil {
		return ReleaseReport{Remaining: state.Count()}, errors.New("release port is required")
	}

	plan := state.ReleasePlan()
	report := ReleaseReport{Attempted: len(plan), Remaining: state.Count()}
	failures := make([]error, 0)
	for _, token := range plan {
		if err := port.Release(ctx, token); err != nil {
			failures = append(failures, TokenReleaseError{Token: token, Err: err})
			continue
		}
		released, err := state.RecordRelease(token)
		if err != nil {
			failures = append(failures, TokenReleaseError{Token: token, Err: err})
			continue
		}
		if released {
			report.Released++
		}
	}
	report.Remaining = state.Count()
	return report, errors.Join(failures...)
}

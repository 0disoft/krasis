package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

type SafetyEffectExecutor struct {
	delegate EffectExecutor
	pressed  *control.PressedState
	release  ReleasePort
}

func NewSafetyEffectExecutor(
	delegate EffectExecutor,
	pressed *control.PressedState,
	release ReleasePort,
) (*SafetyEffectExecutor, error) {
	if delegate == nil || pressed == nil || release == nil {
		return nil, errors.New("delegate, pressed state, and release port are required")
	}
	return &SafetyEffectExecutor{delegate: delegate, pressed: pressed, release: release}, nil
}

func (executor *SafetyEffectExecutor) Execute(ctx context.Context, effect control.Effect) error {
	if effect != control.EffectReleaseAll {
		return executor.delegate.Execute(ctx, effect)
	}
	// Stopping control can cancel the session or exhaust its deadline before
	// key-up begins. Recovery owns a fresh bounded budget, never live input.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), InputFailureCleanupTimeout)
	defer cancel()
	report, err := ReleaseAll(cleanup, executor.pressed, executor.release)
	if err != nil {
		return err
	}
	if report.Remaining != 0 {
		return fmt.Errorf("ReleaseAll completed with %d pressed inputs remaining", report.Remaining)
	}
	return nil
}

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
	report, err := ReleaseAll(ctx, executor.pressed, executor.release)
	if err != nil {
		return err
	}
	if report.Remaining != 0 {
		return fmt.Errorf("ReleaseAll completed with %d pressed inputs remaining", report.Remaining)
	}
	return nil
}

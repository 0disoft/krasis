//go:build !windows

package wininput

import (
	"context"
	"fmt"
)

type SourceHook struct{}

func NewSourceHook(...HookTimingObserver) *SourceHook { return &SourceHook{} }

func (hook *SourceHook) Run(context.Context, *SourceSession, func()) error {
	return fmt.Errorf("%w: source hook requires Windows", ErrUnsupportedInput)
}

func (hook *SourceHook) RunWithBoundary(context.Context, *SourceSession, BoundaryObserver, func()) error {
	return fmt.Errorf("%w: source hook requires Windows", ErrUnsupportedInput)
}

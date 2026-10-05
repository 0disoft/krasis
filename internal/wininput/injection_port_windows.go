//go:build windows

package wininput

import (
	"context"
	"sync/atomic"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

// NewInjectionPort supplies native injection only. The installed worker owns
// authorization, desktop binding and pressed-state recovery; the resident agent
// owns cursor entry transactions and return tracking, using this port's cursor
// I/O under the same worker scope. This port grants no input authority.
func NewInjectionPort() application.InputPort {
	return nativeInjectionPort{placementProbes: &atomic.Uint32{}}
}

type nativeInjectionPort struct{ placementProbes *atomic.Uint32 }

func (nativeInjectionPort) Press(ctx context.Context, token control.PressedToken) error {
	return writeNativeDiscrete(ctx, token, false)
}

func (nativeInjectionPort) Release(ctx context.Context, token control.PressedToken) error {
	return writeNativeDiscrete(ctx, token, true)
}

func (nativeInjectionPort) Move(ctx context.Context, x, y int32) error {
	event, err := pointerMoveEvent(x, y)
	if err != nil {
		return err
	}
	return sendInputs(ctx, []input{event})
}

func (nativeInjectionPort) Wheel(ctx context.Context, x, y int32) error {
	events, err := wheelEvents(x, y)
	if err != nil {
		return err
	}
	return sendInputs(ctx, events)
}

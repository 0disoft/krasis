//go:build windows

package wininput

import (
	"context"
	"errors"
)

var hookSessionGate = func() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}()

func acquireHookSession(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, errors.New("hook session context is required")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-hookSessionGate:
		return func() { hookSessionGate <- struct{}{} }, nil
	}
}

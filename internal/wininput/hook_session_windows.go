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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wait, cancel := context.WithTimeout(ctx, SourceCleanupTimeout)
	defer cancel()
	select {
	case <-wait.Done():
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrSourceOwnerUnavailable
	case <-hookSessionGate:
		return func() { hookSessionGate <- struct{}{} }, nil
	}
}

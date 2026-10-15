//go:build windows

package inputworker

import (
	"context"
	"time"
)

func (c *sourceClient) pollHeartbeat(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Never queue idle maintenance behind a cursor/capture command. That owner
	// already supplies authenticated traffic, and skipping sends nothing and
	// renews no deadline. Actual sent requests retain their bounded ACK policy.
	if !c.mu.TryLock() {
		return false, nil
	}
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	check, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_, err := c.callLocked(check, sourceHeartbeat)
	return true, err
}

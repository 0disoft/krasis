//go:build windows

package inputworker

import (
	"context"
	"time"
)

// Keep 1.5 seconds of the worker's two-second idle budget for authority checks,
// scheduling and the existing one-second request. No receive deadline changes.
const sourceHeartbeatQuiet = IdleTimeout / 4

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
	if c.commandWaiting.Load() != 0 || !c.mu.TryLock() {
		return false, nil
	}
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Recheck after winning mu: a command can publish demand during TryLock.
	// Skipping unsent maintenance neither grants authority nor fabricates ACKs.
	if c.commandWaiting.Load() != 0 {
		return false, nil
	}
	if !c.lastReplyRequestAt.IsZero() {
		age := time.Since(c.lastReplyRequestAt)
		if age >= 0 && age < sourceHeartbeatQuiet {
			return false, nil
		}
	}
	check, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_, err := c.callLocked(check, sourceHeartbeat)
	return true, err
}

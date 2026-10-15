//go:build windows

package inputworker

import (
	"context"
	"time"
)

// Keep 1.5 seconds of the worker's two-second idle budget for authority checks,
// scheduling and the existing one-second request. No receive deadline changes.
const sourceHeartbeatQuiet = IdleTimeout / 4

type sourceHeartbeatCheck struct {
	owner   *sourceClient
	elapsed time.Duration
	used    bool
}

func (check *sourceHeartbeatCheck) validate(c *sourceClient, ctx context.Context, op byte) error {
	if check.owner != c || check.used || op != sourceHeartbeat || ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	check.used = true
	// The full immutable-handle check just completed. Closing the channel or
	// its proof between preflight and immediate admission must still reject it.
	a := c.channel.authority
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed || a.checkInput == nil || a.checkRecovery == nil || a.closeHandles == nil {
		return ErrAuthority
	}
	return nil
}

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
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return false, err
	}
	// Recheck after winning mu: a command can publish demand during TryLock.
	// Skipping unsent maintenance neither grants authority nor fabricates ACKs.
	if c.commandWaiting.Load() != 0 {
		c.mu.Unlock()
		return false, nil
	}
	if !c.lastReplyRequestAt.IsZero() {
		age := time.Since(c.lastReplyRequestAt)
		if age >= 0 && age < sourceHeartbeatQuiet {
			c.mu.Unlock()
			return false, nil
		}
	}
	c.mu.Unlock()
	// SCM can block. Do not own the ordered IPC gate while querying it: an
	// independently authenticated activation must be able to pass maintenance.
	check, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	started := time.Now()
	if err := c.channel.authority.check(false); err != nil {
		return true, err
	}
	proof := &sourceHeartbeatCheck{owner: c, elapsed: time.Since(started)}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if check.Err() != nil {
		if !c.preflightExpired.Swap(true) && c.heartbeatTiming.emit != nil {
			emitTiming(c.heartbeatTiming.emit, "installed_source_heartbeat_preflight_expired")
		}
		return false, nil // Nothing sent; no sequence, ACK or liveness renewal.
	}
	if c.commandWaiting.Load() != 0 || !c.mu.TryLock() {
		return false, nil
	}
	defer c.mu.Unlock()
	if check.Err() != nil || c.commandWaiting.Load() != 0 {
		return false, nil
	}
	if !c.lastReplyRequestAt.IsZero() {
		age := time.Since(c.lastReplyRequestAt)
		if age >= 0 && age < sourceHeartbeatQuiet {
			return false, nil
		}
	}
	_, err := c.callLockedChecked(check, sourceHeartbeat, proof)
	return true, err
}

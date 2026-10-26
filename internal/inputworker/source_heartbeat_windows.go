//go:build windows

package inputworker

import (
	"context"
	"time"
)

// Start maintenance after a quarter of the active owner's idle budget. Replies
// may use the remaining budget; they never extend the worker's receive deadline.
const sourceHeartbeatQuiet = IdleTimeout / 4

// A keepalive may be delayed by scheduling even though its pinned owner is
// still within the existing two-second active deadline. Do not fail it after
// the one-second input-command budget. Standby is bounded by this too, so an
// activation never queues behind its full ten-second idle receive window.
const sourceHeartbeatReplyTimeout = IdleTimeout

type sourceHeartbeatCheck struct {
	owner   *sourceClient
	elapsed time.Duration
	used    bool
}

func (check *sourceHeartbeatCheck) validate(c *sourceClient, ctx context.Context, op byte) error {
	if check.owner != c || check.used || op != sourceHeartbeat || ctx == nil {
		return ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	check.used = true
	// The full immutable-handle check just completed. Closing the channel or
	// its proof between preflight and immediate admission must still reject it.
	a := c.channel.authority
	if a == nil || a.fenced.Load() || !a.mu.TryRLock() {
		return ErrAuthority
	}
	defer a.mu.RUnlock()
	if a.fenced.Load() || a.closed || a.checkInput == nil || a.checkRecovery == nil || a.closeHandles == nil {
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
	// Presence is not input authorization. SCM/WTS RPC belongs to the fresh
	// command/input checks, not keepalive traffic that can otherwise starve its
	// own two-second reader while waiting for a broker. Kernel-owner checks are
	// still performed outside the gate, so capture commands can pass maintenance.
	// No request exists yet: owner cancellation must stop admission/preflight
	// immediately so retirement can proceed. Optional query ownership retains
	// any in-flight native handles until completion. Only the sent request below
	// detaches cancellation to preserve its exact ACK/retirement contract.
	check, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	started := time.Now()
	err := c.channel.authorizeHeartbeatPresence(check)
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
	if err != nil {
		return true, err
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
	// Preflight is not a sent request. Give the actual request its own budget,
	// capped by the conservative idle deadline from the last successful request
	// start. A slow SCM check must neither consume the ACK budget nor extend the
	// worker's state-specific idle limit. Standby has no suppressed input; the
	// worker independently observes the pinned process while waiting for it.
	deadline := time.Now().Add(sourceHeartbeatReplyTimeout)
	if !c.lastReplyRequestAt.IsZero() {
		idle := c.lastReplyRequestAt.Add(sourceReadTimeout(true, c.epoch.Load() != 0 || c.cursorPrepared))
		if idle.Before(deadline) {
			deadline = idle
		}
	}
	if !time.Now().Before(deadline) {
		return false, nil
	}
	request, stopRequest := context.WithDeadline(context.WithoutCancel(ctx), deadline)
	defer stopRequest()
	sequence := c.sequence
	_, err = c.callLockedChecked(request, sourceHeartbeat, proof)
	if c.sequence == sequence && (err == context.DeadlineExceeded || err == context.Canceled) {
		// Admission expired before any frame existed. Actual sent requests and
		// failed owner checks still take the ordinary failure/retirement path.
		return false, ctx.Err()
	}
	return true, err
}

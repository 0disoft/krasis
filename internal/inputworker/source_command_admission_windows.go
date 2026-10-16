//go:build windows

package inputworker

import (
	"context"
	"time"
)

// A command proof is local, single-use and bound to this exact owner/op. It
// never survives writer contention and cannot be supplied by a wire peer.
type sourceCommandCheck struct {
	owner   *sourceClient
	op      byte
	elapsed time.Duration
	err     error
	used    bool
}

func (check *sourceCommandCheck) validate(c *sourceClient, ctx context.Context, op byte) error {
	if check.owner != c || check.op != op || check.used || ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	check.used = true
	if check.err != nil {
		return check.err
	}
	// Full SCM/WTS authorization just finished without waiting for the gate.
	// Before bytes, still reject a closed or exited pinned service owner.
	return c.channel.authority.checkHeartbeat()
}

func (c *sourceClient) call(ctx context.Context, op byte, reference ...uint64) (sourcePacket, error) {
	// Stop owns pending-reply draining and retirement, including an already
	// retired owner whose live authority is no longer queryable.
	if op == sourceStop {
		c.commandWaiting.Add(1)
		defer c.commandWaiting.Add(-1)
		started := time.Now()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.lockWait = time.Since(started)
		return c.callLocked(ctx, op, reference...)
	}
	var waited, authorization time.Duration
	for {
		if ctx == nil {
			return sourcePacket{}, ErrAuthority
		}
		if err := ctx.Err(); err != nil {
			return sourcePacket{}, err
		}
		started := time.Now()
		err := c.channel.authority.check(false)
		authorization += time.Since(started)
		if err == nil && ctx.Err() != nil {
			if !c.commandPreflightExpired.Swap(true) && c.timing.emit != nil {
				emitTiming(c.timing.emit, "installed_source_command_preflight_expired")
			}
			return sourcePacket{}, ctx.Err() // No wire request or epoch exists.
		}
		// Unsent native authorization is not wire demand: maintenance must
		// continue while this query is blocked. Publish priority only now.
		c.commandWaiting.Add(1)
		started = time.Now()
		if c.mu.TryLock() {
			defer c.mu.Unlock()
			defer c.commandWaiting.Add(-1)
			c.lockWait = waited + time.Since(started)
			proof := &sourceCommandCheck{owner: c, op: op, elapsed: authorization, err: err}
			return c.callLockedAuthorized(ctx, op, nil, proof, reference...)
		}
		// Discard the full proof when another request owns the wire. Wait for
		// that bounded request, then reauthorize outside the gate before retry.
		c.mu.Lock()
		c.mu.Unlock()
		waited += time.Since(started)
		c.commandWaiting.Add(-1)
	}
}

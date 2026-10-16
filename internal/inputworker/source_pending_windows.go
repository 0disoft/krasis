//go:build windows

package inputworker

import (
	"context"
	"time"
)

func optionalSourceCursor(op byte) bool {
	return op == sourceCursorPrepare || op == sourceCursorShow || op == sourceCursorRestore
}

// A heartbeat changes no native state. Only an idle capture may retain its
// fully written request for exact-reply draining and proven retirement. Active
// input cannot use this path, even if its heartbeat response is late.
func drainableSourceResponse(op byte, epoch uint64) bool {
	return optionalSourceCursor(op) || op == sourceHeartbeat && epoch == 0
}

// Caller owns the IPC mutex. This consumes only the exact request whose entire
// frame was sent, never retries it, and cannot extend the cleanup deadline.
func (c *sourceClient) drainPending(ctx context.Context) error {
	limit := time.Now().Add(RecoveryTimeout + time.Second)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(limit) {
		limit = deadline
	}
	timer := time.NewTimer(time.Until(limit))
	defer timer.Stop()
	accept := func(reply sourcePacket) error {
		if reply.reply != c.pending.sequence || reply.op != c.pending.op {
			return ErrProtocol
		}
		c.pending = sourcePacket{}
		return nil // Rejection is a known outcome too; no input is granted.
	}
	select {
	case reply := <-c.replies:
		return accept(reply)
	case <-c.done:
		select {
		case reply := <-c.replies:
			return accept(reply)
		default:
		}
		if c.retired.Load() && c.err == nil {
			c.pending = sourcePacket{}
			return nil
		}
	case <-ctx.Done():
	case <-timer.C:
	}
	return ErrAuthority
}

// The child may have closed its inherited pipe after publishing retirement,
// before our reader has consumed that final packet. A failed owner query must
// not discard that proof. Read only on the original authenticated channel;
// never send another command or infer a join from EOF/timeout alone.
func (c *sourceClient) awaitRetirement(ctx context.Context) error {
	limit := time.Now().Add(RecoveryTimeout + time.Second)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(limit) {
		limit = deadline
	}
	timer := time.NewTimer(time.Until(limit))
	defer timer.Stop()
	select {
	case <-c.done:
		if c.retired.Load() && c.err == nil {
			return nil
		}
	case <-ctx.Done():
	case <-timer.C:
	}
	return ErrAuthority
}

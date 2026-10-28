package inputworker

import (
	"context"
	"math"
	"time"
)

// A fresh, single-use presence check for maintenance only. It never authorizes
// input, renews a lease, or substitutes for the matching worker ACK.
type inputHeartbeatCheck struct {
	owner   *Client
	elapsed time.Duration
	used    bool
}

func (check *inputHeartbeatCheck) validate(client *Client, ctx context.Context, op operation) error {
	if check.owner != client || check.used || op != opHeartbeat || ctx.Err() != nil {
		return ErrAuthority
	}
	check.used = true
	a := client.channel.authority
	if a == nil || a.fenced.Load() || !a.mu.TryRLock() {
		return ErrAuthority
	}
	defer a.mu.RUnlock()
	if a.fenced.Load() || a.closed || a.checkInput == nil || a.checkRecovery == nil || a.closeHandles == nil {
		return ErrAuthority
	}
	return nil
}

func (client *Client) pollInputHeartbeat(ctx context.Context) error {
	if client == nil || ctx == nil || ctx.Err() != nil {
		return notSent(ErrAuthority)
	}
	if client.localExchange != nil {
		return client.perform(ctx, request{op: opHeartbeat})
	}
	// Bound the whole call, including preflight, by the unchanged idle budget.
	call, cancel := context.WithTimeout(ctx, IdleTimeout)
	defer cancel()
	if !client.mu.TryLock() {
		return nil
	}
	if client.failed || client.channel == nil {
		client.mu.Unlock()
		return notSent(ErrAuthority)
	}
	if !time.Now().Before(client.lease.ExpiresAt) || client.sequence == math.MaxUint64 {
		client.fail()
		client.mu.Unlock()
		return notSent(ErrExpired)
	}
	if !client.lastAcknowledged.IsZero() && time.Since(client.lastAcknowledged) < 250*time.Millisecond {
		client.mu.Unlock()
		return nil
	}
	channel, emit := client.channel, client.heartbeatTiming.emit
	client.mu.Unlock()
	// Keep maintenance off both the input gate and mandatory presence lane.
	// Expired unsent checks retain native handles until completion, without
	// fencing input. Capacity is still bounded by the existing query owner.
	check, stop := context.WithTimeout(call, IdleTimeout/2)
	defer stop()
	started := time.Now()
	err := channel.authorizeHeartbeatPresence(check)
	proof := &inputHeartbeatCheck{owner: client, elapsed: time.Since(started)}
	if ctx.Err() != nil {
		return notSent(ctx.Err())
	}
	if check.Err() != nil {
		if !client.heartbeatExpired.Swap(true) && emit != nil {
			emitTiming(emit, "installed_input_heartbeat_preflight_expired")
		}
		return nil // No frame, sequence consumption, fabricated ACK or renewal.
	}
	if err != nil {
		if emit != nil {
			emitTiming(emit, "installed_input_heartbeat_presence_failed")
		}
		return notSent(err)
	}
	// Recheck expiry, channel ownership and recent ACK under the input gate.
	// Busy input wins; actual writes and ACK failures keep their fatal policy.
	return client.performResponseChecked(call, request{op: opHeartbeat}, nil, proof)
}

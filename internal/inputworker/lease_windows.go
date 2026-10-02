//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"io"
	"time"
)

// ConnectLease is called only after resident-agent peer/lease approval. The
// service authenticates the actual agent image before reading this metadata.
// Readiness means the child has authenticated the transferred pipe and bound
// its input thread; it does not prove that any particular input was accepted.
func ConnectLease(ctx context.Context, serviceDigest [sha256.Size]byte, lease Lease) (*Client, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrAuthority
	}
	data, err := encodeLease(lease, time.Now())
	if err != nil {
		return nil, connectionFailed("lease_metadata", err)
	}
	channel, err := ConnectService(ctx, serviceDigest)
	if err != nil {
		return nil, err
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = channel.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = channel.Close() })
	defer stop()
	deadline := time.Now().Add(startupTimeout)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := channel.stream.SetDeadline(deadline); err != nil {
		return nil, connectionFailed("lease_deadline", err)
	}
	if err := writeFrame(channel.stream, data[:]); err != nil {
		return nil, connectionFailed("lease_write", err)
	}
	if err := readReady(channel.stream); err != nil {
		return nil, connectionFailed("worker_ready", err)
	}
	if ctx.Err() != nil {
		return nil, connectionFailed("worker_ready", ctx.Err())
	}
	if channel.stream.SetDeadline(time.Time{}) != nil {
		return nil, connectionFailed("lease_deadline", ErrAuthority)
	}
	client, err := NewClient(ctx, channel, lease)
	if err != nil {
		return nil, connectionFailed("client_validation", err)
	}
	accepted = true
	return client, nil
}

func receiveLease(ctx context.Context, channel *Channel) (Lease, error) {
	if ctx == nil || ctx.Err() != nil || channel == nil || channel.authority == nil || channel.authority.kind != authorityAgent || channel.authority.check(false) != nil {
		return Lease{}, ErrAuthority
	}
	stop := context.AfterFunc(ctx, func() { _ = channel.stream.Close() })
	defer stop()
	if channel.stream.SetReadDeadline(time.Now().Add(startupTimeout)) != nil {
		return Lease{}, ErrAuthority
	}
	var data [leaseSize]byte
	if _, err := io.ReadFull(channel.stream, data[:]); err != nil {
		return Lease{}, ErrAuthority
	}
	lease, err := decodeLease(data, time.Now())
	if err != nil || ctx.Err() != nil || channel.authority.check(false) != nil || channel.stream.SetReadDeadline(time.Time{}) != nil {
		return Lease{}, ErrAuthority
	}
	return lease, nil
}

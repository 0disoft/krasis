//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// ConnectLease is called only after resident-agent peer/lease approval. The
// service authenticates the actual agent image before reading this metadata.
// Readiness means the child has authenticated the transferred pipe and bound
// its input thread; it does not prove that any particular input was accepted.
func ConnectLease(ctx context.Context, serviceDigest [sha256.Size]byte, lease Lease) (*Client, error) {
	return connectInstalledLease(ctx, serviceDigest, lease, nil)
}

func connectInstalledLease(ctx context.Context, serviceDigest [sha256.Size]byte, lease Lease, installation *Installation) (*Client, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrAuthority
	}
	data, err := encodeLease(lease, time.Now())
	if err != nil {
		return nil, connectionFailed("lease_metadata", err)
	}
	channel, err := connectInstalledService(ctx, serviceDigest, installation)
	if err != nil {
		return nil, err
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = channel.Close()
		}
	}()
	finish := startupCancellation(ctx, channel)
	defer finish()
	deadline := time.Now().Add(startupTimeout)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := channel.stream.SetDeadline(deadline); err != nil {
		return nil, connectionFailed("lease_deadline", err)
	}
	started := time.Now()
	writeErr := writeFrame(channel.stream, data[:])
	handoff.Record(ctx, handoff.LeaseWrite, time.Since(started))
	if err := writeErr; err != nil {
		return nil, connectionFailed("lease_write", err)
	}
	started = time.Now()
	profile, readyErr := readInputLeaseReady(channel.stream)
	handoff.Record(ctx, handoff.ReadyWait, time.Since(started))
	if err := readyErr; err != nil {
		return nil, connectionFailed("worker_ready", err)
	}
	handoff.RecordStartup(ctx, profile)
	if ctx.Err() != nil {
		return nil, connectionFailed("worker_ready", ctx.Err())
	}
	if channel.stream.SetDeadline(time.Time{}) != nil {
		return nil, connectionFailed("lease_deadline", ErrAuthority)
	}
	started = time.Now()
	client, err := NewClient(ctx, channel, lease)
	handoff.Record(ctx, handoff.NewClientCheck, time.Since(started))
	if err != nil {
		return nil, connectionFailed("client_validation", err)
	}
	if err := finish(); err != nil {
		return nil, connectionFailed("client_validation", err)
	}
	// The protected installed ordinary worker always routes through its
	// independently authenticated native pair. Delivery need not repeat that
	// child's live SCM RPC. Readiness diagnostics never select this policy.
	client.isolatedDelivery = nativeLeasePreparation(installation != nil, lease)
	accepted = true
	return client, nil
}

func receiveLease(ctx context.Context, channel *Channel) (Lease, error) {
	lease, prepare, err := receiveAgentRequest(ctx, channel)
	if prepare {
		return Lease{}, ErrProtocol
	}
	return lease, err
}

func receiveAgentRequest(ctx context.Context, channel *Channel) (Lease, bool, error) {
	if ctx == nil || ctx.Err() != nil || channel == nil || channel.authority == nil || channel.authority.kind != authorityAgent {
		return Lease{}, false, ErrAuthority
	}
	// One budget covers both fresh native proofs and the request bytes. Native
	// checks must use owned admission: a stuck SCM/WTS call retains its handles
	// and capacity after timeout, without blocking this serial service endpoint.
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if channel.authorizeInput(ctx, false) != nil {
		return Lease{}, false, ErrAuthority
	}
	finish := startupCancellation(ctx, channel)
	defer finish()
	deadline, _ := ctx.Deadline()
	if channel.stream.SetReadDeadline(deadline) != nil {
		return Lease{}, false, ErrAuthority
	}
	var data [leaseSize]byte
	if _, err := io.ReadFull(channel.stream, data[:]); err != nil {
		return Lease{}, false, ErrAuthority
	}
	if ctx.Err() != nil || channel.authorizeInput(ctx, false) != nil || channel.stream.SetReadDeadline(time.Time{}) != nil {
		return Lease{}, false, ErrAuthority
	}
	if finish() != nil {
		return Lease{}, false, ErrAuthority
	}
	if isPreparation(data) {
		return Lease{}, true, nil
	}
	lease, err := decodeLease(data, time.Now())
	return lease, false, err
}

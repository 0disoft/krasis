package inputworker

import (
	"context"
	"io"
	"math"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

// Client implements InputPort only over an authenticated service channel and
// an already-approved lease. Service bootstrap must agree this exact binding
// and expiry before construction; this constructor does not authorize a peer.
// A lost acknowledgement is never retried or sent to another input backend.
type Client struct {
	mu       sync.Mutex
	channel  *Channel
	lease    Lease
	sequence uint64
	failed   bool
}

var _ application.InputPort = (*Client)(nil)

func NewClient(ctx context.Context, channel *Channel, lease Lease) (*Client, error) {
	client := &Client{channel: channel, lease: lease, sequence: 1}
	if channel == nil {
		return nil, ErrAuthority
	}
	if !validLease(ctx, channel.stream, client, lease, time.Now()) || channel.authority == nil || channel.authority.kind != authorityService || channel.authority.check(false) != nil {
		_ = channel.Close()
		return nil, ErrAuthority
	}
	return client, nil
}

func (client *Client) Press(ctx context.Context, token control.PressedToken) error {
	if token.Validate() != nil {
		return notSent(ErrProtocol)
	}
	return client.perform(ctx, request{op: opPress, token: token})
}
func (client *Client) Release(ctx context.Context, token control.PressedToken) error {
	if token.Validate() != nil {
		return notSent(ErrProtocol)
	}
	return client.perform(ctx, request{op: opRelease, token: token})
}
func (client *Client) Move(ctx context.Context, x, y int32) error {
	return client.delta(ctx, opMove, x, y)
}
func (client *Client) Wheel(ctx context.Context, x, y int32) error {
	return client.delta(ctx, opWheel, x, y)
}
func (client *Client) Heartbeat(ctx context.Context) error {
	return client.perform(ctx, request{op: opHeartbeat})
}
func (client *Client) delta(ctx context.Context, op operation, x, y int32) error {
	if x < -maxDelta || x > maxDelta || y < -maxDelta || y > maxDelta || (x == 0 && y == 0) {
		return notSent(ErrProtocol)
	}
	return client.perform(ctx, request{op: op, x: x, y: y})
}

func (client *Client) perform(ctx context.Context, command request) error {
	if client == nil || ctx == nil {
		return notSent(ErrAuthority)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.failed || client.channel == nil || ctx.Err() != nil {
		return notSent(ErrAuthority)
	}
	if !time.Now().Before(client.lease.ExpiresAt) || client.sequence == math.MaxUint64 || client.channel.authority.check(false) != nil {
		client.fail()
		return notSent(ErrExpired)
	}
	deadline := time.Now().Add(IdleTimeout)
	if client.lease.ExpiresAt.Before(deadline) {
		deadline = client.lease.ExpiresAt
	}
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	stream := client.channel.stream
	if stream.SetDeadline(deadline) != nil {
		client.fail()
		return notSent(ErrAuthority)
	}
	command.sequence, command.binding = client.sequence, client.lease.Binding
	data, err := encodeRequest(command)
	if err != nil {
		return notSent(ErrProtocol)
	}
	// Interrupt both read and write promptly, without waiting for the mutex.
	stopCancel := context.AfterFunc(ctx, func() { _ = client.channel.Close() })
	defer stopCancel()
	client.sequence++
	n, err := stream.Write(data[:])
	if err != nil || n != len(data) {
		client.fail()
		// Even a zero-byte write error does not prove that a prior native
		// overlapped write was not consumed; classify transport loss as unknown.
		return unknownInput(ErrProtocol)
	}
	ack, err := readResponse(stream)
	if err != nil || ack.sequence != command.sequence || ack.op != command.op {
		client.fail()
		return unknownInput(ErrProtocol)
	}
	if ack.status == 0 {
		return nil
	}
	client.fail()
	certainty := application.InjectionOutcomeUnknown
	if ack.certainty == 1 {
		certainty = application.InjectionNotApplied
	}
	if ack.certainty == 2 {
		certainty = application.InjectionPartiallyApplied
	}
	return &application.InjectionError{Certainty: certainty, Requested: int(ack.requested), Inserted: int(ack.inserted)}
}

// Close interrupts pending I/O and triggers worker-owned recovery. A nil
// result proves channel cleanup only; it is not a release-recovery receipt.
func (client *Client) Close() error {
	if client == nil || client.channel == nil {
		return nil
	}
	err := client.channel.Close()
	client.mu.Lock()
	client.failed = true
	client.mu.Unlock()
	return err
}

func (client *Client) fail() { client.failed = true; _ = client.channel.Close() }
func notSent(cause error) error {
	return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: cause}
}
func unknownInput(cause error) error {
	return &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: cause}
}

func readResponse(reader io.Reader) (response, error) {
	var data [responseSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return response{}, ErrProtocol
	}
	return decodeResponse(data)
}

package inputworker

import (
	"bufio"
	"context"
	"errors"
	"io"
	"math"
	"net"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/syncgate"
)

// Client implements InputPort only over an authenticated service channel and
// an already-approved lease. Service bootstrap must agree this exact binding
// and expiry before construction; this constructor does not authorize a peer.
// A lost acknowledgement is never retried or sent to another input backend.
type Client struct {
	mu                  syncgate.Mutex
	channel             *Channel
	ackReader           *bufio.Reader // Owned by mu; retain any prefetched bytes across requests.
	lease               Lease
	sequence            uint64
	failed              bool
	timing              timingSampler
	heartbeatTiming     timingSampler
	lastAcknowledged    time.Time
	lastDesktop         inputdesktop.DesktopClass
	desktopObservations uint8
	localExchange       func(context.Context, request, func(response)) error
	localClose          func() error
	heartbeatExpired    atomic.Bool
}

// SetTimingObserver enables bounded diagnostic summaries. The observer must
// not block or call back into this client; it cannot change input outcomes.
func (client *Client) SetTimingObserver(emit func(string)) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.timing = timingSampler{emit: emit}
	client.heartbeatTiming = timingSampler{emit: emit, heartbeat: true}
}

var _ application.InputPort = (*Client)(nil)

func NewClient(ctx context.Context, channel *Channel, lease Lease) (*Client, error) {
	if ctx == nil {
		if channel != nil {
			_ = channel.Close()
		}
		return nil, ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, IdleTimeout)
	defer cancel()
	if lease.Presentation {
		if channel != nil {
			_ = channel.Close()
		}
		return nil, ErrAuthority
	}
	client := &Client{channel: channel, lease: lease, sequence: 1}
	if channel == nil {
		return nil, ErrAuthority
	}
	if !validLease(ctx, channel.stream, client, lease, time.Now()) || channel.authority == nil || channel.authority.kind != authorityService || channel.authorizeInput(ctx, false) != nil || !validLease(ctx, channel.stream, client, lease, time.Now()) {
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
	// Best-effort idle maintenance, not a liveness receipt. An in-flight
	// operation owns bounded I/O and worker recovery; never queue maintenance
	// behind it or add a roundtrip after a recent matching acknowledgement.
	return client.pollInputHeartbeat(ctx)
}
func (client *Client) delta(ctx context.Context, op operation, x, y int32) error {
	if x < -maxDelta || x > maxDelta || y < -maxDelta || y > maxDelta || (x == 0 && y == 0) {
		return notSent(ErrProtocol)
	}
	return client.perform(ctx, request{op: op, x: x, y: y})
}

func (client *Client) perform(ctx context.Context, command request) error {
	return client.performResponse(ctx, command, nil)
}

func (client *Client) performResponse(ctx context.Context, command request, observed func(response)) error {
	return client.performResponseChecked(ctx, command, observed, nil)
}

func (client *Client) performResponseChecked(ctx context.Context, command request, observed func(response), heartbeat *inputHeartbeatCheck) error {
	if client != nil && client.localExchange != nil {
		return client.localExchange(ctx, command, observed)
	}
	if client == nil || ctx == nil || ctx.Err() != nil {
		return notSent(ErrAuthority)
	}
	// The budget includes admission and authority work, not just pipe I/O.
	ctx, cancel := context.WithTimeout(ctx, IdleTimeout)
	defer cancel()
	start := time.Now()
	var lockElapsed, authorityElapsed time.Duration
	var authorityErr error
	if command.op == opHeartbeat {
		if !client.mu.TryLock() {
			return nil
		}
		lockElapsed = time.Since(start)
	} else {
		if client.channel == nil {
			return notSent(ErrAuthority)
		}
		for {
			if err := ctx.Err(); err != nil {
				return notSent(errors.Join(ErrAuthority, err))
			}
			// SCM queries are not wire demand. Keep the serial input gate
			// free for maintenance until full authorization has completed.
			start = time.Now()
			authorityErr = client.channel.authorizeInput(ctx, true)
			authorityElapsed += time.Since(start)
			if ctx.Err() != nil {
				handoff.Record(ctx, handoff.ClientAuthority, authorityElapsed)
				return notSent(ctx.Err())
			}
			start = time.Now()
			if client.mu.TryLock() {
				lockElapsed += time.Since(start)
				break
			}
			err := client.mu.LockContext(ctx)
			lockElapsed += time.Since(start)
			if err != nil {
				handoff.Record(ctx, handoff.ClientLock, lockElapsed)
				return notSent(errors.Join(ErrAuthority, err))
			}
			client.mu.Unlock()
			// Another operation owned the wire after this check. Discard
			// the proof and reauthorize rather than caching it across a wait.
		}
	}
	var sample [7]byte
	wasFailed := client.failed
	sample[0] = timingBucket(lockElapsed)
	if command.op != opHeartbeat {
		handoff.Record(ctx, handoff.ClientLock, lockElapsed)
	}
	defer func() {
		if !wasFailed {
			sampler := &client.timing
			if command.op == opHeartbeat {
				sampler = &client.heartbeatTiming
			}
			sampler.observe(sample, time.Now())
			if client.failed {
				client.timing.flush(time.Now())
				client.heartbeatTiming.flush(time.Now())
			}
		}
		client.mu.Unlock()
	}()
	if client.failed || client.channel == nil || ctx.Err() != nil {
		return notSent(errors.Join(ErrAuthority, ctx.Err()))
	}
	if !time.Now().Before(client.lease.ExpiresAt) || client.sequence == math.MaxUint64 {
		client.fail()
		return notSent(ErrExpired)
	}
	if command.op == opHeartbeat && !client.lastAcknowledged.IsZero() && time.Since(client.lastAcknowledged) < 250*time.Millisecond {
		// Every successful worker response renews its idle deadline. No wire
		// request means no sequence consumption or fabricated worker timings.
		sample = [7]byte{}
		return nil
	}
	start = time.Now()
	if heartbeat != nil {
		authorityErr = heartbeat.validate(client, ctx, command.op)
		authorityElapsed += heartbeat.elapsed
	} else if authorityErr == nil {
		// Maintenance checks only the pinned pipe owner. Input has just
		// completed full authorization without waiting for this gate; still
		// reject a closed/exited owner before submitting any request.
		authorityErr = client.channel.authorizePresence(ctx)
	}
	authorityElapsed += time.Since(start)
	sample[1] = timingBucket(authorityElapsed)
	if command.op != opHeartbeat {
		handoff.Record(ctx, handoff.ClientAuthority, authorityElapsed)
	}
	if authorityErr != nil {
		if ctx.Err() != nil && !client.channel.authority.fenced.Load() {
			// The read-only query already completed. Cancellation before bytes
			// does not require retiring its otherwise available process proof.
			return notSent(ctx.Err())
		}
		client.fail()
		return notSent(errors.Join(ErrExpired, ctx.Err()))
	}
	// Native authorization can outlast the caller or the exact lease. No
	// request has been submitted yet, so retain not-sent certainty and the
	// sequence; cancellation alone must not retire an otherwise live channel.
	if err := ctx.Err(); err != nil {
		return notSent(err)
	}
	if !time.Now().Before(client.lease.ExpiresAt) {
		client.fail()
		return notSent(ErrExpired)
	}
	if command.op != opHeartbeat {
		command.applyDetail = handoff.WantWorkerApply(ctx)
		command.submissionDetail = command.applyDetail
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
	// All writes and hard stops remain interruptible. A verified desktop
	// transition can drain only this fully written input's matching ACK,
	// bounded by the original request deadline and immutable lease expiry.
	var submitted atomic.Bool
	joinCancellation := requestCancellation(ctx, client.channel, deadline, command.op != opHeartbeat, &submitted)
	defer joinCancellation()
	client.sequence++
	start = time.Now()
	n, err := stream.Write(data[:])
	if err == nil && n == len(data) {
		submitted.Store(true)
	}
	elapsed2 := time.Since(start)
	sample[2] = timingBucket(elapsed2)
	if command.op != opHeartbeat {
		handoff.Record(ctx, handoff.ClientWrite, elapsed2)
	}
	if err != nil || n != len(data) {
		client.fail()
		var unsubmitted *unsubmittedPipeWrite
		if n == 0 && errors.As(err, &unsubmitted) {
			return notSent(errors.Join(ErrProtocol, err))
		}
		// Even a zero-byte write error does not prove that a prior native
		// overlapped write was not consumed. Only the native admission checks
		// above prove no WriteFile submission; transport loss remains unknown.
		return unknownInput(ErrProtocol)
	}
	start = time.Now()
	var ackRead *handoff.ACKReadProfile
	if command.applyDetail {
		ackRead = &handoff.ACKReadProfile{}
	}
	if client.ackReader == nil {
		client.ackReader = newACKReader(stream)
	}
	ack, err := readResponseObserved(client.ackReader, ackRead)
	elapsed3 := time.Since(start)
	sample[3] = timingBucket(elapsed3)
	if command.op != opHeartbeat {
		handoff.Record(ctx, handoff.ClientResponse, elapsed3)
	}
	if ackRead != nil {
		if err == nil && ack.sequence == command.sequence && ack.op == command.op {
			ackRead.Progress = handoff.ACKReadMatched
		}
		handoff.RecordACKRead(ctx, ackRead)
	}
	if err != nil || ack.sequence != command.sequence || ack.op != command.op {
		client.observeFailure(ackFailureCode(err))
		client.observeFailure("installed_input_failure_ack_unconfirmed")
		client.fail()
		return unknownInput(ErrProtocol)
	}
	copy(sample[4:], ack.timing[:])
	client.observeDesktop(ack.desktop)
	if command.op != opHeartbeat {
		handoff.WorkerDetail(ctx, ack.timing, ack.millis, ack.measured)
		if command.applyDetail {
			handoff.RecordWorkerApply(ctx, ack.applyDetail)
		}
	}
	if ack.status == 0 {
		client.lastAcknowledged = time.Now()
		if observed != nil {
			observed(ack)
		}
		return nil
	}
	if code := ack.failureStage.code(); code != "" {
		client.observeFailure(code)
	}
	if ack.timing[2] != 0 {
		client.observeFailure("installed_input_failure_after_dispatch")
	} else if ack.timing[0] != 0 {
		client.observeFailure("installed_input_failure_before_apply")
	} else {
		client.observeFailure("installed_input_failure_stage_unknown")
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

// Fixed phase evidence only; the existing observer contract forbids reentry.
func (client *Client) observeFailure(code string) {
	if client.timing.emit != nil {
		emitTiming(client.timing.emit, code)
	}
}

// Close interrupts pending I/O and triggers worker-owned recovery. A nil
// result proves channel cleanup only; it is not a release-recovery receipt.
func (client *Client) Close() error {
	if client != nil && client.localClose != nil {
		return client.localClose()
	}
	if client == nil || client.channel == nil {
		return nil
	}
	err := client.channel.Close()
	client.mu.Lock()
	client.failed = true
	client.timing.flush(time.Now())
	client.heartbeatTiming.flush(time.Now())
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
	return readResponseObserved(reader, nil)
}

func readResponseObserved(reader io.Reader, profile *handoff.ACKReadProfile) (response, error) {
	var observer *ackReadObserver
	if profile != nil {
		observer = &ackReadObserver{reader: reader, profile: profile, started: time.Now()}
		reader = observer
	}
	var data [responseSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		observer.headerResult(false)
		return response{}, errors.Join(ErrProtocol, err)
	}
	value, err := decodeResponse(data)
	observer.headerResult(err == nil)
	if err != nil {
		return response{}, err
	}
	err = readResponseSuffix(reader, data[4], &value)
	observer.suffixResult(err == nil)
	if err != nil {
		return response{}, err
	}
	return value, nil
}

func ackFailureCode(err error) string {
	if err == nil {
		return "installed_input_failure_ack_mismatch"
	}
	var timeout net.Error
	switch {
	case errors.As(err, &timeout) && timeout.Timeout():
		return "installed_input_failure_ack_timeout"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "installed_input_failure_ack_eof"
	case errors.Is(err, net.ErrClosed), errors.Is(err, io.ErrClosedPipe):
		return "installed_input_failure_ack_closed"
	default:
		return "installed_input_failure_ack_unclassified"
	}
}

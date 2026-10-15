//go:build windows

package inputworker

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

// CaptureSource owns only authenticated local capture IPC. It has no peer
// socket, file/clipboard operation, injection port or caller-selected desktop.
type CaptureSource struct {
	service *AgentService
	mu      sync.Mutex
	current *sourceClient
	observe func(string) // configured before Run; fixed timing/route codes only.
}
type sourceClient struct {
	channel                 *Channel
	binding                 Binding
	mu                      sync.Mutex
	commandWaiting          atomic.Int32 // Published before mu; idle maintenance yields to queued commands.
	sequence                uint64
	lastReplyRequestAt      time.Time // Under mu; request start, only after its exact successful reply.
	epoch                   atomic.Uint64
	retired                 atomic.Bool
	retiring                atomic.Bool
	pending                 sourcePacket // Under mu; only the fully written request being retired.
	failed                  chan struct{}
	replies                 chan sourcePacket
	done                    chan struct{}
	err                     error
	timing, heartbeatTiming timingSampler // Protected by mu; no input payloads.
	lockWait                time.Duration
}

func (s *AgentService) NewSourceCapture() *CaptureSource          { return &CaptureSource{service: s} }
func (c *CaptureSource) SourceCapture() wininput.SourceActivation { return c }
func (c *CaptureSource) client() *sourceClient                    { c.mu.Lock(); defer c.mu.Unlock(); return c.current }
func (c *CaptureSource) Active() bool {
	live := c.client()
	return live != nil && live.epoch.Load() != 0
}
func (c *CaptureSource) Activate() error {
	live := c.client()
	if live == nil {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := live.call(ctx, sourceActivate)
	return err
}
func (c *CaptureSource) Deactivate() {
	live := c.client()
	if live == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := live.call(ctx, sourceDeactivate); err != nil {
		if !live.retiring.Load() {
			_ = live.channel.Close()
		}
	}
}
func (c *CaptureSource) AdmitBoundary(ctx context.Context, _ control.BoundaryIntent) (peer.BoundaryAdmissionDecision, error) {
	live := c.client()
	if live == nil {
		return 0, peer.ErrBoundaryInspectionUnavailable
	}
	p, err := live.call(ctx, sourceAdmit)
	return sourceAdmissionResult(p, err)
}

func (c *sourceClient) call(ctx context.Context, op byte, reference ...uint64) (sourcePacket, error) {
	c.commandWaiting.Add(1)
	defer c.commandWaiting.Add(-1)
	started := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lockWait = time.Since(started)
	return c.callLocked(ctx, op, reference...)
}

// Caller holds mu through the entire command and exact reply observation.
func (c *sourceClient) callLocked(ctx context.Context, op byte, reference ...uint64) (response sourcePacket, result error) {
	sample := [7]byte{timingBucket(c.lockWait)}
	c.lockWait = 0
	sampler := &c.timing
	if op == sourceHeartbeat {
		sampler = &c.heartbeatTiming
	}
	stage := "context"
	defer func() {
		now := time.Now()
		sampler.observe(sample, now)
		if result != nil || op == sourceStop {
			sampler.flush(now)
		}
		if result != nil && sampler.emit != nil {
			emitTiming(sampler.emit, "installed_source_failure_"+stage)
		}
	}()
	if op == sourceStop {
		select {
		case <-c.done:
			if c.retired.Load() && c.err == nil {
				return sourcePacket{}, nil
			}
		default:
		}
	}
	accepted := false
	defer func() {
		if !accepted {
			c.epoch.Store(0)
			_ = c.channel.Close()
		}
	}()
	if ctx == nil || ctx.Err() != nil {
		accepted = true // No request was started; a cancelled optional query cannot retire this owner.
		return sourcePacket{}, ErrAuthority
	}
	if c.retiring.Load() && op != sourceStop {
		accepted = true // Only shutdown may follow an outcome still being drained.
		return sourcePacket{}, ErrAuthority
	}
	if op == sourceStop && c.pending.sequence != 0 {
		if err := c.drainPending(ctx); err != nil {
			return sourcePacket{}, err
		}
		select {
		case <-c.done:
			if c.retired.Load() && c.err == nil {
				accepted = true
				return sourcePacket{}, nil
			}
		default:
		}
	}
	stage = "authority"
	started := time.Now()
	authorityErr := c.channel.authority.check(false)
	sample[1] = timingBucket(time.Since(started))
	if authorityErr != nil {
		return sourcePacket{}, ErrAuthority
	}
	select {
	case <-c.done:
		return sourcePacket{}, ErrAuthority
	default:
	}
	c.sequence++
	if op == sourceActivate {
		c.epoch.Store(c.sequence)
	}
	if op == sourceDeactivate {
		c.epoch.Store(0)
	}
	p := sourcePacket{kind: sourceCommand, op: op, sequence: c.sequence, binding: c.binding}
	if op == sourceCursorShow && len(reference) == 1 {
		p.reply = reference[0]
	}
	data, err := encodeSourcePacket(p)
	if err != nil {
		return sourcePacket{}, err
	}
	limit := time.Now().Add(time.Second)
	if op == sourceStop {
		limit = time.Now().Add(RecoveryTimeout + time.Second)
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(limit) {
		limit = deadline
	}
	stage = "write"
	started = time.Now()
	requestStarted := started
	writeErr := c.channel.stream.SetWriteDeadline(limit)
	if writeErr == nil {
		writeErr = writeFrame(c.channel.stream, data[:])
	}
	sample[2] = timingBucket(time.Since(started))
	if writeErr != nil {
		c.epoch.Store(0)
		_ = c.channel.Close()
		return sourcePacket{}, ErrAuthority
	}
	timer := time.NewTimer(time.Until(limit))
	defer timer.Stop()
	stage = "response"
	started = time.Now()
	reply, known := waitSourceReply(ctx, timer.C, c.replies, c.done)
	sample[3] = timingBucket(time.Since(started))
	if known {
		result, err := sourceReplyResult(p, reply)
		if err == ErrProtocol {
			stage = "protocol"
		}
		accepted = err != ErrProtocol
		if err == nil {
			// A completed authenticated command already supplies liveness.
			// Use its request start, never a late reply's completion, so slow
			// responses cannot postpone the worker's unchanged idle deadline.
			c.lastReplyRequestAt = requestStarted
		}
		if err != nil && op == sourceActivate {
			c.epoch.Store(0)
		}
		return result, err
	}
	select {
	case <-c.done:
		if op == sourceStop && c.retired.Load() && c.err == nil {
			accepted = true // Independent authenticated native retirement proof.
			return sourcePacket{}, nil
		}
	default:
	}
	if drainableSourceResponse(op, c.epoch.Load()) && !time.Now().Before(limit) {
		// No replay or further capture command is allowed. Keep the authenticated
		// pipe solely to drain this exact reply and request native retirement.
		// An idle heartbeat does not certify retirement or grant new authority.
		c.pending = p
		c.epoch.Store(0)
		accepted = true
		if !c.retiring.Swap(true) && c.failed != nil {
			close(c.failed)
		}
		return sourcePacket{}, ErrAuthority
	}
	c.epoch.Store(0)
	_ = c.channel.Close()
	return sourcePacket{}, ErrAuthority
}

func (c *CaptureSource) RunWithBoundary(ctx context.Context, session *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) (result error) {
	if ctx == nil || session == nil || boundary == nil || ready == nil || c.service == nil {
		return ErrAuthority
	}
	binding, err := NewBinding()
	if err != nil {
		return err
	}
	id := hex.EncodeToString(binding[:])
	lease := Lease{Identity: application.LeaseIdentity{SessionID: id, LeaseID: id, Epoch: id}, Binding: binding, ExpiresAt: time.Now().Add(maxLeaseDuration - time.Second), Capture: true}
	data, err := encodeLease(lease, time.Now())
	if err != nil {
		return err
	}
	channel, err := connectInstalledNamed(ctx, c.service.digest, c.service.installation, sourcePipeName)
	if err != nil {
		return err
	}
	defer channel.Close()
	startup, stopStartup := context.WithTimeout(ctx, startupTimeout)
	stopPipe := context.AfterFunc(startup, func() { _ = channel.Close() })
	deadline, _ := startup.Deadline()
	if channel.stream.SetDeadline(deadline) != nil || writeFrame(channel.stream, data[:]) != nil {
		stopPipe()
		stopStartup()
		return ErrAuthority
	}
	_, retired, readyErr := readSourceStartup(channel.stream, binding)
	err = readyErr
	if !stopPipe() {
		err = ErrAuthority
	}
	stopStartup()
	if err == wininput.ErrEmergencyRelease {
		return err
	}
	if err != nil || ctx.Err() != nil || channel.stream.SetDeadline(time.Time{}) != nil {
		return ErrAuthority
	}
	if retired {
		return errors.Join(wininput.ErrProtectedInput, ErrSourceCaptureRetired)
	}
	live := &sourceClient{channel: channel, binding: binding, replies: make(chan sourcePacket, 1), done: make(chan struct{}), failed: make(chan struct{})}
	live.timing = timingSampler{emit: c.observe, source: true}
	live.heartbeatTiming = timingSampler{emit: c.observe, source: true, heartbeat: true}
	go func() {
		defer close(live.done)
		var sequence uint64
		for {
			p, err := readSourcePacket(channel.stream)
			if err != nil {
				live.err = err
				return
			}
			if p.binding != binding || p.sequence != sequence+1 {
				live.err = ErrProtocol
				return
			}
			sequence = p.sequence
			switch p.kind {
			case sourceReply:
				select {
				case live.replies <- p:
				default:
					live.err = ErrProtocol
					return
				}
			case sourceBoundary:
				if ctx.Err() == nil {
					if err := boundary.Observe(p.action, p.x); err != nil && !errors.Is(err, control.ErrBoundaryIntentQueueFull) {
						live.err = err
						return
					}
				}
			case sourceAction:
				if ctx.Err() == nil && p.reply == live.epoch.Load() {
					session.Offer(p.action)
				}
			case sourceReturn:
				if ctx.Err() == nil && p.reply == live.epoch.Load() {
					session.RequestLocalReturn()
				}
			case sourceRetired:
				if p.status == 0 {
					if p.op == 1 {
						live.err = wininput.ErrEmergencyRelease
					}
					live.retired.Store(true)
				} else {
					live.err = ErrRecovery
				}
				return
			default:
				live.err = ErrProtocol
				return
			}
		}
	}()
	defer func() { _ = channel.Close(); <-live.done }()
	if _, err = live.call(ctx, sourceStart); err != nil {
		return err
	}
	c.mu.Lock()
	if c.current != nil {
		c.mu.Unlock()
		return ErrAuthority
	}
	c.current = live
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.current = nil; c.mu.Unlock() }()
	ready()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	heartbeatDeferred := false
	for {
		select {
		case <-live.failed:
			cleanup, cancel := context.WithTimeout(context.Background(), RecoveryTimeout+time.Second)
			_, err := live.call(cleanup, sourceStop)
			cancel()
			if err != nil {
				return errors.Join(err, ErrRecovery)
			}
			return errors.Join(wininput.ErrProtectedInput, ErrSourceCaptureRetired)
		case <-ctx.Done():
			// Native owners must acknowledge retirement before the resident may
			// start ordinary desktop hooks. Missing proof forces full recovery.
			if live.retired.Load() {
				return ctx.Err()
			}
			cleanup, cancel := context.WithTimeout(context.Background(), RecoveryTimeout+time.Second)
			_, err := live.call(cleanup, sourceStop)
			cancel()
			if err != nil && !live.retired.Load() {
				return errors.Join(ctx.Err(), ErrRecovery)
			}
			return ctx.Err()
		case <-live.done:
			if result, confirmed := sourceRetirementResult(ctx, live.retired.Load(), live.err); confirmed {
				return result
			}
			if live.retired.Load() {
				return live.err
			}
			return errors.Join(live.err, ErrRecovery)
		case <-ticker.C:
			if live.retiring.Load() {
				continue
			} // The owner must drain/stop, not heartbeat.
			// Finish the already bounded heartbeat before explicit retirement.
			// Cancelling a source refresh must not cut the ACK stream mid-frame.
			attempted, err := live.pollHeartbeat(ctx)
			if !attempted && err == nil && !heartbeatDeferred {
				heartbeatDeferred = true
				if c.observe != nil {
					emitTiming(c.observe, "installed_source_heartbeat_deferred")
				}
			}
			if err != nil && ctx.Err() == nil {
				if live.retiring.Load() {
					continue
				} // Idle heartbeat/cursor timeout won the IPC mutex.
				if live.retired.Load() {
					if live.err != nil {
						return live.err
					}
					return errors.Join(wininput.ErrProtectedInput, ErrSourceCaptureRetired)
				}
				if c.observe != nil {
					emitTiming(c.observe, "installed_source_heartbeat_failed")
				}
				return errors.Join(err, ErrRecovery)
			}
		}
	}
}

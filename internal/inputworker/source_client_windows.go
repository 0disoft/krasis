//go:build windows

package inputworker

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/syncgate"
	"github.com/0disoft/krasis/internal/wininput"
)

// Activation can cross a scheduler/desktop dispatch delay. Its exact ACK may
// use the existing owner idle budget, never extend an already active owner.
const sourceActivationReplyTimeout = IdleTimeout

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
	mu                      syncgate.Mutex
	commandWaiting          atomic.Int32 // Published before mu; idle maintenance yields to queued commands.
	preflightExpired        atomic.Bool  // At most one fixed diagnostic per capture owner.
	commandPreflightExpired atomic.Bool  // Unsent command query exhausted its caller budget.
	sequence                uint64
	lastReplyRequestAt      time.Time // Under mu; request start, only after its exact successful reply.
	cursorPrepared          bool      // Under mu; pending native cursor presentation retains the active idle bound.
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
	return c.ActivateProfile(context.Background())
}
func (c *CaptureSource) ActivateProfile(profile context.Context) error {
	if profile == nil {
		profile = context.Background()
	}
	live := c.client()
	if live == nil {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(profile, sourceActivationReplyTimeout)
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
	if err == nil && c.observe != nil {
		if code := sourcePressedCode(p.pressedEvidence); code != "" {
			c.observe(code)
		}
	}
	return sourceAdmissionResult(p, err)
}

// Caller holds mu through the entire command and exact reply observation.
func (c *sourceClient) callLocked(ctx context.Context, op byte, reference ...uint64) (response sourcePacket, result error) {
	return c.callLockedChecked(ctx, op, nil, reference...)
}

// Only pollHeartbeat can supply a fresh check made immediately before TryLock.
// It cannot authorize a capture/cursor command or survive a wait for the gate.
func (c *sourceClient) callLockedChecked(ctx context.Context, op byte, heartbeat *sourceHeartbeatCheck, reference ...uint64) (response sourcePacket, result error) {
	return c.callLockedAuthorized(ctx, op, heartbeat, nil, reference...)
}

func (c *sourceClient) callLockedAuthorized(ctx context.Context, op byte, heartbeat *sourceHeartbeatCheck, command *sourceCommandCheck, reference ...uint64) (response sourcePacket, result error) {
	sample := [7]byte{timingBucket(c.lockWait)}
	if op == sourceActivate {
		handoff.RecordCapture(ctx, handoff.CaptureIPCLock, c.lockWait)
	}
	c.lockWait = 0
	sampler := &c.timing
	if op == sourceHeartbeat {
		sampler = &c.heartbeatTiming
	}
	stage := "context"
	replyOutcome := ""
	unsentExpired := false
	defer func() {
		now := time.Now()
		sampler.observe(sample, now)
		if unsentExpired {
			if sampler.emit != nil {
				emitTiming(sampler.emit, "installed_source_heartbeat_unsent_expired")
			}
			return
		}
		if result != nil || op == sourceStop {
			sampler.flush(now)
		}
		if result != nil && sampler.emit != nil {
			emitTiming(sampler.emit, "installed_source_failure_"+stage)
			emitTiming(sampler.emit, sourceCommandFailureCode(op))
			var write *sourceIOFailure
			if errors.As(result, &write) {
				emitTiming(sampler.emit, "installed_source_io_stage_"+write.stage)
				emitTiming(sampler.emit, "installed_source_io_"+sourceIOFailureReason(write))
				emitTiming(sampler.emit, connectionNativeFailureEvent(write, "installed_source_native_"))
			}
			if replyOutcome != "" {
				emitTiming(sampler.emit, "installed_source_response_"+replyOutcome)
			}
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
	if ctx == nil {
		accepted = true // No request was started; a cancelled optional query cannot retire this owner.
		return sourcePacket{}, ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		accepted = true
		return sourcePacket{}, err
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
	var authorityErr error
	var authorityElapsed time.Duration
	if command != nil {
		authorityErr = command.validate(c, ctx, op)
		authorityElapsed = command.elapsed + time.Since(started)
	} else if heartbeat == nil {
		authorityErr = c.channel.authorizeInput(ctx, false)
		authorityElapsed = time.Since(started)
	} else {
		authorityErr = heartbeat.validate(c, ctx, op)
		authorityElapsed = heartbeat.elapsed
	}
	sample[1] = timingBucket(authorityElapsed)
	if op == sourceActivate {
		handoff.RecordCapture(ctx, handoff.CaptureIPCAuthority, authorityElapsed)
	}
	if authorityErr != nil {
		if ctx.Err() != nil {
			// Proof admission can race the entry check above. No request was
			// sent; a fenced pending native query still requires retirement.
			accepted = true
			if c.channel.authority.fenced.Load() {
				c.epoch.Store(0)
			}
			return sourcePacket{}, ctx.Err()
		}
		if op == sourceStop && c.awaitRetirement(ctx) == nil {
			accepted = true
			return sourcePacket{}, nil
		}
		return sourcePacket{}, ErrAuthority
	}
	if ctx.Err() != nil {
		// Cancellation can race fresh proof completion. No command was written:
		// retain the sequence and never publish a new epoch from this proof.
		accepted = true
		return sourcePacket{}, ctx.Err()
	}
	select {
	case <-c.done:
		if op == sourceStart {
			if result, confirmed := sourceRetirementResult(ctx, c.retired.Load(), c.err); confirmed {
				accepted = true
				return sourcePacket{}, result
			}
		}
		return sourcePacket{}, ErrAuthority
	default:
	}
	limit := time.Now().Add(time.Second)
	if op == sourceActivate {
		limit = time.Now().Add(sourceActivationReplyTimeout)
		if !c.lastReplyRequestAt.IsZero() {
			idle := c.lastReplyRequestAt.Add(sourceReadTimeout(true, c.epoch.Load() != 0 || c.cursorPrepared))
			if idle.Before(limit) {
				limit = idle
			}
		}
	}
	if op == sourceHeartbeat {
		limit = time.Now().Add(sourceHeartbeatReplyTimeout)
	}
	if op == sourceStop {
		limit = time.Now().Add(RecoveryTimeout + time.Second)
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(limit) {
		limit = deadline
	}
	// An expired existing owner cannot be activated by writing a fresh request.
	if op == sourceActivate && !time.Now().Before(limit) {
		return sourcePacket{}, ErrAuthority
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
	stage = "write"
	started = time.Now()
	requestStarted := started
	writeErr := writeSourceCommand(c.channel.stream, data[:], limit)
	sample[2] = timingBucket(time.Since(started))
	if op == sourceActivate {
		handoff.RecordCapture(ctx, handoff.CaptureIPCWrite, time.Since(started))
	}
	if writeErr != nil {
		var unsent *sourceIOFailure
		if op == sourceHeartbeat && errors.As(writeErr, &unsent) && unsent.unsubmitted && errors.Is(unsent, os.ErrDeadlineExceeded) {
			// mu still owns the sequence, and the native pipe proves WriteFile
			// was never called. Discard this unsent maintenance attempt only.
			// No ACK, traffic, pending request or idle renewal is fabricated.
			c.sequence--
			accepted, unsentExpired = true, true
			return sourcePacket{}, context.DeadlineExceeded
		}
		c.epoch.Store(0)
		_ = c.channel.Close()
		return sourcePacket{}, writeErr
	}
	timer := time.NewTimer(time.Until(limit))
	defer timer.Stop()
	stage = "response"
	started = time.Now()
	reply, known, observed := waitSourceReplyObserved(ctx, timer.C, c.replies, c.done)
	replyOutcome = observed
	sample[3] = timingBucket(time.Since(started))
	if op == sourceActivate {
		handoff.RecordCapture(ctx, handoff.CaptureIPCResponse, time.Since(started))
	}
	if known {
		result, err := sourceReplyResult(p, reply)
		if err != ErrProtocol {
			copy(sample[4:], reply.timing[:])
			if op == sourceActivate {
				handoff.RecordCaptureWorker(ctx, reply.timing)
			}
		}
		if err == ErrProtocol {
			stage = "protocol"
			replyOutcome = "protocol"
		} else if err != nil {
			replyOutcome = "rejected"
		}
		accepted = err != ErrProtocol
		if err == nil {
			// A completed authenticated command already supplies liveness.
			// Use its request start, never a late reply's completion, so slow
			// responses cannot postpone the worker's unchanged idle deadline.
			c.lastReplyRequestAt = requestStarted
			switch op {
			case sourceCursorPrepare:
				c.cursorPrepared = true
			case sourceCursorShow, sourceCursorRestore:
				c.cursorPrepared = false
			}
		}
		if err != nil && op == sourceActivate {
			c.epoch.Store(0)
		}
		return result, err
	}
	select {
	case <-c.done:
		// sourceStart only establishes the capture stream; it dispatches no
		// input. A complete independent retirement may replace its missing
		// ACK with a terminal transition, never successful readiness. An
		// observed negative/foreign ACK has already returned above.
		if op == sourceStart {
			if result, confirmed := sourceRetirementResult(ctx, c.retired.Load(), c.err); confirmed {
				accepted = true
				return sourcePacket{}, result
			}
		}
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
		if c.observe != nil {
			c.observe("installed_source_connect_failed")
			c.observe(connectionFailureEvent(err, "installed_source_stage_"))
			c.observe(connectionNativeFailureEvent(err, "installed_source_native_"))
		}
		return err
	}
	var live *sourceClient
	startupStage := "lease_deadline"
	defer func() {
		if startupStage != "" && result != nil && c.observe != nil {
			c.observe("installed_source_startup_failed")
			c.observe("installed_source_stage_" + startupStage)
			c.observe(connectionNativeFailureEvent(result, "installed_source_native_"))
			c.observe(sourceStartupFailureCode(result))
		}
	}()
	defer func() {
		if live == nil {
			result = closeSourceChannelResult(channel, result)
		} else {
			result = live.closeResult(result)
		}
	}()
	startup, stopStartup := context.WithTimeout(ctx, startupTimeout)
	stopPipe := context.AfterFunc(startup, func() { _ = channel.Close() })
	deadline, _ := startup.Deadline()
	if err := channel.stream.SetDeadline(deadline); err != nil {
		stopPipe()
		stopStartup()
		return errors.Join(ErrAuthority, err)
	}
	startupStage = "lease_write"
	if err := writeFrame(channel.stream, data[:]); err != nil {
		stopPipe()
		stopStartup()
		return errors.Join(ErrAuthority, err)
	}
	startupStage = "worker_ready"
	_, retired, readyErr := readSourceStartup(channel.stream, binding)
	err = readyErr
	if !stopPipe() {
		err = ErrAuthority
	}
	stopStartup()
	if err == wininput.ErrEmergencyRelease {
		return err
	}
	if err != nil || ctx.Err() != nil {
		return errors.Join(ErrAuthority, err, ctx.Err())
	}
	if retired {
		// This terminal proof needs no live channel. The service may already
		// have closed it after joining its independent final worker report.
		startupStage = ""
		if c.observe != nil {
			c.observe("installed_source_startup_retired")
		}
		return errors.Join(wininput.ErrProtectedInput, ErrSourceCaptureRetired)
	}
	startupStage = "client_validation"
	if err := channel.stream.SetDeadline(time.Time{}); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	startupStage = ""
	live = &sourceClient{channel: channel, binding: binding, replies: make(chan sourcePacket, 1), done: make(chan struct{}), failed: make(chan struct{})}
	live.timing = timingSampler{emit: c.observe, source: true}
	live.heartbeatTiming = timingSampler{emit: c.observe, source: true, heartbeat: true}
	go func() {
		defer func() {
			code := sourceClientReaderCode(live.err, live.retired.Load())
			close(live.done) // Observability must not delay the terminal signal.
			if c.observe != nil {
				emitTiming(c.observe, code)
			}
		}()
		var sequence uint64
		reader := newSourcePacketReader(channel.stream)
		for {
			p, err := readSourcePacket(reader)
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
	if _, err = live.call(ctx, sourceStart); err != nil {
		if CleanSourceRetirement(err) && c.observe != nil {
			c.observe("installed_source_startup_retired")
		}
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

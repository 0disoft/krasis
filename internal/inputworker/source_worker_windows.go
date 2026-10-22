//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/syncgate"
	"github.com/0disoft/krasis/internal/wininput"
)

type sourceWriter struct {
	ctx       context.Context
	mu        syncgate.Mutex
	channel   *Channel
	binding   Binding
	sequence  uint64
	broken    bool // Under mu; an attempted failed write cannot carry further frames.
	epoch     atomic.Uint64
	started   atomic.Bool
	heartbeat *sourceHeartbeatProgress
}

func (w *sourceWriter) send(p sourcePacket, current func() bool) error {
	return w.write(p, current, false)
}
func (w *sourceWriter) write(p sourcePacket, current func() bool, retirement bool) error {
	return w.writePacket(p, current, retirement, false)
}

func (w *sourceWriter) writePacket(p sourcePacket, current func() bool, retirement, completedReply bool) error {
	return w.writePacketContext(nil, p, current, retirement, completedReply)
}

func (w *sourceWriter) writePacketContext(ctx context.Context, p sourcePacket, current func() bool, retirement, completedReply bool) error {
	if !w.started.Load() && (p.kind == sourceAction || p.kind == sourceBoundary || p.kind == sourceReturn) {
		return nil
	}
	ctx, cancel := w.operation(ctx, retirement || completedReply)
	defer cancel()
	var progress *sourceHeartbeatProgress
	if p.kind == sourceReply && p.op == sourceHeartbeat {
		progress = w.heartbeat
	}
	if progress != nil {
		ctx = progress.presence.context(ctx)
	}
	gateDone := func() {}
	if progress != nil {
		gateDone = progress.measure(heartbeatReplyGate)
	}
	if retirement || completedReply || p.kind == sourceReply && p.op == sourceHeartbeat {
		if err := w.mu.LockContext(ctx); err != nil {
			gateDone()
			return sourceTransportFailure(err)
		}
	} else {
		// Full WTS/SCM authorization may block. It must not own the wire gate
		// needed by keepalive replies. Admit a fresh proof only with immediate
		// TryLock success; after contention, wait without a proof and recheck.
		if err := w.lockAuthorized(ctx, false); err != nil {
			return sourceTransportFailure(err)
		}
	}
	gateDone()
	defer w.mu.Unlock()
	if !w.started.Load() && (p.kind == sourceAction || p.kind == sourceBoundary || p.kind == sourceReturn) {
		return nil
	}
	if current != nil && !current() {
		return control.ErrSourceActionDiscarded
	}
	// Retirement grants no authority. It is sent only after native/sender join
	// on the exact already authenticated channel, even if logon authority changed.
	if !retirement {
		// Input and observations retain their full check above. A completed
		// command reply reports an already known outcome, not new authority.
		// Recheck the pinned owner's presence after admission and before bytes.
		presenceDone := func() {}
		if progress != nil {
			presenceDone = progress.measure(heartbeatReplyPresence)
		}
		err := w.channel.authorizeSourcePresence(ctx)
		presenceDone()
		if err != nil {
			return sourceTransportFailure(err)
		}
	}
	p.sequence, p.binding = w.sequence+1, w.binding
	if progress != nil {
		p.timing = progress.replyTiming()
	}
	data, err := encodeSourcePacket(p)
	if err != nil {
		return err
	}
	writeDone := func() {}
	if progress != nil {
		writeDone = progress.measure(heartbeatReplyWrite)
	}
	origin := p.kind
	if retirement {
		origin = sourceRetired // A stop ACK is also retirement delivery.
	}
	err = w.writeSequenced(ctx, data[:], p.sequence, origin)
	writeDone()
	if progress != nil && err == nil {
		progress.stage = heartbeatReplied
	}
	return err
}

// Caller owns mu. Only native pre-submission proof (or failure to configure a
// deadline) can leave the sequence untouched. No failed operation is replayed:
// this permits only a later, distinct joined-retirement frame to remain valid.
// Partial/attempted failures poison the stream, even when Write reports zero.
func (w *sourceWriter) writeSequenced(ctx context.Context, data []byte, sequence uint64, origin byte) error {
	if w.broken {
		return sourceTransportFailure(ErrProtocol)
	}
	if err := ctx.Err(); err != nil {
		return sourceTransportFailure(err)
	}
	deadline, _ := ctx.Deadline()
	err := writeSourceCommand(w.channel.stream, data, deadline)
	if failure, ok := err.(*sourceIOFailure); ok {
		failure.origin = origin // The failed local call has one owner.
	}
	if err != nil {
		var failure *sourceIOFailure
		if errors.As(err, &failure) && (failure.stage == "deadline" || failure.unsubmitted) {
			return sourceTransportFailure(err)
		}
		w.broken = true
	}
	w.sequence = sequence
	return sourceTransportFailure(err)
}

func (w *sourceWriter) SendAction(ctx context.Context, action control.InputAction, current func() bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return w.writePacketContext(ctx, sourcePacket{kind: sourceAction, reply: w.epoch.Load(), action: action}, current, false, false)
}
func (w *sourceWriter) Interrupt(error) error {
	return sourceTransportFailure(w.channel.stream.SetWriteDeadline(time.Now()))
}
func (w *sourceWriter) Fail(context.Context, error) error { return nil }

type sourcePointerQueue struct {
	events    chan sourcePacket
	emergency wininput.EmergencyReleaseDetector
	session   *wininput.SourceSession
}

func (q *sourcePointerQueue) Observe(a control.InputAction, x int32) error {
	if q.emergency.Observe(a) {
		q.session.Abort(wininput.ErrEmergencyRelease)
		return wininput.ErrEmergencyRelease
	}
	if a.Kind == control.InputPointerMove {
		select {
		case q.events <- sourcePacket{kind: sourceBoundary, action: a, x: x}:
		default:
		}
	}
	return nil
}

// The child owns native suppression and physical pressed state. The medium
// agent keeps mTLS, outbound lease negotiation and the network action sender.
func runSourceCapture(ctx context.Context, channel, service *Channel, lease Lease, ready func() error, exit *logindiag.SourceExit) (result error) {
	if exit == nil {
		return ErrProtocol
	}
	*exit = emptySourceExit()
	if !lease.Capture || lease.Presentation {
		return ErrProtocol
	}
	pipe, ok := channel.stream.(*pipeConn)
	if !ok {
		return ErrProtocol
	}
	run, cancel := context.WithDeadline(ctx, lease.ExpiresAt)
	defer cancel()
	authorize := sourceCaptureAuthorization(run, service, channel)
	var heartbeat sourceHeartbeatProgress
	authorizeCommand := func(op byte) error {
		return authorizeSourceReadProgress(run, service, channel, op, &heartbeat)
	}
	hook, err := wininput.NewProtectedSourceHook(authorize)
	if err != nil {
		return err
	}
	hook.SetBoundaryGuard(authorize)
	var pressedEvidence atomic.Uint32
	hook.SetAdmissionReporter(func(code string) { pressedEvidence.Store(uint32(sourcePressedEvidence(code))) })
	writer := &sourceWriter{ctx: run, channel: channel, binding: lease.Binding, heartbeat: &heartbeat}
	session, err := wininput.NewSourceSession(1024, writer)
	if err != nil {
		return err
	}
	queue := &sourcePointerQueue{events: make(chan sourcePacket, 64), session: session}
	boundaryStart, boundaryDone := make(chan struct{}), make(chan error, 1)
	go func() {
		var err error
		select {
		case <-boundaryStart:
			err = pumpSourceBoundaries(run, queue.events, writer.sendBoundaries)
		case <-run.Done():
			err = run.Err()
		}
		boundaryDone <- err
		if err != nil {
			cancel()
		}
	}()
	sourceReady := make(chan struct{})
	hookReady := make(chan struct{})
	sourceDone, hookDone := make(chan error, 1), make(chan error, 1)
	go func() { sourceDone <- session.RunWithReady(run, func() { close(sourceReady) }); cancel() }()
	go func() {
		select {
		case <-sourceReady:
		case <-run.Done():
			hookDone <- run.Err()
			return
		}
		hookDone <- hook.RunWithBoundary(run, session, queue, func() { close(hookReady) })
		cancel()
	}()
	var stopping *sourcePacket
	var firstCommandFailure error
	defer func() {
		cancel()
		capture, sender := <-hookDone, <-sourceDone
		// No advisory writer may survive the final retirement frame. Joining
		// this sender also preserves a real preflight/write failure on cleanup.
		boundary := <-boundaryDone
		exit.Capture = "joined"
		if !sourceTransitionOnly(capture, run.Err() == context.DeadlineExceeded) && !wininput.SourceHookProgressRetiredOnly(capture) {
			exit.Capture = "failed"
		}
		if started, senderJoined, interruptJoined := session.CleanupCompletion(); started {
			exit.Sender, exit.Interrupt = joinedSourceState(senderJoined), joinedSourceState(interruptJoined)
		}
		// Preserve the first rejected command ahead of a later command failure.
		// Concrete native/authority details still take priority in the report.
		result = errors.Join(firstCommandFailure, result, boundary)
		emergency := errors.Is(errors.Join(result, capture, sender), wininput.ErrEmergencyRelease)
		result = sourceCaptureFinal(result, capture, sender, run.Err() == context.DeadlineExceeded)
		if stopping != nil {
			response := sourcePacket{kind: sourceReply, op: sourceStop, reply: stopping.sequence}
			response.status = sourceRetirementStatus(result)
			delivery := writer.write(response, nil, true)
			exit.Retirement = sourceRetirementOutcome(response.status, delivery)
			result = sourceRetirementDelivery(result, delivery)
		} else {
			response := sourcePacket{kind: sourceRetired}
			response.status = sourceRetirementStatus(result)
			if emergency && result == nil {
				response.op = 1
			}
			delivery := writer.write(response, nil, true)
			exit.Retirement = sourceRetirementOutcome(response.status, delivery)
			result = sourceRetirementDelivery(result, delivery)
		}
		if closeErr := channel.Close(); closeErr != nil {
			result = errors.Join(result, ErrRecovery, &workerReportFailure{operation: "source-channel-close", cause: closeErr})
		}
		// Both command/watchdog callers have joined. A service query may still
		// own native handles, so the service report must not claim cleanup yet.
		// Keep its control pipe open for the final report; bootstrap retires it.
		service.queryMu.Lock()
		pendingService := service.pendingQueriesLocked()
		service.queryMu.Unlock()
		if pendingService {
			result = errors.Join(result, ErrRecovery, &workerReportFailure{operation: "source-channel-close", cause: ErrAuthority})
		}
	}()
	select {
	case <-hookReady:
	case <-run.Done():
		return run.Err()
	}
	if ready() != nil {
		return ErrAuthority
	}
	// Events cannot precede the service's readiness receipt on the shared pipe.
	var sequence uint64
	readContext, stopReads := context.WithCancel(context.Background())
	defer stopReads()
	interruptReads := context.AfterFunc(run, stopReads)
	defer interruptReads()
	reader := sourceContextReader{ctx: readContext, read: pipe.readContext}
	var cursorPending atomic.Bool
	read := func() (sourcePacket, error) {
		deadline := time.Now().Add(sourceReadTimeout(writer.started.Load(), session.Active() || cursorPending.Load()))
		if err := channel.stream.SetReadDeadline(deadline); err != nil {
			return sourcePacket{}, sourceTransportFailure(errors.Join(ErrAuthority, err))
		}
		frame := &sourceFrameReader{Reader: reader, setDeadline: channel.stream.SetReadDeadline, deadline: deadline}
		p, err := readSourceCommandAuthorized(frame, readContext, lease.Binding, sequence, authorizeCommand)
		if err == nil {
			sequence = p.sequence
		}
		return p, err
	}
	first, err := read()
	if err != nil {
		exit.Reader = sourceReaderOutcome(err)
		return err
	}
	exit.Reader = "stopped"
	if first.op != sourceStart {
		exit.Reader = "invalid"
		return ErrProtocol
	}
	if err := writer.sendCompletedReply(first, sourcePacket{kind: sourceReply, op: sourceStart, reply: first.sequence}); err != nil {
		return err
	}
	writer.started.Store(true)
	close(boundaryStart)
	presenceDone := make(chan error, 1)
	go func() {
		err := watchSourcePresence(run, func() error { return authorizeSourceRead(run, service, channel, sourceHeartbeat) })
		presenceDone <- err
		if err != nil {
			cancel()
		}
	}()
	defer func() {
		cancel()
		if err := <-presenceDone; err != nil {
			exit.Reader = "authority"
			result = errors.Join(result, sourceTransportFailure(&sourceReaderFailure{operation: "source-reader-authority", cause: err}))
		}
	}()
	exit.Command = "ok"
	requests := make(chan sourcePacket, 1)
	completed := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- pumpSourceCommandsOrdered(run, read, requests, cancel, completed, func(p sourcePacket) error {
			return sourceCommandFailed(sourceHeartbeat, writer.sendCompletedReply(p, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: p.sequence}))
		})
	}()
	// Retire only this read; the final native response still needs the writer.
	defer func() {
		cancel()
		stopReads()
		readerErr := <-readDone
		heartbeat.snapshot(exit)
		exit.Reader = sourceReaderOutcome(readerErr)
		result = errors.Join(result, readerErr)
	}()
	var pendingReturn uint64
	var cursorPrepared func()
	var cursorSequence uint64
	return hook.WithSourceCommandDesktop(run, func(commandContext context.Context) error {
		for {
			work, err := nextSourceWork(run, requests, nil, session.LocalReturns())
			if err != nil {
				return err
			}
			switch p := work.packet; p.kind {
			case sourceReturn:
				pendingReturn = work.returnGeneration
				if err := writer.send(sourcePacket{kind: sourceReturn, reply: writer.epoch.Load()}, nil); err != nil {
					return err
				}
			case sourceCommand:
				if p.op == sourceStop {
					stopping = &p
					return nil
				}
				response := sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
				operation := func() error {
					switch p.op {
					case sourceAdmit:
						pressedEvidence.Store(0)
						decision, failure := hook.AdmitBoundary(run, control.BoundaryIntent{})
						response.decision = decision
						if failure == nil {
							response.pressedEvidence = byte(pressedEvidence.Load())
						}
						return failure
					case sourceActivate:
						writer.epoch.Store(p.sequence)
						if err := activateSourceSession(session); err != nil {
							writer.epoch.Store(0)
							return err
						}
						if err := hook.SourceCapture().Activate(); err != nil {
							session.Deactivate()
							writer.epoch.Store(0)
							return err
						}
					case sourceDeactivate:
						session.Deactivate()
						writer.epoch.Store(0)
						hook.SourceCapture().Deactivate()
						if hook.SourceCapture().Active() {
							return ErrRecovery
						}
						session.CompleteLocalReturn(pendingReturn)
						pendingReturn = 0
					case sourceHeartbeat:
					case sourceCursorPrepare:
						hook.RestoreReturnCursor()
						cursorPrepared = hook.PrepareReturnCursor()
						cursorSequence = p.sequence
						cursorPending.Store(cursorPrepared != nil)
						if cursorPrepared == nil {
							return ErrAuthority
						}
					case sourceCursorShow:
						if cursorPrepared == nil || p.reply != cursorSequence {
							return ErrProtocol
						}
						cursorPrepared()
						cursorPrepared = nil
						cursorPending.Store(false)
					case sourceCursorRestore:
						cursorPrepared = nil
						cursorPending.Store(false)
						cursorSequence = 0
						hook.RestoreReturnCursor()
					case sourceStart:
						return ErrProtocol
					default:
						return ErrProtocol
					}
					return nil
				}
				// State retirement operates on the existing native owner. It must
				// remain available while the desktop is between Default/Winlogon.
				timing, err := executeSourceCommand(commandContext, p.op, hook.ExecuteSourceDesktop, operation)
				response.timing = timing
				err = sourceCommandFailed(p.op, err)
				if exit.Command == "not_started" || exit.Command == "ok" {
					exit.Command = sourceCommandOutcome(err)
				}
				if p.op == sourceAdmit && err != nil {
					if !sourceRetryableInspection(err) {
						// No generic negative query ACK: it would erase authority or
						// cleanup failure before the worker's final report arrives.
						if sourceCleanupUncertain(err) {
							return errors.Join(ErrRecovery, err)
						}
						return err
					}
					err = &sourceInspectionRejected{cause: err}
				}
				if err != nil {
					if firstCommandFailure == nil {
						firstCommandFailure = err
					}
					response.status = 1
					response.decision = 0
					response.pressedEvidence = 0
				}
				if failure := sourceCommandCompletion(err, writer.sendCompletedReply(p, response)); failure != nil {
					return failure
				}
				select {
				case completed <- struct{}{}:
				case <-run.Done():
					return run.Err()
				}
			}
		}
	})
}

var _ peer.BoundaryAdmissionGate = (*CaptureSource)(nil)

//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

type sourceWriter struct {
	mu       sync.Mutex
	channel  *Channel
	binding  Binding
	sequence uint64
	epoch    atomic.Uint64
	started  atomic.Bool
}

func (w *sourceWriter) send(p sourcePacket, current func() bool) error {
	return w.write(p, current, false)
}
func (w *sourceWriter) write(p sourcePacket, current func() bool, retirement bool) error {
	w.mu.Lock()
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
		check := func() error { return w.channel.authority.check(false) }
		if p.kind == sourceReply && p.op == sourceHeartbeat {
			check = w.channel.authority.checkHeartbeat
		}
		if check() != nil {
			return sourceTransportFailure(ErrAuthority)
		}
	}
	w.sequence++
	p.sequence, p.binding = w.sequence, w.binding
	data, err := encodeSourcePacket(p)
	if err != nil {
		return err
	}
	if err := w.channel.stream.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return sourceTransportFailure(errors.Join(ErrAuthority, err))
	}
	return sourceTransportFailure(writeFrame(w.channel.stream, data[:]))
}

func (w *sourceWriter) SendAction(ctx context.Context, action control.InputAction, current func() bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return w.send(sourcePacket{kind: sourceAction, reply: w.epoch.Load(), action: action}, current)
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
func runSourceCapture(ctx context.Context, channel *Channel, proof *Authority, lease Lease, ready func() error, exit *logindiag.SourceExit) (result error) {
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
	authorize := func() error {
		if proof.check(false) != nil || channel.authority.check(false) != nil {
			return ErrAuthority
		}
		return nil
	}
	authorizeCommand := func(op byte) error {
		if op == sourceHeartbeat {
			if proof.checkHeartbeat() != nil || channel.authority.checkHeartbeat() != nil {
				return ErrAuthority
			}
			return nil
		}
		return authorize()
	}
	hook, err := wininput.NewProtectedSourceHook(authorize)
	if err != nil {
		return err
	}
	hook.SetBoundaryGuard(authorize)
	writer := &sourceWriter{channel: channel, binding: lease.Binding}
	session, err := wininput.NewSourceSession(1024, writer)
	if err != nil {
		return err
	}
	queue := &sourcePointerQueue{events: make(chan sourcePacket, 64), session: session}
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
		exit.Capture = "joined"
		if !sourceTransitionOnly(capture, run.Err() == context.DeadlineExceeded) {
			exit.Capture = "failed"
		}
		if started, senderJoined, interruptJoined := session.CleanupCompletion(); started {
			exit.Sender, exit.Interrupt = joinedSourceState(senderJoined), joinedSourceState(interruptJoined)
		}
		result = errors.Join(result, firstCommandFailure)
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
	read := func() (sourcePacket, error) {
		if err := channel.stream.SetReadDeadline(time.Now().Add(IdleTimeout)); err != nil {
			return sourcePacket{}, sourceTransportFailure(errors.Join(ErrAuthority, err))
		}
		p, err := readSourceCommandAuthorized(reader, readContext, lease.Binding, sequence, authorizeCommand)
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
	if err := writer.send(sourcePacket{kind: sourceReply, op: sourceStart, reply: first.sequence}, nil); err != nil {
		return err
	}
	writer.started.Store(true)
	exit.Command = "ok"
	requests := make(chan sourcePacket, 1)
	completed := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- pumpSourceCommandsOrdered(run, read, requests, cancel, completed, func(p sourcePacket) error {
			return writer.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: p.sequence}, nil)
		})
	}()
	// Retire only this read; the final native response still needs the writer.
	defer func() {
		cancel()
		stopReads()
		readerErr := <-readDone
		exit.Reader = sourceReaderOutcome(readerErr)
		result = errors.Join(result, readerErr)
	}()
	var pendingReturn uint64
	var cursorPrepared func()
	var cursorSequence uint64
	return hook.WithSourceCommandDesktop(run, func(commandContext context.Context) error {
		for {
			work, err := nextSourceWork(run, requests, queue.events, session.LocalReturns())
			if err != nil {
				return err
			}
			switch p := work.packet; p.kind {
			case sourceBoundary:
				var batch [sourceBoundaryBatchLimit]sourcePacket
				n := collectSourceBoundaries(p, queue.events, requests, &batch)
				if err := writer.sendBoundaries(batch[:n]); err != nil {
					return err
				}
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
						decision, failure := hook.AdmitBoundary(run, control.BoundaryIntent{})
						response.decision = decision
						return failure
					case sourceActivate:
						writer.epoch.Store(p.sequence)
						if err := session.Activate(); err != nil {
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
						if cursorPrepared == nil {
							return ErrAuthority
						}
					case sourceCursorShow:
						if cursorPrepared == nil || p.reply != cursorSequence {
							return ErrProtocol
						}
						cursorPrepared()
						cursorPrepared = nil
					case sourceCursorRestore:
						cursorPrepared = nil
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
				var err error
				if p.op == sourceDeactivate || p.op == sourceHeartbeat || p.op == sourceCursorRestore {
					err = operation()
				} else {
					err = hook.ExecuteSourceDesktop(commandContext, operation)
				}
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
				}
				if failure := sourceCommandCompletion(err, writer.send(response, nil)); failure != nil {
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

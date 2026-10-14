//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
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
		return nil
	}
	// Retirement grants no authority. It is sent only after native/sender join
	// on the exact already authenticated channel, even if logon authority changed.
	if !retirement && w.channel.authority.check(false) != nil {
		return ErrAuthority
	}
	w.sequence++
	p.sequence, p.binding = w.sequence, w.binding
	data, err := encodeSourcePacket(p)
	if err != nil {
		return err
	}
	if w.channel.stream.SetWriteDeadline(time.Now().Add(time.Second)) != nil {
		return ErrAuthority
	}
	return writeFrame(w.channel.stream, data[:])
}

func (w *sourceWriter) SendAction(ctx context.Context, action control.InputAction, current func() bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return w.send(sourcePacket{kind: sourceAction, reply: w.epoch.Load(), action: action}, current)
}
func (w *sourceWriter) Interrupt(error) error             { return w.channel.stream.SetWriteDeadline(time.Now()) }
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
func runSourceCapture(ctx context.Context, channel *Channel, proof *Authority, lease Lease, ready func() error) (result error) {
	if !lease.Capture || lease.Presentation {
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
	defer func() {
		cancel()
		capture, sender := <-hookDone, <-sourceDone
		emergency := errors.Is(errors.Join(result, capture, sender), wininput.ErrEmergencyRelease)
		result = sourceCaptureFinal(result, capture, sender, run.Err() == context.DeadlineExceeded)
		if stopping != nil {
			response := sourcePacket{kind: sourceReply, op: sourceStop, reply: stopping.sequence}
			if result != nil {
				response.status = 1
			}
			result = sourceRetirementDelivery(result, writer.write(response, nil, true))
		} else {
			response := sourcePacket{kind: sourceRetired}
			if result != nil {
				response.status = 1
			}
			if emergency && result == nil {
				response.op = 1
			}
			result = sourceRetirementDelivery(result, writer.write(response, nil, true))
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
	read := func() (sourcePacket, error) {
		if channel.stream.SetReadDeadline(time.Now().Add(IdleTimeout)) != nil {
			return sourcePacket{}, ErrAuthority
		}
		p, err := readSourcePacket(channel.stream)
		if err != nil || p.kind != sourceCommand || p.binding != lease.Binding || p.sequence != sequence+1 || authorize() != nil {
			return sourcePacket{}, ErrProtocol
		}
		sequence = p.sequence
		return p, nil
	}
	first, err := read()
	if err != nil || first.op != sourceStart {
		return ErrProtocol
	}
	if writer.send(sourcePacket{kind: sourceReply, op: sourceStart, reply: first.sequence}, nil) != nil {
		return ErrAuthority
	}
	writer.started.Store(true)
	requests := make(chan sourcePacket, 1)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			p, err := read()
			if err != nil {
				cancel()
				return
			}
			select {
			case requests <- p:
			case <-run.Done():
				return
			}
			if p.op == sourceStop {
				return
			}
		}
	}()
	// Reader exits after the explicit stop; on other exits close it before join.
	defer func() {
		cancel()
		if stopping == nil {
			_ = channel.stream.SetReadDeadline(time.Now())
		}
		<-readDone
	}()
	var pendingReturn uint64
	var cursorPrepared func()
	var cursorSequence uint64
	for {
		select {
		case <-run.Done():
			return run.Err()
		case p := <-queue.events:
			if writer.send(p, nil) != nil {
				return ErrAuthority
			}
		case generation := <-session.LocalReturns():
			pendingReturn = generation
			if writer.send(sourcePacket{kind: sourceReturn, reply: writer.epoch.Load()}, nil) != nil {
				return ErrAuthority
			}
		case p := <-requests:
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
				err = hook.ExecuteSourceDesktop(run, operation)
			}
			if err != nil {
				response.status = 1
				response.decision = 0
			}
			if writer.send(response, nil) != nil {
				return ErrAuthority
			}
			if errors.Is(err, ErrRecovery) {
				return err
			}
		}
	}
}

var _ peer.BoundaryAdmissionGate = (*CaptureSource)(nil)

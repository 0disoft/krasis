package peer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

type FrameWriter interface {
	WriteFrameContext(context.Context, transport.Frame) error
}

type OutboundActionSink struct {
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Connection  interface{ Close() error }
}

type localConnectionInterrupter interface {
	InterruptLocal(error) error
}

func NewOutboundActionSink(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	connection interface{ Close() error },
) (*OutboundActionSink, error) {
	if coordinator == nil || runtime == nil || connection == nil {
		return nil, errors.New("write coordinator, runtime, and connection are required")
	}
	return &OutboundActionSink{Coordinator: coordinator, Runtime: runtime, Connection: connection}, nil
}

func (sink *OutboundActionSink) Interrupt(cause error) error {
	if sink == nil || sink.Connection == nil {
		return errors.New("outbound action sink is not initialized")
	}
	if interrupter, ok := sink.Connection.(localConnectionInterrupter); ok {
		return interrupter.InterruptLocal(errors.Join(ErrSourceSessionInterrupted, cause))
	}
	return sink.Connection.Close()
}

func (sink *OutboundActionSink) SendAction(ctx context.Context, action control.InputAction, isCurrent func() bool) error {
	if sink == nil || sink.Coordinator == nil || sink.Runtime == nil {
		return errors.New("outbound action sink is not initialized")
	}
	if isCurrent == nil {
		return errors.New("source generation check is required")
	}
	return sendInputAction(ctx, sink.Coordinator, sink.Runtime, action, isCurrent)
}

func (sink *OutboundActionSink) Fail(ctx context.Context, cause error) error {
	if sink == nil || sink.Coordinator == nil || sink.Runtime == nil {
		return errors.New("outbound action sink is not initialized")
	}
	return failRuntimeCoordinated(ctx, sink.Coordinator, sink.Runtime, cause, nil)
}

func BeginOutboundSession(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	sessionID string,
	epoch string,
	edge control.ScreenEdge,
) error {
	return BeginOutboundSessionObserved(ctx, coordinator, runtime, sessionID, epoch, edge, nil)
}

func BeginOutboundSessionObserved(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	sessionID string,
	epoch string,
	edge control.ScreenEdge,
	observer ControlFlowObserver,
) error {
	if coordinator == nil || runtime == nil {
		return errors.New("write coordinator and runtime are required")
	}
	queued := time.Now()
	return coordinator.Execute(ctx, func(writer FrameWriter) error {
		handoff.Record(ctx, handoff.RequestCoordinator, time.Since(queued))
		if _, err := runtime.BeginOutbound(ctx, sessionID, epoch); err != nil {
			return fmt.Errorf("%w: %w", ErrOutboundBegin, err)
		}
		started := time.Now()
		err := writeControlOrFail(ctx, writer, runtime, transport.ControlCommand{
			Kind:      transport.ControlLeaseRequest,
			SessionID: sessionID,
			Epoch:     epoch,
			Edge:      edge,
		})
		handoff.Record(ctx, handoff.RequestWrite, time.Since(started))
		if err != nil {
			observeControlFlow(observer, ControlFlowRequestWriteOutcomeUnknown)
			return fmt.Errorf("%w: %w", ErrOutboundRequestWrite, err)
		}
		observeControlFlow(observer, ControlFlowRequestWriteCompleted)
		return nil
	})
}

func SendInputAction(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	action control.InputAction,
) error {
	return sendInputAction(ctx, coordinator, runtime, action, nil)
}

var ErrStaleSourceAction = errors.New("source action belongs to a retired generation")

func sendInputAction(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	action control.InputAction,
	isCurrent func() bool,
) error {
	if coordinator == nil || runtime == nil {
		return errors.New("write coordinator and runtime are required")
	}
	if err := action.Validate(); err != nil {
		return err
	}
	ctx, complete := handoff.Input(ctx, "source")
	recorded := false
	queued := time.Now()
	err := coordinator.Execute(ctx, func(writer FrameWriter) (operationErr error) {
		defer func() { complete(operationErr); recorded = true }()
		handoff.Record(ctx, handoff.InputCoordinator, time.Since(queued))
		if isCurrent != nil && !isCurrent() {
			return ErrStaleSourceAction
		}
		envelope, err := runtime.NextOutboundEnvelope(ctx)
		if err != nil {
			return err
		}
		payload, err := transport.EncodeInputPayload(envelope, action)
		if err != nil {
			return failRuntime(ctx, runtime, err)
		}
		started := time.Now()
		err = writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageInput, Payload: payload})
		handoff.Record(ctx, handoff.InputWrite, time.Since(started))
		if err != nil {
			return failRuntime(ctx, runtime, err)
		}
		return nil
	})
	if !recorded {
		complete(err)
	}
	return err
}

func SendHeartbeat(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
) error {
	if coordinator == nil || runtime == nil {
		return errors.New("write coordinator and runtime are required")
	}
	return coordinator.Execute(ctx, func(writer FrameWriter) error {
		snapshot := runtime.Snapshot()
		if snapshot.Control.State != control.StateInboundActive && snapshot.Control.State != control.StateOutboundActive {
			return errors.New("heartbeat requires an active lease")
		}
		payload, err := transport.EncodeHeartbeatPayload(snapshot.Control.SessionID)
		if err != nil {
			return failRuntime(ctx, runtime, err)
		}
		if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageHeartbeat, Payload: payload}); err != nil {
			return failRuntime(ctx, runtime, err)
		}
		return nil
	})
}

type LeaseAuthorizer interface {
	AuthorizeLease(context.Context, transport.AuthenticatedPeerContext, string, string, time.Time) (leaseID string, accepted bool, err error)
}

type CursorEntryPort interface {
	PrepareEntry(context.Context, control.ScreenEdge) error
	CommitEntry()
	RollbackEntry(context.Context) (application.CursorRollbackOutcome, error)
}

// Optional prepared-state validation immediately before committing an entry.
// A failure uses the same rollback path as any other pending lease failure.
type CursorEntryValidator interface{ ValidateEntry(context.Context) error }

type ReturnBoundaryPort interface {
	ArmReturn(control.ReturnIntent, control.ScreenEdge) error
	RearmReturn(control.ReturnIntent)
	DisarmReturn(control.ReturnIntent)
}

type LeaseResponder struct {
	Preparation func(context.Context, time.Time)
	Cursor      ReturnCursorPresentation
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Authorizer  LeaseAuthorizer
	Peer        transport.AuthenticatedPeerContext
	Entry       CursorEntryPort
	Returns     ReturnBoundaryPort
	Observer    ControlFlowObserver
	pending     *pendingInboundLease
}

func (responder *LeaseResponder) HandleInputPreparation(ctx context.Context, now time.Time) {
	if responder != nil && responder.Preparation != nil && ctx != nil && ctx.Err() == nil {
		responder.Preparation(ctx, now)
	}
}

type pendingInboundLease struct {
	command      transport.ControlCommand
	returnIntent control.ReturnIntent
}

func (responder *LeaseResponder) HandleLeaseRequest(
	ctx context.Context,
	command transport.ControlCommand,
	observedAt time.Time,
) (resultErr error) {
	if responder == nil || responder.Coordinator == nil || responder.Runtime == nil || responder.Authorizer == nil || responder.Entry == nil || responder.Returns == nil {
		return errors.New("lease responder is not initialized")
	}
	if err := responder.Peer.Validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseRequest {
		return errors.New("lease responder requires a lease request")
	}
	ctx = handoff.Begin(ctx, "receiver", command.SessionID, command.Epoch, time.Now())
	rejected := false
	defer func() {
		if resultErr != nil || rejected {
			handoff.Finish(ctx, "error")
		}
	}()
	queued := time.Now()
	observeControlFlow(responder.Observer, ControlFlowRequestReceived)
	return responder.Coordinator.Execute(ctx, func(writer FrameWriter) error {
		handoff.Record(ctx, handoff.RequestCoordinator, time.Since(queued))
		if responder.Runtime.Snapshot().Control.State != control.StateLocal {
			rejected = true
			// A concurrent outbound request or an existing lease is ordinary
			// contention, not a malformed frame or a broken connection.
			return writeControlOrFail(ctx, writer, responder.Runtime, transport.ControlCommand{
				Kind: transport.ControlLeaseReject, SessionID: command.SessionID, Epoch: command.Epoch,
			})
		}
		handoff.Activate(ctx)
		started := time.Now()
		leaseID, accepted, err := responder.Authorizer.AuthorizeLease(ctx, responder.Peer, command.SessionID, command.Epoch, observedAt)
		handoff.Record(ctx, handoff.AuthorizeTotal, time.Since(started))
		if err != nil {
			return err
		}
		if !accepted {
			rejected = true
			return writeControlOrFail(ctx, writer, responder.Runtime, transport.ControlCommand{
				Kind:      transport.ControlLeaseReject,
				SessionID: command.SessionID,
				Epoch:     command.Epoch,
			})
		}
		if leaseID == "" {
			return errors.New("accepted lease requires an identifier")
		}
		returnIntent := control.ReturnIntent{SessionID: command.SessionID, Epoch: command.Epoch}
		started = time.Now()
		armErr := responder.Returns.ArmReturn(returnIntent, command.Edge)
		handoff.Record(ctx, handoff.ReturnArm, time.Since(started))
		if err := armErr; err != nil {
			return err
		}
		observeControlFlow(responder.Observer, ControlFlowReturnArmSucceeded)
		rollbackEntry := func(cause error) error {
			responder.Returns.DisarmReturn(returnIntent)
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
			defer cancel()
			observeControlFlow(responder.Observer, ControlFlowEntryRollbackAttempted)
			_, rollbackErr := responder.Entry.RollbackEntry(cleanupCtx)
			return errors.Join(cause, rollbackErr)
		}
		observeControlFlow(responder.Observer, ControlFlowEntryPrepareAttempted)
		if responder.Cursor != nil {
			responder.Cursor.RestoreReturnCursor()
		}
		started = time.Now()
		entryErr := responder.Entry.PrepareEntry(ctx, command.Edge)
		handoff.Record(ctx, handoff.EntryPrepare, time.Since(started))
		if err := entryErr; err != nil {
			return rollbackEntry(err)
		}
		observeControlFlow(responder.Observer, ControlFlowEntryPrepared)
		started = time.Now()
		_, prepareErr := responder.Runtime.PrepareInbound(ctx, command.SessionID, leaseID, command.Epoch)
		handoff.Record(ctx, handoff.PrepareInbound, time.Since(started))
		if err := prepareErr; err != nil {
			return rollbackEntry(err)
		}
		responder.pending = &pendingInboundLease{
			command: transport.ControlCommand{
				Kind: transport.ControlLeaseGrant, SessionID: command.SessionID, LeaseID: leaseID, Epoch: command.Epoch,
			},
			returnIntent: returnIntent,
		}
		payload, err := transport.EncodeControlPayload(transport.ControlCommand{
			Kind:      transport.ControlLeaseGrant,
			SessionID: command.SessionID,
			LeaseID:   leaseID,
			Epoch:     command.Epoch,
		})
		started = time.Now()
		if err == nil {
			err = writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload})
		}
		handoff.Record(ctx, handoff.GrantWrite, time.Since(started))
		if err != nil {
			observeControlFlow(responder.Observer, ControlFlowGrantWriteOutcomeUnknown)
			return responder.abortPendingLocked(ctx, fmt.Errorf("%w: %w", ErrInboundGrantWrite, err))
		}
		observeControlFlow(responder.Observer, ControlFlowGrantWriteCompleted)
		return nil
	})
}

func (responder *LeaseResponder) HandleLeaseCommit(
	ctx context.Context,
	command transport.ControlCommand,
	observedAt time.Time,
) (resultErr error) {
	if responder == nil || responder.Coordinator == nil || responder.Runtime == nil || responder.Entry == nil || responder.Returns == nil {
		return errors.New("lease responder is not initialized")
	}
	if command.Kind != transport.ControlLeaseCommit {
		return errors.New("lease responder requires a lease commit")
	}
	ctx = handoff.Matching(ctx, "receiver", command.SessionID, command.Epoch)
	defer func() {
		if resultErr != nil {
			handoff.Finish(ctx, "error")
		}
	}()
	queued := time.Now()
	return responder.Coordinator.Execute(ctx, func(FrameWriter) error {
		handoff.Record(ctx, handoff.CommitCoordinator, time.Since(queued))
		started := time.Now()
		defer func() { handoff.Record(ctx, handoff.CommitApply, time.Since(started)) }()
		observeControlFlow(responder.Observer, ControlFlowCommitReceived)
		if !responder.matchesPending(command) {
			return errors.New("lease commit does not match the prepared inbound lease")
		}
		if validator, ok := responder.Entry.(CursorEntryValidator); ok {
			if err := validator.ValidateEntry(ctx); err != nil {
				return responder.abortPendingLocked(ctx, err)
			}
		}
		if _, err := responder.Runtime.CommitInbound(ctx, command.SessionID, command.LeaseID, command.Epoch, observedAt); err != nil {
			return responder.abortPendingLocked(ctx, err)
		}
		responder.Entry.CommitEntry()
		responder.pending = nil
		observeControlFlow(responder.Observer, ControlFlowEntryCommitted)
		return nil
	})
}

func (responder *LeaseResponder) HandleLeaseAbort(
	ctx context.Context,
	command transport.ControlCommand,
) error {
	if responder == nil || responder.Coordinator == nil || responder.Runtime == nil || responder.Entry == nil || responder.Returns == nil {
		return errors.New("lease responder is not initialized")
	}
	if command.Kind != transport.ControlLeaseAbort {
		return errors.New("lease responder requires a lease abort")
	}
	ctx = handoff.Matching(ctx, "receiver", command.SessionID, command.Epoch)
	var emitProfile func()
	err := responder.Coordinator.Execute(ctx, func(FrameWriter) error {
		observeControlFlow(responder.Observer, ControlFlowAbortReceived)
		if !responder.matchesPending(command) {
			return errors.New("lease abort does not match the prepared inbound lease")
		}
		err := responder.abortPendingLocked(ctx, nil)
		emitProfile = handoff.Retire(ctx, "error")
		return err
	})
	if emitProfile != nil {
		emitProfile()
	}
	return err
}

func (responder *LeaseResponder) AbortPending(ctx context.Context) error {
	if responder == nil || responder.Coordinator == nil {
		return errors.New("lease responder is not initialized")
	}
	var emitProfile func()
	err := responder.Coordinator.Execute(ctx, func(FrameWriter) error {
		if pending := responder.pending; pending != nil {
			profile := handoff.Matching(ctx, "receiver", pending.command.SessionID, pending.command.Epoch)
			err := responder.abortPendingLocked(ctx, nil)
			emitProfile = handoff.Retire(profile, "error")
			return err
		}
		return nil
	})
	if emitProfile != nil {
		emitProfile()
	}
	return err
}

func (responder *LeaseResponder) matchesPending(command transport.ControlCommand) bool {
	return responder.pending != nil &&
		responder.pending.command.SessionID == command.SessionID &&
		responder.pending.command.LeaseID == command.LeaseID &&
		responder.pending.command.Epoch == command.Epoch
}

func (responder *LeaseResponder) abortPendingLocked(ctx context.Context, cause error) error {
	if responder.pending == nil {
		return cause
	}
	pending := responder.pending
	responder.pending = nil
	responder.Returns.DisarmReturn(pending.returnIntent)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
	defer cancel()
	_, runtimeErr := responder.Runtime.AbortInbound(
		cleanupCtx,
		pending.command.SessionID,
		pending.command.LeaseID,
		pending.command.Epoch,
	)
	observeControlFlow(responder.Observer, ControlFlowEntryRollbackAttempted)
	_, rollbackErr := responder.Entry.RollbackEntry(cleanupCtx)
	return errors.Join(cause, runtimeErr, rollbackErr)
}

func writeControlOrFail(
	ctx context.Context,
	writer FrameWriter,
	runtime *application.SessionRuntime,
	command transport.ControlCommand,
) error {
	payload, err := transport.EncodeControlPayload(command)
	if err != nil {
		return failRuntime(ctx, runtime, err)
	}
	if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload}); err != nil {
		return failRuntime(ctx, runtime, err)
	}
	return nil
}

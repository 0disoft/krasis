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

var ErrOutboundNotReady = errors.New("outbound controller is not ready")

type SourceActivation interface {
	Activate() error
	Deactivate()
	Active() bool
}

type OutboundController struct {
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Source      SourceActivation
	Observer    ControlFlowObserver
	// Bidirectional sessions also receive source-initiated inbound revokes.
	// Wire the same lease owner used by the responder and local return path.
	Returns ReturnBoundaryPort
}

func NewOutboundController(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	source SourceActivation,
) (*OutboundController, error) {
	return NewOutboundControllerObserved(coordinator, runtime, source, nil)
}

func NewOutboundControllerObserved(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	source SourceActivation,
	observer ControlFlowObserver,
) (*OutboundController, error) {
	if coordinator == nil || runtime == nil || source == nil {
		return nil, errors.New("write coordinator, runtime, and source activation are required")
	}
	return &OutboundController{Coordinator: coordinator, Runtime: runtime, Source: source, Observer: observer}, nil
}

func (controller *OutboundController) Begin(
	ctx context.Context,
	sessionID string,
	epoch string,
	edge control.ScreenEdge,
) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if !controller.Ready() {
		return ErrOutboundNotReady
	}
	return BeginOutboundSessionObserved(ctx, controller.Coordinator, controller.Runtime, sessionID, epoch, edge, controller.Observer)
}

func (controller *OutboundController) Ready() bool {
	return controller != nil && controller.Coordinator != nil && controller.Runtime != nil && controller.Source != nil &&
		!controller.Source.Active() && controller.Runtime.Snapshot().Control.State == control.StateLocal
}

func (controller *OutboundController) BeginBoundary(
	ctx context.Context,
	intent control.BoundaryIntent,
	identity OutboundIdentity,
) error {
	if intent.Edge != control.ScreenEdgeLeft && intent.Edge != control.ScreenEdgeRight {
		return errors.New("outbound boundary intent requires a LEFT or RIGHT edge")
	}
	ctx = handoff.Begin(ctx, "source", identity.SessionID, identity.Epoch, intent.DecidedAt)
	if !intent.DecidedAt.IsZero() {
		handoff.Record(ctx, handoff.BoundaryToBegin, time.Since(intent.DecidedAt))
	}
	err := controller.Begin(ctx, identity.SessionID, identity.Epoch, intent.Edge)
	if err != nil {
		handoff.Finish(ctx, "error")
	}
	return err
}

func (controller *OutboundController) GrantPeer(
	ctx context.Context,
	command transport.ControlCommand,
	observedAt time.Time,
) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseGrant {
		return errors.New("outbound controller requires a lease grant")
	}
	ctx = handoff.Matching(ctx, "source", command.SessionID, command.Epoch)
	handoff.Granted(ctx)
	observeControlFlow(controller.Observer, ControlFlowGrantReceived)
	preparationStarted := time.Now()
	var emitProfile func()
	aborted := false
	resultErr := controller.Coordinator.Execute(ctx, func(writer FrameWriter) (operationErr error) {
		defer func() {
			if operationErr != nil || aborted {
				emitProfile = handoff.Retire(ctx, "error")
			}
		}()
		handoff.Record(ctx, handoff.GrantCoordinator, time.Since(preparationStarted))
		if err := controller.Runtime.ValidateOutboundPreparation(ctx, command.SessionID, command.LeaseID, command.Epoch, observedAt); err != nil {
			observeControlFlow(controller.Observer, ControlFlowGrantApplyFailed)
			return failRuntime(ctx, controller.Runtime, fmt.Errorf("%w: %w", ErrOutboundGrantApply, err))
		}
		if err := controller.Source.Activate(); err != nil {
			aborted = true
			controller.Source.Deactivate()
			abort := transport.ControlCommand{
				Kind: transport.ControlLeaseAbort, SessionID: command.SessionID,
				LeaseID: command.LeaseID, Epoch: command.Epoch,
			}
			if writeErr := writeControlOrFail(ctx, writer, controller.Runtime, abort); writeErr != nil {
				observeControlFlow(controller.Observer, ControlFlowAbortWriteOutcomeUnknown)
				return errors.Join(err, writeErr)
			}
			observeControlFlow(controller.Observer, ControlFlowAbortWriteCompleted)
			if _, abortErr := controller.Runtime.AbortOutbound(
				ctx, command.SessionID, command.LeaseID, command.Epoch,
			); abortErr != nil {
				return errors.Join(err, abortErr)
			}
			return nil
		}
		if err := controller.Runtime.ValidateOutboundPreparation(ctx, command.SessionID, command.LeaseID, command.Epoch, observedAt); err != nil {
			controller.Source.Deactivate()
			return failRuntime(ctx, controller.Runtime, fmt.Errorf("%w: %w", ErrOutboundGrantApply, err))
		}
		// Preserve the observation clock's origin, but begin active liveness only
		// after preparation. Input writes remain behind this semantic gate until commit.
		if _, err := controller.Runtime.GrantOutbound(ctx, command.SessionID, command.LeaseID, command.Epoch, observedAt.Add(time.Since(preparationStarted))); err != nil {
			controller.Source.Deactivate()
			observeControlFlow(controller.Observer, ControlFlowGrantApplyFailed)
			return fmt.Errorf("%w: %w", ErrOutboundGrantApply, err)
		}
		observeControlFlow(controller.Observer, ControlFlowGrantApplySucceeded)
		commit := transport.ControlCommand{
			Kind: transport.ControlLeaseCommit, SessionID: command.SessionID,
			LeaseID: command.LeaseID, Epoch: command.Epoch,
		}
		started := time.Now()
		commitErr := writeControlOrFail(ctx, writer, controller.Runtime, commit)
		handoff.Record(ctx, handoff.CommitWrite, time.Since(started))
		if err := commitErr; err != nil {
			observeControlFlow(controller.Observer, ControlFlowCommitWriteOutcomeUnknown)
			controller.Source.Deactivate()
			return err
		}
		observeControlFlow(controller.Observer, ControlFlowCommitWriteCompleted)
		return nil
	})
	if emitProfile != nil {
		emitProfile()
	} else if resultErr != nil {
		handoff.Finish(ctx, "error")
	}
	return resultErr
}

func (controller *OutboundController) RejectPeer(ctx context.Context, command transport.ControlCommand) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseReject {
		return errors.New("outbound controller requires a lease rejection")
	}
	var emitProfile func()
	err := controller.Coordinator.Execute(ctx, func(FrameWriter) error {
		controller.Source.Deactivate()
		_, err := controller.Runtime.RejectOutbound(ctx, command.SessionID, command.Epoch)
		if err == nil {
			emitProfile = handoff.Retire(handoff.Matching(ctx, "source", command.SessionID, command.Epoch), "error")
		}
		return err
	})
	if emitProfile != nil {
		emitProfile()
	}
	return err
}

func (controller *OutboundController) RevokePeer(ctx context.Context, command transport.ControlCommand) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseRevoke {
		return errors.New("outbound controller requires a lease revocation")
	}
	observeControlFlow(controller.Observer, ControlFlowRevokeReceived)
	var emitProfile func()
	err := controller.Coordinator.Execute(ctx, func(FrameWriter) error {
		// Waiting for native input must not retain the semantic writer after
		// this revoke request expires. Runtime rechecks admission below too.
		snapshot, err := controller.Runtime.SnapshotContext(ctx)
		if err != nil {
			return err
		}
		inbound := snapshot.Control.State == control.StateInboundActive
		controller.Source.Deactivate()
		_, err = controller.Runtime.RevokePeer(ctx, command.SessionID, command.LeaseID, command.Epoch)
		if err == nil {
			role := "source"
			if inbound {
				role = "receiver"
				if controller.Returns != nil {
					// Runtime revocation releases held input first. Retire the
					// exact native owner before another request can acquire the
					// coordinator; never disarm an unrelated outbound lease.
					controller.Returns.DisarmReturn(control.ReturnIntent{SessionID: command.SessionID, Epoch: command.Epoch})
				}
			}
			emitProfile = handoff.Retire(handoff.Matching(ctx, role, command.SessionID, command.Epoch), "ok")
		}
		return err
	})
	if emitProfile != nil {
		emitProfile()
	}
	return err
}

func (controller *OutboundController) Fail(ctx context.Context, cause error) error {
	if err := controller.validate(); err != nil {
		return err
	}
	controller.Source.Deactivate()
	return failRuntimeCoordinated(ctx, controller.Coordinator, controller.Runtime, cause, controller.Source.Deactivate)
}

func (controller *OutboundController) validate() error {
	if controller == nil || controller.Coordinator == nil || controller.Runtime == nil || controller.Source == nil {
		return errors.New("outbound controller is not initialized")
	}
	return nil
}

type OutboundLifecycle interface {
	GrantPeer(context.Context, transport.ControlCommand, time.Time) error
	RejectPeer(context.Context, transport.ControlCommand) error
	RevokePeer(context.Context, transport.ControlCommand) error
	Fail(context.Context, error) error
}

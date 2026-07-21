package peer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
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
	return controller.Begin(ctx, identity.SessionID, identity.Epoch, intent.Edge)
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
	observeControlFlow(controller.Observer, ControlFlowGrantReceived)
	return controller.Coordinator.Execute(ctx, func(writer FrameWriter) error {
		if _, err := controller.Runtime.GrantOutbound(
			ctx,
			command.SessionID,
			command.LeaseID,
			command.Epoch,
			observedAt,
		); err != nil {
			observeControlFlow(controller.Observer, ControlFlowGrantApplyFailed)
			return fmt.Errorf("%w: %w", ErrOutboundGrantApply, err)
		}
		observeControlFlow(controller.Observer, ControlFlowGrantApplySucceeded)
		if err := controller.Source.Activate(); err != nil {
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
		commit := transport.ControlCommand{
			Kind: transport.ControlLeaseCommit, SessionID: command.SessionID,
			LeaseID: command.LeaseID, Epoch: command.Epoch,
		}
		if err := writeControlOrFail(ctx, writer, controller.Runtime, commit); err != nil {
			observeControlFlow(controller.Observer, ControlFlowCommitWriteOutcomeUnknown)
			controller.Source.Deactivate()
			return err
		}
		observeControlFlow(controller.Observer, ControlFlowCommitWriteCompleted)
		return nil
	})
}

func (controller *OutboundController) RejectPeer(ctx context.Context, command transport.ControlCommand) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseReject {
		return errors.New("outbound controller requires a lease rejection")
	}
	return controller.Coordinator.Execute(ctx, func(FrameWriter) error {
		controller.Source.Deactivate()
		_, err := controller.Runtime.RejectOutbound(ctx, command.SessionID, command.Epoch)
		return err
	})
}

func (controller *OutboundController) RevokePeer(ctx context.Context, command transport.ControlCommand) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseRevoke {
		return errors.New("outbound controller requires a lease revocation")
	}
	observeControlFlow(controller.Observer, ControlFlowRevokeReceived)
	return controller.Coordinator.Execute(ctx, func(FrameWriter) error {
		controller.Source.Deactivate()
		_, err := controller.Runtime.RevokePeer(ctx, command.SessionID, command.LeaseID, command.Epoch)
		return err
	})
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

package peer

import (
	"context"
	"errors"
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
}

func NewOutboundController(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	source SourceActivation,
) (*OutboundController, error) {
	if coordinator == nil || runtime == nil || source == nil {
		return nil, errors.New("write coordinator, runtime, and source activation are required")
	}
	return &OutboundController{Coordinator: coordinator, Runtime: runtime, Source: source}, nil
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
	return BeginOutboundSession(ctx, controller.Coordinator, controller.Runtime, sessionID, epoch, edge)
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
	return controller.Coordinator.Execute(ctx, func(FrameWriter) error {
		if _, err := controller.Runtime.GrantOutbound(
			ctx,
			command.SessionID,
			command.LeaseID,
			command.Epoch,
			observedAt,
		); err != nil {
			return err
		}
		if err := controller.Source.Activate(); err != nil {
			controller.Source.Deactivate()
			return failRuntime(ctx, controller.Runtime, err)
		}
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

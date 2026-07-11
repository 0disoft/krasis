package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

type SourceActivation interface {
	Activate() error
	Deactivate()
	Active() bool
}

type OutboundController struct {
	Writer  FrameWriter
	Runtime *application.SessionRuntime
	Source  SourceActivation
}

func NewOutboundController(
	writer FrameWriter,
	runtime *application.SessionRuntime,
	source SourceActivation,
) (*OutboundController, error) {
	if writer == nil || runtime == nil || source == nil {
		return nil, errors.New("frame writer, runtime, and source activation are required")
	}
	return &OutboundController{Writer: writer, Runtime: runtime, Source: source}, nil
}

func (controller *OutboundController) Begin(ctx context.Context, sessionID, epoch string) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if controller.Source.Active() {
		return errors.New("source suppression is already active")
	}
	return BeginOutboundSession(ctx, controller.Writer, controller.Runtime, sessionID, epoch)
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
}

func (controller *OutboundController) RejectPeer(ctx context.Context, command transport.ControlCommand) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseReject {
		return errors.New("outbound controller requires a lease rejection")
	}
	controller.Source.Deactivate()
	_, err := controller.Runtime.RejectOutbound(ctx, command.SessionID, command.Epoch)
	return err
}

func (controller *OutboundController) RevokePeer(ctx context.Context, command transport.ControlCommand) error {
	if err := controller.validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseRevoke {
		return errors.New("outbound controller requires a lease revocation")
	}
	controller.Source.Deactivate()
	_, err := controller.Runtime.RevokePeer(ctx, command.SessionID, command.LeaseID, command.Epoch)
	return err
}

func (controller *OutboundController) Fail(ctx context.Context, cause error) error {
	if err := controller.validate(); err != nil {
		return err
	}
	controller.Source.Deactivate()
	return failRuntime(ctx, controller.Runtime, cause)
}

func (controller *OutboundController) validate() error {
	if controller == nil || controller.Writer == nil || controller.Runtime == nil || controller.Source == nil {
		return errors.New("outbound controller is not initialized")
	}
	return nil
}

type OutboundLifecycle interface {
	GrantPeer(context.Context, transport.ControlCommand, time.Time) error
	RejectPeer(context.Context, transport.ControlCommand) error
	RevokePeer(context.Context, transport.ControlCommand) error
}

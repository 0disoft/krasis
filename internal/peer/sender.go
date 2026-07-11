package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type FrameWriter interface {
	WriteFrameContext(context.Context, transport.Frame) error
}

type OutboundActionSink struct {
	Writer  FrameWriter
	Runtime *application.SessionRuntime
}

func NewOutboundActionSink(writer FrameWriter, runtime *application.SessionRuntime) (*OutboundActionSink, error) {
	if writer == nil || runtime == nil {
		return nil, errors.New("frame writer and runtime are required")
	}
	return &OutboundActionSink{Writer: writer, Runtime: runtime}, nil
}

func (sink *OutboundActionSink) SendAction(ctx context.Context, action control.InputAction) error {
	if sink == nil || sink.Writer == nil || sink.Runtime == nil {
		return errors.New("outbound action sink is not initialized")
	}
	return SendInputAction(ctx, sink.Writer, sink.Runtime, action)
}

func (sink *OutboundActionSink) Fail(ctx context.Context, cause error) error {
	if sink == nil || sink.Runtime == nil {
		return errors.New("outbound action sink is not initialized")
	}
	return sink.Runtime.Fail(ctx, cause)
}

func BeginOutboundSession(
	ctx context.Context,
	writer FrameWriter,
	runtime *application.SessionRuntime,
	sessionID string,
	epoch string,
	edge control.ScreenEdge,
) error {
	if writer == nil || runtime == nil {
		return errors.New("frame writer and runtime are required")
	}
	if _, err := runtime.BeginOutbound(ctx, sessionID, epoch); err != nil {
		return err
	}
	return writeControlOrFail(ctx, writer, runtime, transport.ControlCommand{
		Kind:      transport.ControlLeaseRequest,
		SessionID: sessionID,
		Epoch:     epoch,
		Edge:      edge,
	})
}

func SendInputAction(
	ctx context.Context,
	writer FrameWriter,
	runtime *application.SessionRuntime,
	action control.InputAction,
) error {
	if writer == nil || runtime == nil {
		return errors.New("frame writer and runtime are required")
	}
	if err := action.Validate(); err != nil {
		return err
	}
	envelope, err := runtime.NextOutboundEnvelope(ctx)
	if err != nil {
		return err
	}
	payload, err := transport.EncodeInputPayload(envelope, action)
	if err != nil {
		return failRuntime(ctx, runtime, err)
	}
	if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageInput, Payload: payload}); err != nil {
		return failRuntime(ctx, runtime, err)
	}
	return nil
}

func SendHeartbeat(
	ctx context.Context,
	writer FrameWriter,
	runtime *application.SessionRuntime,
) error {
	if writer == nil || runtime == nil {
		return errors.New("frame writer and runtime are required")
	}
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
}

type LeaseAuthorizer interface {
	AuthorizeLease(context.Context, string, string, time.Time) (leaseID string, accepted bool, err error)
}

type CursorEntryPort interface {
	Enter(context.Context, control.ScreenEdge) error
}

type ReturnBoundaryPort interface {
	ArmReturn(control.ReturnIntent, control.ScreenEdge) error
	RearmReturn(control.ReturnIntent)
	DisarmReturn(control.ReturnIntent)
}

type LeaseResponder struct {
	Writer     FrameWriter
	Runtime    *application.SessionRuntime
	Authorizer LeaseAuthorizer
	Entry      CursorEntryPort
	Returns    ReturnBoundaryPort
}

func (responder *LeaseResponder) HandleLeaseRequest(
	ctx context.Context,
	command transport.ControlCommand,
	observedAt time.Time,
) error {
	if responder == nil || responder.Writer == nil || responder.Runtime == nil || responder.Authorizer == nil || responder.Entry == nil || responder.Returns == nil {
		return errors.New("lease responder is not initialized")
	}
	if command.Kind != transport.ControlLeaseRequest {
		return errors.New("lease responder requires a lease request")
	}
	if responder.Runtime.Snapshot().Control.State != control.StateLocal {
		return errors.New("lease responder requires a local runtime")
	}
	leaseID, accepted, err := responder.Authorizer.AuthorizeLease(ctx, command.SessionID, command.Epoch, observedAt)
	if err != nil {
		return err
	}
	if !accepted {
		return writeControlOrFail(ctx, responder.Writer, responder.Runtime, transport.ControlCommand{
			Kind:      transport.ControlLeaseReject,
			SessionID: command.SessionID,
			Epoch:     command.Epoch,
		})
	}
	if leaseID == "" {
		return errors.New("accepted lease requires an identifier")
	}
	returnIntent := control.ReturnIntent{SessionID: command.SessionID, Epoch: command.Epoch}
	if err := responder.Returns.ArmReturn(returnIntent, command.Edge); err != nil {
		return err
	}
	if err := responder.Entry.Enter(ctx, command.Edge); err != nil {
		responder.Returns.DisarmReturn(returnIntent)
		return err
	}
	if _, err := responder.Runtime.GrantInbound(ctx, control.Event{
		Kind:      control.EventInboundLeaseGranted,
		SessionID: command.SessionID,
		LeaseID:   leaseID,
	}, command.Epoch, observedAt); err != nil {
		responder.Returns.DisarmReturn(returnIntent)
		return err
	}
	err = writeControlOrFail(ctx, responder.Writer, responder.Runtime, transport.ControlCommand{
		Kind:      transport.ControlLeaseGrant,
		SessionID: command.SessionID,
		LeaseID:   leaseID,
		Epoch:     command.Epoch,
	})
	if err != nil {
		responder.Returns.DisarmReturn(returnIntent)
	}
	return err
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

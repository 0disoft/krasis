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
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Connection  interface{ Close() error }
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

func (sink *OutboundActionSink) Interrupt(error) error {
	if sink == nil || sink.Connection == nil {
		return errors.New("outbound action sink is not initialized")
	}
	return sink.Connection.Close()
}

func (sink *OutboundActionSink) SendAction(ctx context.Context, action control.InputAction) error {
	if sink == nil || sink.Coordinator == nil || sink.Runtime == nil {
		return errors.New("outbound action sink is not initialized")
	}
	return SendInputAction(ctx, sink.Coordinator, sink.Runtime, action)
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
	if coordinator == nil || runtime == nil {
		return errors.New("write coordinator and runtime are required")
	}
	return coordinator.Execute(ctx, func(writer FrameWriter) error {
		if _, err := runtime.BeginOutbound(ctx, sessionID, epoch); err != nil {
			return err
		}
		return writeControlOrFail(ctx, writer, runtime, transport.ControlCommand{
			Kind:      transport.ControlLeaseRequest,
			SessionID: sessionID,
			Epoch:     epoch,
			Edge:      edge,
		})
	})
}

func SendInputAction(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	action control.InputAction,
) error {
	if coordinator == nil || runtime == nil {
		return errors.New("write coordinator and runtime are required")
	}
	if err := action.Validate(); err != nil {
		return err
	}
	return coordinator.Execute(ctx, func(writer FrameWriter) error {
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
	})
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
	RollbackEntry(context.Context) error
}

type ReturnBoundaryPort interface {
	ArmReturn(control.ReturnIntent, control.ScreenEdge) error
	RearmReturn(control.ReturnIntent)
	DisarmReturn(control.ReturnIntent)
}

type LeaseResponder struct {
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Authorizer  LeaseAuthorizer
	Peer        transport.AuthenticatedPeerContext
	Entry       CursorEntryPort
	Returns     ReturnBoundaryPort
}

func (responder *LeaseResponder) HandleLeaseRequest(
	ctx context.Context,
	command transport.ControlCommand,
	observedAt time.Time,
) error {
	if responder == nil || responder.Coordinator == nil || responder.Runtime == nil || responder.Authorizer == nil || responder.Entry == nil || responder.Returns == nil {
		return errors.New("lease responder is not initialized")
	}
	if err := responder.Peer.Validate(); err != nil {
		return err
	}
	if command.Kind != transport.ControlLeaseRequest {
		return errors.New("lease responder requires a lease request")
	}
	return responder.Coordinator.Execute(ctx, func(writer FrameWriter) error {
		if responder.Runtime.Snapshot().Control.State != control.StateLocal {
			return errors.New("lease responder requires a local runtime")
		}
		leaseID, accepted, err := responder.Authorizer.AuthorizeLease(ctx, responder.Peer, command.SessionID, command.Epoch, observedAt)
		if err != nil {
			return err
		}
		if !accepted {
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
		if err := responder.Returns.ArmReturn(returnIntent, command.Edge); err != nil {
			return err
		}
		rollbackEntry := func(cause error) error {
			responder.Returns.DisarmReturn(returnIntent)
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
			defer cancel()
			return errors.Join(cause, responder.Entry.RollbackEntry(cleanupCtx))
		}
		if err := responder.Entry.PrepareEntry(ctx, command.Edge); err != nil {
			return rollbackEntry(err)
		}
		if _, err := responder.Runtime.GrantInbound(ctx, control.Event{
			Kind:      control.EventInboundLeaseGranted,
			SessionID: command.SessionID,
			LeaseID:   leaseID,
		}, command.Epoch, observedAt); err != nil {
			return rollbackEntry(err)
		}
		err = writeControlOrFail(ctx, writer, responder.Runtime, transport.ControlCommand{
			Kind:      transport.ControlLeaseGrant,
			SessionID: command.SessionID,
			LeaseID:   leaseID,
			Epoch:     command.Epoch,
		})
		if err != nil {
			return rollbackEntry(err)
		}
		responder.Entry.CommitEntry()
		return nil
	})
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

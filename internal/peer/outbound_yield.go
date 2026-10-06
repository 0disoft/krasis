package peer

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

// YieldLocal uses the same revoke frame as management revocation. Source remains
// unavailable for another grant until the caller acknowledges local cleanup.
func (controller *OutboundController) YieldLocal(ctx context.Context) error {
	if err := controller.validate(); err != nil {
		return err
	}
	var emit func()
	err := controller.Coordinator.Execute(ctx, func(writer FrameWriter) error {
		snapshot := controller.Runtime.Snapshot()
		controller.Source.Deactivate()
		if snapshot.Control.State == control.StateLocal || snapshot.Control.State == control.StateInboundActive {
			return nil // A peer revoke/rollback may have won; never revoke a new inbound lease.
		}
		if snapshot.Control.State != control.StateOutboundActive || !snapshot.HasOutboundStream {
			return errors.New("source local return requires an outbound lease")
		}
		command := transport.ControlCommand{Kind: transport.ControlLeaseRevoke,
			SessionID: snapshot.Control.SessionID, LeaseID: snapshot.Control.LeaseID, Epoch: snapshot.OutboundStream.Epoch}
		if _, err := controller.Runtime.RevokePeer(ctx, command.SessionID, command.LeaseID, command.Epoch); err != nil {
			return err
		}
		if err := writeControlOrFail(ctx, writer, controller.Runtime, command); err != nil {
			return err
		}
		observeControlFlow(controller.Observer, ControlFlowRevokeWriteCompleted)
		emit = handoff.Retire(handoff.Matching(ctx, "source", command.SessionID, command.Epoch), "ok")
		return nil
	})
	if emit != nil {
		emit()
	}
	return err
}

package peer

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

// BidirectionalBoundary permits a physical boundary crossing to reverse an
// existing inbound lease. The normal admission gate still runs before Begin.
type BidirectionalBoundary struct {
	Outbound *OutboundController
	Return   *ReturnController
}

func (boundary *BidirectionalBoundary) Ready() bool {
	if boundary == nil || boundary.Outbound == nil || boundary.Return == nil {
		return false
	}
	if boundary.Outbound.Ready() {
		return true
	}
	if boundary.Outbound.Source == nil || boundary.Outbound.Runtime == nil || boundary.Outbound.Source.Active() {
		return false
	}
	snapshot := boundary.Outbound.Runtime.Snapshot()
	return snapshot.Control.State == control.StateInboundActive && snapshot.PressedCount == 0
}

func (boundary *BidirectionalBoundary) BeginBoundary(ctx context.Context, intent control.BoundaryIntent, identity OutboundIdentity) error {
	if !boundary.Ready() {
		return ErrOutboundNotReady
	}
	if intent.Edge != control.ScreenEdgeLeft && intent.Edge != control.ScreenEdgeRight {
		return errors.New("bidirectional boundary requires a LEFT or RIGHT edge")
	}
	snapshot := boundary.Outbound.Runtime.Snapshot()
	if snapshot.Control.State == control.StateInboundActive {
		// Reuse exact lease revocation and its held-input check. Do not park the
		// local cursor: this machine is immediately becoming the input source.
		returns := *boundary.Return
		returns.Cursor = nil
		err := returns.Revoke(ctx, control.ReturnIntent{SessionID: snapshot.InputStream.SessionID, Epoch: snapshot.InputStream.Epoch})
		if errors.Is(err, application.ErrRuntimeInputHeld) || errors.Is(err, application.ErrRuntimeNotInbound) || errors.Is(err, application.ErrStaleReturnIntent) {
			return ErrOutboundNotReady
		}
		if err != nil {
			return err
		}
	}
	// The shared ordered writer sends REVOKE before the new REQUEST. The peer
	// deactivates its old source before it can grant this opposite-direction lease.
	return boundary.Outbound.BeginBoundary(ctx, intent, identity)
}

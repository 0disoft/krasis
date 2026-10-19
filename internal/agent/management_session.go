package agent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

type TrustRevoker interface {
	RevokeTrust(string, time.Time) error
}

type managementConnection interface {
	peer.FrameWriter
	InterruptLocal(error) error
}

type managedPeerSession struct {
	authenticatedPeer transport.AuthenticatedPeerContext
	runtime           *application.SessionRuntime
	coordinator       *peer.SessionWriteCoordinator
	source            peer.SourceActivation
	connection        managementConnection
	trust             TrustRevoker
	now               func() time.Time
	returns           peer.ReturnBoundaryPort
}

func newManagedPeerSession(
	authenticatedPeer transport.AuthenticatedPeerContext,
	runtime *application.SessionRuntime,
	coordinator *peer.SessionWriteCoordinator,
	source peer.SourceActivation,
	connection managementConnection,
	trust TrustRevoker,
	now func() time.Time,
	returns peer.ReturnBoundaryPort,
) (*managedPeerSession, error) {
	if err := authenticatedPeer.Validate(); err != nil {
		return nil, err
	}
	if runtime == nil || coordinator == nil || source == nil || connection == nil || now == nil || returns == nil {
		return nil, errors.New("managed peer session dependencies are required")
	}
	return &managedPeerSession{
		authenticatedPeer: authenticatedPeer,
		runtime:           runtime,
		coordinator:       coordinator,
		source:            source,
		connection:        connection,
		trust:             trust,
		now:               now,
		returns:           returns,
	}, nil
}

func (session *managedPeerSession) Revoke(ctx context.Context, request LeaseRevocationRequest) error {
	if session == nil || session.runtime == nil || session.coordinator == nil ||
		session.source == nil || session.connection == nil || session.now == nil {
		return errors.New("managed peer session is not initialized")
	}
	if request.Peer != session.authenticatedPeer {
		return ErrManagementPeerMismatch
	}
	return session.coordinator.Execute(ctx, func(writer peer.FrameWriter) error {
		current, active := activeLeaseIdentity(session.runtime.Snapshot())
		if !active || current != request.Lease {
			return ErrManagementLeaseMismatch
		}

		inbound := session.runtime.Snapshot().Control.State == control.StateInboundActive
		session.source.Deactivate()
		if _, err := session.runtime.RevokePeer(
			ctx,
			request.Lease.SessionID,
			request.Lease.LeaseID,
			request.Lease.Epoch,
		); err != nil {
			interruptErr := session.connection.InterruptLocal(ErrManagementLocalCleanup)
			return errors.Join(ErrManagementLocalCleanup, err, interruptErr)
		}
		if inbound {
			// Match peer-initiated and boundary returns: key-up recovery must
			// finish before retiring the exact installed input/return owner.
			session.returns.DisarmReturn(control.ReturnIntent{SessionID: current.SessionID, Epoch: current.Epoch})
		}

		var trustErr error
		if request.Action == LeaseRevocationTrust {
			if session.trust == nil {
				trustErr = errors.Join(ErrManagementTrustRevocation, errors.New("trust revoker is unavailable"))
			} else if err := session.trust.RevokeTrust(request.Peer.CertificateSHA256, session.now()); err != nil {
				trustErr = errors.Join(ErrManagementTrustRevocation, err)
			}
		}

		payload, err := transport.EncodeControlPayload(transport.ControlCommand{
			Kind:      transport.ControlLeaseRevoke,
			SessionID: request.Lease.SessionID,
			LeaseID:   request.Lease.LeaseID,
			Epoch:     request.Lease.Epoch,
		})
		if err != nil {
			interruptErr := session.connection.InterruptLocal(ErrManagementPeerNotification)
			return errors.Join(trustErr, ErrManagementPeerNotification, err, interruptErr)
		}
		if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload}); err != nil {
			interruptErr := session.connection.InterruptLocal(ErrManagementPeerNotification)
			return errors.Join(trustErr, ErrManagementPeerNotification, err, interruptErr)
		}

		if request.Action != LeaseRevocationOnly || trustErr != nil {
			if err := session.connection.InterruptLocal(ErrManagementDisconnect); err != nil {
				return errors.Join(trustErr, ErrManagementDisconnect, err)
			}
		}
		return trustErr
	})
}

func activeLeaseIdentity(snapshot application.RuntimeSnapshot) (application.LeaseIdentity, bool) {
	switch snapshot.Control.State {
	case control.StateInboundActive:
		if !snapshot.HasStream || snapshot.InputStream.Epoch == "" {
			return application.LeaseIdentity{}, false
		}
		return application.LeaseIdentity{
			SessionID: snapshot.Control.SessionID,
			LeaseID:   snapshot.Control.LeaseID,
			Epoch:     snapshot.InputStream.Epoch,
		}, true
	case control.StateOutboundActive:
		if !snapshot.HasOutboundStream || snapshot.OutboundStream.Epoch == "" {
			return application.LeaseIdentity{}, false
		}
		return application.LeaseIdentity{
			SessionID: snapshot.Control.SessionID,
			LeaseID:   snapshot.Control.LeaseID,
			Epoch:     snapshot.OutboundStream.Epoch,
		}, true
	default:
		return application.LeaseIdentity{}, false
	}
}

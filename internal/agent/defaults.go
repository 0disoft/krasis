package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

const leaseIdentifierBytes = 16

type AuthenticatedPeerAuthorizer struct{}

type TrustedPeerGrant struct {
	Peer                transport.AuthenticatedPeerContext
	AllowInboundControl bool
	Revoked             bool
}

type TrustedPeerAuthorizer struct {
	grants map[string]TrustedPeerGrant
}

func NewTrustedPeerAuthorizer(grants []TrustedPeerGrant) (*TrustedPeerAuthorizer, error) {
	if len(grants) == 0 {
		return nil, errors.New("at least one trusted peer grant is required")
	}
	indexed := make(map[string]TrustedPeerGrant, len(grants))
	for _, grant := range grants {
		if err := grant.Peer.Validate(); err != nil {
			return nil, err
		}
		if _, exists := indexed[grant.Peer.CertificateSHA256]; exists {
			return nil, errors.New("trusted peer fingerprint is duplicated")
		}
		indexed[grant.Peer.CertificateSHA256] = grant
	}
	return &TrustedPeerAuthorizer{grants: indexed}, nil
}

func (authorizer *TrustedPeerAuthorizer) AuthorizeLease(
	ctx context.Context,
	peer transport.AuthenticatedPeerContext,
	sessionID,
	epoch string,
	observedAt time.Time,
) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if authorizer == nil || authorizer.grants == nil {
		return "", false, errors.New("trusted peer authorizer is not initialized")
	}
	if err := peer.Validate(); err != nil {
		return "", false, err
	}
	grant, trusted := authorizer.grants[peer.CertificateSHA256]
	if !trusted || grant.Peer.Role != peer.Role || grant.Revoked || !grant.AllowInboundControl {
		return "", false, nil
	}
	return newLeaseIdentifier(sessionID, epoch, observedAt)
}

func (AuthenticatedPeerAuthorizer) AuthorizeLease(
	ctx context.Context,
	peer transport.AuthenticatedPeerContext,
	sessionID,
	epoch string,
	observedAt time.Time,
) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if err := peer.Validate(); err != nil {
		return "", false, err
	}
	return newLeaseIdentifier(sessionID, epoch, observedAt)
}

func newLeaseIdentifier(sessionID, epoch string, observedAt time.Time) (string, bool, error) {
	if sessionID == "" || epoch == "" || observedAt.IsZero() {
		return "", false, errors.New("authenticated lease request is incomplete")
	}
	identifier := make([]byte, leaseIdentifierBytes)
	if _, err := rand.Read(identifier); err != nil {
		return "", false, err
	}
	return hex.EncodeToString(identifier), true, nil
}

type RuntimeEffects struct{}

func (RuntimeEffects) Execute(_ context.Context, effect control.Effect) error {
	switch effect {
	case control.EffectBeginOutbound, control.EffectBeginInbound, control.EffectStopRemoteControl,
		control.EffectRevokeLease, control.EffectResumeLocal:
		return nil
	case control.EffectReleaseAll:
		return errors.New("ReleaseAll must be handled by the safety effect executor")
	default:
		return errors.New("unsupported runtime effect")
	}
}

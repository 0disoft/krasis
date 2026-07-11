package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const leaseIdentifierBytes = 16

type AuthenticatedPeerAuthorizer struct{}

func (AuthenticatedPeerAuthorizer) AuthorizeLease(ctx context.Context, sessionID, epoch string, observedAt time.Time) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
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

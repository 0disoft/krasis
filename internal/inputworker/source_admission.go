package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/peer"
)

// Producer-only: a GUI query marker survives a fully returned desktop scope.
// Matching authentication alone cannot establish this classification.
type sourceInspectionRejected struct{ cause error }

func (e *sourceInspectionRejected) Error() string { return "source GUI inspection unavailable" }
func (e *sourceInspectionRejected) Unwrap() error { return e.cause }

func sourceRetryableInspection(err error) bool {
	return errors.Is(err, peer.ErrBoundaryInspectionUnavailable) && !sourceCleanupUncertain(err) &&
		!errors.Is(err, ErrAuthority) && !errors.Is(err, ErrProtocol) &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, inputdesktop.ErrInactive) && !errors.Is(err, inputdesktop.ErrCallbackPanic) && !errors.Is(err, inputdesktop.ErrCallbackExit)
}

// A matching authenticated negative query reply grants no capture authority.
// Keep it distinct from losing the pipe/authentication or a malformed reply.
func sourceAdmissionResult(p sourcePacket, err error) (peer.BoundaryAdmissionDecision, error) {
	if err != nil {
		if err == ErrAuthority && p.kind == sourceReply && p.op == sourceAdmit && p.reply != 0 && p.status == 1 && p.decision == 0 {
			return 0, errors.Join(peer.ErrBoundaryInspectionUnavailable, err)
		}
		return 0, err
	}
	if !p.decision.Valid() {
		return 0, ErrProtocol
	}
	return p.decision, nil
}

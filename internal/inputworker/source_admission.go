package inputworker

import (
	"errors"

	"github.com/0disoft/krasis/internal/peer"
)

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

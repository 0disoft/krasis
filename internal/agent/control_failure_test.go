package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

func TestControlFailureSeparatesCommandAndCauseWithoutPrivateDetails(t *testing.T) {
	for _, test := range []struct {
		kind        transport.ControlCommandKind
		cause       error
		phase, code string
	}{
		{transport.ControlLeaseCommit, peer.ErrInboundPendingMismatch, "control-commit", "pending_mismatch"},
		{transport.ControlLeaseRevoke, control.ErrInvalidTransition, "control-revoke", "invalid_transition"},
		{transport.ControlLeaseRequest, context.DeadlineExceeded, "control-request", "deadline"},
		{transport.ControlLeaseAbort, errors.New("private-session-and-path"), "control-abort", "failed"},
		{255, peer.ErrInboundPendingMismatch, "control", "failed"},
	} {
		err := errors.Join(peer.ErrPeerControlDispatch, &peer.ControlDispatchFailure{Kind: test.kind, Cause: errors.Join(test.cause, errors.New("private-peer"))})
		value := DescribeSessionFailure(err)
		if value.Component != "peer-dispatch" || value.Phase != test.phase || value.Code != test.code || !errors.Is(err, test.cause) {
			t.Fatalf("unexpected bounded failure: %+v", value)
		}
	}
}

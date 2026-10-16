package agent

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

func describeControlFailure(err error) FailureDescriptor {
	value := FailureDescriptor{Component: "peer-dispatch", Phase: "control", Code: "failed"}
	var failure *peer.ControlDispatchFailure
	if !errors.As(err, &failure) {
		return value
	}
	phase, known := map[transport.ControlCommandKind]string{
		0: "decode", transport.ControlLeaseRequest: "request", transport.ControlLeaseGrant: "grant",
		transport.ControlLeaseReject: "reject", transport.ControlLeaseRevoke: "revoke",
		transport.ControlLeaseCommit: "commit", transport.ControlLeaseAbort: "abort", transport.ControlPrepareInput: "prepare",
	}[failure.Kind]
	if !known {
		return value
	}
	value.Phase = "control-" + phase
	var request *peer.LeaseRequestFailure
	if failure.Kind == transport.ControlLeaseRequest && errors.As(failure.Cause, &request) {
		switch request.Stage {
		case "coordinator", "authorize", "return-arm", "entry", "inbound", "grant":
			value.Phase += "-" + request.Stage
		}
	}
	for _, reason := range []struct {
		cause error
		code  string
	}{
		{peer.ErrInboundPendingMismatch, "pending_mismatch"},
		{application.ErrPendingLeaseExpired, "pending_expired"},
		{control.ErrInvalidTransition, "invalid_transition"},
		{control.ErrInvalidEvent, "invalid_event"},
		{application.ErrRuntimeNotInbound, "not_inbound"},
		{application.ErrRuntimeInputHeld, "input_held"},
		{application.ErrStaleReturnIntent, "stale_return"},
		{control.ErrStaleSession, "stale_session"},
		{control.ErrStaleEpoch, "stale_epoch"},
		{context.DeadlineExceeded, "deadline"},
		{context.Canceled, "canceled"},
	} {
		if errors.Is(failure.Cause, reason.cause) {
			value.Code = reason.code
			break
		}
	}
	return value
}

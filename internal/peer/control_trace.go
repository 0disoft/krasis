package peer

import (
	"context"
	"runtime/trace"

	"github.com/0disoft/krasis/internal/transport"
)

// This marker follows the complete TLS read, not NIC/kernel arrival. Decode
// only while tracing; invalid frames keep their original dispatch/error path.
func traceControlRead(ctx context.Context, payload []byte) {
	if !trace.IsEnabled() {
		return
	}
	var kind transport.ControlCommandKind
	if command, err := transport.DecodeControlPayload(payload); err == nil {
		kind = command.Kind
	}
	trace.Log(ctx, "krasis.peer.control_read", controlTraceName(kind))
}

// All trace labels are fixed program vocabulary, never untrusted wire values.
func controlTraceName(kind transport.ControlCommandKind) string {
	switch kind {
	case transport.ControlPrepareInput:
		return "krasis.peer.prepare_input"
	case transport.ControlLeaseRequest:
		return "krasis.peer.lease_request"
	case transport.ControlLeaseGrant:
		return "krasis.peer.lease_grant"
	case transport.ControlLeaseReject:
		return "krasis.peer.lease_reject"
	case transport.ControlLeaseRevoke:
		return "krasis.peer.lease_revoke"
	case transport.ControlLeaseCommit:
		return "krasis.peer.lease_commit"
	case transport.ControlLeaseAbort:
		return "krasis.peer.lease_abort"
	default:
		return "krasis.peer.control_unknown"
	}
}

package peer

import "github.com/0disoft/krasis/internal/transport"

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

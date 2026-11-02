//go:build windows

package inputworker

// Called by the authenticated serial reader after sequence/authority checks.
// Boundary receipts release only the exact advisory reservation. They need no
// ACK of their own; a bad reference still fails the reader and closes the owner.
// Heartbeats retain their exact matching reply and unchanged liveness policy.
func (w *sourceWriter) handleSourceMaintenance(p sourcePacket) error {
	if p.kind != sourceCommand || !p.valid() || p.binding != w.binding {
		return ErrProtocol
	}
	if p.op == sourceBoundaryAck {
		if w.boundary == nil {
			return ErrProtocol
		}
		return w.boundary.acknowledge(p.reply)
	}
	if p.op != sourceHeartbeat {
		return ErrProtocol
	}
	return sourceCommandFailed(p.op, w.sendCompletedReply(p, sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}))
}

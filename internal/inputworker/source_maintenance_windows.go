//go:build windows

package inputworker

// Called by the authenticated serial reader after sequence/authority checks.
// Consumption receipts release only the exact boundary or eight-action group.
// They need no ACK of their own; a bad reference fails the reader and owner.
// Heartbeats retain their exact matching reply and unchanged liveness policy.
func (w *sourceWriter) handleSourceMaintenance(p sourcePacket) error {
	if p.kind != sourceCommand || !p.valid() || p.binding != w.binding {
		return ErrProtocol
	}
	if p.op == sourceBoundaryAck {
		if w.boundary != nil {
			if r := w.boundary.pending.Load(); r != nil && r.sequence.Load() == p.reply {
				return w.boundary.acknowledge(p.reply)
			}
		}
		if w.actions != nil {
			return w.actions.acknowledge(p.reply)
		}
		return ErrProtocol
	}
	if p.op != sourceHeartbeat {
		return ErrProtocol
	}
	return sourceCommandFailed(p.op, w.sendCompletedReply(p, sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}))
}

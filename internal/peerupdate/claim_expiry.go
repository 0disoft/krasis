package peerupdate

// RejectReady publishes failure only for the exact unclaimed received job.
// It cannot release an active claim, or alter a newer/foreign transfer.
func (m *Manager) RejectReady(id, peer string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != "READY" || m.status.ID != id || m.status.Peer != peer {
		return false
	}
	m.status.State = "APPLY_FAILED"
	m.event("peer_update_apply_failed")
	return true
}

// ExpireClaim releases only the still-applying job that lost its replacement
// UI. A delayed timeout cannot cancel a newer job or a completed update.
func (m *Manager) ExpireClaim(id, peer string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != "APPLYING" || m.status.ID != id || m.status.Peer != peer {
		return false
	}
	m.status.State = "APPLY_FAILED"
	m.event("peer_update_apply_timeout")
	return true
}

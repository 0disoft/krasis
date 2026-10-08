package peerupdate

import "time"

// Caller owns mu. Incoming jobs expire at admission/observation boundaries,
// even if the management window stopped polling. Active claims never expire
// here: their separate replacement timeout owns that fence.
func (m *Manager) expireStagedLocked(now time.Time) {
	state := m.status.State
	if (state != "READY" && state != "STAGED") || (!m.stagedUntil.IsZero() && !now.After(m.stagedUntil)) {
		return
	}
	if state == "READY" {
		m.status.State = "EXPIRED"
	} else {
		m.status.State = "RECEIVE_FAILED"
	}
	m.event("peer_update_receive_expired")
}

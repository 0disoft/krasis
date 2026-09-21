package peerupdate

import "time"

// PeerInspection is a read-only observation of the authenticated management
// channel. An absent or expired observation is never evidence of being offline.
type PeerInspection struct {
	Peer       string `json:"peer"`
	State      string `json:"state"`
	Generation int    `json:"generation"`
	Revision   string `json:"revision"`
	Allowed    bool   `json:"allowed"`
}

func (m *Manager) InspectPeer(peer string, now time.Time) PeerInspection {
	v := PeerInspection{Peer: peer, State: "UNKNOWN"}
	m.mu.Lock()
	defer m.mu.Unlock()
	observation, ok := m.peers[peer]
	if !ok || observation.at.IsZero() {
		return v
	}
	age := now.Sub(observation.at)
	if age < 0 || age > 15*time.Second {
		v.State = "STALE"
		return v
	}
	info := observation.info
	if info.Protocol != 2 || info.Generation < 21 || info.Generation > 1000000 || (info.Revision != "" && !validID(info.Revision, 40)) {
		return v
	}
	v.State = "ONLINE"
	v.Generation = info.Generation
	v.Revision = info.Revision
	v.Allowed = info.Allowed
	return v
}

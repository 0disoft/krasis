package peerupdate

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// PeerInfo is exchanged only inside the pinned mutually authenticated channel.
type PeerInfo struct {
	UpdateID      string `json:"updateID,omitempty"`
	UpdateState   string `json:"updateState,omitempty"`
	Clipboard     bool   `json:"clipboard,omitempty"`
	ClipboardID   string `json:"clipboardID,omitempty"`
	ClipboardSize int    `json:"clipboardSize,omitempty"`
	Protocol      int    `json:"protocol"`
	Generation    int    `json:"generation"`
	Revision      string `json:"revision"`
	Allowed       bool   `json:"allowed"`
}
type peerObservation struct {
	info PeerInfo
	at   time.Time
}

func (m *Manager) info(peer string) PeerInfo {
	allowed, id, size := m.ClipboardInfo(peer)
	info := PeerInfo{Protocol: 2, Generation: m.generation, Revision: m.revision, Allowed: m.permitted(peer), Clipboard: allowed, ClipboardID: id, ClipboardSize: size}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Peer == peer && m.status.State == "APPLY_FAILED" {
		info.UpdateID, info.UpdateState = m.status.ID, m.status.State
	}
	return info
}
func readPeerInfo(reader io.Reader) (PeerInfo, error) {
	var info PeerInfo
	data, err := io.ReadAll(io.LimitReader(reader, 2049))
	if err != nil || len(data) > 2048 {
		return info, ErrDenied
	}
	if json.Unmarshal(data, &info) != nil || info.Protocol != 2 || info.Generation < 21 || info.Generation > 1000000 || (info.Revision != "" && !validID(info.Revision, 40)) {
		return PeerInfo{}, ErrDenied
	}
	if info.ClipboardSize < 0 || info.ClipboardSize > MaxClipboardBytes || (info.ClipboardID != "" && !validID(info.ClipboardID, 32)) {
		return PeerInfo{}, ErrDenied
	}
	if (info.UpdateState != "" && info.UpdateState != "APPLY_FAILED") ||
		(info.UpdateState != "" && !validID(info.UpdateID, 32)) || (info.UpdateState == "" && info.UpdateID != "") {
		return PeerInfo{}, ErrDenied
	}
	return info, nil
}
func (m *Manager) observePeer(peer string, info PeerInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.peers == nil {
		m.peers = map[string]peerObservation{}
	}
	m.peers[peer] = peerObservation{info: info, at: time.Now()}
	if m.status.Peer != peer {
		return
	}
	m.status.PeerGeneration = info.Generation
	m.status.PeerRevision = info.Revision
	if m.status.State == "DELIVERED" || m.status.State == "TRANSFER_UNKNOWN" || m.status.State == "RESTART_UNCONFIRMED" {
		if info.UpdateState == "APPLY_FAILED" && info.UpdateID == m.status.ID && info.UpdateID != "" {
			m.status.State = "PEER_APPLY_FAILED"
		} else if m.status.Revision != "" && info.Revision == m.status.Revision && info.Generation == m.status.Generation {
			m.status.State = "UPDATED"
		}
	}
	if m.out == nil || m.out.peer != peer {
		return
	}
	var reason string
	if !info.Allowed {
		reason = "PEER_NOT_ALLOWED"
	} else if err := compareVersion(info.Generation, info.Revision, m.status.Generation, m.status.Revision); err != nil {
		reason = FailureCode(err)
	}
	if reason != "" {
		m.out = nil
		m.status.State = reason
	}
}
func (m *Manager) failOffer(peer, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.out != nil && m.out.peer == peer {
		m.out = nil
		m.status.State = reason
	}
}
func (m *Manager) progress(peer, id, state string, n, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Peer == peer && m.status.ID == id {
		m.status.State = state
		m.status.Bytes = n
		m.status.Total = total
	}
}

type progressReader struct {
	io.Reader
	update func(int64)
	n      int64
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.n += int64(n)
	r.update(r.n)
	return n, err
}

func (m *Manager) exchangeStatus(client *http.Client, request *http.Request, peer string) bool {
	response, err := client.Do(request)
	if err != nil {
		m.failOffer(peer, "PEER_UNREACHABLE")
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reason := "PEER_UNREACHABLE"
		if response.StatusCode == http.StatusNotFound {
			reason = "PEER_UNSUPPORTED"
		}
		if response.StatusCode == http.StatusForbidden {
			reason = "PEER_NOT_ALLOWED"
		}
		if response.StatusCode == http.StatusTooManyRequests {
			reason = "PEER_BUSY"
		}
		m.failOffer(peer, reason)
		return false
	}
	info, err := readPeerInfo(response.Body)
	if err != nil {
		m.failOffer(peer, "PEER_UNSUPPORTED")
		return false
	}
	m.observePeer(peer, info)
	return true
}

package peerupdate

import "time"

// ClipboardTransfer contains metadata only, never names, paths or clipboard content.
type ClipboardTransfer struct {
	State     string `json:"state"`
	Format    string `json:"format,omitempty"`
	Direction string `json:"direction,omitempty"`
	Code      string `json:"code,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	Total     int64  `json:"total,omitempty"`
}

func (m *Manager) ClipboardTransferStatus() ClipboardTransfer {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if out := m.clipboard.out; out != nil && time.Now().After(out.expires) {
		m.clipboard.out = nil
		if m.clipboard.transferID == out.ID {
			m.clipboard.transfer.State, m.clipboard.transfer.Code = "FAILED", "expired"
		}
	}
	if in := m.clipboard.in; in != nil && time.Now().After(in.expires) {
		m.clipboard.in = nil
		if m.clipboard.transferID == in.ID {
			m.clipboard.transfer.State, m.clipboard.transfer.Code = "FAILED", "apply_expired"
		}
	}
	return m.clipboard.transfer
}

func (m *Manager) clipboardProgress(peer, id, direction, format string, n, total int64) {
	limit := int64(clipboardPayloadLimit(format))
	if n > limit {
		n = limit
	}
	if total > limit {
		total = limit
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer {
		return
	}
	if direction == "SEND" && (m.clipboard.out == nil || m.clipboard.out.ID != id) {
		return
	}
	if direction == "RECEIVE" && m.clipboard.out != nil {
		return
	}
	if direction == "RECEIVE" {
		for _, seen := range m.clipboard.seen {
			if seen == id {
				return
			}
		}
	}
	m.clipboard.transferID = id
	state := "SENDING"
	if direction == "RECEIVE" {
		state = "RECEIVING"
	}
	m.clipboard.transfer = ClipboardTransfer{State: state, Format: format, Direction: direction, Bytes: n, Total: total}
}

func (m *Manager) clipboardReceiveFailed(peer, id string) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer == peer && m.clipboard.transferID == id && m.clipboard.transfer.State == "RECEIVING" {
		m.clipboard.transfer.State, m.clipboard.transfer.Code = "FAILED", "receive_failed"
	}
}

// ReportClipboard records a native preparation/application result after peer authorization.
func (m *Manager) ReportClipboard(peer, id, format, code string) error {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if peer == "" || m.clipboard.peer != peer || !ValidClipboardReportFormat(format) {
		return ErrDenied
	}
	if code == "apply_failed" {
		if m.clipboard.in == nil || m.clipboard.in.ID != id || m.clipboard.in.Format != format {
			return ErrDenied
		}
		m.clipboard.transferID = id
		m.clipboard.transfer = ClipboardTransfer{State: "FAILED", Format: format, Direction: "RECEIVE", Code: code}
		return nil
	}
	if id != "" {
		return ErrDenied
	}
	state := "FAILED"
	if code == "preparing" {
		state = "PREPARING"
	} else if code != "prepare_failed" && code != "size_limit" && code != "item_limit" && code != "clipboard_changed" {
		return ErrDenied
	}
	m.clipboard.transferID = ""
	// A new local copy supersedes previously queued content even if preparation fails.
	m.clipboard.out, m.clipboard.in = nil, nil
	m.clipboard.localEpoch++
	m.clipboard.transfer = ClipboardTransfer{State: state, Format: format, Direction: "SEND", Code: code}
	m.clipboardChanged()
	return nil
}

// Report formats are the exact wire formats, including empty for Unicode text.
// Keep application failures bound to the received payload's format and ID.
func ValidClipboardReportFormat(format string) bool {
	switch format {
	case "", "png", "dib", "files":
		return true
	default:
		return false
	}
}

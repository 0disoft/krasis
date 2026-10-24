package peerupdate

import (
	"bytes"
	"time"
)

// Caller holds m.mu and clipboard.mu, in that order.
func (m *Manager) noteClipboardLocalChangeLocked(peer string) {
	if remoteID := m.peers[peer].info.ClipboardID; remoteID != "" {
		m.clipboard.seen = append(m.clipboard.seen, remoteID)
		if len(m.clipboard.seen) > 64 {
			m.clipboard.seen = m.clipboard.seen[1:]
		}
	}
	m.clipboard.localEpoch++
	m.clipboard.out, m.clipboard.in = nil, nil
	m.clipboardChanged()
}

func (m *Manager) reserveClipboardLocal(peer, format string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if peer == "" || m.clipboard.peer != peer {
		return 0, ErrDenied
	}
	m.clipboard.activeUntil = time.Now().Add(5 * time.Second)
	m.noteClipboardLocalChangeLocked(peer)
	if format != "" && !m.clipboard.imageAllowed || format == "files" && !m.clipboard.filesAllowed {
		return 0, ErrDenied
	}
	return m.clipboard.localEpoch, nil
}

// Reserve order before expensive preparation, then publish only if the same
// selection and local copy still own the slot. No payload work holds m.mu or
// clipboard.mu. The callback permits deterministic tests of concurrent changes.
func (m *Manager) queueClipboardLocal(peer string, local *ClipboardValue, prepare func(*ClipboardValue) (*ClipboardValue, error)) error {
	epoch, err := m.reserveClipboardLocal(peer, local.Format)
	if err != nil {
		return err
	}
	if local.Format != "" {
		// Keep at most one large copy/validation in flight. Text and selection
		// changes remain responsive and can invalidate a waiting media copy.
		m.clipboard.prepareMu.Lock()
		defer m.clipboard.prepareMu.Unlock()
		m.clipboard.mu.Lock()
		current := m.clipboard.peer == peer && m.clipboard.localEpoch == epoch
		m.clipboard.mu.Unlock()
		if !current {
			return ErrDenied
		}
	}
	value, err := prepare(local)
	if err != nil {
		return err
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || m.clipboard.localEpoch != epoch ||
		value.Format != "" && !m.clipboard.imageAllowed || value.Format == "files" && !m.clipboard.filesAllowed {
		return ErrDenied
	}
	lifetime := 15 * time.Second
	if value.Format == "files" {
		lifetime = 75 * time.Second
	}
	value.expires = time.Now().Add(lifetime)
	m.clipboard.activeUntil = time.Now().Add(5 * time.Second)
	m.clipboard.out, m.clipboard.in = value, nil
	m.clipboard.transferID = value.ID
	m.clipboard.transfer = ClipboardTransfer{State: "QUEUED", Format: value.Format, Direction: "SEND", Total: int64(len(value.Data) + len(value.Text))}
	m.clipboardChanged()
	return nil
}

func prepareClipboardLocal(local *ClipboardValue) (*ClipboardValue, error) {
	if local.Format == "" {
		if len(local.Data) != 0 || !validClipboardText(local.Text) {
			return nil, ErrDenied
		}
	} else if local.Text != "" || len(local.Data) == 0 || len(local.Data) > clipboardPayloadLimit(local.Format) {
		return nil, ErrDenied
	}
	value := &ClipboardValue{ID: randomID(), Text: local.Text, Format: local.Format, Data: bytes.Clone(local.Data)}
	if value.Format != "" && !ValidClipboardPayload(value.Format, value.Data) {
		return nil, ErrDenied
	}
	value.legacyFiles = value.Format == "files" && clipboardFilesFitLegacy(value.Data)
	return value, nil
}

package peerupdate

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxClipboardBytes = 64 << 10

type ClipboardValue struct {
	ID      string `json:"id,omitempty"`
	Text    string `json:"text,omitempty"`
	Format  string `json:"format,omitempty"`
	Data    []byte `json:"data,omitempty"`
	expires time.Time
}
type clipboardState struct {
	mu          sync.Mutex
	peer        string
	imageAllowed bool
	out, in     *ClipboardValue
	seen        []string
	activeUntil time.Time
}

func (m *Manager) SetClipboardPeer(peer string) error {
	if peer != "" && !validID(peer, 64) {
		return ErrDenied
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if err := m.writeClipboardSetting("clipboard-peer.txt", peer); err != nil {
		return err
	}
	if m.generation >= 69 {
		if err := m.writeClipboardSetting("clipboard-media-peer.txt", peer); err != nil {
			return err
		}
	}
	m.clipboard.peer = peer
	m.clipboard.imageAllowed = peer != "" && m.generation >= 69
	m.clipboard.in = nil
	m.clipboard.out = nil
	m.clipboard.seen = nil
	m.clipboard.activeUntil = time.Time{}
	return nil
}

func (m *Manager) writeClipboardSetting(target, value string) error {
	name := randomID() + ".clip.tmp"
	f, err := m.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer m.root.Remove(name)
	_, err = f.WriteString(value)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err = m.root.Rename(name, target); err != nil {
		return err
	}
	return nil
}
func (m *Manager) ClipboardPeer() string {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	return m.clipboard.peer
}
func (m *Manager) ClipboardInfo(peer string) (bool, string, int) {
	allowed, id, size, _ := m.ClipboardPayloadInfo(peer)
	return allowed, id, size
}
func (m *Manager) ClipboardPayloadInfo(peer string) (bool, string, int, string) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || time.Now().After(m.clipboard.activeUntil) {
		return false, "", 0, ""
	}
	if m.clipboard.out != nil && time.Now().After(m.clipboard.out.expires) {
		m.clipboard.out = nil
	}
	if m.clipboard.out == nil {
		return true, "", 0, ""
	}
	if m.clipboard.out.Format != "" {
		return true, m.clipboard.out.ID, len(m.clipboard.out.Data), m.clipboard.out.Format
	}
	return true, m.clipboard.out.ID, len(m.clipboard.out.Text), ""
}
func validClipboardText(text string) bool {
	return len(text) <= MaxClipboardBytes && utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}
func (m *Manager) SyncClipboard(peer string, text *string, ack string) (ClipboardValue, error) {
	var local *ClipboardValue
	if text != nil {
		local = &ClipboardValue{Text: *text}
	}
	return m.SyncClipboardPayload(peer, local, ack)
}
func (m *Manager) SyncClipboardPayload(peer string, local *ClipboardValue, ack string) (ClipboardValue, error) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if peer == "" || m.clipboard.peer != peer {
		return ClipboardValue{}, ErrDenied
	}
	m.clipboard.activeUntil = time.Now().Add(5 * time.Second)
	if local != nil {
		if local.Format != "" && !m.clipboard.imageAllowed {
			return ClipboardValue{}, ErrDenied
		}
		if local.Format == "" && (len(local.Data) != 0 || !validClipboardText(local.Text)) ||
			local.Format != "" && (local.Text != "" || !ValidClipboardImage(local.Format, local.Data)) {
			return ClipboardValue{}, ErrDenied
		}
		m.clipboard.out = &ClipboardValue{ID: randomID(), Text: local.Text, Format: local.Format,
			Data: append([]byte(nil), local.Data...), expires: time.Now().Add(15 * time.Second)}
		m.clipboard.in = nil // A new local copy supersedes a pending remote copy.
	}
	if m.clipboard.in != nil && (m.clipboard.in.ID == ack || time.Now().After(m.clipboard.in.expires)) {
		m.clipboard.in = nil
	}
	if m.clipboard.in == nil {
		return ClipboardValue{}, nil
	}
	value := *m.clipboard.in
	value.Data = append([]byte(nil), value.Data...)
	return value, nil
}
func (m *Manager) clipboardOutgoing(peer, id string) *ClipboardValue {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || m.clipboard.out == nil || m.clipboard.out.ID != id || time.Now().After(m.clipboard.out.expires) {
		return nil
	}
	copy := *m.clipboard.out
	copy.Data = append([]byte(nil), copy.Data...)
	return &copy
}
func (m *Manager) clipboardSent(peer, id string) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer == peer && m.clipboard.out != nil && m.clipboard.out.ID == id {
		m.clipboard.out = nil
	}
}
func (m *Manager) clipboardReceive(peer, id string, r io.Reader, authorized func() bool) error {
	return m.clipboardReceivePayload(peer, id, "", r, authorized)
}
func (m *Manager) clipboardReceivePayload(peer, id, format string, r io.Reader, authorized func() bool) error {
	if !validID(id, 32) || !authorized() {
		return ErrDenied
	}
	limit := MaxClipboardBytes
	if format != "" {
		limit = MaxClipboardImageBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil || !authorized() || format == "" && !validClipboardText(string(data)) ||
		format != "" && !ValidClipboardImage(format, data) {
		return ErrDenied
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || time.Now().After(m.clipboard.activeUntil) ||
		format != "" && !m.clipboard.imageAllowed {
		return ErrDenied
	}
	for _, seen := range m.clipboard.seen {
		if seen == id {
			return nil
		}
	}
	if m.clipboard.out != nil && time.Now().Before(m.clipboard.out.expires) {
		return ErrBusy
	}
	value := &ClipboardValue{ID: id, Format: format, expires: time.Now().Add(10 * time.Second)}
	if format == "" {
		value.Text = string(data)
	} else {
		value.Data = data
	}
	m.clipboard.in = value
	m.clipboard.seen = append(m.clipboard.seen, id)
	if len(m.clipboard.seen) > 64 {
		m.clipboard.seen = m.clipboard.seen[1:]
	}
	return nil
}

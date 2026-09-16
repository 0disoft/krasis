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
	expires time.Time
}
type clipboardState struct {
	mu          sync.Mutex
	peer        string
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
	name := randomID() + ".clip.tmp"
	f, err := m.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer m.root.Remove(name)
	_, err = f.WriteString(peer)
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
	if err = m.root.Rename(name, "clipboard-peer.txt"); err != nil {
		return err
	}
	m.clipboard.peer = peer
	m.clipboard.in = nil
	m.clipboard.out = nil
	m.clipboard.seen = nil
	m.clipboard.activeUntil = time.Time{}
	return nil
}
func (m *Manager) ClipboardPeer() string {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	return m.clipboard.peer
}
func (m *Manager) ClipboardInfo(peer string) (bool, string, int) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || time.Now().After(m.clipboard.activeUntil) {
		return false, "", 0
	}
	if m.clipboard.out != nil && time.Now().After(m.clipboard.out.expires) {
		m.clipboard.out = nil
	}
	if m.clipboard.out == nil {
		return true, "", 0
	}
	return true, m.clipboard.out.ID, len(m.clipboard.out.Text)
}
func validClipboardText(text string) bool {
	return len(text) <= MaxClipboardBytes && utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}
func (m *Manager) SyncClipboard(peer string, text *string, ack string) (ClipboardValue, error) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if peer == "" || m.clipboard.peer != peer {
		return ClipboardValue{}, ErrDenied
	}
	m.clipboard.activeUntil = time.Now().Add(5 * time.Second)
	if text != nil {
		if !validClipboardText(*text) {
			return ClipboardValue{}, ErrDenied
		}
		m.clipboard.out = &ClipboardValue{ID: randomID(), Text: *text, expires: time.Now().Add(15 * time.Second)}
		m.clipboard.in = nil // A new local copy supersedes a pending remote copy.
	}
	if m.clipboard.in != nil && (m.clipboard.in.ID == ack || time.Now().After(m.clipboard.in.expires)) {
		m.clipboard.in = nil
	}
	if m.clipboard.in == nil {
		return ClipboardValue{}, nil
	}
	return *m.clipboard.in, nil
}
func (m *Manager) clipboardOutgoing(peer, id string) *ClipboardValue {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || m.clipboard.out == nil || m.clipboard.out.ID != id || time.Now().After(m.clipboard.out.expires) {
		return nil
	}
	copy := *m.clipboard.out
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
	if !validID(id, 32) || !authorized() {
		return ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxClipboardBytes+1))
	if err != nil || !validClipboardText(string(data)) || !authorized() {
		return ErrDenied
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || time.Now().After(m.clipboard.activeUntil) {
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
	m.clipboard.in = &ClipboardValue{ID: id, Text: string(data), expires: time.Now().Add(10 * time.Second)}
	m.clipboard.seen = append(m.clipboard.seen, id)
	if len(m.clipboard.seen) > 64 {
		m.clipboard.seen = m.clipboard.seen[1:]
	}
	return nil
}

package peerupdate

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/0disoft/krasis/internal/syncgate"
)

const MaxClipboardBytes = 64 << 10

func clipboardPayloadLimit(format string) int {
	switch format {
	case "files":
		return MaxClipboardFileBytes
	case "png", "dib":
		return MaxClipboardImageBytes
	default:
		return MaxClipboardBytes
	}
}

type ClipboardValue struct {
	ID          string `json:"id,omitempty"`
	Text        string `json:"text,omitempty"`
	Format      string `json:"format,omitempty"`
	Data        []byte `json:"data,omitempty"`
	expires     time.Time
	legacyFiles bool
}
type clipboardState struct {
	localEpoch   uint64
	transfer     ClipboardTransfer
	transferID   string
	mu           sync.Mutex
	prepareMu    syncgate.Mutex // Bound media preparation; departed requests need not wait for its owner.
	receiving    bool
	peer         string
	imageAllowed bool
	filesAllowed bool
	out, in      *ClipboardValue
	seen         []string
	activeUntil  time.Time
}

func (m *Manager) SetClipboardPeer(peer string) error {
	if peer != "" && !validID(peer, 64) {
		return ErrDenied
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	return m.setClipboardPeerLocked(peer)
}

func (m *Manager) EnableClipboardPeer(peer string) (bool, error) {
	if !validID(peer, 64) {
		return false, ErrDenied
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != "" {
		return m.clipboard.peer == peer, nil
	}
	return true, m.setClipboardPeerLocked(peer)
}

// Clear only the peer the caller observed. Another UI or CLI may have changed
// the selection since that observation; a stale off request must not clear it.
func (m *Manager) ClearClipboardPeer(expected string) (bool, error) {
	if !validID(expected, 64) {
		return false, ErrDenied
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer == "" {
		return true, nil
	}
	if m.clipboard.peer != expected {
		return false, nil
	}
	return true, m.setClipboardPeerLocked("")
}

func (m *Manager) setClipboardPeerLocked(peer string) error {
	if err := m.writeClipboardSetting("clipboard-peer.txt", peer); err != nil {
		return err
	}
	if m.generation >= 69 {
		if err := m.writeClipboardSetting("clipboard-media-peer.txt", peer); err != nil {
			return err
		}
	}
	if m.generation >= 70 {
		if err := m.writeClipboardSetting("clipboard-files-peer.txt", peer); err != nil {
			return err
		}
	}
	m.clipboard.peer = peer
	m.clipboard.localEpoch++
	m.clipboard.imageAllowed = peer != "" && m.generation >= 69
	m.clipboard.filesAllowed = peer != "" && m.generation >= 70
	m.clipboard.in = nil
	m.clipboard.out = nil
	m.clipboard.seen = nil
	m.clipboard.activeUntil = time.Time{}
	m.clipboard.transfer, m.clipboard.transferID = ClipboardTransfer{}, ""
	m.clipboardChanged()
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
	allowed, id, size, _, _ := m.ClipboardPayloadInfo(peer)
	return allowed, id, size
}
func (m *Manager) ClipboardPayloadInfo(peer string) (bool, string, int, string, bool) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || time.Now().After(m.clipboard.activeUntil) {
		return false, "", 0, "", false
	}
	if m.clipboard.out != nil && time.Now().After(m.clipboard.out.expires) {
		if m.clipboard.transferID == m.clipboard.out.ID {
			m.clipboard.transfer.State, m.clipboard.transfer.Code = "FAILED", "expired"
		}
		m.clipboard.out = nil
	}
	if m.clipboard.out == nil {
		return true, "", 0, "", false
	}
	if m.clipboard.out.Format != "" {
		return true, m.clipboard.out.ID, len(m.clipboard.out.Data), m.clipboard.out.Format, m.clipboard.out.legacyFiles
	}
	return true, m.clipboard.out.ID, len(m.clipboard.out.Text), "", false
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
	return m.SyncClipboardLocalChange(context.Background(), peer, local, ack, false)
}

// Local copy order is independent of whether its content is shareable.
func (m *Manager) SyncClipboardLocalChange(ctx context.Context, peer string, local *ClipboardValue, ack string, changed bool) (ClipboardValue, error) {
	if ctx == nil {
		return ClipboardValue{}, ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return ClipboardValue{}, err
	}
	if local != nil {
		return ClipboardValue{}, m.queueClipboardLocal(ctx, peer, local, prepareClipboardLocal)
	}
	value, err := m.clipboardIncomingSnapshot(peer, ack, changed)
	if err == nil {
		// The queue owns immutable bytes. Copy for the caller only after the
		// state locks are released, including when an ACK retires the queue.
		value.Data = bytes.Clone(value.Data)
	}
	return value, err
}

// Internal snapshot only: its Data remains read-only until cloned by the public
// API. Queue replacement/retirement never mutates an admitted payload's bytes.
func (m *Manager) clipboardIncomingSnapshot(peer, ack string, changed bool) (ClipboardValue, error) {
	if changed {
		m.mu.Lock()
		defer m.mu.Unlock()
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if peer == "" || m.clipboard.peer != peer {
		return ClipboardValue{}, ErrDenied
	}
	m.clipboard.activeUntil = time.Now().Add(5 * time.Second)
	if changed {
		m.noteClipboardLocalChangeLocked(peer)
	}
	if m.clipboard.in != nil && (m.clipboard.in.ID == ack || time.Now().After(m.clipboard.in.expires)) {
		if m.clipboard.transferID == m.clipboard.in.ID {
			if m.clipboard.in.ID == ack {
				m.clipboard.transfer.State, m.clipboard.transfer.Code = "APPLIED", ""
			} else {
				m.clipboard.transfer.State, m.clipboard.transfer.Code = "FAILED", "apply_expired"
			}
		}
		m.clipboard.in = nil
	}
	if m.clipboard.in == nil {
		return ClipboardValue{}, nil
	}
	return *m.clipboard.in, nil
}

// Queued payloads are copied on admission and never mutated afterward. Each
// transfer gets its own reader, retaining the old bytes if the queue changes
// without exposing a writable slice or copying up to 64MiB under the mutex.
type clipboardSnapshot struct {
	ID, Text, Format string
	size             int
	legacyFiles      bool
	body             io.Reader
}

func (m *Manager) clipboardOutgoing(peer, id string) *clipboardSnapshot {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || m.clipboard.out == nil || m.clipboard.out.ID != id || time.Now().After(m.clipboard.out.expires) {
		return nil
	}
	out := m.clipboard.out
	var body io.Reader
	if out.Format == "" {
		body = strings.NewReader(out.Text)
	} else {
		body = bytes.NewReader(out.Data)
	}
	return &clipboardSnapshot{ID: out.ID, Text: out.Text, Format: out.Format,
		size: len(out.Text) + len(out.Data), legacyFiles: out.legacyFiles, body: body}
}
func (m *Manager) clipboardSent(peer, id string) {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer == peer && m.clipboard.out != nil && m.clipboard.out.ID == id {
		if m.clipboard.transferID == id {
			m.clipboard.transfer.State, m.clipboard.transfer.Code = "SENT", ""
			m.clipboard.transfer.Bytes = m.clipboard.transfer.Total
		}
		m.clipboard.out = nil
		m.clipboardChanged()
	}
}

// Seen IDs include received copies and copies superseded by local input. Both
// are terminal for this peer and need only a receipt retry, not another body.
func (m *Manager) clipboardSeen(peer, id string) bool {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	return m.clipboard.peer == peer && time.Now().Before(m.clipboard.activeUntil) && slices.Contains(m.clipboard.seen, id)
}

func (m *Manager) clipboardReceive(peer, id string, r io.Reader, authorized func() bool) error {
	return m.clipboardReceivePayload(peer, id, "", r, authorized)
}
func (m *Manager) clipboardReceivePayload(peer, id, format string, r io.Reader, authorized func() bool) error {
	return m.receiveClipboardPayload(peer, id, format, r, authorized, false)
}

func (m *Manager) receiveClipboardPayload(peer, id, format string, r io.Reader, authorized func() bool, advertised bool) error {
	if !validID(id, 32) || !authorized() {
		return ErrDenied
	}
	limit := clipboardPayloadLimit(format)
	m.clipboard.mu.Lock()
	active := m.clipboard.peer == peer && time.Now().Before(m.clipboard.activeUntil) &&
		(format == "" || m.clipboard.imageAllowed) && (format != "files" || m.clipboard.filesAllowed)
	epoch := m.clipboard.localEpoch
	m.clipboard.mu.Unlock()
	if !active {
		return ErrDenied
	}
	if advertised {
		m.mu.Lock()
		remote := m.peers[peer].info
		m.mu.Unlock()
		if !remote.Clipboard || remote.ClipboardID != id || remote.ClipboardFormat != format ||
			remote.ClipboardSize < 0 || remote.ClipboardSize > limit {
			return ErrDenied
		}
		// Enforce the advertised size before reading, not just before publication.
		// The extra byte distinguishes an exact body from an oversized one.
		limit = remote.ClipboardSize
	}
	// A terminal copy needs only a receipt, as in the GET retry path. Keep
	// selection/advertisement checks above and recheck live transfer authority.
	if m.clipboardSeen(peer, id) {
		if !authorized() {
			return ErrDenied
		}
		return nil
	}
	if !m.beginClipboardReceive() {
		return ErrBusy
	}
	defer func() {
		m.clipboard.mu.Lock()
		m.clipboard.receiving = false
		m.clipboard.mu.Unlock()
	}()
	// Only the admitted receiver owns failure reporting for its active copy.
	defer m.clipboardReceiveFailed(peer, id)
	progress := &progressReader{Reader: r, update: func(n int64) { m.clipboardProgress(peer, id, "RECEIVE", format, n, 0) }}
	var data []byte
	var err error
	if advertised {
		data, err = readClipboardBody(progress, limit)
	} else {
		// Legacy callers do not know the actual size. Do not reserve the
		// entire format limit for a potentially small local payload.
		data, err = io.ReadAll(io.LimitReader(progress, int64(limit)+1))
	}
	if err != nil || advertised && len(data) != limit || !authorized() || format == "" && !validClipboardText(string(data)) ||
		format != "" && !ValidClipboardPayload(format, data) {
		return ErrDenied
	}
	if advertised {
		m.mu.Lock()
		defer m.mu.Unlock()
		remote := m.peers[peer].info
		if !remote.Clipboard || remote.ClipboardID != id || remote.ClipboardFormat != format || remote.ClipboardSize != len(data) {
			return ErrDenied
		}
	}
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.peer != peer || m.clipboard.localEpoch != epoch ||
		format != "" && !m.clipboard.imageAllowed || format == "files" && !m.clipboard.filesAllowed {
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
	expires := 10 * time.Second
	if format == "files" {
		expires = 60 * time.Second
	}
	value := &ClipboardValue{ID: id, Format: format, expires: time.Now().Add(expires)}
	if format == "" {
		value.Text = string(data)
	} else {
		value.Data = data
	}
	m.clipboard.in = value
	m.clipboardChanged()
	m.clipboard.transferID = id
	m.clipboard.transfer = ClipboardTransfer{State: "READY", Format: format, Direction: "RECEIVE", Bytes: int64(len(data)), Total: int64(len(data))}
	m.clipboard.seen = append(m.clipboard.seen, id)
	if len(m.clipboard.seen) > 64 {
		m.clipboard.seen = m.clipboard.seen[1:]
	}
	return nil
}

// HTTP uploads, polling downloads and replacement handlers share one body read
// and validation slot. Selection changes must not free a slot still in use.
// Keep the mutex out of body reads; UI changes can still cancel the transfer.
func (m *Manager) beginClipboardReceive() bool {
	m.clipboard.mu.Lock()
	defer m.clipboard.mu.Unlock()
	if m.clipboard.receiving {
		return false
	}
	m.clipboard.receiving = true
	return true
}

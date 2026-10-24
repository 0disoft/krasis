package peerupdate

import (
	"context"
	"time"
)

// Notifications alone do not invalidate a transfer: progress and application
// acknowledgements can wake the same channel. Its copy/selection epoch must
// actually change. Keep the established global -> clipboard lock order.
// Own the format-specific timeout here so every upload and download is bounded,
// including inbound POSTs whose request context has no deadline of its own.
func (m *Manager) clipboardTransferContext(ctx context.Context, peer, id, format string, size int, send bool) (context.Context, func(), func() bool) {
	m.clipboard.mu.Lock()
	epoch := m.clipboard.localEpoch
	m.clipboard.mu.Unlock()
	current := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.clipboard.mu.Lock()
		defer m.clipboard.mu.Unlock()
		remote := m.peers[peer].info
		if m.clipboard.peer != peer || m.clipboard.localEpoch != epoch || !remote.Clipboard {
			return false
		}
		if send {
			out := m.clipboard.out
			return out != nil && out.ID == id && time.Now().Before(out.expires)
		}
		return remote.ClipboardID == id && remote.ClipboardFormat == format && remote.ClipboardSize == size
	}
	ctx, cancel := context.WithTimeout(ctx, clipboardTransferTimeout(format))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, changed := m.ClipboardChanges()
			if !current() {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-changed:
			}
		}
	}()
	if !current() {
		cancel()
	}
	return ctx, func() { cancel(); <-done }, current
}

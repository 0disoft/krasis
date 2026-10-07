package peerupdate

import (
	"context"
	"sync"
)

// A coalesced broadcast carries only a counter. No clipboard content or peer
// identity enters notifications. Separate from payload/network ownership locks.
type clipboardChanges struct {
	mu       sync.Mutex
	revision uint64
	wake     chan struct{}
}

func (m *Manager) ClipboardChanges() (uint64, <-chan struct{}) {
	m.clipChanges.mu.Lock()
	defer m.clipChanges.mu.Unlock()
	if m.clipChanges.wake == nil {
		m.clipChanges.wake = make(chan struct{})
	}
	return m.clipChanges.revision, m.clipChanges.wake
}
func (m *Manager) clipboardChanged() {
	m.clipChanges.mu.Lock()
	defer m.clipChanges.mu.Unlock()
	m.clipChanges.revision++
	if m.clipChanges.wake != nil {
		close(m.clipChanges.wake)
	}
	m.clipChanges.wake = make(chan struct{})
}
func (m *Manager) WaitClipboardChange(ctx context.Context, after uint64) uint64 {
	revision, wake := m.ClipboardChanges()
	if revision != after {
		return revision
	}
	select {
	case <-wake:
	case <-ctx.Done():
	}
	revision, _ = m.ClipboardChanges()
	return revision
}

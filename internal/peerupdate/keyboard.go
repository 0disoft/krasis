package peerupdate

import (
	"github.com/0disoft/krasis/internal/keyboardstate"
	"time"
)

const keyboardLifetime = 6 * time.Second

type keyboardObservation struct {
	state keyboardstate.State
	at    time.Time
}

func (m *Manager) ReportKeyboard(state keyboardstate.State, now time.Time) bool {
	if !state.Valid() {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.keyboard = keyboardObservation{state: state, at: now}
	return true
}

func (m *Manager) PeerKeyboard(peer string, now time.Time) keyboardstate.State {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.peers[peer]
	age := now.Sub(v.at)
	if !ok || v.at.IsZero() || age < 0 || v.info.Keyboard == nil || !v.info.Keyboard.Valid() ||
		v.info.KeyboardAgeMillis < 0 || age+time.Duration(v.info.KeyboardAgeMillis)*time.Millisecond >= keyboardLifetime {
		return keyboardstate.Unknown()
	}
	return *v.info.Keyboard
}

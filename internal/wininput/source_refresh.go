package wininput

import "errors"

// The authenticated external native owner checks actual physical presses on its
// bound desktop. Do not query the medium agent's obsolete desktop during grant
// activation. Never erase a held journal or change this while forwarding.
func (session *SourceSession) UseExternalPhysicalCapture() bool {
	if session == nil {
		return false
	}
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	if !session.capturePaused || len(session.physicalHeld) != 0 {
		return false
	}
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	if session.active.Load() || session.terminal.Load() || session.yieldPending != 0 {
		return false
	}
	session.physicalCheck = nil
	session.advancePhysicalRevision()
	return true
}

// Only a verified idle native-owner replacement uses this cancellation cause.
// Ordinary shutdown and failures still retire the source sender.
var ErrSourceCaptureRefresh = errors.New("idle source capture replacement")

func (session *SourceSession) captureRefreshPending() bool {
	if session == nil {
		return false
	}
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.capturePaused && !session.active.Load() && !session.terminal.Load()
}

// PauseCapture fences grant activation under the same gate as physical presses.
// Replacing an idle native owner does not retire the sender or its connection.
// A live outbound generation, local press, or pending local return must use the
// existing full-session recovery instead. No held-input record is erased.
func (session *SourceSession) PauseCapture() bool {
	if session == nil {
		return false
	}
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	if session.terminal.Load() || session.active.Load() || session.yieldPending != 0 || len(session.physicalHeld) != 0 {
		return false
	}
	session.capturePaused = true
	session.advancePhysicalRevision()
	return true
}

// ResumeCapture is called only after the replacement native owners are ready.
// Activation still checks both the physical ledger and the native pressed state.
func (session *SourceSession) ResumeCapture() {
	if session != nil {
		session.physicalMu.Lock()
		session.capturePaused = false
		session.advancePhysicalRevision()
		session.physicalMu.Unlock()
	}
}

// PhysicalPressCount is aggregate evidence only; identities never leave the
// physical hook owner. A restart must not disguise a missing release as empty.
func (session *SourceSession) PhysicalPressCount() int {
	if session == nil {
		return 0
	}
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	return len(session.physicalHeld)
}

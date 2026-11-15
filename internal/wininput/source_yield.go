package wininput

// RequestLocalReturn runs on the capture owner. Stop suppression and invalidate
// queued actions synchronously, but keep the source worker reusable. The agent
// owns native cleanup and the exact lease notification under the write gate.
func (session *SourceSession) RequestLocalReturn() bool {
	if session == nil {
		return false
	}
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	return session.requestLocalReturnLocked()
}

// Caller holds stateMu; pending return fences another grant until revoke finishes.
func (session *SourceSession) requestLocalReturnLocked() bool {
	if !session.active.Load() || session.terminal.Load() || session.yieldPending != 0 {
		return false
	}
	session.yieldPending = session.generation.Load()
	session.active.Store(false)
	session.generation.Add(1)
	session.yields <- session.yieldPending // One pending generation, capacity one.
	return true
}

// Only the owner of a verified, joined native retirement may call this. Fence
// replacement grants before requesting the ordinary outbound revoke; completing
// that revoke must not reopen capture before the replacement owner is ready.
func (session *SourceSession) PauseRetiredCapture() bool {
	if session == nil {
		return false
	}
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	if session.terminal.Load() || len(session.physicalHeld) != 0 {
		return false
	}
	session.capturePaused = true
	session.advancePhysicalRevision()
	if session.active.Load() {
		return session.requestLocalReturnLocked()
	}
	return true // A previous return may already be pending; its owner still joins it.
}

func (session *SourceSession) LocalReturns() <-chan uint64 { return session.yields }

func (session *SourceSession) LocalReturnPending() bool {
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.yieldPending != 0
}

func (session *SourceSession) CompleteLocalReturn(generation uint64) {
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	if generation != 0 && session.yieldPending == generation {
		session.yieldPending = 0
	}
}

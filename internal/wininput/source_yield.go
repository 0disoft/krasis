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
	if !session.active.Load() || session.terminal.Load() || session.yieldPending != 0 {
		return false
	}
	session.yieldPending = session.generation
	session.active.Store(false)
	session.generation++
	session.yields <- session.yieldPending // One pending generation, capacity one.
	return true
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

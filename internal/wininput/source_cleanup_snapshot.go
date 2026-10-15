package wininput

// Completion observations are diagnostic only; they never authorize capture or
// replace the cleanup result. Returning from Run alone is not sender join proof.
func (session *SourceSession) CleanupCompletion() (started, sender, interrupt bool) {
	if session == nil {
		return false, false, false
	}
	state := session.cleanupState.Load()
	return state&1 != 0, state&2 != 0, state&4 != 0
}

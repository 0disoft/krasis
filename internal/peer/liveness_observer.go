package peer

import "time"

type LivenessObserver interface {
	// Implementations must return immediately and must not perform network or
	// filesystem I/O because observations run on session worker paths.
	ObserveHeartbeatSent(sessionID string, observedAt time.Time)
	ObserveHeartbeatReceived(sessionID string, observedAt time.Time)
}

type LivenessPulse struct {
	Active     bool
	Sent       bool
	SessionID  string
	ObservedAt time.Time
}

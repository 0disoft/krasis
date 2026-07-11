package control

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidLiveness = errors.New("invalid peer liveness")
	ErrStaleHeartbeat  = errors.New("stale peer heartbeat")
)

type PeerLiveness struct {
	SessionID    string
	LastObserved time.Time
}

func NewPeerLiveness(sessionID string, observedAt time.Time) (PeerLiveness, error) {
	if sessionID == "" || observedAt.IsZero() {
		return PeerLiveness{}, fmt.Errorf("%w: session and observation time are required", ErrInvalidLiveness)
	}
	return PeerLiveness{SessionID: sessionID, LastObserved: observedAt}, nil
}

func (liveness PeerLiveness) Observe(sessionID string, observedAt time.Time) (PeerLiveness, error) {
	if liveness.SessionID == "" || liveness.LastObserved.IsZero() {
		return PeerLiveness{}, fmt.Errorf("%w: liveness is not initialized", ErrInvalidLiveness)
	}
	if sessionID != liveness.SessionID {
		return PeerLiveness{}, fmt.Errorf("%w: heartbeat session does not match", ErrStaleHeartbeat)
	}
	if observedAt.IsZero() || observedAt.Before(liveness.LastObserved) {
		return PeerLiveness{}, fmt.Errorf("%w: heartbeat time moved backwards", ErrStaleHeartbeat)
	}
	next := liveness
	next.LastObserved = observedAt
	return next, nil
}

func (liveness PeerLiveness) FaultEventIfExpired(now time.Time, timeout time.Duration) (Event, bool, error) {
	if liveness.SessionID == "" || liveness.LastObserved.IsZero() || now.IsZero() || timeout <= 0 {
		return Event{}, false, fmt.Errorf("%w: initialized liveness, current time, and positive timeout are required", ErrInvalidLiveness)
	}
	if now.Before(liveness.LastObserved) {
		return Event{}, false, fmt.Errorf("%w: current time moved backwards", ErrInvalidLiveness)
	}
	if now.Sub(liveness.LastObserved) < timeout {
		return Event{}, false, nil
	}
	return Event{Kind: EventFaultDetected}, true, nil
}

package control

import (
	"errors"
	"testing"
	"time"
)

func TestPeerLivenessProducesFaultAtDeadline(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := NewPeerLiveness("session-1", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}

	event, expired, err := liveness.FaultEventIfExpired(started.Add(3*time.Second), 3*time.Second)
	if err != nil {
		t.Fatalf("FaultEventIfExpired returned error: %v", err)
	}
	if !expired || event.Kind != EventFaultDetected {
		t.Fatalf("unexpected expiry result: expired=%t event=%+v", expired, event)
	}
}

func TestPeerLivenessDoesNotInventTimeoutBudget(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := NewPeerLiveness("session-1", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}

	_, expired, err := liveness.FaultEventIfExpired(started.Add(2999*time.Millisecond), 3*time.Second)
	if err != nil {
		t.Fatalf("FaultEventIfExpired returned error: %v", err)
	}
	if expired {
		t.Fatal("liveness expired before the caller-provided deadline")
	}
}

func TestPeerLivenessRejectsStaleSessionAndTime(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := NewPeerLiveness("session-new", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}
	tests := []struct {
		name      string
		sessionID string
		observed  time.Time
	}{
		{name: "stale session", sessionID: "session-old", observed: started.Add(time.Second)},
		{name: "backwards time", sessionID: "session-new", observed: started.Add(-time.Nanosecond)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next, observeErr := liveness.Observe(test.sessionID, test.observed)
			if !errors.Is(observeErr, ErrStaleHeartbeat) {
				t.Fatalf("error = %v, want ErrStaleHeartbeat", observeErr)
			}
			if next != (PeerLiveness{}) {
				t.Fatalf("rejected heartbeat advanced liveness: %+v", next)
			}
		})
	}
}

func TestPeerLivenessRefreshesDeadline(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := NewPeerLiveness("session-1", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}
	refreshed, err := liveness.Observe("session-1", started.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}
	_, expired, err := refreshed.FaultEventIfExpired(started.Add(4*time.Second), 3*time.Second)
	if err != nil {
		t.Fatalf("FaultEventIfExpired returned error: %v", err)
	}
	if expired {
		t.Fatal("refreshed liveness used the old deadline")
	}
}

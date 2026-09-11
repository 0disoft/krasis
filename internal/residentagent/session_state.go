package residentagent

import "github.com/0disoft/krasis/internal/agent"

// Each observer belongs to exactly one runner generation, identified by its
// completion channel. Retained callbacks cannot update a later session.
type sessionObserver struct {
	operations *Operations
	done       chan struct{}
	retrying   bool
}

func (observer *sessionObserver) ObserveSupervisor(snapshot agent.SupervisorSnapshot) {
	operations := observer.operations
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.sessionDone != observer.done || operations.root.Err() != nil || operations.sessionState == "DISCONNECTING" {
		return
	}
	var state string
	switch snapshot.State {
	case agent.SupervisorAcquiring:
		state = "CONNECTING"
		if observer.retrying {
			state = "RECONNECTING"
		}
	case agent.SupervisorActive:
		state = "CONNECTED"
	case agent.SupervisorBackoff:
		observer.retrying = true
		state = "RECONNECTING"
	case agent.SupervisorStopped:
		// Completion is published by the runner return, after cleanup, not by
		// this earlier supervisor event.
		state = "DISCONNECTING"
	default:
		return
	}
	operations.sessionState = state
}

func (operations *Operations) stateLocked() string {
	if operations.sessionDone == nil {
		return "LOCAL"
	}
	if operations.root.Err() != nil {
		return "DISCONNECTING"
	}
	return operations.sessionState
}

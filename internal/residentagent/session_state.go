package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/productlog"
)

type sessionOutcome struct{ failed bool }

// A requested stop must not hide joined cleanup or deadline failures.
func peerDisconnectOnly(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !peerDisconnectOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return peerDisconnectOnly(wrapped.Unwrap())
	}
	return err == peer.ErrPeerDisconnectRequested
}

func cancellationOnly(err, cause error) bool {
	if err == nil {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !cancellationOnly(child, cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if child := wrapped.Unwrap(); child != nil {
			return cancellationOnly(child, cause)
		}
	}
	return err == cause || err == context.Canceled
}

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
		reason := productlog.ReconnectCode(snapshot.FailureKind, snapshot.FailureComponent, snapshot.FailurePhase, snapshot.FailureCode)
		operations.log.Event(reason)
		operations.counters[reason]++
		if snapshot.PeerTimeoutObserved {
			operations.log.Event("heartbeat_timeout_observed")
			operations.counters["heartbeat_timeout_observed"]++
		}
		observer.retrying = true
		state = "RECONNECTING"
	case agent.SupervisorStopped:
		if snapshot.FailureKind == "peer_disconnect" {
			operations.recordLocked("peer_disconnect_requested")
		}
		// Completion is published by the runner return, after cleanup, not by
		// this earlier supervisor event.
		state = "DISCONNECTING"
	default:
		return
	}
	if operations.sessionState != state {
		operations.log.Event(state)
	}
	operations.sessionState = state
}

func (operations *Operations) stateLocked() string {
	if operations.sessionFaulted {
		return "FAULTED"
	}
	if operations.sessionDone == nil {
		return "LOCAL"
	}
	if operations.root.Err() != nil {
		return "DISCONNECTING"
	}
	return operations.sessionState
}

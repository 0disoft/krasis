package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/productlog"
)

type sessionOutcome struct {
	failed     bool
	faultCodes map[string]bool
}

// Caller holds operations.mu; the ledger belongs to one runner generation.
func (operations *Operations) recordSessionFaultLocked(outcome *sessionOutcome, code string) {
	if outcome != nil {
		if outcome.faultCodes[code] {
			return
		}
		if outcome.faultCodes == nil {
			outcome.faultCodes = make(map[string]bool)
		}
		outcome.faultCodes[code] = true
	}
	operations.log.Event(code)
	operations.counters[code]++
}

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
	if operations.sessionDone == observer.done && snapshot.Recovery != nil && snapshot.Recovery.DeadlineExceeded {
		operations.sessionFaulted = true
		operations.recordSessionFaultLocked(operations.sessionOutcome, productlog.FaultCode("recovery", "deadline", "exceeded"))
		operations.recordLocked("session_failed")
		return
	}
	if operations.sessionDone != observer.done || operations.root.Err() != nil {
		return
	}
	if snapshot.State != "" {
		operations.log.Connection(snapshot.ConnectionID)
		jitter := 0
		if snapshot.BackoffPolicy.Jitter {
			jitter = 20
		}
		operations.log.Retry(productlog.RetrySummary{Policy: "agent-exponential-v1", Unlimited: true, Attempt: snapshot.Attempt, NextMillis: snapshot.RetryAfter.Milliseconds(), InitialMillis: snapshot.BackoffPolicy.Initial.Milliseconds(), MaximumMillis: snapshot.BackoffPolicy.Maximum.Milliseconds(), StableMillis: snapshot.BackoffPolicy.StableAfter.Milliseconds(), StableReset: snapshot.StableReset, JitterPercent: jitter})
	}
	if snapshot.State == agent.SupervisorStopped && snapshot.FailureKind == "session" {
		reason := productlog.FaultCode(snapshot.FailureComponent, snapshot.FailurePhase, snapshot.FailureCode)
		operations.recordSessionFaultLocked(operations.sessionOutcome, reason)
	}
	if operations.sessionState == "DISCONNECTING" {
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

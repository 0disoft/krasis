package control

import (
	"errors"
	"fmt"
)

type State string

const (
	StateLocal          State = "LOCAL"
	StatePending        State = "PENDING"
	StateOutboundActive State = "OUTBOUND_ACTIVE"
	StateInboundActive  State = "INBOUND_ACTIVE"
	StateRevoking       State = "REVOKING"
	StateFault          State = "FAULT"
)

type EventKind string

const (
	EventOutboundIntentDetected EventKind = "OUTBOUND_INTENT_DETECTED"
	EventOutboundLeaseGranted   EventKind = "OUTBOUND_LEASE_GRANTED"
	EventInboundLeaseGranted    EventKind = "INBOUND_LEASE_GRANTED"
	EventRevokeRequested        EventKind = "REVOKE_REQUESTED"
	EventFaultDetected          EventKind = "FAULT_DETECTED"
	EventReleaseAllSucceeded    EventKind = "RELEASE_ALL_SUCCEEDED"
	EventReleaseAllFailed       EventKind = "RELEASE_ALL_FAILED"
)

type Effect string

const (
	EffectRequestOutboundLease Effect = "REQUEST_OUTBOUND_LEASE"
	EffectBeginOutbound        Effect = "BEGIN_OUTBOUND"
	EffectBeginInbound         Effect = "BEGIN_INBOUND"
	EffectStopRemoteControl    Effect = "STOP_REMOTE_CONTROL"
	EffectRevokeLease          Effect = "REVOKE_LEASE"
	EffectReleaseAll           Effect = "RELEASE_ALL"
	EffectResumeLocal          Effect = "RESUME_LOCAL"
)

var (
	ErrInvalidTransition = errors.New("invalid control lease transition")
	ErrInvalidEvent      = errors.New("invalid control lease event")
)

type Snapshot struct {
	State     State
	SessionID string
	LeaseID   string
	Version   uint64
}

func InitialSnapshot() Snapshot {
	return Snapshot{State: StateLocal}
}

type Event struct {
	Kind      EventKind
	SessionID string
	LeaseID   string
}

type Transition struct {
	Previous Snapshot
	Next     Snapshot
	Event    Event
	Effects  []Effect
}

type rule struct {
	from    State
	event   EventKind
	to      State
	effects []Effect
	guard   func(Snapshot, Event) error
	update  func(*Snapshot, Event)
}

var transitionRules = []rule{
	{from: StateLocal, event: EventOutboundIntentDetected, to: StatePending, effects: []Effect{EffectRequestOutboundLease}, guard: requireSessionWithoutLease, update: setSession},
	{from: StatePending, event: EventOutboundLeaseGranted, to: StateOutboundActive, effects: []Effect{EffectBeginOutbound}, guard: requireMatchingSessionAndLease, update: setLease},
	{from: StateLocal, event: EventInboundLeaseGranted, to: StateInboundActive, effects: []Effect{EffectBeginInbound}, guard: requireSessionAndLease, update: setSessionAndLease},

	{from: StatePending, event: EventRevokeRequested, to: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
	{from: StateOutboundActive, event: EventRevokeRequested, to: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
	{from: StateInboundActive, event: EventRevokeRequested, to: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},

	{from: StatePending, event: EventFaultDetected, to: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
	{from: StateOutboundActive, event: EventFaultDetected, to: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
	{from: StateInboundActive, event: EventFaultDetected, to: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
	{from: StateRevoking, event: EventFaultDetected, to: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
	{from: StateFault, event: EventFaultDetected, to: StateFault, effects: []Effect{EffectReleaseAll}},

	{from: StateRevoking, event: EventReleaseAllSucceeded, to: StateLocal, effects: []Effect{EffectResumeLocal}, update: clearLease},
	{from: StateFault, event: EventReleaseAllSucceeded, to: StateLocal, effects: []Effect{EffectResumeLocal}, update: clearLease},
	{from: StateRevoking, event: EventReleaseAllFailed, to: StateFault, effects: []Effect{EffectReleaseAll}},
	{from: StateFault, event: EventReleaseAllFailed, to: StateFault, effects: []Effect{EffectReleaseAll}},
}

func Apply(current Snapshot, event Event) (Transition, error) {
	if err := validateSnapshot(current); err != nil {
		return Transition{}, err
	}
	for _, candidate := range transitionRules {
		if candidate.from != current.State || candidate.event != event.Kind {
			continue
		}
		if candidate.guard != nil {
			if err := candidate.guard(current, event); err != nil {
				return Transition{}, fmt.Errorf("%w: %v", ErrInvalidEvent, err)
			}
		}
		next := current
		next.State = candidate.to
		next.Version++
		if candidate.update != nil {
			candidate.update(&next, event)
		}
		return Transition{
			Previous: current,
			Next:     next,
			Event:    event,
			Effects:  append([]Effect(nil), candidate.effects...),
		}, nil
	}
	return Transition{}, fmt.Errorf("%w: state=%s event=%s", ErrInvalidTransition, current.State, event.Kind)
}

func validateSnapshot(snapshot Snapshot) error {
	switch snapshot.State {
	case StateLocal:
		if snapshot.SessionID != "" || snapshot.LeaseID != "" {
			return fmt.Errorf("local state must not retain a session or lease")
		}
	case StatePending:
		if snapshot.SessionID == "" || snapshot.LeaseID != "" {
			return fmt.Errorf("pending state requires only a session")
		}
	case StateOutboundActive, StateInboundActive:
		if snapshot.SessionID == "" || snapshot.LeaseID == "" {
			return fmt.Errorf("active state requires a session and lease")
		}
	case StateRevoking, StateFault:
		if snapshot.SessionID == "" {
			return fmt.Errorf("%s state requires the failed or revoking session", snapshot.State)
		}
	default:
		return fmt.Errorf("unknown control state %q", snapshot.State)
	}
	return nil
}

func requireSessionWithoutLease(_ Snapshot, event Event) error {
	if event.SessionID == "" || event.LeaseID != "" {
		return errors.New("outbound intent requires a session and no lease")
	}
	return nil
}

func requireSessionAndLease(_ Snapshot, event Event) error {
	if event.SessionID == "" || event.LeaseID == "" {
		return errors.New("lease grant requires a session and lease")
	}
	return nil
}

func requireMatchingSessionAndLease(current Snapshot, event Event) error {
	if event.SessionID == "" || event.SessionID != current.SessionID || event.LeaseID == "" {
		return errors.New("outbound lease grant must match the pending session and include a lease")
	}
	return nil
}

func setSession(snapshot *Snapshot, event Event) {
	snapshot.SessionID = event.SessionID
}

func setLease(snapshot *Snapshot, event Event) {
	snapshot.LeaseID = event.LeaseID
}

func setSessionAndLease(snapshot *Snapshot, event Event) {
	snapshot.SessionID = event.SessionID
	snapshot.LeaseID = event.LeaseID
}

func clearLease(snapshot *Snapshot, _ Event) {
	snapshot.SessionID = ""
	snapshot.LeaseID = ""
}

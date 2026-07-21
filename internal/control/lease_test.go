package control

import (
	"errors"
	"reflect"
	"testing"
)

func TestTransitionTable(t *testing.T) {
	tests := []struct {
		name    string
		current Snapshot
		event   Event
		state   State
		effects []Effect
	}{
		{name: "outbound intent", current: InitialSnapshot(), event: Event{Kind: EventOutboundIntentDetected, SessionID: "session-1"}, state: StatePending},
		{name: "outbound grant", current: Snapshot{State: StatePending, SessionID: "session-1", Version: 1}, event: Event{Kind: EventOutboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1"}, state: StateOutboundActive, effects: []Effect{EffectBeginOutbound}},
		{name: "outbound reject", current: Snapshot{State: StatePending, SessionID: "session-1", Version: 1}, event: Event{Kind: EventOutboundLeaseRejected, SessionID: "session-1"}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectReleaseAll}},
		{name: "pending outbound abort", current: Snapshot{State: StatePending, SessionID: "session-1", Version: 1}, event: Event{Kind: EventOutboundLeaseAborted, SessionID: "session-1", LeaseID: "lease-1"}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectReleaseAll}},
		{name: "active outbound abort", current: activeSnapshot(StateOutboundActive), event: Event{Kind: EventOutboundLeaseAborted, SessionID: "session-1", LeaseID: "lease-1"}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectReleaseAll}},
		{name: "inbound prepare", current: InitialSnapshot(), event: Event{Kind: EventInboundLeasePrepared, SessionID: "session-1", LeaseID: "lease-1"}, state: StateInboundPending},
		{name: "inbound commit", current: activeSnapshot(StateInboundPending), event: Event{Kind: EventInboundLeaseCommitted, SessionID: "session-1", LeaseID: "lease-1"}, state: StateInboundActive, effects: []Effect{EffectBeginInbound}},
		{name: "inbound abort", current: activeSnapshot(StateInboundPending), event: Event{Kind: EventInboundLeaseAborted, SessionID: "session-1", LeaseID: "lease-1"}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectReleaseAll}},
		{name: "pending revoke", current: Snapshot{State: StatePending, SessionID: "session-1"}, event: Event{Kind: EventRevokeRequested}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "inbound pending revoke", current: activeSnapshot(StateInboundPending), event: Event{Kind: EventRevokeRequested}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectReleaseAll}},
		{name: "outbound revoke", current: activeSnapshot(StateOutboundActive), event: Event{Kind: EventRevokeRequested}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "inbound revoke", current: activeSnapshot(StateInboundActive), event: Event{Kind: EventRevokeRequested}, state: StateRevoking, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "pending fault", current: Snapshot{State: StatePending, SessionID: "session-1"}, event: Event{Kind: EventFaultDetected}, state: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "inbound pending fault", current: activeSnapshot(StateInboundPending), event: Event{Kind: EventFaultDetected}, state: StateFault, effects: []Effect{EffectStopRemoteControl, EffectReleaseAll}},
		{name: "outbound fault", current: activeSnapshot(StateOutboundActive), event: Event{Kind: EventFaultDetected}, state: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "inbound fault", current: activeSnapshot(StateInboundActive), event: Event{Kind: EventFaultDetected}, state: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "revoking fault", current: activeSnapshot(StateRevoking), event: Event{Kind: EventFaultDetected}, state: StateFault, effects: []Effect{EffectStopRemoteControl, EffectRevokeLease, EffectReleaseAll}},
		{name: "fault retry", current: activeSnapshot(StateFault), event: Event{Kind: EventFaultDetected}, state: StateFault, effects: []Effect{EffectReleaseAll}},
		{name: "revoking release succeeds", current: activeSnapshot(StateRevoking), event: Event{Kind: EventReleaseAllSucceeded}, state: StateLocal, effects: []Effect{EffectResumeLocal}},
		{name: "fault release succeeds", current: activeSnapshot(StateFault), event: Event{Kind: EventReleaseAllSucceeded}, state: StateLocal, effects: []Effect{EffectResumeLocal}},
		{name: "revoking release fails", current: activeSnapshot(StateRevoking), event: Event{Kind: EventReleaseAllFailed}, state: StateFault, effects: []Effect{EffectReleaseAll}},
		{name: "fault release fails", current: activeSnapshot(StateFault), event: Event{Kind: EventReleaseAllFailed}, state: StateFault, effects: []Effect{EffectReleaseAll}},
	}

	if len(tests) != len(transitionRules) {
		t.Fatalf("test cases = %d, transition rules = %d", len(tests), len(transitionRules))
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition, err := Apply(test.current, test.event)
			if err != nil {
				t.Fatalf("Apply returned error: %v", err)
			}
			if transition.Next.State != test.state || transition.Next.Version != test.current.Version+1 {
				t.Fatalf("unexpected next snapshot: %+v", transition.Next)
			}
			if !reflect.DeepEqual(transition.Effects, test.effects) {
				t.Fatalf("effects = %v, want %v", transition.Effects, test.effects)
			}
			if test.state == StateLocal && (transition.Next.SessionID != "" || transition.Next.LeaseID != "") {
				t.Fatalf("local transition retained ownership identifiers: %+v", transition.Next)
			}
		})
	}
}

func TestOutboundGrantRejectsStaleSession(t *testing.T) {
	current := Snapshot{State: StatePending, SessionID: "session-new"}
	_, err := Apply(current, Event{Kind: EventOutboundLeaseGranted, SessionID: "session-old", LeaseID: "lease-old"})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error = %v, want ErrInvalidEvent", err)
	}
}

func TestActiveLeaseEventsRejectStaleLease(t *testing.T) {
	tests := []struct {
		name  string
		state State
		event EventKind
	}{
		{name: "inbound commit", state: StateInboundPending, event: EventInboundLeaseCommitted},
		{name: "inbound abort", state: StateInboundPending, event: EventInboundLeaseAborted},
		{name: "outbound abort", state: StateOutboundActive, event: EventOutboundLeaseAborted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Apply(activeSnapshot(test.state), Event{
				Kind: test.event, SessionID: "session-1", LeaseID: "lease-stale",
			})
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("error = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestInvalidTransitionProducesNoResult(t *testing.T) {
	transition, err := Apply(InitialSnapshot(), Event{Kind: EventReleaseAllSucceeded})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, want ErrInvalidTransition", err)
	}
	if transition.Previous != (Snapshot{}) || transition.Next != (Snapshot{}) || transition.Event != (Event{}) || len(transition.Effects) != 0 {
		t.Fatalf("invalid transition produced effects: %+v", transition)
	}
}

func TestInvalidSnapshotCannotTransition(t *testing.T) {
	current := Snapshot{State: StateLocal, SessionID: "stale-session"}
	_, err := Apply(current, Event{Kind: EventOutboundIntentDetected, SessionID: "session-new"})
	if err == nil {
		t.Fatal("Apply accepted local state with stale ownership")
	}
}

func TestNewSessionDoesNotReuseReleasedLease(t *testing.T) {
	released, err := Apply(activeSnapshot(StateFault), Event{Kind: EventReleaseAllSucceeded})
	if err != nil {
		t.Fatalf("release transition failed: %v", err)
	}
	pending, err := Apply(released.Next, Event{Kind: EventOutboundIntentDetected, SessionID: "session-2"})
	if err != nil {
		t.Fatalf("new session transition failed: %v", err)
	}
	if pending.Next.SessionID != "session-2" || pending.Next.LeaseID != "" {
		t.Fatalf("new session reused stale ownership: %+v", pending.Next)
	}
}

func activeSnapshot(state State) Snapshot {
	return Snapshot{State: state, SessionID: "session-1", LeaseID: "lease-1", Version: 4}
}

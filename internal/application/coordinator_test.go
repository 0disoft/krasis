package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

var errEffectFailed = errors.New("effect failed")

type recordingEffectExecutor struct {
	fail  map[control.Effect]error
	calls []control.Effect
}

func (executor *recordingEffectExecutor) Execute(_ context.Context, effect control.Effect) error {
	executor.calls = append(executor.calls, effect)
	return executor.fail[effect]
}

func TestHandleControlEventExecutesFaultCleanupInOrder(t *testing.T) {
	current := control.Snapshot{State: control.StateOutboundActive, SessionID: "session-1", LeaseID: "lease-1"}
	executor := &recordingEffectExecutor{}

	outcome, err := HandleControlEvent(context.Background(), current, control.Event{Kind: control.EventFaultDetected}, executor)
	if err != nil {
		t.Fatalf("HandleControlEvent returned error: %v", err)
	}
	want := []control.Effect{control.EffectStopRemoteControl, control.EffectRevokeLease, control.EffectReleaseAll, control.EffectResumeLocal}
	if outcome.Snapshot.State != control.StateLocal || len(outcome.Transitions) != 2 || !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("unexpected outcome: snapshot=%+v calls=%v", outcome.Snapshot, executor.calls)
	}
}

func TestHandleControlEventFallsBackToFaultWhenActivationFails(t *testing.T) {
	current := control.Snapshot{State: control.StatePending, SessionID: "session-1"}
	executor := &recordingEffectExecutor{fail: map[control.Effect]error{control.EffectBeginOutbound: errEffectFailed}}
	event := control.Event{Kind: control.EventOutboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1"}

	outcome, err := HandleControlEvent(context.Background(), current, event, executor)
	if !errors.Is(err, ErrControlEffectsFailed) || !errors.Is(err, errEffectFailed) {
		t.Fatalf("error = %v, want effect failure", err)
	}
	want := []control.Effect{
		control.EffectBeginOutbound,
		control.EffectStopRemoteControl,
		control.EffectRevokeLease,
		control.EffectReleaseAll,
		control.EffectResumeLocal,
	}
	if outcome.Snapshot.State != control.StateLocal || len(outcome.Transitions) != 3 || !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("unexpected fallback outcome: %+v calls=%v", outcome, executor.calls)
	}
}

func TestHandleControlEventContinuesFaultCleanupAfterMultipleFailures(t *testing.T) {
	current := control.Snapshot{State: control.StateInboundActive, SessionID: "session-1", LeaseID: "lease-1"}
	executor := &recordingEffectExecutor{fail: map[control.Effect]error{
		control.EffectStopRemoteControl: errEffectFailed,
		control.EffectRevokeLease:       errEffectFailed,
	}}

	outcome, err := HandleControlEvent(context.Background(), current, control.Event{Kind: control.EventFaultDetected}, executor)
	if !errors.Is(err, ErrControlEffectsFailed) {
		t.Fatalf("error = %v, want ErrControlEffectsFailed", err)
	}
	if outcome.Snapshot.State != control.StateFault || len(executor.calls) != 3 || executor.calls[2] != control.EffectReleaseAll {
		t.Fatalf("cleanup stopped early: outcome=%+v calls=%v", outcome, executor.calls)
	}
}

func TestHandleControlEventDoesNotCommitLocalWhenResumeFails(t *testing.T) {
	current := control.Snapshot{State: control.StateFault, SessionID: "session-1", LeaseID: "lease-1"}
	executor := &recordingEffectExecutor{fail: map[control.Effect]error{control.EffectResumeLocal: errEffectFailed}}

	outcome, err := HandleControlEvent(context.Background(), current, control.Event{Kind: control.EventReleaseAllSucceeded}, executor)
	if !errors.Is(err, ErrControlEffectsFailed) {
		t.Fatalf("error = %v, want ErrControlEffectsFailed", err)
	}
	want := []control.Effect{control.EffectResumeLocal, control.EffectReleaseAll}
	if outcome.Snapshot.State != control.StateFault || outcome.Snapshot.SessionID != "session-1" || !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("failed resume lost recovery state: outcome=%+v calls=%v", outcome, executor.calls)
	}
}

func TestHandleLivenessTurnsExpiryIntoFault(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := control.NewPeerLiveness("session-1", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}
	current := control.Snapshot{State: control.StateOutboundActive, SessionID: "session-1", LeaseID: "lease-1"}
	executor := &recordingEffectExecutor{}

	outcome, expired, err := HandleLiveness(context.Background(), current, liveness, started.Add(3*time.Second), 3*time.Second, executor)
	if err != nil {
		t.Fatalf("HandleLiveness returned error: %v", err)
	}
	if !expired || outcome.Snapshot.State != control.StateLocal || len(executor.calls) != 4 {
		t.Fatalf("unexpected liveness outcome: expired=%t outcome=%+v calls=%v", expired, outcome, executor.calls)
	}
}

func TestHandleLivenessLeavesLiveSessionUntouched(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := control.NewPeerLiveness("session-1", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}
	current := control.Snapshot{State: control.StateInboundActive, SessionID: "session-1", LeaseID: "lease-1", Version: 7}
	executor := &recordingEffectExecutor{}

	outcome, expired, err := HandleLiveness(context.Background(), current, liveness, started.Add(time.Second), 3*time.Second, executor)
	if err != nil || expired || outcome.Snapshot != current || len(executor.calls) != 0 {
		t.Fatalf("live session changed: expired=%t outcome=%+v calls=%v error=%v", expired, outcome, executor.calls, err)
	}
}

func TestHandleLivenessFailsSafeOnUncertainClock(t *testing.T) {
	started := time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	liveness, err := control.NewPeerLiveness("session-1", started)
	if err != nil {
		t.Fatalf("NewPeerLiveness returned error: %v", err)
	}
	current := control.Snapshot{State: control.StateOutboundActive, SessionID: "session-1", LeaseID: "lease-1"}
	executor := &recordingEffectExecutor{}

	outcome, unsafe, err := HandleLiveness(context.Background(), current, liveness, started.Add(-time.Second), 3*time.Second, executor)
	if err == nil || !unsafe || outcome.Snapshot.State != control.StateLocal || len(executor.calls) != 4 {
		t.Fatalf("uncertain clock did not fail safe: unsafe=%t outcome=%+v calls=%v error=%v", unsafe, outcome, executor.calls, err)
	}
}

func TestHandleInputEnvelopeAcceptsOnlyCurrentOrderedInput(t *testing.T) {
	current := control.Snapshot{State: control.StateInboundActive, SessionID: "session-1", LeaseID: "lease-1", Version: 3}
	stream, err := control.NewInputStream("session-1", "epoch-1")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	executor := &recordingEffectExecutor{}

	next, outcome, accepted, err := HandleInputEnvelope(
		context.Background(),
		current,
		stream,
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		executor,
	)
	if err != nil || !accepted || next.NextSequence != 2 || outcome.Snapshot != current || len(executor.calls) != 0 {
		t.Fatalf("valid input was not accepted: next=%+v outcome=%+v accepted=%t calls=%v error=%v", next, outcome, accepted, executor.calls, err)
	}
}

func TestHandleInputEnvelopeFaultsOnSequenceGap(t *testing.T) {
	current := control.Snapshot{State: control.StateInboundActive, SessionID: "session-1", LeaseID: "lease-1"}
	stream, err := control.NewInputStream("session-1", "epoch-1")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	executor := &recordingEffectExecutor{}

	next, outcome, accepted, err := HandleInputEnvelope(
		context.Background(),
		current,
		stream,
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 2},
		executor,
	)
	if !errors.Is(err, control.ErrInputSequenceGap) || accepted || next != (control.InputStream{}) || outcome.Snapshot.State != control.StateLocal || len(executor.calls) != 4 {
		t.Fatalf("sequence gap did not fail safe: next=%+v outcome=%+v accepted=%t calls=%v error=%v", next, outcome, accepted, executor.calls, err)
	}
}

func TestHandleInputEnvelopeFaultsNonLocalWrongDirection(t *testing.T) {
	current := control.Snapshot{State: control.StateOutboundActive, SessionID: "session-1", LeaseID: "lease-1"}
	stream, err := control.NewInputStream("session-1", "epoch-1")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	executor := &recordingEffectExecutor{}

	_, outcome, accepted, err := HandleInputEnvelope(
		context.Background(),
		current,
		stream,
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		executor,
	)
	if err == nil || accepted || outcome.Snapshot.State != control.StateLocal || len(executor.calls) != 4 {
		t.Fatalf("wrong-direction input did not fail safe: outcome=%+v accepted=%t calls=%v error=%v", outcome, accepted, executor.calls, err)
	}
}

package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type controlFlowRecorder struct {
	events []ControlFlowEvent
}

func (recorder *controlFlowRecorder) ObserveControlFlow(event ControlFlowEvent) {
	recorder.events = append(recorder.events, event)
}

func (recorder *controlFlowRecorder) count(want ControlFlowEvent) int {
	count := 0
	for _, event := range recorder.events {
		if event == want {
			count++
		}
	}
	return count
}

func TestBoundaryOrchestratorObservesAdmissionAndNotReadyDrop(t *testing.T) {
	recorder := &controlFlowRecorder{}
	orchestrator, err := NewBoundaryOrchestratorObserved(
		&recordingBoundaryBeginner{},
		func() (OutboundIdentity, error) {
			return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
		},
		recorder,
	)
	if err != nil {
		t.Fatal(err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatal(err)
	}
	if recorder.count(ControlFlowBoundaryDroppedNotReady) != 1 {
		t.Fatalf("events = %v", recorder.events)
	}

	recorder = &controlFlowRecorder{}
	orchestrator, err = NewBoundaryOrchestratorObserved(
		&recordingBoundaryBeginner{ready: true},
		func() (OutboundIdentity, error) {
			return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
		},
		recorder,
	)
	if err != nil {
		t.Fatal(err)
	}
	intents = make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatal(err)
	}
	if recorder.count(ControlFlowBoundaryAdmitted) != 1 {
		t.Fatalf("events = %v", recorder.events)
	}
}

func TestBoundaryOrchestratorClassifiesIdentityFailureAsAdmissionFailure(t *testing.T) {
	recorder := &controlFlowRecorder{}
	orchestrator, err := NewBoundaryOrchestratorObserved(
		&recordingBoundaryBeginner{ready: true},
		func() (OutboundIdentity, error) { return OutboundIdentity{}, errors.New("identity failed") },
		recorder,
	)
	if err != nil {
		t.Fatal(err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err == nil {
		t.Fatal("Run() accepted identity failure")
	}
	if recorder.count(ControlFlowBoundaryAdmissionFailed) != 1 {
		t.Fatalf("events = %v", recorder.events)
	}
}

func TestBoundaryOrchestratorBlocksLocalMenuBeforeIdentityAndRequest(t *testing.T) {
	recorder := &controlFlowRecorder{}
	beginner := &recordingBoundaryBeginner{ready: true}
	identityCalls := 0
	orchestrator, err := NewBoundaryOrchestratorObservedWithGate(
		beginner,
		func() (OutboundIdentity, error) {
			identityCalls++
			return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
		},
		recorder,
		boundaryAdmissionGateFunc(func(context.Context, control.BoundaryIntent) (BoundaryAdmissionDecision, error) {
			return BoundaryAdmissionBlockedLocalMenu, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatal(err)
	}
	if beginner.calls != 0 || identityCalls != 0 || recorder.count(ControlFlowBoundaryBlockedLocalMenu) != 1 {
		t.Fatalf("beginner=%+v identityCalls=%d events=%v", beginner, identityCalls, recorder.events)
	}
}

func TestBoundaryOrchestratorClassifiesLocalInteractionBeforeIdentityAndRequest(t *testing.T) {
	for _, test := range []struct {
		name     string
		decision BoundaryAdmissionDecision
		event    ControlFlowEvent
	}{
		{name: "mouse capture", decision: BoundaryAdmissionBlockedMouseCapture, event: ControlFlowBoundaryBlockedMouseCapture},
		{name: "pressed input", decision: BoundaryAdmissionBlockedPressedInput, event: ControlFlowBoundaryBlockedPressedInput},
		{name: "move size", decision: BoundaryAdmissionBlockedMoveSize, event: ControlFlowBoundaryBlockedMoveSize},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &controlFlowRecorder{}
			beginner := &recordingBoundaryBeginner{ready: true}
			identityCalls := 0
			orchestrator, err := NewBoundaryOrchestratorObservedWithGate(
				beginner,
				func() (OutboundIdentity, error) {
					identityCalls++
					return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
				},
				recorder,
				boundaryAdmissionGateFunc(func(context.Context, control.BoundaryIntent) (BoundaryAdmissionDecision, error) {
					return test.decision, nil
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			intents := make(chan control.BoundaryIntent, 1)
			intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
			close(intents)
			if err := orchestrator.Run(context.Background(), intents); err != nil {
				t.Fatal(err)
			}
			if beginner.calls != 0 || identityCalls != 0 || recorder.count(test.event) != 1 {
				t.Fatalf("beginner=%+v identityCalls=%d events=%v", beginner, identityCalls, recorder.events)
			}
		})
	}
}

func TestBoundaryOrchestratorFailsClosedWhenAdmissionInspectionFails(t *testing.T) {
	cause := errors.New("menu state unavailable")
	recorder := &controlFlowRecorder{}
	orchestrator, err := NewBoundaryOrchestratorObservedWithGate(
		&recordingBoundaryBeginner{ready: true},
		func() (OutboundIdentity, error) {
			return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
		},
		recorder,
		boundaryAdmissionGateFunc(func(context.Context, control.BoundaryIntent) (BoundaryAdmissionDecision, error) {
			return 0, cause
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v", err)
	}
	if recorder.count(ControlFlowBoundaryAdmissionFailed) != 1 {
		t.Fatalf("events = %v", recorder.events)
	}
}

type boundaryAdmissionGateFunc func(context.Context, control.BoundaryIntent) (BoundaryAdmissionDecision, error)

func (gate boundaryAdmissionGateFunc) AdmitBoundary(
	ctx context.Context,
	intent control.BoundaryIntent,
) (BoundaryAdmissionDecision, error) {
	return gate(ctx, intent)
}

func TestObservedControlFlowSeparatesRequestGrantAndEntryStages(t *testing.T) {
	recorder := &controlFlowRecorder{}
	sourceRuntime := newLocalRuntime(t)
	controller, err := NewOutboundControllerObserved(
		testWriteCoordinator(t, &recordingFrameWriter{}),
		sourceRuntime,
		&recordingSourceActivation{},
		recorder,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Begin(context.Background(), "session-1", "epoch-1", control.ScreenEdgeLeft); err != nil {
		t.Fatal(err)
	}
	grant := transport.ControlCommand{
		Kind: transport.ControlLeaseGrant, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1",
	}
	if err := controller.GrantPeer(context.Background(), grant, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []ControlFlowEvent{
		ControlFlowRequestWriteCompleted,
		ControlFlowGrantReceived,
		ControlFlowGrantApplySucceeded,
		ControlFlowCommitWriteCompleted,
	} {
		if recorder.count(want) != 1 {
			t.Fatalf("event %d count = %d; events=%v", want, recorder.count(want), recorder.events)
		}
	}

	recorder = &controlFlowRecorder{}
	destinationRuntime := newLocalRuntime(t)
	entry := &recordingCursorEntry{runtime: destinationRuntime}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, &recordingFrameWriter{}),
		Runtime:     destinationRuntime, Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry, Observer: recorder,
	}
	request := transport.ControlCommand{
		Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeLeft,
	}
	if err := responder.HandleLeaseRequest(context.Background(), request, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	commit := transport.ControlCommand{
		Kind: transport.ControlLeaseCommit, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1",
	}
	if err := responder.HandleLeaseCommit(context.Background(), commit, time.Unix(101, 0)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []ControlFlowEvent{
		ControlFlowRequestReceived,
		ControlFlowReturnArmSucceeded,
		ControlFlowEntryPrepareAttempted,
		ControlFlowEntryPrepared,
		ControlFlowGrantWriteCompleted,
		ControlFlowCommitReceived,
		ControlFlowEntryCommitted,
	} {
		if recorder.count(want) != 1 {
			t.Fatalf("event %d count = %d; events=%v", want, recorder.count(want), recorder.events)
		}
	}
}

type interruptedReadConnection struct {
	cause   error
	readHit int
}

func (connection *interruptedReadConnection) ReadFrame() (transport.Frame, error) {
	return transport.Frame{}, io.ErrClosedPipe
}

func (*interruptedReadConnection) Close() error                            { return nil }
func (connection *interruptedReadConnection) LocalInterruptObserved() bool { return true }
func (connection *interruptedReadConnection) LocalInterruptCause() error   { return connection.cause }
func (connection *interruptedReadConnection) ObserveReadAfterLocalInterrupt() {
	connection.readHit++
}

func TestClassifyPeerReadFailurePreservesLocalInterruptCause(t *testing.T) {
	cause := errors.New("source root")
	connection := &interruptedReadConnection{cause: cause}
	err := classifyPeerReadFailure(connection, io.ErrClosedPipe)
	if !errors.Is(err, ErrPeerReadAfterLocalInterrupt) || !errors.Is(err, cause) || connection.readHit != 1 {
		t.Fatalf("classified error=%v readHit=%d", err, connection.readHit)
	}
}

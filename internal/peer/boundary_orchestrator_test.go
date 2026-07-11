package peer

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestBoundaryOrchestratorStartsOutboundRequest(t *testing.T) {
	beginner := &recordingBoundaryBeginner{ready: true}
	wantIdentity := OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}
	orchestrator, err := NewBoundaryOrchestrator(beginner, func() (OutboundIdentity, error) {
		return wantIdentity, nil
	})
	if err != nil {
		t.Fatalf("NewBoundaryOrchestrator() error = %v", err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	wantIntent := control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919}
	intents <- wantIntent
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if beginner.calls != 1 || beginner.intent != wantIntent || beginner.identity != wantIdentity {
		t.Fatalf("beginner = %+v", beginner)
	}
}

func TestBoundaryOrchestratorDropsIntentWhileOutboundIsNotReady(t *testing.T) {
	beginner := &recordingBoundaryBeginner{}
	identityCalls := 0
	orchestrator, err := NewBoundaryOrchestrator(beginner, func() (OutboundIdentity, error) {
		identityCalls++
		return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
	})
	if err != nil {
		t.Fatalf("NewBoundaryOrchestrator() error = %v", err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 99}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if beginner.calls != 0 || identityCalls != 0 {
		t.Fatalf("begin calls = %d, identity calls = %d", beginner.calls, identityCalls)
	}
}

func TestBoundaryOrchestratorReturnsIdentityAndBeginFailures(t *testing.T) {
	cause := errors.New("entropy unavailable")
	orchestrator, err := NewBoundaryOrchestrator(&recordingBoundaryBeginner{ready: true}, func() (OutboundIdentity, error) {
		return OutboundIdentity{}, cause
	})
	if err != nil {
		t.Fatalf("NewBoundaryOrchestrator() error = %v", err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v", err)
	}

	beginCause := errors.New("write failed")
	orchestrator, _ = NewBoundaryOrchestrator(&recordingBoundaryBeginner{ready: true, err: beginCause}, func() (OutboundIdentity, error) {
		return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
	})
	intents = make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); !errors.Is(err, beginCause) {
		t.Fatalf("Run(begin failure) error = %v", err)
	}
}

func TestRandomOutboundIdentityUsesIndependent128BitValues(t *testing.T) {
	input := make([]byte, outboundIdentifierBytes*2)
	for index := range input {
		input[index] = byte(index)
	}
	identity, err := randomOutboundIdentity(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("randomOutboundIdentity() error = %v", err)
	}
	if identity.SessionID != "000102030405060708090a0b0c0d0e0f" {
		t.Fatalf("session ID = %q", identity.SessionID)
	}
	if identity.Epoch != "101112131415161718191a1b1c1d1e1f" {
		t.Fatalf("epoch = %q", identity.Epoch)
	}
}

type recordingBoundaryBeginner struct {
	ready    bool
	calls    int
	intent   control.BoundaryIntent
	identity OutboundIdentity
	err      error
}

func (beginner *recordingBoundaryBeginner) Ready() bool { return beginner.ready }

func (beginner *recordingBoundaryBeginner) BeginBoundary(
	_ context.Context,
	intent control.BoundaryIntent,
	identity OutboundIdentity,
) error {
	beginner.calls++
	beginner.intent = intent
	beginner.identity = identity
	return beginner.err
}

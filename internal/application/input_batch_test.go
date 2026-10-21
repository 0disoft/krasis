package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func moveEnvelopes(first uint64) []control.InputEnvelope {
	return []control.InputEnvelope{
		{SessionID: "session-1", Epoch: "epoch-1", Sequence: first},
		{SessionID: "session-1", Epoch: "epoch-1", Sequence: first + 1},
		{SessionID: "session-1", Epoch: "epoch-1", Sequence: first + 2},
	}
}

func TestPointerBatchConsumesEverySequenceBeforeOneInjection(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Now())
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 6, DeltaY: -3}
	if err := runtime.HandlePointerMoveBatch(context.Background(), moveEnvelopes(1), move, time.Now, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(port.moves) != 1 || port.moves[0] != [2]int32{6, -3} {
		t.Fatal("batch split or lost displacement", port.moves)
	}
	// A following frame must use the next original sequence, not batch count.
	if err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 4}, control.InputAction{Kind: control.InputKeyDown, Code: 30}); err != nil {
		t.Fatal(err)
	}
}

func TestPointerBatchRejectsInvalidMiddleEnvelopeBeforeInjection(t *testing.T) {
	for _, mode := range []string{"gap", "duplicate", "epoch", "session"} {
		t.Run(mode, func(t *testing.T) {
			port := &recordingInputPort{}
			runtime := newTestRuntime(t, port)
			grantInbound(t, runtime, time.Now())
			batch := moveEnvelopes(1)
			switch mode {
			case "gap":
				batch[1].Sequence = 3
			case "duplicate":
				batch[1].Sequence = 1
			case "epoch":
				batch[1].Epoch = "stale"
			case "session":
				batch[1].SessionID = "stale"
			}
			err := runtime.HandlePointerMoveBatch(context.Background(), batch, control.InputAction{Kind: control.InputPointerMove, DeltaX: 3}, time.Now, 2*time.Second)
			if err == nil || len(port.moves) != 0 || runtime.Snapshot().Control.State != control.StateLocal {
				t.Fatal("invalid batch injected or retained lease", err, port.moves)
			}
		})
	}
}

func TestPointerBatchFailureRecoversHeldInput(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Now())
	if err := runtime.HandleInputAction(context.Background(), moveEnvelopes(1)[0], control.InputAction{Kind: control.InputKeyDown, Code: 30}); err != nil {
		t.Fatal(err)
	}
	port.moveErr = errors.New("native movement failed")
	err := runtime.HandlePointerMoveBatch(context.Background(), moveEnvelopes(2), control.InputAction{Kind: control.InputPointerMove, DeltaX: 3}, time.Now, 2*time.Second)
	if err == nil || runtime.Snapshot().Control.State != control.StateLocal || runtime.pressed.Count() != 0 || len(port.releaseCalls) != 1 {
		t.Fatal("failed batch lost held-input recovery", err)
	}
}

package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type slowBatchInput struct {
	inputPort
	order   []string
	pressAt int32
}

func TestPointerBatchPreservesControlBarrierReadTime(t *testing.T) {
	r := &inputReadAhead{ctx: context.Background(), frames: make(chan queuedSessionFrame, 1)}
	at := time.Now()
	r.frames <- queuedSessionFrame{frame: transport.Frame{Kind: transport.MessageControl}, controlReadAt: at}
	r.pointerBatch(batchInputFrame(t, 1, control.InputPointerMove, 1))
	p := r.readFrame()
	if p.frame.Kind != transport.MessageControl || !p.controlReadAt.Equal(at) || r.hasPending || !r.pending.controlReadAt.IsZero() {
		t.Fatal("batch barrier lost its read boundary or retained storage", p)
	}
}

func (p *slowBatchInput) Move(ctx context.Context, x, y int32) error {
	time.Sleep(20 * time.Millisecond)
	p.order = append(p.order, "move")
	return p.inputPort.Move(ctx, x, y)
}
func (p *slowBatchInput) Press(ctx context.Context, token control.PressedToken) error {
	for _, move := range p.moves {
		p.pressAt += move[0]
	}
	p.order = append(p.order, "press")
	return p.inputPort.Press(ctx, token)
}

func batchInputFrame(t *testing.T, sequence uint64, kind control.InputActionKind, x int32) transport.Frame {
	t.Helper()
	a := control.InputAction{Kind: kind, DeltaX: x}
	if kind == control.InputKeyDown {
		a.Code = 30
	}
	p, err := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: sequence}, a)
	if err != nil {
		t.Fatal(err)
	}
	return transport.Frame{Kind: transport.MessageInput, Payload: p}
}

func TestReceiverBacklogCombinesMovesWithoutCrossingDiscreteInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		port := &slowBatchInput{}
		runtime, err := application.NewSessionRuntime(effectExecutor{}, port)
		if err != nil {
			t.Fatal(err)
		}
		_, err = runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1"}, "epoch-1", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		connection := &scriptedConnection{}
		for seq := uint64(1); seq <= 129; seq++ {
			kind, x := control.InputPointerMove, int32(1)
			if seq == 65 {
				kind, x = control.InputKeyDown, 0
			}
			connection.frames = append(connection.frames, batchInputFrame(t, seq, kind, x))
		}
		start := time.Now()
		err = runSession(context.Background(), connection, nil, runtime, nil, nil, time.Now, 2*time.Second, nil)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal(err)
		}
		var distance int32
		for _, m := range port.moves {
			distance += m[0]
		}
		if distance != 128 || len(port.moves) > 5 || time.Since(start) > 100*time.Millisecond {
			t.Fatal("backlog still injects every wire move", distance, len(port.moves), time.Since(start))
		}
		if port.pressAt != 64 || len(port.presses) != 1 || len(port.releases) != 1 || runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatal("batch lost discrete input or EOF recovery")
		}
		press := -1
		for i, op := range port.order {
			if op == "press" {
				press = i
			}
		}
		if press < 1 || press == len(port.order)-1 {
			t.Fatal("key barrier crossed", port.order)
		}
	})
}

func TestPointerBatchKeepsMalformedAndSequenceBarrier(t *testing.T) {
	for _, mode := range []string{"gap", "malformed", "heartbeat", "overflow", "zero"} {
		t.Run(mode, func(t *testing.T) {
			first := batchInputFrame(t, 1, control.InputPointerMove, 1)
			barrier := batchInputFrame(t, 2, control.InputPointerMove, 1)
			switch mode {
			case "gap":
				barrier = batchInputFrame(t, 3, control.InputPointerMove, 1)
			case "malformed":
				barrier.Payload = []byte{1}
			case "heartbeat":
				barrier = transport.Frame{Kind: transport.MessageHeartbeat}
			case "overflow":
				barrier = batchInputFrame(t, 2, control.InputPointerMove, control.MaxInputDelta)
			case "zero":
				barrier = batchInputFrame(t, 2, control.InputPointerMove, -1)
			}
			r := &inputReadAhead{ctx: context.Background(), frames: make(chan queuedSessionFrame, 2)}
			r.frames <- queuedSessionFrame{frame: barrier}
			r.frames <- queuedSessionFrame{frame: batchInputFrame(t, 3, control.InputPointerMove, 1)}
			envelopes, move := r.pointerBatch(first)
			if len(envelopes) != 1 || move.DeltaX != 1 {
				t.Fatal("batch crossed barrier")
			}
			got, err := r.ReadFrame()
			if err != nil || got.Kind != barrier.Kind || string(got.Payload) != string(barrier.Payload) {
				t.Fatal("barrier consumed or reordered")
			}
		})
	}
}

func TestPointerBatchReusePreservesLimitBarriersAndPendingError(t *testing.T) {
	r := &inputReadAhead{ctx: context.Background(), frames: make(chan queuedSessionFrame, application.MaxPointerMoveBatch)}
	for seq := uint64(2); seq <= application.MaxPointerMoveBatch; seq++ {
		r.frames <- queuedSessionFrame{frame: batchInputFrame(t, seq, control.InputPointerMove, 1)}
	}
	key := batchInputFrame(t, application.MaxPointerMoveBatch+1, control.InputKeyDown, 0)
	r.frames <- queuedSessionFrame{frame: key}
	envelopes, move := r.pointerBatch(batchInputFrame(t, 1, control.InputPointerMove, 1))
	if len(envelopes) != application.MaxPointerMoveBatch || move.DeltaX != application.MaxPointerMoveBatch || envelopes[len(envelopes)-1].Sequence != application.MaxPointerMoveBatch {
		t.Fatal("full batch exceeded its limit or lost an envelope")
	}
	got, err := r.ReadFrame()
	if err != nil || string(got.Payload) != string(key.Payload) {
		t.Fatal("full batch lost or reordered the key barrier", err)
	}
	heartbeat := transport.Frame{Kind: transport.MessageHeartbeat, Payload: []byte("heartbeat")}
	r.frames <- queuedSessionFrame{frame: batchInputFrame(t, 67, control.InputPointerMove, 1)}
	r.frames <- queuedSessionFrame{frame: heartbeat}
	envelopes, move = r.pointerBatch(batchInputFrame(t, 66, control.InputPointerMove, 1))
	if len(envelopes) != 2 || move.DeltaX != 2 || envelopes[0].Sequence != 66 || envelopes[1].Sequence != 67 {
		t.Fatal("reused batch retained stale envelopes")
	}
	got, err = r.ReadFrame()
	if err != nil || got.Kind != heartbeat.Kind || string(got.Payload) != string(heartbeat.Payload) || r.hasPending || r.pending.frame.Payload != nil {
		t.Fatal("pending heartbeat changed or its storage was retained", err)
	}
	r.frames <- queuedSessionFrame{err: io.ErrUnexpectedEOF}
	envelopes, move = r.pointerBatch(batchInputFrame(t, 68, control.InputPointerMove, 1))
	if len(envelopes) != 1 || move.DeltaX != 1 || envelopes[0].Sequence != 68 {
		t.Fatal("reused batch crossed a read failure")
	}
	if _, err = r.ReadFrame(); !errors.Is(err, io.ErrUnexpectedEOF) || r.hasPending || r.pending.err != nil {
		t.Fatal("pending read failure changed or was retained", err)
	}
}

func TestPointerBatchIdentifierReusePreservesIdentityBarrier(t *testing.T) {
	for _, field := range []string{"session", "epoch"} {
		t.Run(field, func(t *testing.T) {
			r := &inputReadAhead{ctx: context.Background(), frames: make(chan queuedSessionFrame, 2)}
			identity := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 2}
			if field == "session" {
				identity.SessionID = "session-2"
			} else {
				identity.Epoch = "epoch-2"
			}
			action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
			payload, err := transport.EncodeInputPayload(identity, action)
			if err != nil {
				t.Fatal(err)
			}
			barrier := transport.Frame{Kind: transport.MessageInput, Payload: payload}
			r.frames <- queuedSessionFrame{frame: barrier}
			identity.Sequence++
			payload, err = transport.EncodeInputPayload(identity, action)
			if err != nil {
				t.Fatal(err)
			}
			r.frames <- queuedSessionFrame{frame: transport.Frame{Kind: transport.MessageInput, Payload: payload}}
			envelopes, move := r.pointerBatch(batchInputFrame(t, 1, control.InputPointerMove, 1))
			if len(envelopes) != 1 || move.DeltaX != 1 {
				t.Fatal("batch crossed identity barrier")
			}
			got, err := r.ReadFrame()
			if err != nil || got.Kind != barrier.Kind || string(got.Payload) != string(barrier.Payload) {
				t.Fatal("identity barrier consumed or changed", err)
			}
			envelopes, move = r.pointerBatch(got)
			if len(envelopes) != 2 || move.DeltaX != 2 || envelopes[0].Sequence != 2 || envelopes[1] != identity {
				t.Fatal("new identity lost frames or reused stale identifiers", envelopes, move)
			}
			if envelopes[0].SessionID != identity.SessionID || envelopes[0].Epoch != identity.Epoch || r.hasPending {
				t.Fatal("new batch retained old identity or pending frame")
			}
		})
	}
}

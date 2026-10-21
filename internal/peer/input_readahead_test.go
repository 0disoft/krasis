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

package peer

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestInputQueueAbsorbsTwoThousandOscillatingMovesWithoutBlockingHeartbeat(t *testing.T) {
	q := newInputFrameQueue()
	heartbeats := 0
	for seq := uint64(1); seq <= 2000; seq++ {
		dx := int32(1)
		if seq%2 == 0 {
			dx = -1
		}
		if !q.push(q.prepare(queuedSessionFrame{frame: batchInputFrame(t, seq, control.InputPointerMove, dx)})) {
			t.Fatal("mouse backlog exhausted the reader queue", seq)
		}
		if seq%250 == 0 {
			heartbeats++
			if !q.push(q.prepare(queuedSessionFrame{frame: transport.Frame{Kind: transport.MessageHeartbeat}, heartbeatObserved: true})) {
				t.Fatal("heartbeat reader blocked")
			}
		}
	}
	if q.size > 40 {
		t.Fatal("move backlog was not compacted", q.size)
	}
	done := make(chan struct{})
	close(done)
	r := &inputReadAhead{ctx: context.Background(), queue: q, done: done}
	runtime, port := newInboundRuntime(t)
	var distance int32
	for {
		p := r.readFrame()
		if p.err == io.EOF {
			break
		}
		if p.err != nil {
			t.Fatal(p.err)
		}
		if p.frame.Kind == transport.MessageHeartbeat {
			heartbeats--
			continue
		}
		envelopes, move := r.pointerBatch(p.frame)
		if err := runtime.HandlePointerMoveBatch(context.Background(), envelopes, move, fixedClock(time.Unix(101, 0)), 3*time.Second); err != nil {
			t.Fatal("batch changed authorization or ordering", err)
		}
		distance += move.DeltaX
	}
	if heartbeats != 0 || distance != 0 || runtime.Snapshot().InputStream.NextSequence != 2001 || len(port.moves) > 64 {
		t.Fatal("coalescing lost frames, sequence, net distance or barriers", heartbeats, distance, runtime.Snapshot(), len(port.moves))
	}
}

func TestInputQueuePreservesIdentitySequenceDeltaAndControlBarriers(t *testing.T) {
	for _, mode := range []string{"key", "heartbeat", "control", "gap", "identity", "overflow", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			q := newInputFrameQueue()
			first := batchInputFrame(t, 1, control.InputPointerMove, 1)
			second := batchInputFrame(t, 2, control.InputPointerMove, 1)
			switch mode {
			case "key":
				second = batchInputFrame(t, 2, control.InputKeyDown, 0)
			case "heartbeat":
				second = transport.Frame{Kind: transport.MessageHeartbeat}
			case "control":
				second = transport.Frame{Kind: transport.MessageControl}
			case "gap":
				second = batchInputFrame(t, 3, control.InputPointerMove, 1)
			case "identity":
				payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "other", Epoch: "epoch-1", Sequence: 2}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
				second.Payload = payload
			case "overflow":
				second = batchInputFrame(t, 2, control.InputPointerMove, control.MaxInputDelta)
			case "malformed":
				second.Payload = []byte{1}
			}
			q.push(q.prepare(queuedSessionFrame{frame: first}))
			q.push(q.prepare(queuedSessionFrame{frame: second}))
			if q.size != 2 {
				t.Fatal("queue merged across barrier", q.size)
			}
			q.pop()
			got, ok := q.pop()
			if !ok || got.frame.Kind != second.Kind || string(got.frame.Payload) != string(second.Payload) {
				t.Fatal("barrier changed")
			}
		})
	}
}

func BenchmarkInputQueueCoalescing(b *testing.B) {
	q := newInputFrameQueue()
	frames := make([]transport.Frame, application.MaxPointerMoveBatch)
	for i := range frames {
		payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: uint64(i + 1)}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
		frames[i] = transport.Frame{Kind: transport.MessageInput, Payload: payload}
	}
	q.decodeHint = control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1"}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, frame := range frames {
			if !q.push(q.prepare(queuedSessionFrame{frame: frame})) {
				b.Fatal("queue full")
			}
		}
		item, ok := q.pop()
		if !ok || item.count != len(frames) {
			b.Fatal("batch size")
		}
	}
}

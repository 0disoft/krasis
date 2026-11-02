package inputworker

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestResponseBufferReuseMatchesFreshFramesAndClearsPrivateFields(t *testing.T) {
	inner := &handoff.NativeIPCProfile{Micros: [6]uint32{1, 2, 3, 4, 5, 6}, ParentMeasured: true, ParentMicros: [3]uint32{7, 8, 9}}
	cases := []response{
		{sequence: 1, op: opCursorEntryProbe, beforeX: -9, x: 12, targetY: 33, desktop: inputdesktop.DesktopDefault,
			applyDetail: &handoff.WorkerApplyProfile{Measured: 1, Micros: [2]uint32{1000}, SubmissionCalls: 1, SubmissionMicros: 100, NativeIPC: inner}},
		{sequence: 2, op: opHeartbeat},
		{sequence: 3, op: opMove, status: 1, certainty: 1, failureStage: failureStageBeforeDispatch},
		{sequence: 4, op: opCursorRead, x: 1, y: -2},
		{sequence: 5, op: opMove, timing: [3]byte{1, 2, 3}, millis: [3]uint16{0, 1, 10}, measured: 7},
		{sequence: 6, op: opMove},
	}
	var reusable responseBuffer
	var sequence bytes.Buffer
	for _, ack := range cases {
		var fresh, reused bytes.Buffer
		if err := writeResponse(&fresh, ack); err != nil {
			t.Fatal(err)
		}
		if err := reusable.write(&reused, ack); err != nil || !bytes.Equal(fresh.Bytes(), reused.Bytes()) {
			t.Fatal("reused ACK retained stale bytes or changed frame", ack.sequence, err)
		}
		sequence.Write(reused.Bytes())
	}
	r := newACKReader(&sequence)
	for _, ack := range cases {
		got, err := readResponse(r)
		if err != nil || got.sequence != ack.sequence || got.op != ack.op || got.status != ack.status {
			t.Fatal("reused frames lost ordering/result", got, err)
		}
	}
}

func TestResponseBufferKeepsShortAndFailedWritesSingleAttempt(t *testing.T) {
	var data responseBuffer
	short := &shortIPCWriter{}
	if err := data.write(short, response{op: opMove, sequence: 1}); !errors.Is(err, io.ErrShortWrite) || short.calls != 1 {
		t.Fatal("short write retried or hidden", err, short.calls)
	}
	w := &responseErrorWriter{err: io.ErrClosedPipe}
	if err := data.write(w, response{op: opMove, sequence: 1}); !errors.Is(err, io.ErrClosedPipe) || w.calls != 1 {
		t.Fatal("write cause changed", err, w.calls)
	}
}

type responseErrorWriter struct {
	err   error
	calls int
}

func (w *responseErrorWriter) Write([]byte) (int, error) { w.calls++; return 0, w.err }

func BenchmarkServiceACKSerialization(b *testing.B) {
	for _, detailed := range []bool{false, true} {
		name := "plain"
		ack := response{op: opMove, sequence: 1}
		if detailed {
			name = "profiled"
			ack.applyDetail = &handoff.WorkerApplyProfile{Measured: 1, Micros: [2]uint32{1000}, SubmissionCalls: 1, SubmissionMicros: 100,
				NativeIPC: &handoff.NativeIPCProfile{Micros: [6]uint32{1, 2, 3, 4, 5, 6}}}
		}
		b.Run(name+"/fresh", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := writeResponse(io.Discard, ack); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(name+"/owned", func(b *testing.B) {
			var data responseBuffer
			b.ReportAllocs()
			for b.Loop() {
				if err := data.write(io.Discard, ack); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

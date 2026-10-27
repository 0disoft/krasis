package inputworker

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

type oneWakeACKReader struct {
	data  []byte
	reads int
}

func (r *oneWakeACKReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads > 1 {
		return 0, os.ErrDeadlineExceeded
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestBufferedACKAvoidsSecondKernelReadAfterDelayedWake(t *testing.T) {
	var wire bytes.Buffer
	ack := response{op: opMove, sequence: 1, timing: [3]byte{1, 2, 2}, measured: 7, millis: [3]uint16{0, 1, 2}, applyDetail: &handoff.WorkerApplyProfile{Micros: [2]uint32{2000, 0}, Measured: 1}}
	if err := writeResponse(&wire, ack); err != nil {
		t.Fatal(err)
	}
	for _, buffered := range []bool{false, true} {
		r := &oneWakeACKReader{data: append([]byte(nil), wire.Bytes()...)}
		var reader io.Reader = r
		if buffered {
			reader = newACKReader(reader)
		}
		got, err := readResponse(reader)
		if !buffered {
			if err == nil || r.reads != 2 {
				t.Fatalf("old path did not reproduce suffix loss: reads=%d err=%v", r.reads, err)
			}
		} else if err != nil || got.sequence != 1 || got.applyDetail == nil || r.reads != 1 {
			t.Fatalf("buffered ACK: reads=%d ack=%+v err=%v", r.reads, got, err)
		}
	}
}

func TestBufferedACKRetainsPrefetchedNextResponseAndPartialEOF(t *testing.T) {
	var wire bytes.Buffer
	for seq := uint64(1); seq <= 2; seq++ {
		if err := writeResponse(&wire, response{op: opMove, sequence: seq}); err != nil {
			t.Fatal(err)
		}
	}
	reader := newACKReader(&wire)
	for seq := uint64(1); seq <= 2; seq++ {
		ack, err := readResponse(reader)
		if err != nil || ack.sequence != seq {
			t.Fatalf("sequence=%d ack=%+v err=%v", seq, ack, err)
		}
	}
	if _, err := readResponse(newACKReader(bytes.NewReader([]byte("KRA1")))); err == nil {
		t.Fatal("partial header accepted")
	}
}

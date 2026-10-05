package inputworker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestWorkerApplyRequestFlagDoesNotAlterLegacyOrAuthorizeMaintenance(t *testing.T) {
	for _, probe := range []bool{false, true} {
		r := request{op: opMoveRead, sequence: 1, x: 1, applyDetail: probe}
		data, err := encodeRequest(r)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readRequest(iotest.OneByteReader(bytes.NewReader(data[:])))
		if err != nil || got != r {
			t.Fatal("request flag changed data", err)
		}
		if probe {
			data[7] = 2
		} else {
			data[7] = 1
		}
		if _, err = readRequest(bytes.NewReader(data[:])); !errors.Is(err, ErrProtocol) {
			t.Fatal("unknown flag accepted")
		}
	}
	for _, op := range []operation{opHeartbeat, opClose} {
		if _, err := encodeRequest(request{op: op, applyDetail: true}); !errors.Is(err, ErrProtocol) {
			t.Fatal("maintenance requested input detail")
		}
	}
}

func TestWorkerApplyACKKeepsOneWriteAndLegacyCursorFraming(t *testing.T) {
	detail := handoff.WorkerApplyProfile{Micros: [2]uint32{31001, 211}, Measured: 3}
	for _, op := range []operation{opMove, opMoveRead, opCursorRead} {
		w := &cursorACKWriter{}
		value := response{op: op, sequence: 1, x: -4, y: 5, applyDetail: &detail, timing: [3]byte{2, 1, 3}, millis: [3]uint16{8, 0, 31}, measured: 7}
		if err := writeResponse(w, value); err != nil || w.writes != 1 {
			t.Fatal("detail added a write", err)
		}
		plain := response{op: opMove, sequence: 2}
		if err := writeResponse(w, plain); err != nil {
			t.Fatal(err)
		}
		reader := iotest.OneByteReader(w)
		got, err := readResponse(reader)
		if err != nil || got.applyDetail == nil || *got.applyDetail != detail || got.millis != value.millis {
			t.Fatal("detail or outer timings lost", err)
		}
		if op != opMove && (got.x != -4 || got.y != 5) {
			t.Fatal("private cursor suffix shifted")
		}
		got, err = readResponse(reader)
		if err != nil || got.sequence != 2 || got.applyDetail != nil {
			t.Fatal("legacy ACK misaligned", err)
		}
	}
}

func TestWorkerApplyNumericMetadataDropsWithoutLosingCursorOrNextACK(t *testing.T) {
	var w bytes.Buffer
	detail := handoff.WorkerApplyProfile{Measured: 3}
	if err := writeResponse(&w, response{op: opCursorRead, sequence: 1, x: 3, y: 4, applyDetail: &detail}); err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), w.Bytes()...)
	binary.BigEndian.PutUint32(data[responseSize:responseSize+4], uint32(handoff.Window.Microseconds())+1)
	w.Reset()
	w.Write(data)
	_ = writeResponse(&w, response{op: opMove, sequence: 2})
	got, err := readResponse(&w)
	if err != nil || got.applyDetail != nil || got.x != 3 || got.y != 4 || got.status != 0 {
		t.Fatal("invalid diagnostic changed acknowledged input", err)
	}
	if got, err = readResponse(&w); err != nil || got.sequence != 2 {
		t.Fatal("metadata drop lost alignment", err)
	}
	for _, mutation := range []string{"truncated", "reserved"} {
		bad := append([]byte(nil), data...)
		if mutation == "truncated" {
			bad = bad[:responseSize+workerApplyDetailSize-1]
		} else {
			bad[responseSize+10] = 1
		}
		if _, err := readResponse(bytes.NewReader(bad)); !errors.Is(err, ErrProtocol) {
			t.Fatal("bad detail framing accepted", mutation)
		}
	}
	w.Reset()
	short := &cursorACKWriter{short: true}
	if !errors.Is(writeResponse(short, response{op: opMove, sequence: 1, applyDetail: &detail}), io.ErrShortWrite) || short.writes != 1 {
		t.Fatal("short detail write replayed")
	}
}

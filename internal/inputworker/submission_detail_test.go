package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

type submissionInput struct {
	timedApplyInput
	probe *handoff.WorkerApplyProfile
}

func (input *submissionInput) Move(ctx context.Context, x, y int32) error {
	input.probe = handoff.WorkerApplyProbe(ctx)
	time.Sleep(time.Millisecond)
	input.probe.RecordSubmission(30 * time.Millisecond)
	return input.timedApplyInput.Move(ctx, x, y)
}

func (input *submissionInput) CursorPosition(ctx context.Context) (int32, int32, error) {
	if handoff.WorkerApplyProbe(ctx) != nil {
		return 0, 0, errors.New("submission probe escaped into cursor snapshot")
	}
	return input.timedApplyInput.CursorPosition(ctx)
}

func TestSubmissionProbeExplicitRequestAndScope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			input := &submissionInput{}
			command := request{op: opMoveRead, sequence: 1, x: 1, applyDetail: true, submissionDetail: enabled}
			data, err := encodeRequest(command)
			got, decodeErr := readRequest(iotest.OneByteReader(bytes.NewReader(data[:])))
			if err != nil || decodeErr != nil || got != command {
				t.Fatal("explicit submission request lost", err, decodeErr)
			}
			var ack response
			var pressed control.PressedState
			if err := applyCommand(context.Background(), input, &pressed, command, &ack); err != nil {
				t.Fatal(err)
			}
			if (input.probe != nil) != enabled || !ack.applyDetail.Valid() || ack.applyDetail.Micros != [2]uint32{32000, 2000} || (ack.applyDetail.SubmissionCalls == 1) != enabled {
				t.Fatal("submission enabled for legacy request or lost containing span")
			}
		})
	}
	if _, err := encodeRequest(request{op: opMove, submissionDetail: true}); !errors.Is(err, ErrProtocol) {
		t.Fatal("submission requested outside apply detail")
	}
}

func TestSubmissionACKOneWriteWithLegacyAlignmentAndOutcome(t *testing.T) {
	detail := handoff.WorkerApplyProfile{Micros: [2]uint32{31000, 2000}, Measured: 3, SubmissionMicros: 30000, SubmissionCalls: 1}
	for _, op := range []operation{opMove, opMoveRead, opPress} {
		for _, failed := range []bool{false, true} {
			w := &cursorACKWriter{}
			ack := response{op: op, sequence: 1, x: 2, y: -3, applyDetail: &detail, timing: [3]byte{1, 1, 3}, millis: [3]uint16{0, 0, 33}, measured: 7}
			if failed {
				ack.status, ack.certainty, ack.requested, ack.inserted = 1, 2, 2, 1
			}
			if err := writeResponse(w, ack); err != nil || w.writes != 1 || w.Bytes()[4] != 5 {
				t.Fatal("submission ACK added write or lost extension", err)
			}
			_ = writeResponse(w, response{op: opMove, sequence: 2})
			reader := iotest.OneByteReader(w)
			got, err := readResponse(reader)
			if err != nil || got.applyDetail == nil || *got.applyDetail != detail || got.status != ack.status || got.certainty != ack.certainty || got.inserted != ack.inserted {
				t.Fatal("submission changed receipt", err)
			}
			if op == opMoveRead && !failed && (got.x != 2 || got.y != -3) {
				t.Fatal("cursor suffix shifted")
			}
			if got, err = readResponse(reader); err != nil || got.sequence != 2 || got.applyDetail != nil {
				t.Fatal("following legacy ACK shifted", err)
			}
		}
	}
}

func TestInvalidSubmissionMetadataDropsButFramingFails(t *testing.T) {
	detail := handoff.WorkerApplyProfile{Micros: [2]uint32{1000, 0}, Measured: 3, SubmissionMicros: 500, SubmissionCalls: 1}
	var original bytes.Buffer
	_ = writeResponse(&original, response{op: opMoveRead, sequence: 1, x: -2, y: 3, applyDetail: &detail})
	for _, mode := range []string{"parent", "presence", "multiple", "saturation", "reserved", "truncated"} {
		data := append([]byte(nil), original.Bytes()...)
		offset := responseSize + workerApplyDetailSize
		switch mode {
		case "parent":
			binary.BigEndian.PutUint32(data[offset:], 1001)
		case "presence":
			data[offset+4] = 0
		case "multiple":
			data[offset+4] = 2
		case "saturation":
			data[offset+5] = 1
		case "reserved":
			data[offset+6] = 1
		case "truncated":
			data = data[:offset+workerSubmissionDetailSize-1]
		}
		var w bytes.Buffer
		w.Write(data)
		if mode == "truncated" || mode == "reserved" {
			if _, err := readResponse(&w); !errors.Is(err, ErrProtocol) {
				t.Fatal("malformed native suffix accepted", mode)
			}
			continue
		}
		_ = writeResponse(&w, response{op: opMove, sequence: 2})
		got, err := readResponse(&w)
		if err != nil || got.applyDetail != nil || got.x != -2 || got.y != 3 || got.status != 0 {
			t.Fatal("numeric metadata changed ACK", mode, err)
		}
		if got, err := readResponse(&w); err != nil || got.sequence != 2 {
			t.Fatal("invalid metadata shifted next ACK", mode)
		}
	}
}

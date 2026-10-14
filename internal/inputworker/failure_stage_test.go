package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

func TestFailureStageACKPreservesCertaintyAndFollowingFrame(t *testing.T) {
	for _, mode := range []string{"unprofiled", "buckets", "millis", "apply", "submission"} {
		t.Run(mode, func(t *testing.T) {
			ack := response{op: opMoveRead, sequence: 1, status: 1, certainty: 3, failureStage: failureStageCursorReadback}
			switch mode {
			case "buckets":
				ack.timing = [3]byte{1, 2, 3}
			case "millis", "apply", "submission":
				ack.millis, ack.measured = [3]uint16{0, 1, 12}, 7
				ack.timing = [3]byte{1, 2, 3}
				if mode != "millis" {
					ack.applyDetail = &handoff.WorkerApplyProfile{Micros: [2]uint32{10000, 2000}, Measured: 3}
				}
				if mode == "submission" {
					ack.applyDetail.SubmissionMicros, ack.applyDetail.SubmissionCalls = 5000, 1
				}
			}
			writer := &cursorACKWriter{}
			if err := writeResponse(writer, ack); err != nil || writer.writes != 1 {
				t.Fatal("failed ACK lost single-write framing", err)
			}
			if err := writeResponse(writer, response{op: opMove, sequence: 2}); err != nil {
				t.Fatal(err)
			}
			reader := iotest.OneByteReader(writer)
			got, err := readResponse(reader)
			if err != nil || got.failureStage != ack.failureStage || got.certainty != 3 || got.requested != 0 || got.inserted != 0 || got.status != 1 || got.timing != ack.timing {
				t.Fatal("phase changed input outcome or timing", got, err)
			}
			if ack.applyDetail != nil && (got.applyDetail == nil || *got.applyDetail != *ack.applyDetail) || ack.applyDetail == nil && got.applyDetail != nil {
				t.Fatal("phase fabricated or lost apply measurements")
			}
			if got.millis != ack.millis || got.measured != ack.measured {
				t.Fatal("legacy bucket was fabricated as a measured millisecond")
			}
			if next, err := readResponse(reader); err != nil || next.sequence != 2 || next.failureStage != 0 {
				t.Fatal("following legacy frame shifted", err)
			}
		})
	}
}

func TestFailureStageFramingRejectsInvalidAndTruncatedEvidence(t *testing.T) {
	var encoded bytes.Buffer
	if err := writeResponse(&encoded, response{op: opMove, sequence: 1, status: 1, certainty: 1, failureStage: failureStageApply}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"stage", "reserved", "success", "truncated"} {
		data := append([]byte(nil), encoded.Bytes()...)
		switch mode {
		case "stage":
			data[responseSize+10] = 255
		case "reserved":
			data[responseSize+11] = 1
		case "success":
			data[6], data[7] = 0, 0
		case "truncated":
			data = data[:len(data)-1]
		}
		if _, err := readResponse(bytes.NewReader(data)); !errors.Is(err, ErrProtocol) {
			t.Fatal("invalid failure evidence accepted", mode, err)
		}
	}
}

func TestWorkerReadbackFailureReachesClientWithoutInputReplay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &timedApplyInput{cursorErr: errors.New("cursor snapshot unavailable")}
		server, conn := net.Pipe()
		lease := testLease()
		done := make(chan error, 1)
		go func() {
			_, err := serve(context.Background(), server, input, lease, func(ctx context.Context, apply func() error) error { return apply() }, func(ctx context.Context, apply func() error) error { return apply() }, time.Now)
			done <- err
		}()
		client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		var readback int
		client.SetTimingObserver(func(code string) {
			if code == "installed_input_failure_cursor_readback" {
				readback++
			}
		})
		_, _, err = client.MoveWithPosition(context.Background(), 1, 2)
		if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown || input.moves != 1 || input.reads != 1 || readback != 1 {
			t.Fatal("readback failure lost phase/certainty or replayed input", err)
		}
		if err := <-done; err == nil {
			t.Fatal("worker hid unknown failure")
		}
	})
}

func TestApplyStageDoesNotMislabelFailureAsReadback(t *testing.T) {
	input := &timedApplyInput{moveErr: &application.InjectionError{Certainty: application.InjectionNotApplied}}
	var ack response
	var pressed control.PressedState
	if err := applyCommand(context.Background(), input, &pressed, request{op: opMoveRead, x: 1}, &ack); err == nil || ack.failureStage != failureStageApply || input.reads != 0 {
		t.Fatal("failed move fabricated a readback attempt")
	}
}

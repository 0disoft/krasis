package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

type timedApplyInput struct {
	recordingInput
	moveErr, cursorErr error
	moves, reads       int
}

func (input *timedApplyInput) Move(context.Context, int32, int32) error {
	input.moves++
	time.Sleep(31 * time.Millisecond)
	return input.moveErr
}

func (input *timedApplyInput) CursorPosition(context.Context) (int32, int32, error) {
	input.reads++
	time.Sleep(2 * time.Millisecond)
	return 1, 2, input.cursorErr
}

func (*timedApplyInput) SetCursorPosition(context.Context, int32, int32) error { return nil }

func TestWorkerApplySplitMeasuresDisjointPartsWithoutChangingFailures(t *testing.T) {
	for _, mode := range []string{"ok", "apply_failure", "snapshot_failure", "snapshot_only", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cause := &application.InjectionError{Certainty: application.InjectionNotApplied}
				input := &timedApplyInput{}
				command := request{op: opMoveRead, x: 1, applyDetail: mode != "disabled"}
				switch mode {
				case "apply_failure":
					input.moveErr = cause
				case "snapshot_failure":
					input.cursorErr = cause
				case "snapshot_only":
					command.op = opCursorRead
				}
				var pressed control.PressedState
				var ack response
				err := applyCommand(context.Background(), input, &pressed, command, &ack)
				if mode == "disabled" {
					if err != nil || ack.applyDetail != nil || input.moves != 1 || input.reads != 1 {
						t.Fatal("disabled path changed")
					}
					return
				}
				if ack.applyDetail == nil || !ack.applyDetail.Valid() {
					t.Fatal("missing split")
				}
				expectMeasured := uint8(3)
				if mode == "apply_failure" {
					expectMeasured = 1
					if err != cause || input.moves != 1 || input.reads != 0 {
						t.Fatal("failed apply replayed or snapshot attempted")
					}
				} else if mode == "snapshot_failure" {
					if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown || input.moves != 1 || input.reads != 1 {
						t.Fatal("post-move snapshot failure lost unknown outcome")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if mode == "snapshot_only" {
					expectMeasured = 2
					if input.moves != 0 {
						t.Fatal("snapshot applied input")
					}
				}
				if ack.applyDetail.Measured != expectMeasured {
					t.Fatal("unattempted span fabricated")
				}
				if expectMeasured&1 != 0 && ack.applyDetail.Micros[0] != 31000 {
					t.Fatal("apply timer includes snapshot")
				}
				if expectMeasured&2 != 0 && ack.applyDetail.Micros[1] != 2000 {
					t.Fatal("snapshot timing lost")
				}
			})
		})
	}
}

func TestClientRequestsWorkerApplyOnlyForTrackedInputAndKeepsMatchingACK(t *testing.T) {
	var summaries []handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "receiver", "s", "e", time.Now())
	handoff.Backend(p, "installed")
	defer handoff.Finish(p, "ok")
	server, conn := net.Pipe()
	defer server.Close()
	client, err := NewClient(ctx, clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result := make(chan error, 1)
	go func() {
		for i := 0; i < 3; i++ {
			r, err := readRequest(server)
			if err != nil {
				result <- err
				return
			}
			if r.applyDetail != (i == 1) || r.submissionDetail != r.applyDetail {
				result <- errors.New("detail escaped tracked input")
				return
			}
			ack := response{op: r.op, sequence: r.sequence}
			if r.applyDetail {
				ack.applyDetail = &handoff.WorkerApplyProfile{Micros: [2]uint32{31000, 2000}, Measured: 3}
			}
			if err := writeResponse(server, ack); err != nil {
				result <- err
				return
			}
		}
		result <- nil
	}()
	if err := client.Heartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	x, complete := handoff.Input(ctx, "receiver")
	if err := client.Move(x, 1, 0); err != nil {
		t.Fatal(err)
	}
	complete(nil)
	if err := client.Move(context.Background(), 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	handoff.Finish(p, "ok")
	if len(summaries) != 1 || summaries[0].First.WorkerApply == nil || summaries[0].First.WorkerApply.Micros[0] != 31000 {
		t.Fatal("matching detail not exported")
	}
}

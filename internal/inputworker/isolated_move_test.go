package inputworker

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type isolatedMoveCursor struct{ switchingCursorInput }

func (p *isolatedMoveCursor) CursorPosition(ctx context.Context) (int32, int32, error) {
	_, _, err := p.switchingCursorInput.CursorPosition(ctx)
	return 12, -34, err
}

// Exercise the child decoder too: a combined command must execute one move,
// one readback and one desktop admission, even when readback fails after input.
func TestIsolatedMoveReadUsesOneChildTransactionWithoutReplay(t *testing.T) {
	for _, failedRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "readback_failure"}[failedRead], func(t *testing.T) {
			native := &isolatedMoveCursor{switchingCursorInput: switchingCursorInput{persistent: failedRead}}
			if failedRead {
				native.readFailure = application.ErrCursorObservationUnavailable
			}
			admissions, exchanges := 0, 0
			handler := PortableInputHandler(native, func(_ context.Context, work func() error) error {
				admissions++
				return work()
			})
			client := &Client{localExchange: func(_ context.Context, command request, observed func(response)) error {
				exchanges++
				if command.op != opMoveRead || command.x != 3 || command.y != -2 {
					t.Fatal("split or altered child command", command)
				}
				command.binding, command.sequence = Binding{1}, 1
				data, err := encodeRequest(command)
				if err != nil {
					return err
				}
				var wire bytes.Buffer
				if err := handler(bytes.NewReader(data[:]), &wire); err != nil {
					return err
				}
				ack, err := readResponse(&wire)
				if err != nil {
					return err
				}
				if ack.status != 0 {
					if ack.certainty != 3 {
						t.Fatal("applied move lost unknown outcome", ack)
					}
					return unknownInput(ErrAuthority)
				}
				observed(ack)
				return nil
			}}
			var pressed control.PressedState
			var ack response
			err := applyCommand(context.Background(), &isolatedInput{Client: client}, &pressed, request{op: opMoveRead, x: 3, y: -2, applyDetail: true}, &ack)
			if exchanges != 1 || admissions != 1 || native.moves != 1 || native.reads != 1 || pressed.Count() != 0 {
				t.Fatal("duplicate movement, observation or child admission", exchanges, admissions, native.moves, native.reads)
			}
			if failedRead {
				certainty, known := application.InjectionCertaintyOf(err)
				if !known || certainty != application.InjectionOutcomeUnknown || !errors.Is(err, ErrAuthority) {
					t.Fatal("readback failure became success or unapplied", err)
				}
			} else if err != nil || ack.x != 12 || ack.y != -34 {
				t.Fatal("child readback changed", err, ack)
			}
			if ack.applyDetail.Measured != 1 || !ack.applyDetail.Valid() {
				t.Fatal("combined IPC was counted twice", ack.applyDetail)
			}
		})
	}
}

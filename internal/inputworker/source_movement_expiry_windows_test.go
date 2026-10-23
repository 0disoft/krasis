//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceUnsubmittedMovementKeepsCaptureAndWireSequence(t *testing.T) {
	for _, kind := range []control.InputActionKind{control.InputPointerMove, control.InputWheel, control.InputKeyDown, control.InputKeyUp, control.InputButtonDown, control.InputButtonUp} {
		t.Run(string(kind), func(t *testing.T) {
			server, client := nativePair(t)
			w := &sourceWriter{channel: clientChannel(&expireSourceReplyBeforeSubmit{Conn: server}, authorityAgent), binding: Binding{1}, sequence: 10}
			defer w.channel.Close()
			w.started.Store(true)
			w.epoch.Store(1)
			action := control.InputAction{Kind: kind, DeltaX: 1}
			if kind != control.InputPointerMove && kind != control.InputWheel {
				action = control.InputAction{Kind: kind, Code: 1}
			}
			err := w.SendAction(context.Background(), action, func() bool { return true })
			if kind != control.InputPointerMove {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					t.Fatal("discrete or wheel failure hidden", err)
				}
				return
			}
			if err != nil || w.sequence != 10 || w.broken {
				t.Fatal("unsent movement retired capture", err, w.sequence)
			}
			request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 7, binding: w.binding}
			if err := w.sendCompletedReply(request, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}); err != nil {
				t.Fatal(err)
			}
			_ = client.SetReadDeadline(time.Now().Add(time.Second))
			ack, err := readSourcePacket(client)
			if err != nil || ack.kind != sourceReply || ack.sequence != 11 || ack.reply != 7 {
				t.Fatal("movement replay or sequence gap", ack, err)
			}
		})
	}
}

func TestSourceAttemptedMovementFailureStillRetiresCapture(t *testing.T) {
	for _, partial := range []bool{false, true} {
		server, conn := net.Pipe()
		stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded, partial: partial}
		w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
		w.started.Store(true)
		w.epoch.Store(1)
		err := w.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, func() bool { return true })
		_ = server.Close()
		_ = w.channel.Close()
		if !errors.Is(err, os.ErrDeadlineExceeded) || !w.broken || w.sequence != 1 || stream.writes != 1 {
			t.Fatal("uncertain movement write hidden", err, w.sequence)
		}
	}
}

type cancelUnsubmittedMovement struct {
	*expireSourceReplyBeforeSubmit
	cancel context.CancelFunc
}

func (s *cancelUnsubmittedMovement) SetWriteDeadline(deadline time.Time) error {
	s.cancel()
	return s.expireSourceReplyBeforeSubmit.SetWriteDeadline(deadline)
}

func TestSourceUnsubmittedMovementCannotHideCancellation(t *testing.T) {
	server, _ := nativePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := &cancelUnsubmittedMovement{expireSourceReplyBeforeSubmit: &expireSourceReplyBeforeSubmit{Conn: server}, cancel: cancel}
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(1)
	if err := w.SendAction(ctx, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, func() bool { return true }); err == nil || w.sequence != 0 || w.broken {
		t.Fatal("canceled sender treated as a live dropped sample", err, w.sequence)
	}
}

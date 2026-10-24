//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

type sourceGenerationWire struct {
	net.Conn
	wire bytes.Buffer
}

func (c *sourceGenerationWire) Write(data []byte) (int, error) { return c.wire.Write(data) }

func TestSourceWriterGenerationRetiredDuringPresenceCannotReachWire(t *testing.T) {
	for _, kind := range []control.InputActionKind{control.InputPointerMove, control.InputKeyDown, control.InputKeyUp, control.InputButtonDown, control.InputButtonUp} {
		t.Run(string(kind), func(t *testing.T) {
			peer, conn := net.Pipe()
			defer peer.Close()
			wire := &sourceGenerationWire{Conn: conn}
			w := &sourceWriter{channel: clientChannel(wire, authorityAgent), binding: Binding{1}, sequence: 10}
			defer w.channel.Close()
			w.started.Store(true)
			w.epoch.Store(5)
			var current atomic.Bool
			current.Store(true)
			w.channel.authority.checkPresent = func() error { current.Store(false); return nil }
			action := control.InputAction{Kind: kind, Code: 1}
			if kind == control.InputPointerMove {
				action = control.InputAction{Kind: kind, DeltaX: 1}
			}
			err := w.SendAction(context.Background(), action, current.Load)
			if !errors.Is(err, control.ErrSourceActionDiscarded) || wire.wire.Len() != 0 || w.sequence != 10 || w.broken {
				t.Fatalf("retired generation escaped after native presence check: err=%v bytes=%d sequence=%d broken=%t", err, wire.wire.Len(), w.sequence, w.broken)
			}
			w.channel.authority.checkPresent = func() error { return nil }
			request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 7, binding: w.binding}
			if err := w.sendCompletedReply(request, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}); err != nil {
				t.Fatal(err)
			}
			ack, err := readSourcePacket(bytes.NewReader(wire.wire.Bytes()))
			if err != nil || ack.kind != sourceReply || ack.sequence != 11 || ack.reply != 7 || wire.wire.Len() != sourcePacketSize {
				t.Fatal("discard corrupted the next heartbeat", ack, err, wire.wire.Len())
			}
			wire.wire.Reset()
			current.Store(true)
			if err := w.SendAction(context.Background(), action, current.Load); err != nil {
				t.Fatal("live generation was rejected", err)
			}
			packet, err := readSourcePacket(bytes.NewReader(wire.wire.Bytes()))
			if err != nil || packet.kind != sourceAction || packet.sequence != 12 || packet.reply != 5 || packet.action != action || wire.wire.Len() != sourcePacketSize {
				t.Fatal("live input changed or was replayed", packet, err, wire.wire.Len())
			}
		})
	}
}

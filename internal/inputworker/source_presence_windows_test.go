//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceHeartbeatDoesNotWaitForBrokerOrAuthorizeActivation(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	var inputChecks, presenceChecks atomic.Int32
	c.channel.authority.checkInput = func() error { inputChecks.Add(1); return ErrAuthority }
	c.channel.authority.checkPresent = func() error { presenceChecks.Add(1); return nil }
	joined := make(chan error, 1)
	go func() {
		p, err := readSourcePacket(server)
		if err == nil {
			if p.op != sourceHeartbeat {
				err = ErrProtocol
			} else {
				c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
			}
		}
		joined <- err
	}()
	if sent, err := c.pollHeartbeat(context.Background()); !sent || err != nil {
		t.Fatal(sent, err)
	}
	if err := <-joined; err != nil {
		t.Fatal(err)
	}
	if inputChecks.Load() != 0 || presenceChecks.Load() != 1 || c.epoch.Load() != 0 {
		t.Fatal("keepalive queried broker or enabled capture")
	}
	if _, err := c.call(context.Background(), sourceActivate); err != ErrAuthority || inputChecks.Load() != 1 || c.sequence != 1 || c.epoch.Load() != 0 {
		t.Fatal("presence proof escaped into capture", err)
	}
}

func TestSourceReaderPresenceCannotAuthorizeOtherCommandOrBadBinding(t *testing.T) {
	for _, op := range []byte{sourceHeartbeat, sourceAdmit, sourceActivate, sourceCursorPrepare, sourceDeactivate, sourceStop} {
		binding := Binding{1}
		p := sourcePacket{kind: sourceCommand, op: op, sequence: 1, binding: binding}
		data, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		authorize := func(actual byte) error {
			if actual == sourceHeartbeat {
				return nil
			}
			return ErrAuthority
		}
		_, err = readSourceCommandAuthorized(bytes.NewReader(data[:]), context.Background(), binding, 0, authorize)
		if (err == nil) != (op == sourceHeartbeat) {
			t.Fatal("wrong command authority", op, err)
		}
		_, err = readSourceCommandAuthorized(bytes.NewReader(data[:]), context.Background(), Binding{2}, 0, func(byte) error { t.Fatal("bad binding reached authorization"); return nil })
		if err == nil {
			t.Fatal("presence accepted wrong binding")
		}
	}
}

func TestSourceHeartbeatWriterCannotSendInputWithPresenceOnly(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.channel.authority.checkInput = func() error { return ErrAuthority }
	w.channel.authority.checkPresent = func() error { return nil }
	w.started.Store(true)
	joined := make(chan error, 1)
	go func() {
		p, err := readSourcePacket(server)
		if err == nil && (p.kind != sourceReply || p.op != sourceHeartbeat) {
			err = ErrProtocol
		}
		joined <- err
	}()
	if err := w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-joined; err != nil {
		t.Fatal(err)
	}
	if err := w.send(sourcePacket{kind: sourceAction, reply: 1, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}}, nil); err == nil || w.sequence != 1 {
		t.Fatal("presence proof forwarded input")
	}
	_ = w.channel.authority.Close()
	if w.channel.authority.checkHeartbeat() == nil {
		t.Fatal("closed proof kept presence")
	}
}

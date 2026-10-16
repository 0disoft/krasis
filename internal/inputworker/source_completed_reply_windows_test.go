//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceCompletedReplyDoesNotRepeatCommandAuthorization(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	checks := 0
	w.channel.authority.checkInput = func() error {
		checks++
		if checks > 1 {
			return ErrAuthority
		}
		return nil
	}
	w.channel.authority.checkPresent = func() error { return nil }
	request := sourcePacket{kind: sourceCommand, op: sourceActivate, binding: w.binding, sequence: 1}
	data, err := encodeSourcePacket(request)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := readSourceCommandAuthorized(bytes.NewReader(data[:]), context.Background(), w.binding, 0, func(byte) error { return w.channel.authority.check(false) })
	if err != nil {
		t.Fatal(err)
	}
	response := sourcePacket{kind: sourceReply, op: accepted.op, reply: accepted.sequence}
	readDone := make(chan sourcePacket, 1)
	go func() { p, _ := readSourcePacket(server); readDone <- p }()
	if err := w.sendCompletedReply(accepted, response); err != nil {
		t.Fatal("completed command lost its result to a second authorization query", err)
	}
	if p := <-readDone; p.op != accepted.op || p.reply != accepted.sequence || p.status != 0 || p.binding != w.binding || checks != 1 {
		t.Fatal("completion changed or repeated authorization", p, checks)
	}
	if err := w.send(sourcePacket{kind: sourceAction, reply: 1, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}}, nil); err == nil || checks != 2 || w.sequence != 1 {
		t.Fatal("completion authorized subsequent input", err, checks, w.sequence)
	}
}

func TestSourceCompletedReplyRejectsUnrelatedFramesAndRetirement(t *testing.T) {
	for _, invalid := range []string{"binding", "request", "operation", "reference", "input", "retirement", "presequenced"} {
		t.Run(invalid, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.channel.authority.checkInput = func() error { t.Fatal("bad result reached authorization"); return nil }
			request := sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: w.binding}
			response := sourcePacket{kind: sourceReply, op: request.op, reply: request.sequence}
			switch invalid {
			case "binding":
				request.binding = Binding{2}
			case "request":
				request.sequence = 0
			case "operation":
				response.op = sourceDeactivate
			case "reference":
				response.reply++
			case "input":
				response.kind = sourceAction
			case "retirement":
				request.op, response.op = sourceStop, sourceStop
			case "presequenced":
				response.sequence = 1
			}
			if err := w.sendCompletedReply(request, response); err != ErrProtocol || w.sequence != 0 {
				t.Fatal("unmatched result reached the wire", err, w.sequence)
			}
		})
	}
}

func TestSourceCompletedReplyKeepsRejectionAndChecksPinnedOwner(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "owner-gone", true: "negative-result"}[live], func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.channel.authority.checkInput = func() error { t.Fatal("completion repeated command check"); return nil }
			w.channel.authority.checkPresent = func() error {
				if !live {
					return ErrAuthority
				}
				return nil
			}
			request := sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: w.binding}
			response := sourcePacket{kind: sourceReply, op: request.op, reply: request.sequence, status: 1}
			if !live {
				if err := w.sendCompletedReply(request, response); err == nil || w.sequence != 0 {
					t.Fatal("exited owner received result", err)
				}
				return
			}
			readDone := make(chan sourcePacket, 1)
			go func() { p, _ := readSourcePacket(server); readDone <- p }()
			if err := w.sendCompletedReply(request, response); err != nil {
				t.Fatal(err)
			}
			if p := <-readDone; p.status != 1 || p.op != request.op || p.reply != request.sequence {
				t.Fatal("negative outcome became success", p)
			}
		})
	}
}

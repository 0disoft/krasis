//go:build windows

package inputworker

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type expireSourceReplyBeforeSubmit struct {
	net.Conn
	expired bool
}

func (s *expireSourceReplyBeforeSubmit) SetWriteDeadline(deadline time.Time) error {
	if !s.expired {
		s.expired = true
		deadline = time.Now().Add(-time.Second)
	}
	return s.Conn.SetWriteDeadline(deadline)
}

func TestSourceWriterUnsubmittedReplyKeepsRetirementSequence(t *testing.T) {
	server, client := nativePair(t)
	stream := &expireSourceReplyBeforeSubmit{Conn: server}
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}, sequence: 10}
	defer w.channel.Close()
	request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 7, binding: w.binding}
	err := w.sendCompletedReply(request, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7})
	if err == nil {
		t.Fatal("expired native write succeeded")
	}
	// The native pipe proves this reply never reached WriteFile. Native/sender
	// joins can still publish their distinct terminal proof without replaying it.
	if w.sequence != 10 {
		t.Fatal("unsent reply consumed the terminal sequence", w.sequence)
	}
	if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err != nil {
		t.Fatal("joined retirement lost after an unsent reply", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	p, err := readSourcePacket(client)
	if err != nil || p.kind != sourceRetired || p.sequence != 11 || p.status != 0 {
		t.Fatal("terminal proof acquired a sequence gap", p, err)
	}
}

func TestSourceWriterAttemptedFailureCannotAppendRetirement(t *testing.T) {
	for _, partial := range []bool{false, true} {
		server, conn := net.Pipe()
		stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded, partial: partial}
		w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
		w.started.Store(true)
		request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 7, binding: w.binding}
		first := w.sendCompletedReply(request, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7})
		second := w.write(sourcePacket{kind: sourceRetired}, nil, true)
		batch := w.sendBoundaries([]sourcePacket{{kind: sourceBoundary, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}}})
		_ = server.Close()
		_ = w.channel.Close()
		if !errors.Is(first, os.ErrDeadlineExceeded) || second == nil || batch == nil || stream.writes != 1 || w.sequence != 1 {
			t.Fatal("uncertain write was retried or followed by terminal proof", first, second, stream.writes, w.sequence)
		}
	}
}

func TestSourceWriterUnsubmittedAdvisoryBatchKeepsOwnerAndSequence(t *testing.T) {
	server, client := nativePair(t)
	w := &sourceWriter{channel: clientChannel(&expireSourceReplyBeforeSubmit{Conn: server}, authorityAgent), binding: Binding{1}, sequence: 10}
	defer w.channel.Close()
	w.started.Store(true)
	batch := []sourcePacket{
		{kind: sourceBoundary, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}},
		{kind: sourceBoundary, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 2}},
	}
	if err := w.sendBoundaries(batch); err != nil || w.sequence != 10 || w.broken {
		t.Fatal("unsent advisory batch retired owner or consumed sequences", err, w.sequence)
	}
	request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 7, binding: w.binding}
	if err := w.sendCompletedReply(request, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}); err != nil {
		t.Fatal("unsent advisory batch prevented the next heartbeat reply", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	ack, err := readSourcePacket(client)
	if err != nil || ack.kind != sourceReply || ack.op != sourceHeartbeat || ack.sequence != 11 || ack.reply != 7 {
		t.Fatal("stale advisory samples were replayed or heartbeat sequence changed", ack, err)
	}
	if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	p, err := readSourcePacket(client)
	if err != nil || p.kind != sourceRetired || p.sequence != 12 {
		t.Fatal("batch failure corrupted terminal proof", p, err)
	}
}

func TestSourceWriterAttemptedAdvisoryBatchFailureStillRetires(t *testing.T) {
	for _, partial := range []bool{false, true} {
		server, conn := net.Pipe()
		defer server.Close()
		stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded, partial: partial}
		w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}, sequence: 10}
		defer w.channel.Close()
		w.started.Store(true)
		err := w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
		if !errors.Is(err, os.ErrDeadlineExceeded) || !w.broken || w.sequence != 11 {
			t.Fatal("attempted advisory failure was discarded as unsent", err, w.sequence)
		}
		if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err == nil || stream.writes != 1 {
			t.Fatal("uncertain advisory write was followed by another frame", err, stream.writes)
		}
	}
}

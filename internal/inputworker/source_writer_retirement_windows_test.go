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

func TestSourceWriterUnsubmittedBatchKeepsRetirementSequence(t *testing.T) {
	server, client := nativePair(t)
	w := &sourceWriter{channel: clientChannel(&expireSourceReplyBeforeSubmit{Conn: server}, authorityAgent), binding: Binding{1}, sequence: 10}
	defer w.channel.Close()
	w.started.Store(true)
	batch := []sourcePacket{
		{kind: sourceBoundary, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}},
		{kind: sourceBoundary, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 2}},
	}
	if err := w.sendBoundaries(batch); err == nil || w.sequence != 10 {
		t.Fatal("unsent batch consumed sequences", err, w.sequence)
	}
	if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	p, err := readSourcePacket(client)
	if err != nil || p.kind != sourceRetired || p.sequence != 11 {
		t.Fatal("batch failure corrupted terminal proof", p, err)
	}
}

//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

type sourceWriteBarrier struct {
	net.Conn
	ready <-chan struct{}
}

func (c sourceWriteBarrier) Write(data []byte) (int, error) {
	n, err := c.Conn.Write(data)
	<-c.ready
	return n, err
}

func TestSourceClientBufferedNegativeReplyAndEOFHaveOneOutcome(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		server, conn := net.Pipe()
		ready := make(chan struct{})
		c := &sourceClient{channel: clientChannel(sourceWriteBarrier{Conn: conn, ready: ready}, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		go func() {
			defer server.Close()
			request, err := readSourcePacket(server)
			if err == nil {
				reply := sourcePacket{kind: sourceReply, op: sourceAdmit, reply: request.sequence, status: 1}
				if foreign {
					reply.reply++
				}
				c.replies <- reply
			}
			close(c.done)
			close(ready) // Both signals are ready before call finishes its write.
		}()
		p, err := c.call(context.Background(), sourceAdmit)
		decision, err := sourceAdmissionResult(p, err)
		if foreign {
			if err != ErrProtocol || c.channel.authority.check(false) == nil {
				t.Fatal("foreign buffered reply accepted", err)
			}
		} else if decision != 0 || !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) || c.channel.authority.check(false) != nil || c.epoch.Load() != 0 {
			t.Fatal("EOF erased rejection or changed capture authority", decision, err)
		}
		_ = c.channel.Close()
	}
}

func TestSourceClientWrongReplyClosesAuthorityWithoutReplayingActivation(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p, err := readSourcePacket(server)
		if err != nil {
			return
		}
		c.replies <- sourcePacket{kind: sourceReply, op: sourceActivate, reply: p.sequence + 1}
	}()
	if _, err := c.call(context.Background(), sourceActivate); err != ErrProtocol {
		t.Fatal("foreign activation ACK accepted", err)
	}
	<-done
	if c.sequence != 1 {
		t.Fatal("activation replayed")
	}
	if c.epoch.Load() != 0 {
		t.Fatal("failed activation retained suppression authority")
	}
}

func TestSourceAdmissionRejectionAllowsAFreshQueryOnSameCaptureOwner(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			request, err := readSourcePacket(server)
			if err != nil {
				finished <- err
				return
			}
			if request.op != sourceAdmit {
				finished <- ErrProtocol
				return
			}
			reply := sourcePacket{kind: sourceReply, op: sourceAdmit, reply: request.sequence, sequence: request.sequence, binding: request.binding, decision: peer.BoundaryAdmissionAllowed}
			if i == 0 {
				reply.status, reply.decision = 1, 0
			}
			data, err := encodeSourcePacket(reply)
			if err != nil {
				finished <- err
				return
			}
			// Exercise the actual wire parser as well as the request/ACK gate.
			parsed, err := readSourcePacket(bytes.NewReader(data[:]))
			if err != nil {
				finished <- err
				return
			}
			c.replies <- parsed
		}
		finished <- nil
	}()
	p, err := c.call(ctx, sourceAdmit)
	decision, err := sourceAdmissionResult(p, err)
	if decision != 0 || !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) || c.channel.authority.check(false) != nil || c.epoch.Load() != 0 {
		t.Fatal("negative query changed authority or killed capture", decision, err)
	}
	p, err = c.call(ctx, sourceAdmit)
	decision, err = sourceAdmissionResult(p, err)
	if err != nil || decision != peer.BoundaryAdmissionAllowed || c.sequence != 2 || c.epoch.Load() != 0 {
		t.Fatal("fresh query failed or activated/replayed capture", decision, err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestSourceClientRetirementRequiresMatchingAckEvenWithImmediateEOF(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		server, conn := net.Pipe()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		c.epoch.Store(1)
		go func() {
			defer server.Close()
			p, err := readSourcePacket(server)
			if err == nil && accepted {
				c.replies <- sourcePacket{kind: sourceReply, op: sourceStop, reply: p.sequence}
			}
			close(c.done)
		}()
		_, err := c.call(context.Background(), sourceStop)
		if (err == nil) != accepted {
			t.Fatal("retirement proof lost or missing ACK accepted", err)
		}
		_ = c.channel.Close()
	}
}

func TestSourceClientKnownCursorRejectionKeepsCaptureOwner(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	c.epoch.Store(42)
	go func() {
		p, _ := readSourcePacket(server)
		c.replies <- sourcePacket{kind: sourceReply, op: sourceCursorPrepare, reply: p.sequence, status: 1}
	}()
	if _, err := c.call(context.Background(), sourceCursorPrepare); err == nil || c.epoch.Load() != 42 || c.channel.authority.check(false) != nil {
		t.Fatal("optional cursor rejection retired capture", err)
	}
}

func TestSourceClientUnavailableCursorDoesNotPublishPreparedOwner(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{}), cursorPrepared: true}
	defer c.channel.Close()
	c.epoch.Store(42)
	go func() {
		p, _ := readSourcePacket(server)
		c.replies <- sourcePacket{kind: sourceReply, op: sourceCursorPrepare, reply: p.sequence, status: sourceCursorUnavailable}
	}()
	if _, err := c.call(context.Background(), sourceCursorPrepare); err != nil || c.cursorPrepared || c.epoch.Load() != 42 || c.channel.authority.check(false) != nil {
		t.Fatal("optional snapshot failure changed capture authority", err)
	}
}

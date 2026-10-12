//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
)

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

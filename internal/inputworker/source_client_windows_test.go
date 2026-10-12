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
}

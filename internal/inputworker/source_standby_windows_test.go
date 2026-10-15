//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestStandbyHeartbeatDoesNotApplyActiveIdleDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		c.lastReplyRequestAt = time.Now().Add(-3 * time.Second)
		go func() {
			p, err := readSourcePacket(server)
			if err == nil {
				c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
			}
		}()
		if sent, err := c.pollHeartbeat(context.Background()); !sent || err != nil || c.epoch.Load() != 0 {
			t.Fatal("standby heartbeat used active timeout or enabled capture", sent, err)
		}
		for _, cursor := range []bool{false, true} {
			c.lastReplyRequestAt = time.Now().Add(-3 * time.Second)
			c.cursorPrepared = cursor
			if cursor {
				c.epoch.Store(0)
			} else {
				c.epoch.Store(1)
			}
			if sent, err := c.pollHeartbeat(context.Background()); sent || err != nil || c.sequence != 1 {
				t.Fatal("active capture or pending cursor gained standby grace", cursor, sent, err)
			}
		}
	})
}

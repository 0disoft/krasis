//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceHeartbeatAllowsReplyWithinExistingOwnerDeadline(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "standby", true: "active"}[active], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, conn := net.Pipe()
				defer server.Close()
				c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
				defer c.channel.Close()
				if active {
					c.epoch.Store(42)
				}
				c.lastReplyRequestAt = time.Now().Add(-sourceHeartbeatQuiet)
				previous := c.lastReplyRequestAt
				replied := make(chan struct{})
				go func() {
					defer close(replied)
					p, err := readSourcePacket(server)
					if err == nil {
						time.Sleep(1200 * time.Millisecond)
						c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
					}
				}()
				defer func() { <-replied }()
				started := time.Now()
				attempted, err := c.pollHeartbeat(context.Background())
				if !attempted || err != nil || c.retiring.Load() || c.channel.authority.closed {
					t.Fatal("timely owner reply forced recovery", attempted, err)
				}
				if !c.lastReplyRequestAt.Equal(started) || c.lastReplyRequestAt.Equal(previous) || c.sequence != 1 {
					t.Fatal("reply did not retain the actual request start")
				}
				if active && c.epoch.Load() != 42 {
					t.Fatal("successful heartbeat changed capture authority")
				}
			})
		})
	}
}

func TestSourceHeartbeatStillExpiresAtActiveOwnerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		c.epoch.Store(42)
		c.lastReplyRequestAt = time.Now().Add(-sourceHeartbeatQuiet)
		previous := c.lastReplyRequestAt
		go func() { _, _ = readSourcePacket(server) }()
		attempted, err := c.pollHeartbeat(context.Background())
		if !attempted || err == nil || !time.Now().Equal(previous.Add(IdleTimeout)) {
			t.Fatal("heartbeat changed the existing active owner deadline", attempted, err, time.Since(previous))
		}
		if c.epoch.Load() != 0 || !c.channel.authority.closed || c.retiring.Load() || !c.lastReplyRequestAt.Equal(previous) {
			t.Fatal("unconfirmed active heartbeat preserved authority or renewed liveness")
		}
	})
}

func TestSourceHeartbeatStandbyWaitIsBoundedBeforeNextCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		c.lastReplyRequestAt = time.Now().Add(-sourceHeartbeatQuiet)
		previous, started := c.lastReplyRequestAt, time.Now()
		go func() { _, _ = readSourcePacket(server) }()
		attempted, err := c.pollHeartbeat(context.Background())
		if !attempted || err == nil || time.Since(started) != IdleTimeout || !c.retiring.Load() {
			t.Fatal("standby heartbeat waited beyond the bounded reply window", attempted, err, time.Since(started))
		}
		if !c.lastReplyRequestAt.Equal(previous) || c.pending.sequence != 1 || c.epoch.Load() != 0 {
			t.Fatal("timeout renewed liveness or allowed capture before exact-reply retirement")
		}
	})
}

func TestSourceHeartbeatBudgetDoesNotExtendOtherCommands(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		go func() { _, _ = readSourcePacket(server) }()
		started := time.Now()
		_, err := c.call(context.Background(), sourceAdmit)
		if err == nil || time.Since(started) != time.Second || c.epoch.Load() != 0 || !c.channel.authority.closed {
			t.Fatal("capture command inherited the heartbeat reply budget", err, time.Since(started))
		}
	})
}

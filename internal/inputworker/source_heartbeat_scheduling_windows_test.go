//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceHeartbeatUsesRequestAgeAndDoesNotRenewBySkipping(t *testing.T) {
	for _, replyDelay := range []time.Duration{0, 600 * time.Millisecond} {
		t.Run(replyDelay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, conn := net.Pipe()
				defer server.Close()
				c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
				defer c.channel.Close()
				packets := make(chan sourcePacket, 2)
				go func() {
					for i := 0; i < 2; i++ {
						p, err := readSourcePacket(server)
						if err != nil {
							return
						}
						packets <- p
						if i == 0 {
							time.Sleep(replyDelay)
						}
						c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
					}
				}()
				if _, err := c.call(context.Background(), sourceActivate); err != nil {
					t.Fatal(err)
				}
				if p := <-packets; p.op != sourceActivate || p.sequence != 1 {
					t.Fatal("wrong first command", p)
				}
				last := c.lastReplyRequestAt
				if last.IsZero() {
					t.Fatal("matching activation ACK did not record request age")
				}
				if replyDelay == 0 {
					for i := 0; i < 2; i++ {
						if attempted, err := c.pollHeartbeat(context.Background()); attempted || err != nil {
							t.Fatal("recent command acquired maintenance", attempted, err)
						}
						time.Sleep(sourceHeartbeatQuiet / 2)
					}
					if !c.lastReplyRequestAt.Equal(last) || c.sequence != 1 || c.epoch.Load() != 1 {
						t.Fatal("skipped heartbeat renewed liveness or changed authority")
					}
				}
				// A delayed reply must not buy another quiet interval.
				if attempted, err := c.pollHeartbeat(context.Background()); !attempted || err != nil {
					t.Fatal("due heartbeat missing", attempted, err)
				}
				if p := <-packets; p.op != sourceHeartbeat || p.sequence != 2 {
					t.Fatal("request skipped or replayed", p)
				}
			})
		})
	}
}

func TestSourceHeartbeatYieldsToQueuedActivationAndFinishesSentReplyInOrder(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	sent := make(chan sourcePacket, 2)
	release := make(chan struct{})
	go func() {
		for i := 0; i < 2; i++ {
			p, err := readSourcePacket(server)
			if err != nil {
				return
			}

			sent <- p
			if i == 0 {
				<-release
			}
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
		}
	}()
	heartbeat := make(chan error, 1)
	go func() { _, err := c.pollHeartbeat(context.Background()); heartbeat <- err }()
	if p := <-sent; p.op != sourceHeartbeat || p.sequence != 1 {
		t.Fatal("wrong first packet", p)
	}
	activation := make(chan error, 1)
	go func() { _, err := c.call(context.Background(), sourceActivate); activation <- err }()
	deadline := time.Now().Add(time.Second)
	for c.commandWaiting.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.commandWaiting.Load() != 1 {
		t.Fatal("queued activation did not publish priority")
	}
	if attempted, err := c.pollHeartbeat(context.Background()); attempted || err != nil {
		t.Fatal("maintenance queued behind activation", attempted, err)
	}
	select {
	case <-activation:
		t.Fatal("activation crossed outstanding heartbeat")
	default:
	}
	close(release)
	if err := <-heartbeat; err != nil {
		t.Fatal(err)
	}
	if err := <-activation; err != nil {
		t.Fatal(err)
	}
	if p := <-sent; p.op != sourceActivate || p.sequence != 2 {
		t.Fatal("activation reordered or replayed", p)
	}
	if c.commandWaiting.Load() != 0 || c.epoch.Load() != 2 {
		t.Fatal("command priority leaked or epoch changed")
	}
}

func TestSourceHeartbeatYieldsDuringCommandPublicationEvenWithFreeMutex(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService)}
	defer c.channel.Close()
	// The call has published demand, but has not yet acquired the otherwise
	// free IPC gate. TryLock alone would let maintenance overtake this call.
	c.commandWaiting.Store(1)
	if attempted, err := c.pollHeartbeat(context.Background()); attempted || err != nil {
		t.Fatal("heartbeat overtook a published command", attempted, err)
	}
	if c.sequence != 0 || !c.lastReplyRequestAt.IsZero() || c.channel.authority.check(false) != nil {
		t.Fatal("skipped maintenance changed sequencing, liveness or authority")
	}
}
